package img2ansi

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wbrown/img2ansi/imageutil"
)

// This file is the diffusion quality harness. It renders BlockRune output
// back to pixels and scores it against the pre-dither reference image.
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

// gaussianBlurPlanes applies a separable Gaussian blur to an image,
// returning per-pixel RGB float triples (edge pixels use clamped sampling).
func gaussianBlurPlanes(img *imageutil.RGBAImage, sigma float64) [][][3]float64 {
	width, height := img.Width(), img.Height()
	radius := int(math.Ceil(3 * sigma))
	kernel := make([]float64, 2*radius+1)
	var sum float64
	for i := range kernel {
		d := float64(i - radius)
		kernel[i] = math.Exp(-d * d / (2 * sigma * sigma))
		sum += kernel[i]
	}
	for i := range kernel {
		kernel[i] /= sum
	}

	clamp := func(v, hi int) int {
		if v < 0 {
			return 0
		}
		if v >= hi {
			return hi - 1
		}
		return v
	}

	// Horizontal pass
	horiz := make([][][3]float64, height)
	for y := 0; y < height; y++ {
		horiz[y] = make([][3]float64, width)
		for x := 0; x < width; x++ {
			var acc [3]float64
			for k, w := range kernel {
				p := img.GetRGB(clamp(x+k-radius, width), y)
				acc[0] += float64(p.R) * w
				acc[1] += float64(p.G) * w
				acc[2] += float64(p.B) * w
			}
			horiz[y][x] = acc
		}
	}

	// Vertical pass
	out := make([][][3]float64, height)
	for y := 0; y < height; y++ {
		out[y] = make([][3]float64, width)
		for x := 0; x < width; x++ {
			var acc [3]float64
			for k, w := range kernel {
				p := horiz[clamp(y+k-radius, height)][x]
				acc[0] += p[0] * w
				acc[1] += p[1] * w
				acc[2] += p[2] * w
			}
			out[y][x] = acc
		}
	}
	return out
}

// blurredLabError blurs both images with the given sigma and returns the
// mean delta-E (CIE76, via LAB) between them. This is the primary
// diffusion quality metric: it measures local average tone accuracy.
func blurredLabError(a, b *imageutil.RGBAImage, sigma float64) float64 {
	pa := gaussianBlurPlanes(a, sigma)
	pb := gaussianBlurPlanes(b, sigma)
	lab := LABMethod{}
	var total float64
	var count int
	toRGB := func(p [3]float64) RGB {
		c := func(v float64) uint8 {
			return uint8(math.Max(0, math.Min(255, math.Round(v))))
		}
		return RGB{c(p[0]), c(p[1]), c(p[2])}
	}
	for y := range pa {
		for x := range pa[y] {
			total += lab.Distance(toRGB(pa[y][x]), toRGB(pb[y][x]))
			count++
		}
	}
	return total / float64(count)
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
			labSigma1:   blurredLabError(rendered, reference, 1),
			labSigma2:   blurredLabError(rendered, reference, 2),
			labSigma4:   blurredLabError(rendered, reference, 4),
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

// --- Cross-converter harness -------------------------------------------
//
// Any BlockConverter can be scored against any other on the same cell
// grid: each arm's input is prepared at its native source resolution
// (SourcePixelsPerCell), its output rendered back to pixels at a common
// scoring resolution, and all arms compared against the same reference.

// scorePxPerCell is the common resolution converter arms are scored at:
// every arm's output is rendered at 8 px per cell and compared against
// a reference prepared at the same size.
const scorePxPerCell = GlyphWidth

// converterArm pairs a BlockConverter with a renderer that turns its
// output back into pixels at scorePxPerCell for scoring.
type converterArm struct {
	name   string
	conv   BlockConverter
	render func([][]BlockRune) *imageutil.RGBAImage
}

// quadrantArm scores a Renderer's quadrant dither, rendered via the
// quadrant geometry table.
func quadrantArm(name string, r *Renderer) converterArm {
	return converterArm{
		name: name,
		conv: r,
		render: func(blocks [][]BlockRune) *imageutil.RGBAImage {
			return renderBlocksToImageScaled(blocks, scorePxPerCell)
		},
	}
}

// fontArm scores a converter whose output is rendered through font
// glyph bitmaps.
func fontArm(name string, conv BlockConverter, font *FontBitmaps) converterArm {
	return converterArm{
		name: name,
		conv: conv,
		render: func(blocks [][]BlockRune) *imageutil.RGBAImage {
			return imageutil.RGBAImageFromImage(
				font.RenderBlocks(blocks, scorePxPerCell/GlyphWidth))
		},
	}
}

// imageSource produces the source content at a requested pixel size,
// with its edge map. Synthetic patterns regenerate analytically so each
// arm sees the same content at its native resolution; photos go through
// the standard prepare pipeline.
type imageSource func(pxW, pxH int) (*imageutil.RGBAImage, *imageutil.GrayImage)

func syntheticSource(gen func(w, h int) *imageutil.RGBAImage) imageSource {
	return func(pxW, pxH int) (*imageutil.RGBAImage, *imageutil.GrayImage) {
		return gen(pxW, pxH), imageutil.NewGrayImage(pxW, pxH)
	}
}

func photoSource(img *imageutil.RGBAImage) imageSource {
	return func(pxW, pxH int) (*imageutil.RGBAImage, *imageutil.GrayImage) {
		return imageutil.PrepareForANSI(img, pxW/2, pxH/2)
	}
}

// measureConverterArms runs each converter over the same cell grid and
// scores every arm against the same reference at scorePxPerCell. Blur
// sigma is expressed in cell widths so the metric is comparable across
// converters regardless of their source resolution. With DIFFUSION_PNGS
// set, a labeled side-by-side comparison image (reference plus every
// arm) is written alongside the per-arm renders.
func measureConverterArms(
	t *testing.T,
	name string,
	src imageSource,
	cellsW, cellsH int,
	arms []converterArm,
) map[string]float64 {
	t.Helper()
	reference, _ := src(cellsW*scorePxPerCell, cellsH*scorePxPerCell)

	labels := []string{"reference"}
	panels := []*imageutil.RGBAImage{reference}

	results := make(map[string]float64)
	for _, arm := range arms {
		k := arm.conv.SourcePixelsPerCell()
		input, edges := src(cellsW*k, cellsH*k)
		blocks := arm.conv.Convert(input, edges)
		rendered := arm.render(blocks)

		halfCell := blurredLabError(rendered, reference, 0.5*scorePxPerCell)
		oneCell := blurredLabError(rendered, reference, 1.0*scorePxPerCell)
		results[arm.name] = oneCell
		t.Logf("%-14s %-18s blurredΔE σ=0.5cell %6.2f  σ=1cell %6.2f",
			name, arm.name, halfCell, oneCell)

		labels = append(labels, fmt.Sprintf("%s dE=%.2f", arm.name, oneCell))
		panels = append(panels, rendered)

		if dir := os.Getenv("DIFFUSION_PNGS"); dir != "" {
			path := filepath.Join(dir, fmt.Sprintf("%s_%s.png", name, arm.name))
			if err := imageutil.SavePNG(rendered.RGBA, path); err != nil {
				t.Logf("could not save %s: %v", path, err)
			}
		}
	}

	if dir := os.Getenv("DIFFUSION_PNGS"); dir != "" {
		font, err := LoadEmbeddedFont("font8x8")
		if err != nil {
			t.Fatalf("loading label font: %v", err)
		}
		path := filepath.Join(dir, fmt.Sprintf("%s_compare.png", name))
		if err := imageutil.SavePNG(composeComparison(font, labels, panels), path); err != nil {
			t.Logf("could not save %s: %v", path, err)
		}
	}
	return results
}

// composeComparison stacks labeled panels vertically into a single
// comparison image. Labels are rendered through the font's own glyphs.
func composeComparison(font *FontBitmaps, labels []string, panels []*imageutil.RGBAImage) *image.RGBA {
	const gutter = 4
	labelH := GlyphHeight

	width, height := 0, gutter
	for _, p := range panels {
		if p.Width() > width {
			width = p.Width()
		}
		height += labelH + 2 + p.Height() + gutter
	}

	dark := color.RGBA{24, 24, 24, 255}
	out := image.NewRGBA(image.Rect(0, 0, width+2*gutter, height))
	draw.Draw(out, out.Bounds(), &image.Uniform{dark}, image.Point{}, draw.Src)

	y := gutter
	for i, p := range panels {
		row := make([]BlockRune, 0, len(labels[i]))
		for _, ch := range labels[i] {
			row = append(row, BlockRune{
				Rune: ch,
				FG:   RGB{255, 255, 255},
				BG:   RGB{24, 24, 24},
			})
		}
		lbl := font.RenderBlocks([][]BlockRune{row}, 1)
		draw.Draw(out,
			image.Rect(gutter, y, gutter+lbl.Bounds().Dx(), y+labelH),
			lbl, image.Point{}, draw.Src)
		y += labelH + 2

		draw.Draw(out,
			image.Rect(gutter, y, gutter+p.Width(), y+p.Height()),
			p.RGBA, image.Point{}, draw.Src)
		y += p.Height() + gutter
	}
	return out
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
// and ansi256 palettes, render through their own paths (quadrant table
// vs font glyphs), and score against the same reference. The original
// research found 256 colors rescue 8x8 matching; this is where that
// claim is measured.
func TestConverterArms(t *testing.T) {
	font, err := LoadEmbeddedFont("font8x8")
	if err != nil {
		t.Fatal(err)
	}

	patterns := []struct {
		name           string
		src            imageSource
		cellsW, cellsH int
	}{
		{"gray-gradient", syntheticSource(makeGradient), 64, 16},
		{"fleshtone", syntheticSource(makeFleshtone), 64, 16},
		{"color-ramp", syntheticSource(makeColorRamp), 64, 32},
	}

	for _, pal := range []string{"ansi16", "ansi256"} {
		t.Run(pal, func(t *testing.T) {
			r := NewRenderer(WithPalette(pal))
			arms := []converterArm{
				quadrantArm("quadrant-dither", r),
				fontArm("glyph-matcher", NewGlyphMatcher(r, font), font),
				fontArm("mean-color-block", meanColorConverter{r}, font),
			}

			for _, p := range patterns {
				res := measureConverterArms(t, p.name+"-"+pal,
					p.src, p.cellsW, p.cellsH, arms)
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
			}

			photos, _ := filepath.Glob("images/*.png")
			for _, path := range photos {
				img, err := imageutil.LoadImage(path)
				if err != nil {
					continue
				}
				cellsW := 100
				aspect := float64(img.Width()) / float64(img.Height())
				cellsH := int(float64(cellsW) / aspect / 2.0)
				name := filepath.Base(path)
				measureConverterArms(t, name[:len(name)-len(".png")]+"-"+pal,
					photoSource(img), cellsW, cellsH, arms)
			}
		})
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
		return blurredLabError(renderBlocksToImage(blocks), ref, 2)
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
	// testdata/mandrill.tiff is committed; images/*.png is an optional
	// local corpus for broader measurement runs (kept out of the repo).
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
