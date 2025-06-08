package main

import (
	"image"
	"image/color"
	"os"
	"testing"
)

// TestAtkinsonDithering tests the Atkinson dithering algorithm
func TestAtkinsonDithering(t *testing.T) {
	// Load test image
	file, err := os.Open("mandrill_original.png")
	if err != nil {
		t.Skip("Test image not found")
	}
	defer file.Close()
	
	img, _, err := image.Decode(file)
	if err != nil {
		t.Fatalf("Failed to decode image: %v", err)
	}
	
	// Create smaller test image
	bounds := image.Rect(0, 0, 32, 32)
	testImg := image.NewRGBA(bounds)
	
	// Sample from original
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			testImg.Set(x, y, img.At(x*8, y*8))
		}
	}
	
	// Test dithering
	result := AtkinsonDither(testImg)
	
	// Verify result is binary (only black and white)
	blackCount := 0
	whiteCount := 0
	
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			c := result.At(x, y)
			gray, _, _, _ := color.GrayModel.Convert(c).RGBA()
			
			if gray == 0 {
				blackCount++
			} else if gray == 0xFFFF {
				whiteCount++
			} else {
				t.Errorf("Non-binary pixel at (%d, %d): %v", x, y, c)
			}
		}
	}
	
	t.Logf("Atkinson dithering result: %d black, %d white pixels", blackCount, whiteCount)
	
	// Basic sanity check
	if blackCount == 0 || whiteCount == 0 {
		t.Error("Dithering produced all black or all white image")
	}
}

// TestSegmentation tests 8x8 block segmentation
func TestSegmentation(t *testing.T) {
	// Create test image
	testImg := image.NewRGBA(image.Rect(0, 0, 80, 40))
	
	// Fill with pattern
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			if (x/8+y/8)%2 == 0 {
				testImg.Set(x, y, color.White)
			} else {
				testImg.Set(x, y, color.Black)
			}
		}
	}
	
	// Segment into blocks
	blocks := Segment8x8(testImg)
	
	// Verify dimensions
	expectedHeight := 40 / 8
	expectedWidth := 80 / 8
	
	if len(blocks) != expectedHeight {
		t.Errorf("Expected %d rows, got %d", expectedHeight, len(blocks))
	}
	
	if len(blocks) > 0 && len(blocks[0]) != expectedWidth {
		t.Errorf("Expected %d columns, got %d", expectedWidth, len(blocks[0]))
	}
	
	// Verify checkerboard pattern
	for y := 0; y < len(blocks); y++ {
		for x := 0; x < len(blocks[y]); x++ {
			block := blocks[y][x]
			expectedWhite := (x+y)%2 == 0
			
			// Check if block is all white or all black
			isWhite := true
			for i := uint(0); i < 64; i++ {
				if (block & (1 << i)) == 0 {
					isWhite = false
					break
				}
			}
			
			if isWhite != expectedWhite {
				t.Errorf("Block at (%d, %d) has wrong pattern", x, y)
			}
		}
	}
}

// TestMandrillAnalysisIntegration tests the full mandrill analysis pipeline
func TestMandrillAnalysisIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	// Load fonts
	font, err := loadFont("PxPlus_IBM_BIOS.ttf")
	if err != nil {
		t.Skip("Font not available")
	}
	
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	if safeFont == nil {
		safeFont = font
	}
	
	glyphs := analyzeFont(font, safeFont)
	lookup := NewGlyphLookup(glyphs)
	
	// Load test image
	file, err := os.Open("mandrill_original.png")
	if err != nil {
		t.Skip("Test image not found")
	}
	defer file.Close()
	
	img, _, err := image.Decode(file)
	if err != nil {
		t.Fatalf("Failed to decode image: %v", err)
	}
	
	// Resize to small test size
	bounds := img.Bounds()
	targetSize := image.NewRGBA(image.Rect(0, 0, 80, 40))
	
	xRatio := float64(bounds.Dx()) / 80.0
	yRatio := float64(bounds.Dy()) / 40.0
	
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			srcX := int(float64(x) * xRatio)
			srcY := int(float64(y) * yRatio)
			targetSize.Set(x, y, img.At(srcX, srcY))
		}
	}
	
	// Test threshold method
	t.Run("Threshold", func(t *testing.T) {
		threshold := 128
		binaryImg := image.NewGray(targetSize.Bounds())
		
		for y := 0; y < 40; y++ {
			for x := 0; x < 80; x++ {
				gray := color.GrayModel.Convert(targetSize.At(x, y)).(color.Gray)
				if gray.Y > uint8(threshold) {
					binaryImg.Set(x, y, color.White)
				} else {
					binaryImg.Set(x, y, color.Black)
				}
			}
		}
		
		blocks := Segment8x8(binaryImg)
		
		// Analyze block patterns
		patternCounts := make(map[GlyphBitmap]int)
		for _, row := range blocks {
			for _, block := range row {
				patternCounts[block]++
			}
		}
		
		t.Logf("Threshold method found %d unique patterns", len(patternCounts))
	})
	
	// Test Atkinson dithering method
	t.Run("Atkinson", func(t *testing.T) {
		dithered := AtkinsonDither(targetSize)
		blocks := Segment8x8(dithered)
		
		// Analyze with glyph matching
		charCounts := make(map[rune]int)
		for _, row := range blocks {
			for _, block := range row {
				match := lookup.FindClosestGlyph(block)
				charCounts[match.Rune]++
			}
		}
		
		t.Logf("Atkinson method matched %d unique characters", len(charCounts))
		
		// Show top characters
		type runeCount struct {
			r rune
			c int
		}
		var sorted []runeCount
		for r, c := range charCounts {
			sorted = append(sorted, runeCount{r, c})
		}
		
		// Simple sort
		for i := 0; i < len(sorted)-1; i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[j].c > sorted[i].c {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}
		
		t.Log("Top 5 characters:")
		for i := 0; i < 5 && i < len(sorted); i++ {
			t.Logf("  '%c': %d", sorted[i].r, sorted[i].c)
		}
	})
}

// BenchmarkAtkinsonDithering benchmarks the dithering algorithm
func BenchmarkAtkinsonDithering(b *testing.B) {
	// Create test image
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			gray := uint8((x + y) / 2)
			img.Set(x, y, color.RGBA{gray, gray, gray, 255})
		}
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = AtkinsonDither(img)
	}
}

// TestMandrillMatchingWithOutput is an enhanced version that saves outputs
func TestMandrillMatchingWithOutput(t *testing.T) {
	// Load fonts
	font, _ := loadFont("PxPlus_IBM_BIOS.ttf")
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	glyphs := analyzeFont(font, safeFont)
	lookup := NewGlyphLookup(glyphs)

	// Load the baboon image
	t.Log("Loading baboon test image...")

	file, err := os.Open("../../examples/baboon_256.png")
	if err != nil {
		t.Skipf("Could not load baboon image: %v", err)
		return
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		t.Fatalf("Could not decode image: %v", err)
	}

	// Apply different dithering methods
	t.Log("Applying dithering methods...")

	// 1. Simple threshold
	threshold := applyThreshold(img, 127)
	blocks1 := Segment8x8(threshold)

	// Save threshold output
	t.Log("Saving threshold output...")
	err = SaveVisualOutput(blocks1, lookup, OutputPath("baboon_threshold_output.txt"))
	if err != nil {
		t.Errorf("Error saving threshold output: %v", err)
	}

	// 2. Atkinson dithering
	atkinson := AtkinsonDither(img)
	blocks2 := Segment8x8(atkinson)

	// Save Atkinson output
	t.Log("Saving Atkinson output...")
	err = SaveVisualOutput(blocks2, lookup, OutputPath("baboon_atkinson_output.txt"))
	if err != nil {
		t.Errorf("Error saving Atkinson output: %v", err)
	}

	t.Log("Output files created in outputs/ directory")
}