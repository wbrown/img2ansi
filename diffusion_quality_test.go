package img2ansi

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wbrown/img2ansi/imageutil"
)

// These are the diffusion and converter quality tests. The shared
// measurement machinery — the blurred-LAB metric, the display-geometry
// chain, the cross-converter arm runner, the CRT display model — lives
// in harness.go, where cmd/quality reaches it too; this file holds the
// tests, their control arms, and the legacy 2x2 rendering used by the
// original diffusion harness.
//
// The primary metric is low-pass perceptual error: both images are blurred
// with a Gaussian approximating viewing distance, then compared as mean
// delta-E in LAB space. Error diffusion exists to make local average tone
// match the source, and the blur exposes exactly that; unblurred MSE
// penalizes the dither pattern itself and rewards banding, so it is only
// reported as a secondary signal alongside the color-transition count from
// the original harness (more transitions on a gradient = smoother ramp).

// renderBlocksToImage renders a BlockRune grid at native resolution
// (2x2 pixels per block) using the quadrant table as ground truth.
func renderBlocksToImage(blocks [][]BlockRune) *imageutil.RGBAImage {
	return renderBlocksToImageScaled(blocks, 2)
}

// renderBlocksToImageScaled renders a BlockRune grid via the quadrant
// table at pxPerCell pixels per cell edge (must be even).
func renderBlocksToImageScaled(blocks [][]BlockRune, pxPerCell int) *imageutil.RGBAImage {
	height, width := len(blocks), len(blocks[0])
	half := pxPerCell / 2
	out := imageutil.NewRGBAImage(width*pxPerCell, height*pxPerCell)
	for by, row := range blocks {
		for bx, block := range row {
			quad := getQuadrantsForRune(block.Rune)
			fills := [4]bool{
				quad.TopLeft, quad.TopRight,
				quad.BottomLeft, quad.BottomRight,
			}
			for i, filled := range fills {
				c := block.BG
				if filled {
					c = block.FG
				}
				baseX := bx*pxPerCell + (i%2)*half
				baseY := by*pxPerCell + (i/2)*half
				for y := 0; y < half; y++ {
					for x := 0; x < half; x++ {
						out.SetRGB(baseX+x, baseY+y,
							imageutil.RGB{R: c.R, G: c.G, B: c.B})
					}
				}
			}
		}
	}
	return out
}

// ditherNoDiffusion runs block selection without error diffusion, as a
// baseline arm for the harness (port of the old brownDitherNoDiffusion).
func ditherNoDiffusion(r *Renderer, img *imageutil.RGBAImage, edges *imageutil.GrayImage) [][]BlockRune {
	height, width := img.Height(), img.Width()
	result := make([][]BlockRune, height/2)
	for by := range result {
		result[by] = make([]BlockRune, width/2)
		for bx := range result[by] {
			block := [4]RGB{
				rgbFromImageutil(img.GetRGB(bx*2, by*2)),
				rgbFromImageutil(img.GetRGB(bx*2+1, by*2)),
				rgbFromImageutil(img.GetRGB(bx*2, by*2+1)),
				rgbFromImageutil(img.GetRGB(bx*2+1, by*2+1)),
			}
			isEdge := edges.GrayAt(bx*2, by*2).Y > 128 ||
				edges.GrayAt(bx*2+1, by*2).Y > 128 ||
				edges.GrayAt(bx*2, by*2+1).Y > 128 ||
				edges.GrayAt(bx*2+1, by*2+1).Y > 128
			rn, fg, bg := r.FindBestBlockRepresentation(block, isEdge)
			result[by][bx] = BlockRune{Rune: rn, FG: fg, BG: bg}
		}
	}
	return result
}

// rawMSE returns the unblurred per-channel mean squared error.
func rawMSE(a, b *imageutil.RGBAImage) float64 {
	var total float64
	var count int
	for y := 0; y < a.Height(); y++ {
		for x := 0; x < a.Width(); x++ {
			pa, pb := a.GetRGB(x, y), b.GetRGB(x, y)
			dr := float64(pa.R) - float64(pb.R)
			dg := float64(pa.G) - float64(pb.G)
			db := float64(pa.B) - float64(pb.B)
			total += dr*dr + dg*dg + db*db
			count += 3
		}
	}
	return total / float64(count)
}

// countColorTransitions counts attribute changes along rows (port of the
// old harness metric; more transitions on a gradient = smoother ramp).
func countColorTransitions(blocks [][]BlockRune) int {
	transitions := 0
	for y := range blocks {
		for x := 1; x < len(blocks[y]); x++ {
			prev, curr := blocks[y][x-1], blocks[y][x]
			if prev.FG != curr.FG || prev.BG != curr.BG || prev.Rune != curr.Rune {
				transitions++
			}
		}
	}
	return transitions
}

// scaleNearest scales an image by an integer factor for visual inspection.
func scaleNearest(img *imageutil.RGBAImage, factor int) *image.RGBA {
	w, h := img.Width()*factor, img.Height()*factor
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := img.GetRGB(x/factor, y/factor)
			i := out.PixOffset(x, y)
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = p.R, p.G, p.B, 255
		}
	}
	return out
}

type diffusionResult struct {
	name        string
	labSigma1   float64
	labSigma2   float64
	labSigma4   float64
	mse         float64
	transitions int
}

// measureArms runs the no-diffusion and diffusion arms on the given image
// and reports metrics against the (pre-dither) reference.
func measureArms(t *testing.T, name string, r *Renderer, img *imageutil.RGBAImage, edges *imageutil.GrayImage) []diffusionResult {
	t.Helper()
	reference := img.Clone()

	arms := []struct {
		name   string
		dither func() [][]BlockRune
	}{
		// BrownDitherForBlocks mutates its input, so each arm gets a copy
		{"no-diffusion", func() [][]BlockRune {
			return ditherNoDiffusion(r, img.Clone(), edges)
		}},
		{"diffusion", func() [][]BlockRune {
			return r.BrownDitherForBlocks(img.Clone(), edges)
		}},
	}

	var results []diffusionResult
	for _, arm := range arms {
		blocks := arm.dither()
		rendered := renderBlocksToImage(blocks)
		res := diffusionResult{
			name:        fmt.Sprintf("%s/%s", name, arm.name),
			labSigma1:   BlurredLabError(rendered, reference, 1),
			labSigma2:   BlurredLabError(rendered, reference, 2),
			labSigma4:   BlurredLabError(rendered, reference, 4),
			mse:         rawMSE(rendered, reference),
			transitions: countColorTransitions(blocks),
		}
		results = append(results, res)
		t.Logf("%-28s blurredΔE σ1=%6.2f σ2=%6.2f σ4=%6.2f  rawMSE=%8.1f  transitions=%d",
			res.name, res.labSigma1, res.labSigma2, res.labSigma4, res.mse, res.transitions)

		if dir := os.Getenv("DIFFUSION_PNGS"); dir != "" {
			path := filepath.Join(dir, fmt.Sprintf("%s_%s.png", name, arm.name))
			if err := imageutil.SavePNG(scaleNearest(rendered, 4), path); err != nil {
				t.Logf("could not save %s: %v", path, err)
			}
		}
	}
	if dir := os.Getenv("DIFFUSION_PNGS"); dir != "" {
		path := filepath.Join(dir, fmt.Sprintf("%s_reference.png", name))
		if err := imageutil.SavePNG(scaleNearest(reference, 4), path); err != nil {
			t.Logf("could not save %s: %v", path, err)
		}
	}
	return results
}

// makeGradient builds a horizontal gray ramp (the old harness pattern,
// scaled up for stable statistics).
func makeGradient(width, height int) *imageutil.RGBAImage {
	img := imageutil.NewRGBAImage(width, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			gray := uint8(64 + x*128/width)
			img.SetRGB(x, y, imageutil.RGB{R: gray, G: gray, B: gray})
		}
	}
	return img
}

// makeFleshtone builds the brown-to-pink ramp from the old harness.
func makeFleshtone(width, height int) *imageutil.RGBAImage {
	img := imageutil.NewRGBAImage(width, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			f := float64(x) / float64(width-1)
			img.SetRGB(x, y, imageutil.RGB{
				R: uint8(170 + f*50),
				G: uint8(100 + f*80),
				B: uint8(60 + f*90),
			})
		}
	}
	return img
}

// makeColorRamp builds a two-axis color ramp (red-green by x, blue by y).
func makeColorRamp(width, height int) *imageutil.RGBAImage {
	img := imageutil.NewRGBAImage(width, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGB(x, y, imageutil.RGB{
				R: uint8(x * 255 / (width - 1)),
				G: uint8(255 - x*255/(width-1)),
				B: uint8(y * 255 / (height - 1)),
			})
		}
	}
	return img
}

// measureConverterArms adapts MeasureConverterArms (harness.go) to the
// test: rows log through t.Logf, comparison renders land in the
// DIFFUSION_PNGS directory when set, and scores come back keyed by arm
// name for assertions.
func measureConverterArms(
	t *testing.T,
	name string,
	src ImageSource,
	cellsW, cellsH int,
	arms []ConverterArm,
) map[string]float64 {
	t.Helper()
	scores, _, err := MeasureConverterArms(
		name, src, cellsW, cellsH, arms, t.Logf, os.Getenv("DIFFUSION_PNGS"))
	if err != nil {
		t.Fatal(err)
	}
	results := make(map[string]float64, len(scores))
	for _, s := range scores {
		results[s.Name] = s.OneCell
	}
	return results
}

// nearestFg maps a color to the nearest foreground palette color using
// the renderer's tables.
func nearestFg(r *Renderer, c RGB) RGB {
	if r.fgClosestColor != nil {
		return (*r.fgClosestColor)[c.toUint32()]
	}
	nearest, _ := r.fgTree.nearestNeighbor(
		c, r.fgTree.Color, math.MaxFloat64, 0, r.ColorMethod)
	return nearest
}

// noDiffusionQuadrant is the diffusion-ablated control arm: the same
// per-block search as the quadrant dither with no error diffusion.
// Against the glyph matcher (also undiffused) it isolates the
// REPRESENTATION variable — 2x2 quadrants vs 8x8 glyphs — and against
// the full dither it isolates diffusion. Without it, dither-vs-matcher
// comparisons conflate the two.
type noDiffusionQuadrant struct{ r *Renderer }

func (n noDiffusionQuadrant) SourcePixelsPerCell() int { return 2 }

func (n noDiffusionQuadrant) Convert(img *imageutil.RGBAImage, edges *imageutil.GrayImage) [][]BlockRune {
	return ditherNoDiffusion(n.r, img, edges)
}

// meanColorConverter is the floor any 8x8 glyph matcher must beat: each
// cell becomes a full block in the palette color nearest the cell mean.
type meanColorConverter struct{ r *Renderer }

func (m meanColorConverter) SourcePixelsPerCell() int { return GlyphWidth }

func (m meanColorConverter) Convert(img *imageutil.RGBAImage, edges *imageutil.GrayImage) [][]BlockRune {
	k := m.SourcePixelsPerCell()
	cellsH, cellsW := img.Height()/k, img.Width()/k
	out := make([][]BlockRune, cellsH)
	for cy := range out {
		out[cy] = make([]BlockRune, cellsW)
		for cx := range out[cy] {
			var rSum, gSum, bSum int
			for y := 0; y < k; y++ {
				for x := 0; x < k; x++ {
					p := img.GetRGB(cx*k+x, cy*k+y)
					rSum += int(p.R)
					gSum += int(p.G)
					bSum += int(p.B)
				}
			}
			n := k * k
			fg := nearestFg(m.r, RGB{
				uint8(rSum / n), uint8(gSum / n), uint8(bSum / n)})
			out[cy][cx] = BlockRune{Rune: '█', FG: fg, BG: fg}
		}
	}
	return out
}

// TestConverterArms exercises the cross-converter harness end to end:
// the quadrant dither, the glyph matcher, and an 8x8 full-block
// mean-color baseline run on the same cell grid under both the ansi16
// and ansi256 palettes, all rendered through font glyphs and the
// display chain (the quadrant runes are font glyphs like any others),
// and scored against the same reference. Photos size width-first to
// the full 80 columns, rows following the source aspect (FitGrid);
// synthetics use the 80x25 screen.
func TestConverterArms(t *testing.T) {
	font, err := LoadEmbeddedFont("font8x8")
	if err != nil {
		t.Fatal(err)
	}

	patterns := []struct {
		name string
		src  ImageSource
	}{
		{"gray-gradient", SyntheticSource(makeGradient)},
		{"fleshtone", SyntheticSource(makeFleshtone)},
		{"color-ramp", SyntheticSource(makeColorRamp)},
	}

	for _, pal := range []string{"ansi16", "ansi256"} {
		t.Run(pal, func(t *testing.T) {
			r := NewRenderer(WithPalette(pal))
			diffusedMatcher := NewGlyphMatcher(r, font)
			diffusedMatcher.Diffusion = true
			// Exhaustive-color matcher (no diffusion): the visual form of
			// TestGlyphMatcherAnchorsMatchExhaustive — at a small palette
			// its panel must read identical to glyph-matcher.
			exhaustiveMatcher := NewGlyphMatcher(r, font)
			exhaustiveMatcher.ExhaustiveColors = true
			// Display-aware matcher (diffusion + beam): the variant the
			// blurred-ΔE numbers say wins at 16 colors, shown so the claim
			// can be judged by eye, not just by metric.
			displayMatcher := NewGlyphMatcher(r, font)
			displayMatcher.Diffusion = true
			if err := displayMatcher.SetBeamSigma(0.5); err != nil {
				t.Fatal(err)
			}
			// Penalty comparison arms: isolate gutter vs readable vs both,
			// byte-matched + diffusion, at a high weight to read the
			// ceiling. The readable/gutter % columns quantify what the
			// blurred-LAB metric cannot see — delta-E barely moves across
			// all three while the composition changes completely.
			penaltyArm := func(gutter, readable float64) BlockConverter {
				m := NewGlyphMatcher(r, font)
				m.Diffusion = true
				m.GutterPenalty = gutter
				m.ReadablePenalty = readable
				return m
			}
			arms := []ConverterArm{
				FontArm("quadrant-dither", r, font),
				FontArm("quadrant-no-diff", noDiffusionQuadrant{r}, font),
				FontArm("glyph-matcher", NewGlyphMatcher(r, font), font),
				FontArm("glyph-matcher-exhaust", exhaustiveMatcher, font),
				FontArm("glyph-matcher-diff", diffusedMatcher, font),
				FontArm("glyph-matcher-display", displayMatcher, font),
				FontArm("glyph-gutter", penaltyArm(1e5, 0), font),
				FontArm("glyph-readable", penaltyArm(0, 1e5), font),
				FontArm("glyph-both", penaltyArm(1e5, 1e5), font),
				FontArm("mean-color-block", meanColorConverter{r}, font),
			}

			for _, p := range patterns {
				res := measureConverterArms(t, p.name+"-"+pal,
					p.src, targetCols, targetRows, arms)
				if res["quadrant-dither"] >= res["mean-color-block"] {
					t.Errorf("%s/%s: quadrant dither (ΔE %.2f) should beat the mean-color baseline (ΔE %.2f)",
						p.name, pal, res["quadrant-dither"], res["mean-color-block"])
				}
				// The flat block is inside the matcher's search space
				// (the cell-mean anchor guarantees it), so exhaustive
				// matching can never be meaningfully worse than the
				// mean-color baseline. Whether it does much BETTER is
				// palette-dependent: one (fg, bg) pair per 8x8 cell
				// cannot follow a gradient at 16 colors.
				if res["glyph-matcher"] > res["mean-color-block"]*1.05 {
					t.Errorf("%s/%s: glyph matcher (ΔE %.2f) should not lose to the mean-color baseline (ΔE %.2f)",
						p.name, pal, res["glyph-matcher"], res["mean-color-block"])
				}
				// Diffusion's job is local tone on content the palette
				// cannot represent exactly; on those ramps it must beat
				// the undiffused matcher by a wide margin. The gray
				// gradient is excluded: at 256 colors it is nearly
				// exactly representable, and diffusing sub-quantum
				// residuals adds noise (measured: 0.45 -> 0.63).
				if p.name != "gray-gradient" &&
					res["glyph-matcher-diff"] >= res["glyph-matcher"] {
					t.Errorf("%s/%s: diffused matcher (ΔE %.2f) should beat the undiffused matcher (ΔE %.2f)",
						p.name, pal, res["glyph-matcher-diff"], res["glyph-matcher"])
				}
			}

			photos, _ := filepath.Glob("images/*.png")
			for _, path := range photos {
				img, err := imageutil.LoadImage(path)
				if err != nil {
					continue
				}
				cols, rows := FitGrid(float64(img.Width()) / float64(img.Height()))
				name := filepath.Base(path)
				measureConverterArms(t, name[:len(name)-len(".png")]+"-"+pal,
					PhotoSource(img), cols, rows, arms)
			}
		})
	}
}

// TestAlphabetLadderUnderCRT scores the matcher's alphabet ladder both
// as raw rectangle pixels and through the CRT display model. The 8x8
// fonts assume a beam-spot low-pass, so letterform spacing columns are
// penalized by hard-pixel scoring in a way no period display ever
// showed; the display model measures how much of that penalty is a
// rendering anachronism.
func TestAlphabetLadderUnderCRT(t *testing.T) {
	font, err := LoadEmbeddedFont("font8x8")
	if err != nil {
		t.Fatal(err)
	}
	img, err := imageutil.LoadImage("images/mandrill.png")
	if err != nil {
		t.Fatal(err)
	}
	cols, rows := FitGrid(float64(img.Width()) / float64(img.Height()))
	screenW, screenH := screenDims(cols, rows)
	src := PhotoSource(img)
	reference, _ := src(screenW, screenH)

	const beamSigma = 0.5 // beam spot, in font pixels

	renderArm := func(m *GlyphMatcher) *imageutil.RGBAImage {
		input, edges := src(cols*scorePxPerCell, rows*scorePxPerCell)
		return displayView(imageutil.RGBAImageFromImage(
			font.RenderBlocks(m.Convert(input, edges), 1)), screenH)
	}

	alphabets := []struct {
		name  string
		runes []rune
	}{
		{"full", nil},
		{"blocks+box", append(append([]rune{}, AlphabetBlocks...), AlphabetBoxDrawing...)},
		{"ascii", AlphabetASCII},
	}

	for _, pal := range []string{"ansi16", "ansi256"} {
		r := NewRenderer(WithPalette(pal))
		for _, a := range alphabets {
			m := NewGlyphMatcher(r, font)
			m.Diffusion = true
			if err := m.RestrictAlphabet(a.runes); err != nil {
				t.Fatal(err)
			}
			rendered := renderArm(m)
			raw := BlurredLabError(rendered, reference, 1.0*scorePxPerCell)
			crt := BlurredLabError(CRTDisplay(rendered, beamSigma), reference, 1.0*scorePxPerCell)
			t.Logf("%-8s %-11s ΔE raw=%6.2f  crt(σ=%.1f)=%6.2f  (%+.0f%%)",
				pal, a.name, raw, beamSigma, crt, (crt/raw-1)*100)
		}

		// Display-aware matching: the matcher scores candidates as their
		// CRT'd appearance (SetBeamSigma), so its objective aligns with
		// the CRT-scored metric. Judged as displayed, matching what the
		// display shows should not lose to matching what the bytes say.
		byteMatched := NewGlyphMatcher(r, font)
		byteMatched.Diffusion = true
		displayMatched := NewGlyphMatcher(r, font)
		displayMatched.Diffusion = true
		if err := displayMatched.SetBeamSigma(beamSigma); err != nil {
			t.Fatal(err)
		}
		crtScores := make(map[string]float64)
		for _, m := range []struct {
			name    string
			matcher *GlyphMatcher
		}{{"byte-matched", byteMatched}, {"display-matched", displayMatched}} {
			rendered := renderArm(m.matcher)
			raw := BlurredLabError(rendered, reference, 1.0*scorePxPerCell)
			crt := BlurredLabError(CRTDisplay(rendered, beamSigma), reference, 1.0*scorePxPerCell)
			crtScores[m.name] = crt
			t.Logf("%-8s %-15s ΔE raw=%6.2f  crt(σ=%.1f)=%6.2f",
				pal, m.name, raw, beamSigma, crt)
		}
		// Objective alignment: judged as displayed, matching what the
		// display shows must not lose to matching what the bytes say.
		if crtScores["display-matched"] >= crtScores["byte-matched"] {
			t.Errorf("%s: display-matched (ΔE %.2f) should beat byte-matched (ΔE %.2f) under the display model",
				pal, crtScores["display-matched"], crtScores["byte-matched"])
		}
	}
}

func TestDiffusionQualityGradients(t *testing.T) {
	r := NewRenderer(WithPalette("ansi16"))

	patterns := []struct {
		name string
		img  *imageutil.RGBAImage
	}{
		{"gray-gradient", makeGradient(128, 32)},
		{"fleshtone", makeFleshtone(128, 32)},
		{"color-ramp", makeColorRamp(128, 64)},
	}

	for _, p := range patterns {
		edges := imageutil.NewGrayImage(p.img.Width(), p.img.Height())
		results := measureArms(t, p.name, r, p.img, edges)

		// Diffusion exists to improve local tone accuracy; on smooth
		// ramps it must beat per-block quantization on the blurred
		// perceptual metric by a wide margin.
		noDiff, diff := results[0], results[1]
		if diff.labSigma2 >= noDiff.labSigma2 {
			t.Errorf("%s: diffusion (ΔE=%.2f) should beat no-diffusion (ΔE=%.2f) on blurred σ2 error",
				p.name, diff.labSigma2, noDiff.labSigma2)
		}
	}
}

// TestBlockAlphabetQuality measures what restricting the pattern
// alphabet costs: the full 16-block set against the 6 CP437 blocks,
// with an identical palette so only the alphabet differs. The full
// alphabet searches a superset, so it must never be meaningfully worse.
// Photos in images/ are included in the log when present.
func TestBlockAlphabetQuality(t *testing.T) {
	full := NewRenderer(WithPalette("ansi16"))
	cp437 := NewRenderer(WithBBSMode(), WithPalette("ansi16"))

	score := func(r *Renderer, img *imageutil.RGBAImage, edges *imageutil.GrayImage, ref *imageutil.RGBAImage) float64 {
		blocks := r.BrownDitherForBlocks(img.Clone(), edges)
		return BlurredLabError(renderBlocksToImage(blocks), ref, 2)
	}

	measure := func(name string, img *imageutil.RGBAImage, edges *imageutil.GrayImage, assert bool) {
		ref := img.Clone()
		f := score(full, img, edges, ref)
		c := score(cp437, img, edges, ref)
		t.Logf("%-14s blurredΔE σ2: 16-block=%6.2f  6-block=%6.2f  (restriction cost %+.0f%%)",
			name, f, c, (c/f-1)*100)
		if assert && f > c*1.10 {
			t.Errorf("%s: full alphabet (%.2f) should not be worse than restricted (%.2f)",
				name, f, c)
		}
	}

	patterns := []struct {
		name string
		img  *imageutil.RGBAImage
	}{
		{"gray-gradient", makeGradient(128, 32)},
		{"fleshtone", makeFleshtone(128, 32)},
		{"color-ramp", makeColorRamp(128, 64)},
	}
	for _, p := range patterns {
		edges := imageutil.NewGrayImage(p.img.Width(), p.img.Height())
		measure(p.name, p.img, edges, true)
	}

	photos, _ := filepath.Glob("images/*.png")
	for _, path := range photos {
		img, err := imageutil.LoadImage(path)
		if err != nil {
			continue
		}
		width := 100
		aspect := float64(img.Width()) / float64(img.Height())
		height := int(float64(width) / aspect / 2.0)
		resized, edges := imageutil.PrepareForANSI(img, width, height)
		name := filepath.Base(path)
		measure(name[:len(name)-len(".png")], resized, edges, false)
	}
}

func TestDiffusionQualityPhotos(t *testing.T) {
	// testdata/mandrill.tiff is committed; images/*.png is the committed
	// reference corpus (see images/README.md).
	photos, _ := filepath.Glob("images/*.png")
	photos = append([]string{"testdata/mandrill.tiff"}, photos...)

	r := NewRenderer(WithPalette("ansi16"))
	for _, path := range photos {
		img, err := imageutil.LoadImage(path)
		if err != nil {
			t.Errorf("loading %s: %v", path, err)
			continue
		}
		width := 100
		aspect := float64(img.Width()) / float64(img.Height())
		height := int(float64(width) / aspect / 2.0)
		resized, edges := imageutil.PrepareForANSI(img, width, height)

		name := strings.TrimSuffix(
			strings.ReplaceAll(path, string(filepath.Separator), "-"),
			filepath.Ext(path))
		measureArms(t, name, r, resized, edges)
	}
}
