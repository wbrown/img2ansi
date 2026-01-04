package img2ansi

import (
	"fmt"
	"testing"

	"github.com/wbrown/img2ansi/imageutil"
)

func TestErrorDiffusionEffect(t *testing.T) {
	t.Parallel()

	// Create Renderer with LAB color method
	r := NewRenderer(
		WithColorMethod(LABMethod{}),
		WithPalette("colordata/ansi16.json"),
	)

	// Create a simple test image with brown/gray gradient
	width, height := 8, 8
	img := imageutil.NewRGBAImage(width, height)

	// Fill with gradient from brown to gray
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Interpolate from brown (170,85,0) to gray (170,170,170)
			factor := float64(x) / float64(width-1)
			r := uint8(170)
			g := uint8(85 + factor*85)
			b := uint8(0 + factor*170)

			img.SetRGB(x, y, imageutil.RGB{R: r, G: g, B: b})
		}
	}

	// Create dummy edges (no edges)
	edges := imageutil.NewGrayImage(width, height)

	// Process without error diffusion (comment out the diffusion in actual code)
	fmt.Println("Processing gradient image...")

	// Count colors in the result
	colorCounts := make(map[RGB]int)
	blocks := r.BrownDitherForBlocks(img, edges)

	for _, row := range blocks {
		for _, block := range row {
			colorCounts[block.FG]++
			colorCounts[block.BG]++
		}
	}

	fmt.Println("\nColors used in gradient (block-based):")
	for color, count := range colorCounts {
		code := "?"
		r.fgAnsi.Iterate(func(key, value interface{}) {
			if rgbFromUint32(key.(uint32)) == color {
				code = value.(string)
			}
		})
		fmt.Printf("  RGB(%d,%d,%d) [code %s]: used %d times\n",
			color.R, color.G, color.B, code, count)
	}

	// Also test a natural image patch
	fmt.Println("\nSimulating natural brown/gray image patch:")
	// Create a more varied test pattern
	img2 := imageutil.NewRGBAImage(width, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Add some noise to make it more natural
			base := 140 + (x*20)%60
			r := uint8(base + (y*10)%30)
			g := uint8(base - 20 + (x*5)%20)
			b := uint8(base/2 - 30 + (y*7)%25)

			img2.SetRGB(x, y, imageutil.RGB{R: r, G: g, B: b})
		}
	}

	colorCounts2 := make(map[RGB]int)
	blocks2 := r.BrownDitherForBlocks(img2, edges)

	for _, row := range blocks2 {
		for _, block := range row {
			colorCounts2[block.FG]++
			colorCounts2[block.BG]++
		}
	}

	fmt.Println("\nColors used in natural patch:")
	for color, count := range colorCounts2 {
		if count > 0 {
			code := "?"
			r.fgAnsi.Iterate(func(key, value interface{}) {
				if rgbFromUint32(key.(uint32)) == color {
					code = value.(string)
				}
			})
			fmt.Printf("  RGB(%d,%d,%d) [code %s]: used %d times\n",
				color.R, color.G, color.B, code, count)
		}
	}
}
