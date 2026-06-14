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

// TestGlyphMatcherMultiCellRoundTrip: an image painted entirely from
// font glyphs at cell-aligned positions must survive match -> render
// pixel-perfectly at every cell position. This pins the registration
// between matchCell's pixel indexing, the ideal-mask bit layout, and
// FontBitmaps.RenderBlocks — any off-by-one in any of the three shows
// up as mismatches at specific cells. (Written while diagnosing a
// suspected rendering offset that turned out to be font typography:
// letterform glyphs keep column 7 as spacing, which reads as a 1px
// shift when they are used as dither texture on photos.)
func TestGlyphMatcherMultiCellRoundTrip(t *testing.T) {
	r := NewRenderer(WithPalette("ansi16"))
	font, err := LoadEmbeddedFont("font8x8")
	if err != nil {
		t.Fatal(err)
	}

	white := RGB{0xFF, 0xFF, 0xFF}
	black := RGB{0x00, 0x00, 0x00}
	runes := []rune{'X', 'O', '/', '╋', '▚', 'A', '#', '%'}

	const cells = 8
	img := imageutil.NewRGBAImage(cells*GlyphWidth, cells*GlyphHeight)
	for cy := 0; cy < cells; cy++ {
		for cx := 0; cx < cells; cx++ {
			g, ok := font.GenuineGlyph(runes[(cx+cy)%len(runes)])
			if !ok {
				t.Fatalf("font8x8 missing %q", runes[(cx+cy)%len(runes)])
			}
			for y := 0; y < GlyphHeight; y++ {
				for x := 0; x < GlyphWidth; x++ {
					c := black
					if g.Bit(x, y) {
						c = white
					}
					img.SetRGB(cx*GlyphWidth+x, cy*GlyphHeight+y,
						imageutil.RGB{R: c.R, G: c.G, B: c.B})
				}
			}
		}
	}

	m := NewGlyphMatcher(r, font)
	blocks := m.Convert(img.Clone(),
		imageutil.NewGrayImage(cells*GlyphWidth, cells*GlyphHeight))
	rendered := imageutil.RGBAImageFromImage(font.RenderBlocks(blocks, 1))

	mismatches := 0
	for y := 0; y < cells*GlyphHeight; y++ {
		for x := 0; x < cells*GlyphWidth; x++ {
			if img.GetRGB(x, y) != rendered.GetRGB(x, y) {
				if mismatches < 5 {
					t.Errorf("pixel (%d,%d) in cell (%d,%d) does not round-trip",
						x, y, x/GlyphWidth, y/GlyphHeight)
				}
				mismatches++
			}
		}
	}
	if mismatches > 0 {
		t.Errorf("%d mismatched pixels of %d", mismatches,
			cells*cells*GlyphWidth*GlyphHeight)
	}
}

// TestGlyphMatcherRestrictAlphabet verifies the alphabet knob: the
// candidate set is the requested runes ∩ genuine glyphs, output never
// strays outside it, an empty intersection is rejected without
// clobbering the current alphabet, and nil restores the full set.
func TestGlyphMatcherRestrictAlphabet(t *testing.T) {
	r := NewRenderer(WithPalette("ansi16"))
	font, err := LoadEmbeddedFont("font8x8")
	if err != nil {
		t.Fatal(err)
	}
	m := NewGlyphMatcher(r, font)
	fullCount := len(m.runes)

	if err := m.RestrictAlphabet(AlphabetBlocks); err != nil {
		t.Fatal(err)
	}
	if len(m.runes) == 0 || len(m.runes) >= fullCount {
		t.Fatalf("blocks alphabet should be a proper subset: %d of %d",
			len(m.runes), fullCount)
	}
	allowed := make(map[rune]bool, len(m.runes))
	for _, ru := range m.runes {
		if ru != ' ' && (ru < 0x2580 || ru > 0x259F) {
			t.Errorf("rune %q escaped the blocks alphabet", ru)
		}
		allowed[ru] = true
	}

	// Output stays inside the alphabet.
	ramp := makeColorRamp(64, 32)
	blocks := m.Convert(ramp, imageutil.NewGrayImage(64, 32))
	for _, row := range blocks {
		for _, cell := range row {
			if !allowed[cell.Rune] {
				t.Fatalf("matcher emitted %q outside the restricted alphabet", cell.Rune)
			}
		}
	}

	// Empty intersection: error, alphabet unchanged.
	before := len(m.runes)
	if err := m.RestrictAlphabet([]rune{'あ'}); err == nil {
		t.Error("alphabet with no genuine glyphs should be rejected")
	}
	if len(m.runes) != before {
		t.Errorf("failed restriction clobbered the alphabet: %d -> %d",
			before, len(m.runes))
	}

	// The caller's preset slice must not be reordered by the call.
	if AlphabetBlocks[len(AlphabetBlocks)-1] != ' ' {
		t.Error("RestrictAlphabet mutated the caller's slice")
	}

	// nil restores the full genuine set.
	if err := m.RestrictAlphabet(nil); err != nil {
		t.Fatal(err)
	}
	if len(m.runes) != fullCount {
		t.Errorf("nil should restore the full set: %d of %d", len(m.runes), fullCount)
	}
}

// TestGlyphMatcherBeamSigma pins the display-aware matching contract:
// footprints of known glyphs quantize sensibly, hard-glyph input still
// resolves to its own glyph under a moderate beam, zero restores exact
// matching, negative sigma is rejected, and the mode is deterministic.
func TestGlyphMatcherBeamSigma(t *testing.T) {
	r := NewRenderer(WithPalette("ansi16"))
	font, err := LoadEmbeddedFont("font8x8")
	if err != nil {
		t.Fatal(err)
	}
	m := NewGlyphMatcher(r, font)

	// Footprint sanity: a full block blurs to full coverage everywhere;
	// an empty mask stays empty.
	full := glyphFootprint(^uint64(0), 0.5)
	empty := glyphFootprint(0, 0.5)
	for i := 0; i < cellPixels; i++ {
		if full[i] != footprintLevels-1 {
			t.Fatalf("full block footprint not saturated at pixel %d: %d", i, full[i])
		}
		if empty[i] != 0 {
			t.Fatalf("empty footprint not zero at pixel %d: %d", i, empty[i])
		}
	}

	if err := m.SetBeamSigma(-1); err == nil {
		t.Error("negative sigma should be rejected")
	}
	if err := m.SetBeamSigma(0.5); err != nil {
		t.Fatal(err)
	}
	if m.footprints == nil || len(m.footprints) != len(m.masks) {
		t.Fatal("footprints not built alongside masks")
	}

	// A cell that IS a glyph (hard pixels, max contrast) should still
	// resolve to that glyph under a moderate beam: its own footprint is
	// the closest displayed appearance to its hard rendering.
	white := RGB{0xFF, 0xFF, 0xFF}
	black := RGB{0x00, 0x00, 0x00}
	for _, ch := range []rune{'O', '╋', '▚'} {
		want, _ := font.GenuineGlyph(ch)
		blocks := m.Convert(glyphCellImage(want, white, black),
			imageutil.NewGrayImage(GlyphWidth, GlyphHeight))
		gotMask, ok := font.GenuineGlyph(blocks[0][0].Rune)
		if !ok || (gotMask != want && gotMask != ^want) {
			t.Errorf("beam matching of hard %q chose %q", ch, blocks[0][0].Rune)
		}
	}

	// Restriction rebuilds footprints too.
	if err := m.RestrictAlphabet(AlphabetBlocks); err != nil {
		t.Fatal(err)
	}
	if len(m.footprints) != len(m.masks) {
		t.Fatal("RestrictAlphabet did not rebuild footprints")
	}

	// Zero disables.
	if err := m.SetBeamSigma(0); err != nil {
		t.Fatal(err)
	}
	if m.footprints != nil {
		t.Fatal("zero sigma should clear footprints")
	}

	// Deterministic under beam matching.
	if err := m.RestrictAlphabet(nil); err != nil {
		t.Fatal(err)
	}
	if err := m.SetBeamSigma(0.5); err != nil {
		t.Fatal(err)
	}
	ramp := makeColorRamp(64, 32)
	rampEdges := imageutil.NewGrayImage(64, 32)
	a := m.Convert(ramp.Clone(), rampEdges)
	b := m.Convert(ramp.Clone(), rampEdges)
	for cy := range a {
		for cx := range a[cy] {
			if a[cy][cx] != b[cy][cx] {
				t.Fatalf("beam matching non-deterministic at (%d,%d)", cx, cy)
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

// TestGlyphMatcherAnchorsMatchExhaustive is the oracle that pins the
// measured 16-color finding: the dominant-anchor candidate heuristic
// reaches the same per-cell optimum as enumerating the entire palette.
// ExhaustiveColors is the exhaustive referee, the default anchor search
// the clever structure — the same shape as
// TestNearestNeighborMatchesLinearScan validating the KD-tree against a
// linear scan. Per-cell error is compared in the matcher's own (Redmean)
// objective; equality on every cell is the evidence that the 8x8
// matcher's 16-color limitation is the medium (two colors per 64-pixel
// cell), not the color search. If this ever reddens, full enumeration
// found a pair the anchors missed and the premise has changed —
// investigate it, do not relax the bound.
func TestGlyphMatcherAnchorsMatchExhaustive(t *testing.T) {
	font, err := LoadEmbeddedFont("font8x8")
	if err != nil {
		t.Fatal(err)
	}
	r := NewRenderer(WithPalette("ansi16"))

	cases := []struct {
		name string
		img  *imageutil.RGBAImage
	}{
		{"color-ramp", makeColorRamp(64, 64)},
		{"fleshtone", makeFleshtone(64, 64)},
		{"gray-gradient", makeGradient(64, 64)},
	}

	for _, tc := range cases {
		anchorMatcher := NewGlyphMatcher(r, font)
		exhaustive := NewGlyphMatcher(r, font)
		exhaustive.ExhaustiveColors = true

		edges := imageutil.NewGrayImage(tc.img.Width(), tc.img.Height())
		aOut := anchorMatcher.Convert(tc.img.Clone(), edges)
		eOut := exhaustive.Convert(tc.img.Clone(), edges)

		k := GlyphWidth
		var worst float64
		suboptimal, cells := 0, 0
		for cy := range aOut {
			for cx := range aOut[cy] {
				var cell [cellPixels]RGB
				for i := 0; i < cellPixels; i++ {
					p := tc.img.GetRGB(cx*k+i%k, cy*k+i/k)
					cell[i] = RGB{p.R, p.G, p.B}
				}
				aErr := cellRenderError(t, r, font, aOut[cy][cx], &cell)
				eErr := cellRenderError(t, r, font, eOut[cy][cx], &cell)
				cells++
				// exhaustive searches a superset, so eErr is the true
				// minimum; anchors can only tie or lose. The finding is
				// that they tie everywhere.
				if aErr > eErr+1e-6 {
					suboptimal++
					if d := aErr - eErr; d > worst {
						worst = d
					}
				}
			}
		}
		if suboptimal > 0 {
			t.Errorf("%s/ansi16: anchors suboptimal on %d/%d cells (worst excess Δ=%.4f) — "+
				"full enumeration beats the heuristic, the small-palette gap is the search",
				tc.name, suboptimal, cells, worst)
		} else {
			t.Logf("%s/ansi16: anchors reach the exhaustive optimum on all %d cells", tc.name, cells)
		}
	}
}

// cellRenderError is the matcher's own objective for one cell: the sum
// over the 64 pixels of the color distance (in the renderer's method) to
// the color each pixel renders as — fg where the chosen glyph's mask is
// set, bg elsewhere. It recomputes naively what matchCell derives via the
// ideal-mask formula, so it doubles as an independent check on that
// formula.
func cellRenderError(t *testing.T, r *Renderer, font *FontBitmaps, br BlockRune, cell *[cellPixels]RGB) float64 {
	t.Helper()
	g, ok := font.GenuineGlyph(br.Rune)
	if !ok {
		t.Fatalf("matcher emitted non-genuine rune %q", br.Rune)
	}
	var sum float64
	for i := 0; i < cellPixels; i++ {
		c := br.BG
		if g.Bit(i%GlyphWidth, i/GlyphWidth) {
			c = br.FG
		}
		sum += r.ColorMethod.Distance(cell[i], c)
	}
	return sum
}
