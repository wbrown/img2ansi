package img2ansi

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"sort"
	"strconv"
	"strings"
)

//go:embed colordata/ansi16.json
//go:embed colordata/ansi16.palette
//go:embed colordata/ansi256.json
//go:embed colordata/ansi256.palette
//go:embed colordata/jetbrains32.json
//go:embed colordata/jetbrains32.palette
var f embed.FS

// ByAnsiCode implements sort.Interface for AnsiData based on the numeric
// value of the ANSI code
type ByAnsiCode AnsiData

func (a ByAnsiCode) Len() int      { return len(a) }
func (a ByAnsiCode) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByAnsiCode) Less(i, j int) bool {
	return parseAnsiCodeForSort(a[i].Value) < parseAnsiCodeForSort(a[j].Value)
}

// parseAnsiCodeForSort extracts a sortable numeric value from an ANSI code string.
// For basic codes like "30" or "90", returns the code directly.
// For 256-color codes like "38;5;123", returns 1000+N to sort after basic codes.
// For 24-bit codes like "38;2;r;g;b", returns 2000000+RGB to sort after 256-color.
func parseAnsiCodeForSort(code string) int {
	// Handle 256-color codes: "38;5;N" or "48;5;N"
	if strings.HasPrefix(code, "38;5;") || strings.HasPrefix(code, "48;5;") {
		parts := strings.Split(code, ";")
		if len(parts) >= 3 {
			if n, err := strconv.Atoi(parts[2]); err == nil {
				return 1000 + n // Sort after basic codes (0-999)
			}
		}
	}
	// Handle 24-bit color codes: "38;2;r;g;b" or "48;2;r;g;b"
	if strings.HasPrefix(code, "38;2;") || strings.HasPrefix(code, "48;2;") {
		parts := strings.Split(code, ";")
		if len(parts) >= 5 {
			r, _ := strconv.Atoi(parts[2])
			g, _ := strconv.Atoi(parts[3])
			b, _ := strconv.Atoi(parts[4])
			return 2000000 + r*65536 + g*256 + b // Sort after 256-color codes
		}
	}
	// Basic codes: just parse the number
	if n, err := strconv.Atoi(code); err == nil {
		return n
	}
	return 0
}

// ReadAnsiDataFromJSON reads ANSI color data from a JSON file and returns
// the foreground and background ANSI color data as AnsiData slices. The
// function takes a filename as a string and returns the foreground and
// background AnsiData slices, or an error if the file cannot be read or
// the data cannot be unmarshalled.
func ReadAnsiDataFromJSON(filename string) (AnsiData, AnsiData, error) {
	var data []byte
	// First, try the VFS.
	template := "colordata/%s.json"
	data, vfsErr := f.ReadFile(fmt.Sprintf(template, filename))
	if vfsErr != nil {
		// If the VFS fails, try the filesystem.
		var fsErr error
		data, fsErr = ioutil.ReadFile(filename)
		if fsErr != nil {
			return nil, nil, fmt.Errorf("error reading file: %v", fsErr)
		}
	}

	// Unmarshal JSON data
	var colorMap map[string]string
	if jsonErr := json.Unmarshal(data, &colorMap); jsonErr != nil {
		return nil, nil, fmt.Errorf("error unmarshalling JSON: %v",
			jsonErr)
	}

	// Initialize foreground and background AnsiData slices
	var fgAnsiData, bgAnsiData AnsiData

	// Process each entry in the color map
	for code, hexColor := range colorMap {
		// Convert hex color to uint32
		hexColor = strings.TrimPrefix(hexColor, "#")
		colorUint, cErr := strconv.ParseUint(hexColor, 16, 32)
		if cErr != nil {
			return nil, nil, fmt.Errorf("error parsing color %s: %v",
				hexColor, cErr)
		}

		// Create AnsiEntry
		entry := AnsiEntry{
			Key:   uint32(colorUint),
			Value: code,
		}

		// Add to appropriate slice based on foreground/background
		if colorIsForeground(code) {
			fgAnsiData = append(fgAnsiData, entry)
		} else if colorIsBackground(code) {
			bgAnsiData = append(bgAnsiData, entry)
		} else {
			return nil, nil, fmt.Errorf("unknown color code type: %s",
				code)
		}
	}

	// Sort the AnsiData slices
	sort.Sort(ByAnsiCode(fgAnsiData))
	sort.Sort(ByAnsiCode(bgAnsiData))

	return fgAnsiData, bgAnsiData, nil
}

type ComputedTables struct {
	AnsiData        AnsiData
	ColorArr        *[]RGB
	ClosestColorArr *[]RGB
	ColorTable      *map[RGB]uint32
	KdTree          *ColorNode
}

// ComputeTables computes the color tables and KD-tree for a given color map.
// The function takes an OrderedMap of color codes and RGB colors as input,
// and returns a ComputedTables struct containing the color array, closest
// color array, color table, and KD-tree.
//
// Note: This function is slow (~30-40 seconds) because it precomputes
// the nearest palette color for all 16.7 million possible RGB values.
// For custom ColorDistanceMethod implementations, use ComputeTablesForKdSearch
// instead, which skips this expensive computation.
func ComputeTables(colorData AnsiData, method ColorDistanceMethod) ComputedTables {
	closestColorArr := make([]RGB, 256*256*256)
	colorTable := make(map[RGB]uint32)
	colorArr := make([]RGB, len(colorData))
	for idx, entry := range colorData {
		colorArr[idx] = rgbFromUint32(entry.Key)
		colorTable[rgbFromUint32(entry.Key)] = uint32(idx)
	}
	maxDepth := int(math.Log2(float64(len(colorArr))) + 1)
	kdTree := buildKDTree(colorArr, 0, maxDepth)

	for r := 0; r < 256; r++ {
		for g := 0; g < 256; g++ {
			for b := 0; b < 256; b++ {
				rgb := RGB{uint8(r), uint8(g), uint8(b)}
				closest, _ := kdTree.nearestNeighbor(
					rgb, kdTree.Color, math.MaxFloat64, 0, method)
				closestColorArr[r<<16|g<<8|b] = closest
			}
		}
	}
	return ComputedTables{
		ColorArr:        &colorArr,
		ClosestColorArr: &closestColorArr,
		ColorTable:      &colorTable,
		KdTree:          kdTree,
	}
}

// ComputeTablesForKdSearch computes only the structures needed for KD-tree
// based color matching. This is much faster than ComputeTables because it
// skips the expensive 16.7 million entry lookup table computation.
//
// Use this for custom ColorDistanceMethod implementations where you don't
// have precomputed .palette files. The Renderer will automatically use
// KD-tree search at runtime instead of table lookups.
//
// Returns ComputedTables with ClosestColorArr set to nil. The Renderer
// detects this and uses runtime KD-tree lookups instead.
func ComputeTablesForKdSearch(colorData AnsiData) ComputedTables {
	colorTable := make(map[RGB]uint32)
	colorArr := make([]RGB, len(colorData))
	for idx, entry := range colorData {
		colorArr[idx] = rgbFromUint32(entry.Key)
		colorTable[rgbFromUint32(entry.Key)] = uint32(idx)
	}
	maxDepth := int(math.Log2(float64(len(colorArr))) + 1)
	kdTree := buildKDTree(colorArr, 0, maxDepth)

	return ComputedTables{
		ColorArr:        &colorArr,
		ClosestColorArr: nil, // Not computed - use KD-tree search at runtime
		ColorTable:      &colorTable,
		KdTree:          kdTree,
	}
}

type ColorTableEntry struct {
	Color RGB
	Index uint32
}

type CompactTablePair struct {
	Fg CompactComputedTables
	Bg CompactComputedTables
}

type ColorMethodCompactTables map[string]CompactTablePair

type CompactComputedTables struct {
	ColorArr        []RGB
	AnsiData        AnsiData
	ClosestColorIdx []byte // Use 1 byte per color instead of full RGB
	ColorTable      []ColorTableEntry
	KDTreeData      []byte // Serialized KD-tree data
}

func CompactComputeTables(colorData AnsiData, method ColorDistanceMethod) CompactComputedTables {
	tables := ComputeTables(colorData, method)
	colorTable := make([]ColorTableEntry, len(colorData))
	closestColorArr := make([]uint8, 256*256*256)

	for idx, entry := range colorData {
		rgb := rgbFromUint32(entry.Key)
		colorTable[idx] = ColorTableEntry{Color: rgb, Index: uint32(idx)}
	}

	for i, color := range *tables.ClosestColorArr {
		closestColorIdx := findColorIndex(colorTable, color)
		closestColorArr[i] = uint8(closestColorIdx)
	}

	// Serialize KD-tree
	kdTreeData := tables.KdTree.Serialize()

	return CompactComputedTables{
		ColorArr:        *tables.ColorArr,
		AnsiData:        colorData,
		ClosestColorIdx: closestColorArr,
		ColorTable:      colorTable,
		KDTreeData:      kdTreeData,
	}
}

func findColorIndex(colorTable []ColorTableEntry, color RGB) uint32 {
	for _, entry := range colorTable {
		if entry.Color == color {
			return entry.Index
		}
	}
	return 0 // or handle error
}

func LoadPaletteAsCompactTables(path string, method ColorDistanceMethod) (CompactComputedTables,
	CompactComputedTables, error) {
	fgData, bgData, err := ReadAnsiDataFromJSON(path)
	if err != nil {
		return CompactComputedTables{}, CompactComputedTables{}, err
	}
	fgComputedTable := CompactComputeTables(fgData, method)
	fgComputedTable.AnsiData = fgData

	var bgComputedTable CompactComputedTables
	if !PaletteSame(fgData, bgData) {
		bgComputedTable = CompactComputeTables(bgData, method)
	} else {
		bgComputedTable = CompactComputedTables{
			AnsiData: bgData,
		}
	}
	return fgComputedTable, bgComputedTable, nil
}

func PaletteSame(fgData AnsiData, bgData AnsiData) bool {
	// Check if every color in bgData is also in fgData
	fgBgSame := true

	// Check if the sizes are the same
	if len(fgData) != len(fgData) {
		fgBgSame = false
	} else {
		fgExist := make(map[uint32]bool)
		for _, entry := range fgData {
			fgExist[entry.Key] = true
		}
		for _, entry := range bgData {
			if _, ok := fgExist[entry.Key]; !ok {
				fgBgSame = false
				break
			}
		}
	}
	return fgBgSame
}

func (cct CompactComputedTables) Restore() ComputedTables {
	closestColorArr := make([]RGB, 256*256*256)

	for i, idx := range cct.ClosestColorIdx {
		closestColorArr[i] = cct.ColorTable[idx].Color
	}
	colorTable := make(map[RGB]uint32, len(cct.ColorTable))
	for _, entry := range cct.ColorTable {
		colorTable[entry.Color] = entry.Index
	}

	var kdTree *ColorNode
	if cct.KDTreeData != nil {
		kdTree, _ = DeserializeKDTree(cct.KDTreeData)
	}

	return ComputedTables{
		ColorArr:        &cct.ColorArr,
		AnsiData:        cct.AnsiData,
		ClosestColorArr: &closestColorArr,
		ColorTable:      &colorTable,
		KdTree:          kdTree,
	}
}


// Old global-based palette loading functions (LoadPalette, LoadPaletteBinary,
// LoadPaletteJSON, ComputeDistinctColors, LookupClosestColor, GetPaletteColors,
// GetPaletteSize, GetANSICode) removed in v1.0.0.
// Use Renderer.LoadPalette() and Renderer methods instead.
