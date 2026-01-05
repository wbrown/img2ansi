package img2ansi

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"
)

// Renderer encapsulates all state for ANSI image conversion.
// This allows for multiple independent renderers with different
// configurations, thread-safe concurrent rendering, and efficient
// reuse of expensive palette loading and cache state.
type Renderer struct {
	// Configuration options
	TargetWidth    int
	ScaleFactor    float64
	MaxChars       int
	Quantization   int
	KdSearch       int
	CacheThreshold float64
	ColorMethod    ColorDistanceMethod

	// Palette state (private)
	palettePath   string
	paletteLoaded bool
	fgAnsi        *OrderedMap
	bgAnsi        *OrderedMap
	fgAnsiRev     map[string]uint32
	bgAnsiRev     map[string]uint32

	// Color tables (private)
	fgColors       []RGB
	bgColors       []RGB
	fgColorTable   map[RGB]uint32
	bgColorTable   map[RGB]uint32
	fgClosestColor *[]RGB
	bgClosestColor *[]RGB
	fgTree         *ColorNode
	bgTree         *ColorNode
	distinctColors int

	// Cache (private)
	lookupTable  ApproximateCache
	lookupHits   int
	lookupMisses int

	// Stats (private)
	beginInitTime       time.Time
	bestBlockTime       time.Duration
	usingPrecomputed    bool // true if using precomputed tables, false if KD-tree fallback
}

// RendererOption is a functional option for configuring a Renderer.
type RendererOption func(*Renderer)

// NewRenderer creates a new Renderer with the given options.
// Default values: KdSearch=0 (use precomputed tables), ColorMethod=RedmeanMethod{},
// ScaleFactor=2.0, CacheThreshold=200.0, MaxChars=1048576, TargetWidth=100, Quantization=256.
func NewRenderer(opts ...RendererOption) *Renderer {
	r := &Renderer{
		// Default configuration
		TargetWidth:    100,
		ScaleFactor:    2.0,
		MaxChars:       1048576,
		Quantization:   256,
		KdSearch:       0, // Use precomputed tables by default
		CacheThreshold: 200.0,
		ColorMethod:    RedmeanMethod{},

		// Initialize maps and cache
		fgAnsiRev:     make(map[string]uint32),
		bgAnsiRev:     make(map[string]uint32),
		fgColorTable:  make(map[RGB]uint32),
		bgColorTable:  make(map[RGB]uint32),
		lookupTable:   make(ApproximateCache),
		beginInitTime: time.Now(),
	}

	// Apply options
	for _, opt := range opts {
		opt(r)
	}

	return r
}

// WithPalette sets the palette and loads it immediately.
func WithPalette(path string) RendererOption {
	return func(r *Renderer) {
		r.LoadPalette(path)
	}
}

// WithColorMethod sets the color distance calculation method.
func WithColorMethod(method ColorDistanceMethod) RendererOption {
	return func(r *Renderer) {
		r.ColorMethod = method
	}
}

// WithKdSearch sets the KD-tree search depth (0 = use precomputed tables).
func WithKdSearch(depth int) RendererOption {
	return func(r *Renderer) {
		r.KdSearch = depth
	}
}

// WithScaleFactor sets the aspect ratio scale factor for terminal characters.
func WithScaleFactor(factor float64) RendererOption {
	return func(r *Renderer) {
		r.ScaleFactor = factor
	}
}

// WithCacheThreshold sets the error threshold for cache lookups.
func WithCacheThreshold(threshold float64) RendererOption {
	return func(r *Renderer) {
		r.CacheThreshold = threshold
	}
}

// WithMaxChars sets the maximum number of characters in output.
func WithMaxChars(max int) RendererOption {
	return func(r *Renderer) {
		r.MaxChars = max
	}
}

// WithTargetWidth sets the target width in characters.
func WithTargetWidth(width int) RendererOption {
	return func(r *Renderer) {
		r.TargetWidth = width
	}
}

// LoadPalette loads a color palette from the given path.
// If the same palette with the same color method is already loaded,
// this is a no-op (smart caching). The lookupTable cache is preserved
// unless the palette actually changes.
func (r *Renderer) LoadPalette(path string) error {
	// Smart caching: skip reload if already loaded with same settings
	if r.paletteLoaded && r.palettePath == path {
		// Palette already loaded, nothing to do
		return nil
	}

	// Determine format and delegate
	var fgTables, bgTables *ComputedTables
	var err error

	if strings.HasSuffix(path, ".palette") || !strings.HasSuffix(path, ".json") {
		fgTables, bgTables, err = r.loadPaletteBinary(path)
	} else {
		// Direct JSON loading - use full computation for built-in methods
		fgTables, bgTables, err = r.loadPaletteJSON(path, false)
	}

	if err != nil {
		return err
	}

	// Populate Renderer fields
	r.fgAnsi = fgTables.AnsiData.ToOrderedMap()
	r.bgAnsi = bgTables.AnsiData.ToOrderedMap()
	r.fgClosestColor = fgTables.ClosestColorArr
	r.fgColorTable = *fgTables.ColorTable
	r.fgTree = fgTables.KdTree
	r.fgColors = *fgTables.ColorArr

	if bgTables.ColorTable == nil || len(*bgTables.ColorTable) == 0 {
		// Background uses same table as foreground
		r.bgClosestColor = r.fgClosestColor
		r.bgColorTable = r.fgColorTable
		r.bgTree = r.fgTree
		r.bgColors = r.fgColors
	} else {
		r.bgClosestColor = bgTables.ClosestColorArr
		r.bgColorTable = *bgTables.ColorTable
		r.bgTree = bgTables.KdTree
		r.bgColors = *bgTables.ColorArr
	}

	// Build reverse maps from raw AnsiData (includes all ANSI codes, not deduplicated)
	// This supports future ANSI parsing where any valid code needs to map back to RGB
	r.buildReverseMap(fgTables.AnsiData, bgTables.AnsiData)

	// Only clear cache if palette changed
	if r.palettePath != path {
		r.lookupTable = make(ApproximateCache)
		r.lookupHits = 0
		r.lookupMisses = 0
	}

	// Compute distinct colors
	r.computeDistinctColors()

	// Mark as loaded
	r.palettePath = path
	r.paletteLoaded = true

	return nil
}

// buildReverseMap builds reverse lookups for ANSI codes from raw AnsiData.
// This uses the full palette (not deduplicated) so all valid ANSI codes
// can be mapped back to their RGB values for parsing ANSI art.
func (r *Renderer) buildReverseMap(fgData, bgData AnsiData) {
	r.fgAnsiRev = make(map[string]uint32, len(fgData))
	for _, entry := range fgData {
		r.fgAnsiRev[entry.Value] = entry.Key
	}

	r.bgAnsiRev = make(map[string]uint32, len(bgData))
	for _, entry := range bgData {
		r.bgAnsiRev[entry.Value] = entry.Key
	}
}

// computeDistinctColors counts the number of distinct colors in the palette.
func (r *Renderer) computeDistinctColors() {
	seen := make(map[RGB]bool)
	r.fgAnsi.Iterate(func(key, _ interface{}) {
		seen[rgbFromUint32(key.(uint32))] = true
	})
	r.bgAnsi.Iterate(func(key, _ interface{}) {
		seen[rgbFromUint32(key.(uint32))] = true
	})
	r.distinctColors = len(seen)
}

// loadPaletteBinary loads a binary .palette file.
func (r *Renderer) loadPaletteBinary(path string) (*ComputedTables, *ComputedTables, error) {
	// Try VFS first
	template := "colordata/%s.palette"
	data, vfsErr := f.ReadFile(fmt.Sprintf(template, path))
	if vfsErr != nil {
		var fsErr error
		data, fsErr = ioutil.ReadFile(path)
		if fsErr != nil {
			return nil, nil, fmt.Errorf("failed to read file: %v", fsErr)
		}
	}

	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gzr.Close()

	var cmct ColorMethodCompactTables
	dec := gob.NewDecoder(gzr)
	if err := dec.Decode(&cmct); err != nil {
		return nil, nil, fmt.Errorf("failed to decode palette data: %v", err)
	}

	cct, ok := cmct[r.ColorMethod.Name()]
	if !ok {
		// ColorMethod not in precomputed binary - fall back to KD-tree search
		// This is MUCH slower than precomputed tables
		fmt.Fprintf(os.Stderr, "WARNING: ColorMethod %q not found in precomputed tables for palette %q.\n", r.ColorMethod.Name(), path)
		fmt.Fprintf(os.Stderr, "         Falling back to slow KD-tree search. This will be 100-1000x slower.\n")
		fmt.Fprintf(os.Stderr, "         Available methods in palette: ")
		for name := range cmct {
			fmt.Fprintf(os.Stderr, "%s ", name)
		}
		fmt.Fprintf(os.Stderr, "\n")
		r.usingPrecomputed = false
		jsonPath := fmt.Sprintf("colordata/%s.json", path)
		return r.loadPaletteJSON(jsonPath, true)
	}
	r.usingPrecomputed = true
	fgTables := cct.Fg.Restore()
	bgTables := cct.Bg.Restore()

	return &fgTables, &bgTables, nil
}

// loadPaletteJSON loads a JSON palette file.
// If fastMode is true, uses ComputeTablesForKdSearch which skips the expensive
// 16.7M entry lookup table (used for custom ColorDistanceMethod).
// If fastMode is false, uses full ComputeTables (for built-in methods).
func (r *Renderer) loadPaletteJSON(path string, fastMode bool) (*ComputedTables, *ComputedTables, error) {
	fgData, bgData, err := ReadAnsiDataFromJSON(path)
	if err != nil {
		return nil, nil, err
	}

	var fgComputedTable ComputedTables
	if fastMode {
		fgComputedTable = ComputeTablesForKdSearch(fgData)
	} else {
		fgComputedTable = ComputeTables(fgData, r.ColorMethod)
	}
	fgComputedTable.AnsiData = fgData

	var bgComputedTable ComputedTables
	if !PaletteSame(fgData, bgData) {
		if fastMode {
			bgComputedTable = ComputeTablesForKdSearch(bgData)
		} else {
			bgComputedTable = ComputeTables(bgData, r.ColorMethod)
		}
		bgComputedTable.AnsiData = bgData
	} else {
		bgComputedTable = ComputedTables{AnsiData: bgData}
	}

	return &fgComputedTable, &bgComputedTable, nil
}

// CacheStats returns cache hit/miss statistics.
func (r *Renderer) CacheStats() (hits, misses int, hitRate float64) {
	total := r.lookupHits + r.lookupMisses
	if total == 0 {
		return 0, 0, 0
	}
	return r.lookupHits, r.lookupMisses, float64(r.lookupHits) / float64(total)
}

// CacheKeyStats returns statistics about cache key distribution.
// Returns: uniqueKeys (number of distinct cache keys), sharedKeys (keys with multiple blocks),
// totalBlocks (total blocks cached), avgError (average error across all cached blocks).
func (r *Renderer) CacheKeyStats() (uniqueKeys, sharedKeys, totalBlocks int, avgError float64) {
	var errorSum float64
	for _, entry := range r.lookupTable {
		uniqueKeys++
		totalBlocks += len(entry.Matches)
		if len(entry.Matches) > 1 {
			sharedKeys++
		}
		for _, match := range entry.Matches {
			errorSum += match.Error
		}
	}
	if totalBlocks > 0 {
		avgError = errorSum / float64(totalBlocks)
	}
	return
}

// ResetStats resets all statistics counters.
func (r *Renderer) ResetStats() {
	r.lookupHits = 0
	r.lookupMisses = 0
	r.bestBlockTime = 0
	r.beginInitTime = time.Now()
}

// GetBestBlockTime returns the cumulative time spent in FindBestBlockRepresentation.
func (r *Renderer) GetBestBlockTime() time.Duration {
	return r.bestBlockTime
}

// UsingPrecomputedTables returns true if the renderer is using precomputed
// lookup tables for color matching, false if it fell back to KD-tree search.
func (r *Renderer) UsingPrecomputedTables() bool {
	return r.usingPrecomputed
}
