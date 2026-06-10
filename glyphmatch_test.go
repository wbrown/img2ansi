package img2ansi

import (
	"testing"

	"github.com/wbrown/img2ansi/imageutil"
)

// glyphCellImage paints a glyph's bitmap into an 8x8 image, fg where
// bits are set, bg elsewhere.
func glyphCellImage(g GlyphBitmap, fg, bg RGB) *imageutil.RGBAImage {
	img := imageutil.NewRGBAImage(GlyphWidth, GlyphHeight)
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			c := bg
			if g.Bit(x, y) {
				c = fg
			}
			img.SetRGB(x, y, imageutil.RGB{R: c.R, G: c.G, B: c.B})
		}
	}
	return img
}

// TestGlyphMatcherExactGlyph is the promise the retired similarity
// scorer never kept: a cell that IS a glyph must match that glyph.
// The matcher's exhaustive ideal-mask search finds the zero-error
// solution by construction. Asserted on the mask rather than the rune,
// since distinct runes may share a bitmap.
func TestGlyphMatcherExactGlyph(t *testing.T) {
	r := NewRenderer(WithPalette("ansi16"))
	font, err := LoadEmbeddedFont("font8x8")
	if err != nil {
		t.Fatal(err)
	}
	m := NewGlyphMatcher(r, font)

	white := RGB{0xFF, 0xFF, 0xFF} // code 97
	black := RGB{0x00, 0x00, 0x00} // code 30

	for _, ch := range []rune{'O', 'A', '/', '╋', '▚'} {
		want, ok := font.GenuineGlyph(ch)
		if !ok {
			t.Fatalf("font8x8 missing %q", ch)
		}

		blocks := m.Convert(glyphCellImage(want, white, black),
			imageutil.NewGrayImage(GlyphWidth, GlyphHeight))
		got := blocks[0][0]

		gotMask, ok := font.GenuineGlyph(got.Rune)
		if !ok {
			t.Errorf("%q: matcher chose %q which is not a genuine glyph", ch, got.Rune)
			continue
		}
		// Accept the complementary solution (inverse mask with colors
		// swapped) — it renders identically.
		matches := gotMask == want && got.FG == white && got.BG == black
		complement := gotMask == ^want && got.FG == black && got.BG == white
		if !matches && !complement {
			t.Errorf("cell drawn as %q: matcher chose %q (fg=%v bg=%v)\nwant mask:\n%vgot mask:\n%v",
				ch, got.Rune, got.FG, got.BG, want, gotMask)
		}
	}
}

// TestGlyphMatcherFlatCell verifies uniform cells produce a flat block
// with fg == bg.
func TestGlyphMatcherFlatCell(t *testing.T) {
	r := NewRenderer(WithPalette("ansi16"))
	font, err := LoadEmbeddedFont("font8x8")
	if err != nil {
		t.Fatal(err)
	}
	m := NewGlyphMatcher(r, font)

	red := RGB{0xAA, 0x00, 0x00}
	img := imageutil.NewRGBAImage(GlyphWidth, GlyphHeight)
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			img.SetRGB(x, y, imageutil.RGB{R: red.R, G: red.G, B: red.B})
		}
	}

	blocks := m.Convert(img, imageutil.NewGrayImage(GlyphWidth, GlyphHeight))
	got := blocks[0][0]
	if got.FG != red || got.BG != red {
		t.Errorf("flat red cell should map to fg=bg=red, got fg=%v bg=%v", got.FG, got.BG)
	}
	if got.Rune != '█' {
		t.Errorf("flat cell should use the full block, got %q", got.Rune)
	}
}

// TestGlyphMatcherDiffusion verifies the diffusion mechanics: residuals
// of exactly-representable input are zero (so diffusion is a no-op and
// does not mutate the image), residuals of off-palette input propagate
// to later cells (so output differs from the undiffused matcher), and
// the whole process is deterministic.
func TestGlyphMatcherDiffusion(t *testing.T) {
	r := NewRenderer(WithPalette("ansi16"))
	font, err := LoadEmbeddedFont("font8x8")
	if err != nil {
		t.Fatal(err)
	}

	plain := NewGlyphMatcher(r, font)
	diffused := NewGlyphMatcher(r, font)
	diffused.Diffusion = true

	// Exactly-representable input: a flat field of a palette color has
	// zero residual everywhere; diffusion must change nothing.
	red := RGB{0xAA, 0x00, 0x00}
	flat := imageutil.NewRGBAImage(32, 16)
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			flat.SetRGB(x, y, imageutil.RGB{R: red.R, G: red.G, B: red.B})
		}
	}
	edges := imageutil.NewGrayImage(32, 16)
	a := plain.Convert(flat.Clone(), edges)
	b := diffused.Convert(flat.Clone(), edges)
	for cy := range a {
		for cx := range a[cy] {
			if a[cy][cx] != b[cy][cx] {
				t.Fatalf("diffusion changed output on zero-residual input at (%d,%d)", cx, cy)
			}
		}
	}

	// Off-palette ramp: residuals are nonzero, so diffusion must change
	// later cells relative to the undiffused matcher.
	ramp := makeFleshtone(64, 16)
	rampEdges := imageutil.NewGrayImage(64, 16)
	a = plain.Convert(ramp.Clone(), rampEdges)
	b = diffused.Convert(ramp.Clone(), rampEdges)
	same := true
	for cy := range a {
		for cx := range a[cy] {
			if a[cy][cx] != b[cy][cx] {
				same = false
			}
		}
	}
	if same {
		t.Error("diffusion produced identical output on an off-palette ramp")
	}

	// Deterministic with diffusion on.
	c := diffused.Convert(ramp.Clone(), rampEdges)
	d := diffused.Convert(ramp.Clone(), rampEdges)
	for cy := range c {
		for cx := range c[cy] {
			if c[cy][cx] != d[cy][cx] {
				t.Fatalf("diffused matching is non-deterministic at (%d,%d)", cx, cy)
			}
		}
	}
}

// TestGlyphMatcherDeterministic guards the sorted-glyph and sorted-
// anchor tie-breaking: identical input must produce identical output.
func TestGlyphMatcherDeterministic(t *testing.T) {
	r := NewRenderer(WithPalette("ansi16"))
	font, err := LoadEmbeddedFont("font8x8")
	if err != nil {
		t.Fatal(err)
	}
	m := NewGlyphMatcher(r, font)

	img := makeColorRamp(64, 64)
	edges := imageutil.NewGrayImage(64, 64)

	a := m.Convert(img.Clone(), edges)
	b := m.Convert(img.Clone(), edges)
	for y := range a {
		for x := range a[y] {
			if a[y][x] != b[y][x] {
				t.Fatalf("non-deterministic match at (%d,%d): %+v vs %+v",
					x, y, a[y][x], b[y][x])
			}
		}
	}
}
