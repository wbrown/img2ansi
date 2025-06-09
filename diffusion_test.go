package img2ansi

import (
	"fmt"
	"gocv.io/x/gocv"
	"testing"
)

func TestErrorDiffusionEffect(t *testing.T) {
	// Load palette
	CurrentColorDistanceMethod = MethodLAB
	_, _, err := LoadPalette("colordata/ansi16.json")
	if err != nil {
		t.Fatalf("Failed to load palette: %v", err)
	}

	// Create a simple test image with brown/gray gradient
	width, height := 8, 8
	img := gocv.NewMatWithSize(height, width, gocv.MatTypeCV8UC3)
	defer img.Close()

	// Fill with gradient from brown to gray
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Interpolate from brown (170,85,0) to gray (170,170,170)
			factor := float64(x) / float64(width-1)
			r := uint8(170)
			g := uint8(85 + factor*85)
			b := uint8(0 + factor*170)
			
			img.SetUCharAt(y, x*3, b)   // OpenCV uses BGR
			img.SetUCharAt(y, x*3+1, g)
			img.SetUCharAt(y, x*3+2, r)
		}
	}

	// Create dummy edges (no edges)
	edges := gocv.NewMatWithSize(height, width, gocv.MatTypeCV8U)
	defer edges.Close()

	// Process without error diffusion (comment out the diffusion in actual code)
	fmt.Println("Processing gradient image...")
	
	// Count colors in the result
	colorCounts := make(map[RGB]int)
	blocks := BrownDitherForBlocks(img, edges)
	
	for _, row := range blocks {
		for _, block := range row {
			colorCounts[block.FG]++
			colorCounts[block.BG]++
		}
	}

	fmt.Println("\nColors used in gradient (block-based):")
	for color, count := range colorCounts {
		code := "?"
		fgAnsi.Iterate(func(key, value interface{}) {
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
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Add some noise to make it more natural
			base := 140 + (x*20)%60
			r := uint8(base + (y*10)%30)
			g := uint8(base - 20 + (x*5)%20)
			b := uint8(base/2 - 30 + (y*7)%25)
			
			img.SetUCharAt(y, x*3, b)
			img.SetUCharAt(y, x*3+1, g)
			img.SetUCharAt(y, x*3+2, r)
		}
	}

	colorCounts2 := make(map[RGB]int)
	blocks2 := BrownDitherForBlocks(img, edges)
	
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
			fgAnsi.Iterate(func(key, value interface{}) {
				if rgbFromUint32(key.(uint32)) == color {
					code = value.(string)
				}
			})
			fmt.Printf("  RGB(%d,%d,%d) [code %s]: used %d times\n",
				color.R, color.G, color.B, code, count)
		}
	}
}