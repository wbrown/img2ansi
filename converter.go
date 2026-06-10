package img2ansi

import (
	"github.com/wbrown/img2ansi/imageutil"
)

// BlockConverter is the contract for anything that turns a prepared
// image into terminal cell output: the quadrant dither today, glyph
// matchers from the research pipeline tomorrow. The quality harness
// (diffusion_quality_test.go) scores any set of BlockConverters against
// each other on a common cell grid, so a new matcher becomes measurable
// by implementing this interface.
//
// Implementations declare how many source pixels per cell edge they
// consume so a harness can prepare each converter's input at its native
// resolution for the same cell grid: the quadrant dither reads 2x2
// source pixels per cell, an 8x8 glyph matcher reads 8x8.
type BlockConverter interface {
	// Convert turns a prepared image and its edge map into block
	// output. Implementations may mutate img (the quadrant dither
	// diffuses error into it); callers who need the image afterwards
	// should pass a clone.
	Convert(img *imageutil.RGBAImage, edges *imageutil.GrayImage) [][]BlockRune

	// SourcePixelsPerCell reports how many source pixels per cell edge
	// this converter consumes.
	SourcePixelsPerCell() int
}

// The Renderer's quadrant dither is the reference BlockConverter.
var _ BlockConverter = (*Renderer)(nil)

// Convert implements BlockConverter using the Brown quadrant dither.
func (r *Renderer) Convert(img *imageutil.RGBAImage, edges *imageutil.GrayImage) [][]BlockRune {
	return r.BrownDitherForBlocks(img, edges)
}

// SourcePixelsPerCell implements BlockConverter: each character cell
// covers a 2x2 block of source pixels.
func (r *Renderer) SourcePixelsPerCell() int {
	return 2
}
