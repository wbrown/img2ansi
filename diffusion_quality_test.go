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
	height, width := len(blocks), len(blocks[0])
	out := imageutil.NewRGBAImage(width*2, height*2)
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
				out.SetRGB(bx*2+i%2, by*2+i/2,
					imageutil.RGB{R: c.R, G: c.G, B: c.B})
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
