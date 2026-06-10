package img2ansi

import (
	"strings"
	"testing"
)

// TestGlyphBitmapBitOperations tests basic bit operations on GlyphBitmap.
func TestGlyphBitmapBitOperations(t *testing.T) {
	var bitmap GlyphBitmap

	bitmap.SetBit(0, 0, true)
	if !bitmap.Bit(0, 0) {
		t.Error("Expected bit at (0,0) to be set")
	}

	bitmap.SetBit(7, 7, true)
	if !bitmap.Bit(7, 7) {
		t.Error("Expected bit at (7,7) to be set")
	}

	bitmap.SetBit(0, 0, false)
	if bitmap.Bit(0, 0) {
		t.Error("Expected bit at (0,0) to be clear")
	}

	bitmap.SetBit(8, 8, true)
	if bitmap.Bit(8, 8) {
		t.Error("Out of bounds bit should return false")
	}
}

// TestGlyphBitmapOrdering locks in the canonical bit ordering: row-major,
// LSB at top-left. Three different orderings coexisted in earlier
// implementations of this system; this test exists so a fourth never
// sneaks in.
func TestGlyphBitmapOrdering(t *testing.T) {
	// Bit 0 must be (0,0); bit 7 must be (7,0); bit 56 must be (0,7).
	if g := GlyphBitmap(1); !g.Bit(0, 0) {
		t.Error("bit 0 should be pixel (0,0)")
	}
	if g := GlyphBitmap(1 << 7); !g.Bit(7, 0) {
		t.Error("bit 7 should be pixel (7,0) - end of top row")
	}
	if g := GlyphBitmap(1 << 56); !g.Bit(0, 7) {
		t.Error("bit 56 should be pixel (0,7) - start of bottom row")
	}

	// String() must agree: top row first, left pixel first.
	g := GlyphBitmap(1) // only top-left set
	lines := strings.Split(strings.TrimSuffix(g.String(), "\n"), "\n")
	if len(lines) != GlyphHeight {
		t.Fatalf("String() should produce %d rows, got %d", GlyphHeight, len(lines))
	}
	if []rune(lines[0])[0] != '█' {
		t.Error("String(): bit (0,0) should render at the start of the first row")
	}
}

// TestLoadROMFont verifies parsing of the classic 2048-byte ROM format:
// 8 row bytes per glyph, MSB = leftmost pixel, CP437 glyph order.
func TestLoadROMFont(t *testing.T) {
	data := make([]byte, 256*GlyphHeight)

	// Glyph 0xDB (█ in CP437): all rows full.
	for y := 0; y < GlyphHeight; y++ {
		data[0xDB*GlyphHeight+y] = 0xFF
	}
	// Glyph 0xDF (▀): top half full.
	for y := 0; y < GlyphHeight/2; y++ {
		data[0xDF*GlyphHeight+y] = 0xFF
	}
	// Glyph 0x41 ('A'): single pixel at top-left of row 2 to test MSB order.
	data[0x41*GlyphHeight+2] = 0x80

	fb, err := LoadROMFont(data, "test-rom")
	if err != nil {
		t.Fatalf("LoadROMFont failed: %v", err)
	}

	full, ok := fb.GetGlyph('█')
	if !ok || full != ^GlyphBitmap(0) {
		t.Errorf("█ should be all 64 bits set, got:\n%v", full)
	}

	top, ok := fb.GetGlyph('▀')
	if !ok {
		t.Fatal("▀ missing from ROM font")
	}
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			want := y < GlyphHeight/2
			if top.Bit(x, y) != want {
				t.Fatalf("▀ pixel (%d,%d) = %v, want %v\n%v", x, y, !want, want, top)
			}
		}
	}

	a, ok := fb.GetGlyph('A')
	if !ok {
		t.Fatal("'A' missing from ROM font")
	}
	if !a.Bit(0, 2) {
		t.Errorf("ROM byte 0x80 should set the LEFTMOST pixel (MSB-first), got:\n%v", a)
	}
	if a.Bit(7, 2) {
		t.Errorf("ROM byte 0x80 should not set the rightmost pixel, got:\n%v", a)
	}

	// Wrong size must error.
	if _, err := LoadROMFont(make([]byte, 100), "bad"); err == nil {
		t.Error("LoadROMFont should reject data that is not 2048 bytes")
	}
}

// TestCP437TableAnchors spot-checks the CP437 mapping against well-known
// code points, and cross-checks it against the BBS encoder's
// unicodeToCP437 map so the two tables can never drift apart.
func TestCP437TableAnchors(t *testing.T) {
	anchors := map[byte]rune{
		0x07: '•',
		0x20: ' ',
		0x41: 'A',
		0x7C: '|',
		0xB0: '░',
		0xB3: '│',
		0xC4: '─',
		0xDB: '█',
		0xDC: '▄',
		0xDD: '▌',
		0xDE: '▐',
		0xDF: '▀',
		0xFE: '■',
	}
	for code, want := range anchors {
		if got := cp437ToUnicode[code]; got != want {
			t.Errorf("cp437ToUnicode[0x%02X] = %q, want %q", code, got, want)
		}
	}

	// Every entry in the BBS encoder's map must invert through this table.
	for r, code := range unicodeToCP437 {
		if got := cp437ToUnicode[code]; got != r {
			t.Errorf("unicodeToCP437[%q] = 0x%02X, but cp437ToUnicode[0x%02X] = %q",
				r, code, code, got)
		}
	}

	// No duplicate runes (every CP437 glyph must be addressable).
	seen := make(map[rune]int)
	for i, r := range cp437ToUnicode {
		if prev, dup := seen[r]; dup {
			t.Errorf("cp437ToUnicode duplicate rune %q at 0x%02X and 0x%02X", r, prev, i)
		}
		seen[r] = i
	}
}

// TestLoadEmbeddedFont verifies the embedded glyph data decodes and
// contains the characters the research code depends on.
func TestLoadEmbeddedFont(t *testing.T) {
	fb, err := LoadEmbeddedFont("pxplus_ibm_bios")
	if err != nil {
		t.Fatalf("LoadEmbeddedFont failed: %v", err)
	}

	if fb.GlyphCount() == 0 {
		t.Fatal("embedded font has no glyphs")
	}

	for _, r := range []rune{'A', ' ', '█', '▀', '▄'} {
		if _, ok := fb.GetGlyph(r); !ok {
			t.Errorf("embedded font missing glyph %q", r)
		}
	}

	// The full block must actually be full: this is the geometry
	// calibration target for the rasterizer, and if it is not solid the
	// glyph data was generated with broken alignment.
	full, _ := fb.GetGlyph('█')
	if full != ^GlyphBitmap(0) {
		t.Errorf("█ in embedded font should be all 64 bits, got %d bits:\n%v",
			popcount(full), full)
	}

	if _, err := LoadEmbeddedFont("no-such-font"); err == nil {
		t.Error("LoadEmbeddedFont should fail for unknown fonts")
	}
}

func popcount(g GlyphBitmap) int {
	count := 0
	for i := 0; i < 64; i++ {
		if g&(1<<i) != 0 {
			count++
		}
	}
	return count
}

// TestFontBitmapsRendering tests rendering blocks with font glyphs.
func TestFontBitmapsRendering(t *testing.T) {
	fb := &FontBitmaps{
		glyphs:   make(map[rune]GlyphBitmap),
		fallback: make(map[rune]GlyphBitmap),
		name:     "test",
	}

	fb.glyphs['█'] = ^GlyphBitmap(0)
	var halfBlock GlyphBitmap
	for y := 0; y < GlyphHeight/2; y++ {
		for x := 0; x < GlyphWidth; x++ {
			halfBlock.SetBit(x, y, true)
		}
	}
	fb.glyphs['▀'] = halfBlock

	red := RGB{255, 0, 0}
	blue := RGB{0, 0, 255}
	blocks := [][]BlockRune{
		{{Rune: '█', FG: red, BG: blue}, {Rune: '▀', FG: red, BG: blue}},
	}

	img := fb.RenderBlocks(blocks, 1)
	if img.Bounds().Dx() != 16 || img.Bounds().Dy() != 8 {
		t.Fatalf("Expected 16x8 image, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}

	// Full block cell: all FG.
	if r, _, _, _ := img.At(0, 0).RGBA(); r>>8 != 255 {
		t.Error("█ cell should render foreground red at (0,0)")
	}
	// Half block cell: FG top, BG bottom.
	if r, _, _, _ := img.At(8, 0).RGBA(); r>>8 != 255 {
		t.Error("▀ cell should render foreground red at top")
	}
	if _, _, b, _ := img.At(8, 7).RGBA(); b>>8 != 255 {
		t.Error("▀ cell should render background blue at bottom")
	}

	// Unknown rune renders as background.
	blocks = [][]BlockRune{{{Rune: 'Z', FG: red, BG: blue}}}
	img = fb.RenderBlocks(blocks, 2)
	if _, _, b, _ := img.At(5, 5).RGBA(); b>>8 != 255 {
		t.Error("unknown rune should render as background")
	}
}
