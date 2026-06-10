package img2ansi

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"path/filepath"

	"github.com/wbrown/img2ansi/imageutil"
)

// This file is the converter-quality measurement harness, shared by the
// test suite (diffusion_quality_test.go) and the standalone runner
// (cmd/quality). Experiments that exceed the test binary's time budget —
// LAB-matched dither cells, method sweeps, comparison composites — run
// through cmd/quality against this same code, so every number in
// docs/glyph-research/README.md is reproducible from the repository
// either way, with no out-of-tree copies of the scoring pipeline.
//
// The primary metric is low-pass perceptual error: both images are
// blurred with a Gaussian approximating viewing distance, then compared
// as mean delta-E in LAB space. Error diffusion exists to make local
// average tone match the source, and the blur exposes exactly that;
// unblurred MSE penalizes the dither pattern itself and rewards banding.
//
// Display geometry: the harness renders for the canonical 80-column
// text screen. On a 4:3 CRT the 80-column modes displayed cells at
// ~1:2.4 (CGA 640x200: 8x8 glyphs at 1:2.4 pixel aspect; VGA 720x400:
// 9x16 cells at 0.74 width units — both land on 1:2.4). The chain
// modeled here: every cell renders as its 8x8 font glyph, scan-doubled
// to 8x16 (VGA 400-line behavior), then the CRT's 4:3 geometry adds
// the remaining x1.2 vertical stretch, for 1:2.4 displayed cells.
// Photos size width-first to the full 80 columns (FitGrid); synthetic
// patterns use the 80x25 screen. Rendered output and reference share
// the source image's true aspect — nothing is squashed.

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

// BlurredLabError blurs both images with the given sigma (in pixels) and
// returns the mean delta-E (CIE76, via LAB) between them. This is the
// primary quality metric: it measures local average tone accuracy.
func BlurredLabError(a, b *imageutil.RGBAImage, sigma float64) float64 {
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

// --- Cross-converter harness -------------------------------------------
//
// Any BlockConverter can be scored against any other on the same cell
// grid: each arm's input is prepared at its native source resolution
// (SourcePixelsPerCell), its output rendered back to pixels at a common
// scoring resolution, and all arms compared against the same reference.

// scorePxPerCell is the rendered cell width: every arm's output is
// rendered as 8 px wide font glyphs before the display chain.
const scorePxPerCell = GlyphWidth

const (
	targetCols        = 80
	targetRows        = 25
	displayCellAspect = 2.4
	// crtStretch is the vertical stretch remaining after scan-doubling:
	// cell aspect 2.4 over the 2x of scan-doubled rows.
	crtStretch = displayCellAspect / 2
)

// FitGrid sizes a source image's cell grid width-first, like the CLI:
// every image gets the full 80 columns, and rows follow the source
// aspect under the 1:2.4 display cell aspect, uncapped — portrait
// content renders taller than one 25-row screen and scrolls, exactly
// as a terminal would show it. (The 80x25 screen remains the target
// for synthetic patterns, which have no native aspect of their own.)
func FitGrid(aspect float64) (cols, rows int) {
	cols = targetCols
	rows = int(math.Round(float64(cols) / (aspect * displayCellAspect)))
	if rows < 1 {
		rows = 1
	}
	return cols, rows
}

// screenDims returns the displayed pixel dimensions of a cell grid
// after the full display chain (8 px wide cells, scan-doubled rows,
// CRT stretch). Height is forced even so the photo prepare pipeline
// (which works in half-dimensions) reproduces it exactly.
func screenDims(cols, rows int) (w, h int) {
	h = int(math.Round(float64(rows*GlyphHeight*2)*crtStretch/2)) * 2
	return cols * GlyphWidth, h
}

// scanDouble duplicates every row: byte-for-byte what 400-line text
// modes did to the 8x8 font.
func scanDouble(img *imageutil.RGBAImage) *imageutil.RGBAImage {
	w, h := img.Width(), img.Height()
	out := imageutil.NewRGBAImage(w, h*2)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := img.GetRGB(x, y)
			out.SetRGB(x, y*2, p)
			out.SetRGB(x, y*2+1, p)
		}
	}
	return out
}

// stretchVerticalTo resamples vertically to the exact target height
// with linear interpolation — the CRT beam is the interpolator.
func stretchVerticalTo(img *imageutil.RGBAImage, outH int) *imageutil.RGBAImage {
	w, h := img.Width(), img.Height()
	out := imageutil.NewRGBAImage(w, outH)
	for y := 0; y < outH; y++ {
		srcY := (float64(y) + 0.5) * float64(h) / float64(outH)
		y0 := int(srcY - 0.5)
		frac := (srcY - 0.5) - float64(y0)
		y1 := y0 + 1
		if y0 < 0 {
			y0, y1, frac = 0, 0, 0
		}
		if y1 >= h {
			y1 = h - 1
		}
		for x := 0; x < w; x++ {
			a := img.GetRGB(x, y0)
			b := img.GetRGB(x, y1)
			mix := func(p, q uint8) uint8 {
				return uint8(math.Round(float64(p)*(1-frac) + float64(q)*frac))
			}
			out.SetRGB(x, y, imageutil.RGB{
				R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B)})
		}
	}
	return out
}

// displayView runs a square-cell glyph render through the display
// chain: scan-double, then CRT geometry to the exact screen height.
func displayView(rendered *imageutil.RGBAImage, screenH int) *imageutil.RGBAImage {
	return stretchVerticalTo(scanDouble(rendered), screenH)
}

// ConverterArm pairs a BlockConverter with a renderer that turns its
// output into the square-cell glyph image the display chain consumes.
type ConverterArm struct {
	name   string
	conv   BlockConverter
	render func([][]BlockRune) *imageutil.RGBAImage
}

// FontArm renders a converter's output through font glyph bitmaps —
// including the quadrant dither's output: its runes are font glyphs
// like any others, 8x8 bitmaps that the display scan-doubles, never
// idealized rectangles.
func FontArm(name string, conv BlockConverter, font *FontBitmaps) ConverterArm {
	return ConverterArm{
		name: name,
		conv: conv,
		render: func(blocks [][]BlockRune) *imageutil.RGBAImage {
			return imageutil.RGBAImageFromImage(
				font.RenderBlocks(blocks, scorePxPerCell/GlyphWidth))
		},
	}
}

// ImageSource produces the source content at a requested pixel size,
// with its edge map. Synthetic patterns regenerate analytically so each
// arm sees the same content at its native resolution; photos go through
// the standard prepare pipeline.
type ImageSource func(pxW, pxH int) (*imageutil.RGBAImage, *imageutil.GrayImage)

func SyntheticSource(gen func(w, h int) *imageutil.RGBAImage) ImageSource {
	return func(pxW, pxH int) (*imageutil.RGBAImage, *imageutil.GrayImage) {
		return gen(pxW, pxH), imageutil.NewGrayImage(pxW, pxH)
	}
}

func PhotoSource(img *imageutil.RGBAImage) ImageSource {
	return func(pxW, pxH int) (*imageutil.RGBAImage, *imageutil.GrayImage) {
		return imageutil.PrepareForANSI(img, pxW/2, pxH/2)
	}
}

// ArmScore is one arm's result from MeasureConverterArms: the blurred
// delta-E at half-cell and one-cell sigma (one-cell is the headline
// number in the research log) and the displayed render, so callers can
// re-score it under other display models or compose comparisons.
type ArmScore struct {
	Name     string
	HalfCell float64
	OneCell  float64
	Rendered *imageutil.RGBAImage
}

// MeasureConverterArms runs each converter over the same cell grid and
// scores every arm against the same reference at scorePxPerCell. Blur
// sigma is expressed in cell widths so the metric is comparable across
// converters regardless of their source resolution. logf (nil for
// silent) receives one row per arm; with pngDir non-empty, a labeled
// side-by-side comparison image (reference plus every arm) is written
// alongside the per-arm renders.
func MeasureConverterArms(
	name string,
	src ImageSource,
	cellsW, cellsH int,
	arms []ConverterArm,
	logf func(format string, args ...any),
	pngDir string,
) ([]ArmScore, *imageutil.RGBAImage, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	screenW, screenH := screenDims(cellsW, cellsH)
	reference, _ := src(screenW, screenH)

	labels := []string{"reference"}
	panels := []*imageutil.RGBAImage{reference}

	scores := make([]ArmScore, 0, len(arms))
	for _, arm := range arms {
		k := arm.conv.SourcePixelsPerCell()
		input, edges := src(cellsW*k, cellsH*k)
		blocks := arm.conv.Convert(input, edges)
		rendered := displayView(arm.render(blocks), screenH)

		halfCell := BlurredLabError(rendered, reference, 0.5*scorePxPerCell)
		oneCell := BlurredLabError(rendered, reference, 1.0*scorePxPerCell)
		scores = append(scores, ArmScore{
			Name:     arm.name,
			HalfCell: halfCell,
			OneCell:  oneCell,
			Rendered: rendered,
		})
		logf("%-14s %-18s blurredΔE σ=0.5cell %6.2f  σ=1cell %6.2f",
			name, arm.name, halfCell, oneCell)

		labels = append(labels, fmt.Sprintf("%s dE=%.2f", arm.name, oneCell))
		panels = append(panels, rendered)

		if pngDir != "" {
			path := filepath.Join(pngDir, fmt.Sprintf("%s_%s.png", name, arm.name))
			if err := imageutil.SavePNG(rendered.RGBA, path); err != nil {
				logf("could not save %s: %v", path, err)
			}
		}
	}

	if pngDir != "" {
		font, err := LoadEmbeddedFont("font8x8")
		if err != nil {
			return nil, nil, fmt.Errorf("loading label font: %w", err)
		}
		path := filepath.Join(pngDir, fmt.Sprintf("%s_compare.png", name))
		if err := imageutil.SavePNG(ComposeComparison(font, labels, panels), path); err != nil {
			logf("could not save %s: %v", path, err)
		}
	}
	return scores, reference, nil
}

// ComposeComparison stacks labeled panels vertically into a single
// comparison image. Labels are rendered through the font's own glyphs.
func ComposeComparison(font *FontBitmaps, labels []string, panels []*imageutil.RGBAImage) *image.RGBA {
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

// CRTDisplay approximates a CRT's phosphor response: the rendered image
// is convolved with a small Gaussian beam-spot PSF in linear light
// (glow is additive in luminance, not in sRGB code values; the kernel
// is normalized, so total light is preserved). sigma is in rendered
// pixels — one font pixel at scorePxPerCell. This is an asymmetric
// display model: it applies to the RENDERED side only, unlike the
// symmetric viewing-distance blur inside BlurredLabError. The 8x8
// fonts were designed against exactly this low-pass — letterform
// spacing columns read as "slightly dimmer" under a beam spot, not as
// the hard black gutters our rectangle-pixel renderer draws.
func CRTDisplay(img *imageutil.RGBAImage, sigma float64) *imageutil.RGBAImage {
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
	toLinear := func(v uint8) float64 {
		return math.Pow(float64(v)/255, 2.2)
	}
	toSRGB := func(v float64) uint8 {
		return uint8(math.Max(0, math.Min(255,
			math.Round(255*math.Pow(v, 1/2.2)))))
	}

	// Horizontal pass over linearized values
	horiz := make([][][3]float64, height)
	for y := 0; y < height; y++ {
		horiz[y] = make([][3]float64, width)
		for x := 0; x < width; x++ {
			var acc [3]float64
			for k, w := range kernel {
				p := img.GetRGB(clamp(x+k-radius, width), y)
				acc[0] += toLinear(p.R) * w
				acc[1] += toLinear(p.G) * w
				acc[2] += toLinear(p.B) * w
			}
			horiz[y][x] = acc
		}
	}

	// Vertical pass, then back to sRGB
	out := imageutil.NewRGBAImage(width, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var acc [3]float64
			for k, w := range kernel {
				p := horiz[clamp(y+k-radius, height)][x]
				acc[0] += p[0] * w
				acc[1] += p[1] * w
				acc[2] += p[2] * w
			}
			out.SetRGB(x, y, imageutil.RGB{
				R: toSRGB(acc[0]), G: toSRGB(acc[1]), B: toSRGB(acc[2]),
			})
		}
	}
	return out
}
