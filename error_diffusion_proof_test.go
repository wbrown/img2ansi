package img2ansi

import (
	"gocv.io/x/gocv"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// TestErrorDiffusionVisualProof creates visual proof of whether error diffusion is working
func TestErrorDiffusionVisualProof(t *testing.T) {
	// Load palette
	CurrentColorDistanceMethod = MethodLAB
	_, _, err := LoadPalette("colordata/ansi16.json")
	if err != nil {
		t.Fatalf("Failed to load palette: %v", err)
	}

	// Create a gradient test image
	width, height := 32, 16
	img := gocv.NewMatWithSize(height, width, gocv.MatTypeCV8UC3)
	defer img.Close()

	// Create a smooth gradient from dark gray to light gray
	// This should show banding without dithering, smooth transition with dithering
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Gradient from 64 to 192
			gray := uint8(64 + (x * 128 / width))
			img.SetUCharAt(y, x*3, gray)   // B
			img.SetUCharAt(y, x*3+1, gray) // G
			img.SetUCharAt(y, x*3+2, gray) // R
		}
	}

	// Save the original gradient
	saveMatAsImage(img, "test_gradient_original.png", t)

	// Process with error diffusion (normal operation)
	edges := gocv.NewMatWithSize(height, width, gocv.MatTypeCV8U)
	defer edges.Close()

	imgCopy1 := img.Clone()
	defer imgCopy1.Close()
	blocks1 := BrownDitherForBlocks(imgCopy1, edges)

	// Save result with diffusion
	err = SaveBlocksToPNGWithOptions(blocks1, "test_gradient_with_diffusion.png", RenderOptions{
		UseFont: false,
		Scale:   8,
	})
	if err != nil {
		t.Fatalf("Failed to save with diffusion: %v", err)
	}

	// Now we need to test WITHOUT error diffusion
	// We'll create a modified version that skips the distributeError call
	imgCopy2 := img.Clone()
	defer imgCopy2.Close()
	blocks2 := brownDitherNoDiffusion(imgCopy2, edges)

	// Save result without diffusion
	err = SaveBlocksToPNGWithOptions(blocks2, "test_gradient_no_diffusion.png", RenderOptions{
		UseFont: false,
		Scale:   8,
	})
	if err != nil {
		t.Fatalf("Failed to save without diffusion: %v", err)
	}

	// Count unique color transitions
	transitions1 := countColorTransitions(blocks1)
	transitions2 := countColorTransitions(blocks2)

	t.Logf("With error diffusion: %d color transitions", transitions1)
	t.Logf("Without error diffusion: %d color transitions", transitions2)

	// With proper error diffusion, we should see more transitions (smoother gradient)
	if transitions1 <= transitions2 {
		t.Log("WARNING: Error diffusion may not be working correctly")
		t.Log("Expected more color transitions with diffusion")
	}

	// Test with a more challenging image - flesh tones
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Gradient from brown to pink
			factor := float64(x) / float64(width-1)
			r := uint8(170 + factor*50)
			g := uint8(100 + factor*80)
			b := uint8(60 + factor*90)
			img.SetUCharAt(y, x*3, b)
			img.SetUCharAt(y, x*3+1, g)
			img.SetUCharAt(y, x*3+2, r)
		}
	}

	imgCopy3 := img.Clone()
	defer imgCopy3.Close()
	blocks3 := BrownDitherForBlocks(imgCopy3, edges)

	err = SaveBlocksToPNGWithOptions(blocks3, "test_fleshtone_with_diffusion.png", RenderOptions{
		UseFont: false,
		Scale:   8,
	})
	if err != nil {
		t.Fatalf("Failed to save fleshtone with diffusion: %v", err)
	}

	imgCopy4 := img.Clone()
	defer imgCopy4.Close()
	blocks4 := brownDitherNoDiffusion(imgCopy4, edges)

	err = SaveBlocksToPNGWithOptions(blocks4, "test_fleshtone_no_diffusion.png", RenderOptions{
		UseFont: false,
		Scale:   8,
	})
	if err != nil {
		t.Fatalf("Failed to save fleshtone without diffusion: %v", err)
	}

	t.Log("Created test images:")
	t.Log("  test_gradient_original.png - Original gradient")
	t.Log("  test_gradient_with_diffusion.png - With error diffusion")
	t.Log("  test_gradient_no_diffusion.png - Without error diffusion")
	t.Log("  test_fleshtone_with_diffusion.png - Flesh tones with diffusion")
	t.Log("  test_fleshtone_no_diffusion.png - Flesh tones without diffusion")
	t.Log("Compare these images to see if error diffusion is working")
}

// brownDitherNoDiffusion is a copy of BrownDitherForBlocks without error diffusion
func brownDitherNoDiffusion(img gocv.Mat, edges gocv.Mat) [][]BlockRune {
	height, width := img.Rows(), img.Cols()
	blockHeight, blockWidth := height/2, width/2
	result := make([][]BlockRune, blockHeight)

	for by := range result {
		result[by] = make([]BlockRune, blockWidth)
		for bx := range result[by] {
			var block [4]RGB
			for i := 0; i < 4; i++ {
				y, x := by*2+i/2, bx*2+i%2
				if y < height && x < width {
					block[i] = rgbFromVecb(img.GetVecbAt(y, x))
				}
			}

			isEdge := false
			for i := 0; i < 4; i++ {
				y, x := by*2+i/2, bx*2+i%2
				if y < height && x < width && edges.GetUCharAt(y, x) > 0 {
					isEdge = true
					break
				}
			}

			bestRune, fgColor, bgColor := findBestBlockRepresentation(block, isEdge)

			result[by][bx] = BlockRune{
				Rune: bestRune,
				FG:   fgColor,
				BG:   bgColor,
			}

			// SKIP ERROR DIFFUSION - this is the key difference
			// In the real algorithm, we would distribute error here
		}
	}

	return result
}

// countColorTransitions counts how many times colors change in the result
func countColorTransitions(blocks [][]BlockRune) int {
	transitions := 0
	for y := 0; y < len(blocks); y++ {
		for x := 1; x < len(blocks[y]); x++ {
			prev := blocks[y][x-1]
			curr := blocks[y][x]
			if prev.FG != curr.FG || prev.BG != curr.BG || prev.Rune != curr.Rune {
				transitions++
			}
		}
	}
	return transitions
}

// saveMatAsImage saves a gocv.Mat as a PNG image
func saveMatAsImage(mat gocv.Mat, filename string, t *testing.T) {
	height, width := mat.Rows(), mat.Cols()
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			b := mat.GetUCharAt(y, x*3)
			g := mat.GetUCharAt(y, x*3+1)
			r := mat.GetUCharAt(y, x*3+2)
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	f, err := os.Create(filename)
	if err != nil {
		t.Fatalf("Failed to create %s: %v", filename, err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		t.Fatalf("Failed to encode %s: %v", filename, err)
	}
}

// TestErrorDiffusionColorAccumulation tests if errors are actually accumulating
func TestErrorDiffusionColorAccumulation(t *testing.T) {
	// This test will track the error values being distributed
	// Create a simple 4x4 image with a color that doesn't match any palette color perfectly
	CurrentColorDistanceMethod = MethodLAB
	_, _, err := LoadPalette("colordata/ansi16.json")
	if err != nil {
		t.Fatalf("Failed to load palette: %v", err)
	}

	// Use a color that's between palette colors
	// RGB(128, 64, 32) should be between brown (170,85,0) and dark gray (85,85,85)
	testColor := RGB{128, 64, 32}

	// Find nearest palette color using the pre-computed closest color array
	// This is how the actual algorithm finds colors
	index := int(testColor.R)*256*256 + int(testColor.G)*256 + int(testColor.B)
	nearestColor := (*fgClosestColor)[index]
	colorError := testColor.subtractToError(nearestColor)

	t.Logf("Test color: RGB(%d,%d,%d)", testColor.R, testColor.G, testColor.B)
	t.Logf("Nearest palette color: RGB(%d,%d,%d)", nearestColor.R, nearestColor.G, nearestColor.B)
	t.Logf("Error: R=%d, G=%d, B=%d", colorError.R, colorError.G, colorError.B)

	// The error should be non-zero
	if colorError.R == 0 && colorError.G == 0 && colorError.B == 0 {
		t.Skip("Test color matches palette exactly, can't test error diffusion")
	}

	// Create a 4x4 test image
	img := gocv.NewMatWithSize(4, 4, gocv.MatTypeCV8UC3)
	defer img.Close()

	// Fill with test color
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetUCharAt(y, x*3, testColor.B)
			img.SetUCharAt(y, x*3+1, testColor.G)
			img.SetUCharAt(y, x*3+2, testColor.R)
		}
	}

	// Track colors before and after processing
	colorsBefore := make([][]RGB, 4)
	for y := 0; y < 4; y++ {
		colorsBefore[y] = make([]RGB, 4)
		for x := 0; x < 4; x++ {
			colorsBefore[y][x] = rgbFromVecb(img.GetVecbAt(y, x))
		}
	}

	// Process the image
	edges := gocv.NewMatWithSize(4, 4, gocv.MatTypeCV8U)
	defer edges.Close()

	_ = BrownDitherForBlocks(img, edges)

	// Check if any pixels changed due to error diffusion
	pixelsChanged := 0
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			colorAfter := rgbFromVecb(img.GetVecbAt(y, x))
			if colorAfter != colorsBefore[y][x] {
				pixelsChanged++
				t.Logf("Pixel (%d,%d) changed from RGB(%d,%d,%d) to RGB(%d,%d,%d)",
					x, y,
					colorsBefore[y][x].R, colorsBefore[y][x].G, colorsBefore[y][x].B,
					colorAfter.R, colorAfter.G, colorAfter.B)
			}
		}
	}

	t.Logf("Pixels changed by error diffusion: %d out of 16", pixelsChanged)

	if pixelsChanged == 0 {
		t.Error("No pixels were changed by error diffusion - diffusion may not be working")
	}
}
