// compute_glyphs pre-renders font glyphs to 8x8 bitmaps for img2ansi's
// glyph research. It accepts either a classic ROM font dump (-rom, the
// preferred source for 8x8 bitmap fonts: bit-perfect by construction) or
// a TrueType font (-font).
//
// TTF rasterization is calibrated rather than trusted: instead of
// deriving placement from font metrics (the approach that produced
// shifted, mangled glyphs in earlier iterations), the rasterizer renders
// at 8x supersampling with hinting disabled, then searches for the
// (ppem, baseline) pair that reproduces '█' as a solid cell and '▀' as
// exactly the top half. Every other glyph is rendered with that geometry
// and downsampled by coverage.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"flag"
	"fmt"
	"image"
	"log"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/wbrown/img2ansi"
	"golang.org/x/image/font"
)

const superSample = 8 // rasterize at 8x target resolution, downsample by coverage

// glyphSet is the set of characters pre-rendered for matching research:
// printable ASCII, block elements, and box drawing.
func glyphSet() []rune {
	var runes []rune
	for r := rune(32); r <= rune(126); r++ {
		runes = append(runes, r)
	}
	runes = append(runes, []rune{
		'▀', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█',
		'▌', '▍', '▎', '▏', '▐', '░', '▒', '▓',
		'▔', '▕', '▖', '▗', '▘', '▙', '▚', '▛', '▜', '▝', '▞', '▟',
	}...)
	runes = append(runes, []rune{
		'─', '━', '│', '┃', '┄', '┅', '┆', '┇', '┈', '┉', '┊', '┋',
		'┌', '┍', '┎', '┏', '┐', '┑', '┒', '┓',
		'└', '┕', '┖', '┗', '┘', '┙', '┚', '┛',
		'├', '┝', '┞', '┟', '┠', '┡', '┢', '┣',
		'┤', '┥', '┦', '┧', '┨', '┩', '┪', '┫',
		'┬', '┭', '┮', '┯', '┰', '┱', '┲', '┳',
		'┴', '┵', '┶', '┷', '┸', '┹', '┺', '┻',
		'┼', '┽', '┾', '┿', '╀', '╁', '╂', '╃',
		'╄', '╅', '╆', '╇', '╈', '╉', '╊', '╋',
		'╌', '╍', '╎', '╏',
		'═', '║', '╒', '╓', '╔', '╕', '╖', '╗',
		'╘', '╙', '╚', '╛', '╜', '╝', '╞', '╟',
		'╠', '╡', '╢', '╣', '╤', '╥', '╦', '╧',
		'╨', '╩', '╪', '╫', '╬',
	}...)
	return runes
}

// rasterizer renders glyphs with a fixed, calibrated geometry.
type rasterizer struct {
	font     *truetype.Font
	ppem     float64
	baseline int // device pixels at supersampled scale
	xOffset  int // device pixels at supersampled scale
}

// coverage rasterizes one glyph and returns the inked fraction (0..1) of
// each output cell.
func (r *rasterizer) coverage(ch rune) [img2ansi.GlyphHeight][img2ansi.GlyphWidth]float64 {
	var cov [img2ansi.GlyphHeight][img2ansi.GlyphWidth]float64
	canvas := img2ansi.GlyphHeight * superSample
	img := image.NewAlpha(image.Rect(0, 0, canvas, canvas))

	ctx := freetype.NewContext()
	ctx.SetDPI(72)
	ctx.SetFont(r.font)
	ctx.SetFontSize(r.ppem)
	ctx.SetClip(img.Bounds())
	ctx.SetDst(img)
	ctx.SetSrc(image.White)
	ctx.SetHinting(font.HintingNone)

	if _, err := ctx.DrawString(string(ch), freetype.Pt(r.xOffset, r.baseline)); err != nil {
		return cov
	}

	cell := superSample
	for gy := 0; gy < img2ansi.GlyphHeight; gy++ {
		for gx := 0; gx < img2ansi.GlyphWidth; gx++ {
			sum := 0
			for sy := 0; sy < cell; sy++ {
				for sx := 0; sx < cell; sx++ {
					sum += int(img.AlphaAt(gx*cell+sx, gy*cell+sy).A)
				}
			}
			cov[gy][gx] = float64(sum) / float64(cell*cell*255)
		}
	}
	return cov
}

// render rasterizes one glyph at 8x and downsamples by coverage: an
// output pixel is set if at least half of its supersampled area is inked.
func (r *rasterizer) render(ch rune) img2ansi.GlyphBitmap {
	cov := r.coverage(ch)
	var bm img2ansi.GlyphBitmap
	for gy := 0; gy < img2ansi.GlyphHeight; gy++ {
		for gx := 0; gx < img2ansi.GlyphWidth; gx++ {
			if cov[gy][gx] >= 0.5 {
				bm.SetBit(gx, gy, true)
			}
		}
	}
	return bm
}

// bitmapFromCoverage thresholds a coverage grid at 50%.
func bitmapFromCoverage(cov [img2ansi.GlyphHeight][img2ansi.GlyphWidth]float64) img2ansi.GlyphBitmap {
	var bm img2ansi.GlyphBitmap
	for gy := 0; gy < img2ansi.GlyphHeight; gy++ {
		for gx := 0; gx < img2ansi.GlyphWidth; gx++ {
			if cov[gy][gx] >= 0.5 {
				bm.SetBit(gx, gy, true)
			}
		}
	}
	return bm
}

// ambiguity sums how far each cell's coverage strays into the ambiguous
// middle: 0 for a perfectly crisp cell (0% or 100% inked), up to 0.5 for
// a cell that is exactly half covered.
func ambiguity(cov [img2ansi.GlyphHeight][img2ansi.GlyphWidth]float64) float64 {
	var total float64
	for gy := 0; gy < img2ansi.GlyphHeight; gy++ {
		for gx := 0; gx < img2ansi.GlyphWidth; gx++ {
			total += 0.5 - math.Abs(cov[gy][gx]-0.5)
		}
	}
	return total
}

// calibrationProbes are glyphs used to find the correct geometry: the
// blocks pin gross alignment, while letters and dither patterns carry
// single-pixel features that expose half-pixel phase errors.
const calibrationProbes = "█▀▌AMgs+=|░"

// calibrate finds the (ppem, baseline, xOffset) that renders the font on
// its design pixel grid.
//
// A bitmap font rendered at the correct geometry produces only crisp
// cells: every output pixel is either fully inked or fully empty. At a
// half-pixel phase offset, single-pixel strokes smear across two cells
// at ~50% coverage each — '█' and '▀' cannot detect this because their
// edges sit on cell boundaries, which is exactly how an earlier version
// of this calibration went wrong. So: '█' full and '▀' top-half are hard
// requirements, and among geometries satisfying them we minimize total
// coverage ambiguity over probe glyphs with fine detail.
//
// Candidate ppems are the naive size and an advance-corrected size (some
// pixel-font TTFs use 9-pixel advances, emulating VGA 9-dot mode).
func calibrate(f *truetype.Font) (*rasterizer, error) {
	size := float64(img2ansi.GlyphHeight * superSample)
	candidates := []float64{size}

	probe := truetype.NewFace(f, &truetype.Options{
		Size: size, DPI: 72, Hinting: font.HintingNone,
	})
	if adv, ok := probe.GlyphAdvance('M'); ok && adv > 0 {
		advPx := float64(adv) / 64
		if math.Abs(advPx-size) > 0.5 {
			candidates = append(candidates, size*size/advPx)
		}
	}
	probe.Close()

	var fullWant = ^img2ansi.GlyphBitmap(0)
	var topWant img2ansi.GlyphBitmap
	for y := 0; y < img2ansi.GlyphHeight/2; y++ {
		for x := 0; x < img2ansi.GlyphWidth; x++ {
			topWant.SetBit(x, y, true)
		}
	}

	canvas := img2ansi.GlyphHeight * superSample
	var best *rasterizer
	bestExact := false
	bestAmbiguity := math.Inf(1)

	for _, ppem := range candidates {
		for baseline := 0; baseline <= canvas*3/2; baseline++ {
			for xOffset := -superSample; xOffset <= superSample; xOffset++ {
				r := &rasterizer{font: f, ppem: ppem, baseline: baseline, xOffset: xOffset}
				if bitmapFromCoverage(r.coverage('█')) != fullWant {
					continue
				}
				if bitmapFromCoverage(r.coverage('▀')) != topWant {
					continue
				}
				var amb float64
				for _, ch := range calibrationProbes {
					amb += ambiguity(r.coverage(ch))
				}
				if !bestExact || amb < bestAmbiguity {
					best, bestExact, bestAmbiguity = r, true, amb
				}
			}
		}
	}

	if best == nil {
		return nil, fmt.Errorf(
			"calibration failed: no geometry renders '█' solid and '▀' as the top half")
	}
	log.Printf("Calibrated: ppem=%.2f baseline=%d xOffset=%d (ambiguity %.2f over %q)",
		best.ppem, best.baseline, best.xOffset, bestAmbiguity, calibrationProbes)
	if bestAmbiguity > 1.0 {
		log.Printf("WARNING: residual coverage ambiguity is high; inspect with -dump")
	}
	return best, nil
}

// computeFromTTF renders the research glyph set from a TrueType font.
func computeFromTTF(fontPath string) (*img2ansi.FontGlyphData, error) {
	fontBytes, err := os.ReadFile(fontPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read font: %w", err)
	}
	ttf, err := freetype.ParseFont(fontBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse font: %w", err)
	}

	r, err := calibrate(ttf)
	if err != nil {
		return nil, err
	}

	data := &img2ansi.FontGlyphData{
		FontName: fontPath,
		Glyphs:   make(map[rune]img2ansi.GlyphBitmap),
	}
	// Skip runes the font does not map: rendering them would silently
	// embed the font's missing-glyph box (a '?' in PxPlus IBM BIOS) in
	// place of the character. Absent glyphs let the library synthesize
	// geometric block characters or fall back cleanly at render time.
	var skipped []rune
	for _, ch := range glyphSet() {
		if ttf.Index(ch) == 0 {
			skipped = append(skipped, ch)
			continue
		}
		data.Glyphs[ch] = r.render(ch)
	}
	if len(skipped) > 0 {
		log.Printf("Skipped %d runes with no glyph in this font: %q",
			len(skipped), string(skipped))
	}
	return data, nil
}

// font8x8LineRe matches one glyph row in a dhepper/font8x8 C header,
// e.g.: { 0x0F, 0x0F, 0x0F, 0x0F, 0x0F, 0x0F, 0x0F, 0x0F},   // U+258C (left half)
var font8x8LineRe = regexp.MustCompile(
	`\{\s*((?:0x[0-9A-Fa-f]{2}\s*,\s*){7}0x[0-9A-Fa-f]{2})\s*,?\s*\}\s*,?\s*//\s*U\+([0-9A-Fa-f]{4})`)

// computeFromFont8x8 parses dhepper/font8x8-style C headers (public
// domain 8x8 bitmap fonts): one byte per row, LSB = leftmost pixel —
// the same layout as GlyphBitmap, so each row loads with a shift. The
// codepoint comes from the per-line U+XXXX comment, so array order and
// file ranges never need to be assumed.
func computeFromFont8x8(dir string) (*img2ansi.FontGlyphData, error) {
	headers, err := filepath.Glob(filepath.Join(dir, "*.h"))
	if err != nil || len(headers) == 0 {
		return nil, fmt.Errorf("no .h files found in %s", dir)
	}

	data := &img2ansi.FontGlyphData{
		FontName: dir,
		Glyphs:   make(map[rune]img2ansi.GlyphBitmap),
	}
	for _, path := range headers {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", path, err)
		}
		matches := font8x8LineRe.FindAllStringSubmatch(string(raw), -1)
		log.Printf("  %s: %d glyphs", filepath.Base(path), len(matches))
		for _, m := range matches {
			cp, err := strconv.ParseUint(m[2], 16, 32)
			if err != nil {
				continue
			}
			var g img2ansi.GlyphBitmap
			for y, hexByte := range strings.Split(m[1], ",") {
				b, err := strconv.ParseUint(strings.TrimSpace(hexByte), 0, 8)
				if err != nil {
					return nil, fmt.Errorf("%s: bad byte %q for U+%s", path, hexByte, m[2])
				}
				g |= img2ansi.GlyphBitmap(b) << (y * img2ansi.GlyphWidth)
			}
			// Duplicate labels indicate a mislabeled glyph (it shadows one
			// codepoint and leaves another missing); the vendored box
			// header had exactly this bug upstream at U+2547/U+2548/U+254B.
			if _, dup := data.Glyphs[rune(cp)]; dup {
				log.Printf("WARNING: %s: duplicate glyph label U+%s", path, m[2])
			}
			data.Glyphs[rune(cp)] = g
		}
	}
	return data, nil
}

// computeFromROM converts a 2048-byte CP437 ROM font dump.
func computeFromROM(romPath string) (*img2ansi.FontGlyphData, error) {
	raw, err := os.ReadFile(romPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read ROM: %w", err)
	}
	fb, err := img2ansi.LoadROMFont(raw, romPath)
	if err != nil {
		return nil, err
	}
	data := &img2ansi.FontGlyphData{
		FontName: romPath,
		Glyphs:   make(map[rune]img2ansi.GlyphBitmap),
	}
	for _, r := range fb.Runes() {
		g, _ := fb.GetGlyph(r)
		data.Glyphs[r] = g
	}
	return data, nil
}

func saveGlyphData(data *img2ansi.FontGlyphData, outputPath string) error {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if err := gob.NewEncoder(gz).Encode(data); err != nil {
		gz.Close()
		return fmt.Errorf("failed to encode data: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to close gzip: %w", err)
	}
	return os.WriteFile(outputPath, buf.Bytes(), 0644)
}

// compareAgainst diffs the freshly computed glyphs against an existing
// .glyphs file, reporting per-glyph changed pixel counts.
func compareAgainst(data *img2ansi.FontGlyphData, oldPath string) error {
	old, err := img2ansi.LoadGlyphFile(oldPath)
	if err != nil {
		return err
	}

	type diff struct {
		r      rune
		pixels int
	}
	var diffs []diff
	total, changed := 0, 0
	for r, g := range data.Glyphs {
		og, ok := old.GetGlyph(r)
		if !ok {
			continue
		}
		d := bits.OnesCount64(uint64(g ^ og))
		total += d
		if d > 0 {
			changed++
			diffs = append(diffs, diff{r, d})
		}
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].pixels > diffs[j].pixels })

	fmt.Printf("Compared %d glyphs against %s:\n", len(data.Glyphs), oldPath)
	fmt.Printf("  %d glyphs differ, %d total pixels changed\n", changed, total)
	for i, d := range diffs {
		if i >= 15 {
			fmt.Printf("  ... and %d more\n", len(diffs)-15)
			break
		}
		og, _ := old.GetGlyph(d.r)
		fmt.Printf("  %q: %d pixels changed (old %d set, new %d set)\n",
			d.r, d.pixels,
			bits.OnesCount64(uint64(og)),
			bits.OnesCount64(uint64(data.Glyphs[d.r])))
	}
	return nil
}

func dumpGlyphs(data *img2ansi.FontGlyphData, chars string, oldPath string) {
	var old *img2ansi.FontBitmaps
	if oldPath != "" {
		old, _ = img2ansi.LoadGlyphFile(oldPath)
	}
	for _, r := range chars {
		fmt.Printf("--- %q (new) ---\n%v", r, data.Glyphs[r])
		if old != nil {
			if og, ok := old.GetGlyph(r); ok {
				fmt.Printf("--- %q (old) ---\n%v", r, og)
			}
		}
	}
}

func main() {
	inputFont := flag.String("font", "", "Path to a TrueType font file")
	inputROM := flag.String("rom", "", "Path to a 2048-byte CP437 ROM font dump")
	inputF8x8 := flag.String("font8x8", "", "Directory of dhepper/font8x8 C headers")
	outputFile := flag.String("output", "", "Path to save the .glyphs output")
	comparePath := flag.String("compare", "", "Existing .glyphs file to diff against")
	dump := flag.String("dump", "", "Characters to print as bitmaps for inspection")
	flag.Parse()

	sources := 0
	for _, s := range []string{*inputFont, *inputROM, *inputF8x8} {
		if s != "" {
			sources++
		}
	}
	if sources != 1 {
		fmt.Println("Exactly one of -font, -rom, or -font8x8 is required")
		flag.PrintDefaults()
		os.Exit(1)
	}

	var data *img2ansi.FontGlyphData
	var err error
	switch {
	case *inputROM != "":
		log.Printf("Converting ROM font: %s", *inputROM)
		data, err = computeFromROM(*inputROM)
	case *inputF8x8 != "":
		log.Printf("Parsing font8x8 headers in: %s", *inputF8x8)
		data, err = computeFromFont8x8(*inputF8x8)
	default:
		log.Printf("Computing glyphs for font: %s", *inputFont)
		data, err = computeFromTTF(*inputFont)
	}
	if err != nil {
		log.Fatalf("Failed to compute glyphs: %v", err)
	}
	log.Printf("Computed %d glyphs", len(data.Glyphs))

	if *comparePath != "" {
		if err := compareAgainst(data, *comparePath); err != nil {
			log.Fatalf("Comparison failed: %v", err)
		}
	}
	if *dump != "" {
		dumpGlyphs(data, *dump, *comparePath)
	}

	if *outputFile != "" {
		if err := saveGlyphData(data, *outputFile); err != nil {
			log.Fatalf("Failed to save glyph data: %v", err)
		}
		if info, err := os.Stat(*outputFile); err == nil {
			log.Printf("Saved %s (%.2f KB)", *outputFile, float64(info.Size())/1024)
		}
	}
}
