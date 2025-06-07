package main

import (
	"fmt"
	"image"
	"image/color"
	"testing"
)

// TestBitConsistency verifies that analyzeGlyph, getBit, and String all use the same bit ordering
func TestBitConsistency(t *testing.T) {
	tests := []struct {
		name string
		setupImage func() *image.Alpha
		expectedBits []struct{ x, y int }
	}{
		{
			name: "SinglePixelTopLeft",
			setupImage: func() *image.Alpha {
				img := image.NewAlpha(image.Rect(0, 0, 8, 8))
				img.SetAlpha(0, 0, color.Alpha{255})
				return img
			},
			expectedBits: []struct{ x, y int }{{0, 0}},
		},
		{
			name: "SinglePixelBottomRight",
			setupImage: func() *image.Alpha {
				img := image.NewAlpha(image.Rect(0, 0, 8, 8))
				img.SetAlpha(7, 7, color.Alpha{255})
				return img
			},
			expectedBits: []struct{ x, y int }{{7, 7}},
		},
		{
			name: "DiagonalLine",
			setupImage: func() *image.Alpha {
				img := image.NewAlpha(image.Rect(0, 0, 8, 8))
				for i := 0; i < 8; i++ {
					img.SetAlpha(i, i, color.Alpha{255})
				}
				return img
			},
			expectedBits: []struct{ x, y int }{
				{0, 0}, {1, 1}, {2, 2}, {3, 3}, 
				{4, 4}, {5, 5}, {6, 6}, {7, 7},
			},
		},
		{
			name: "HorizontalLine",
			setupImage: func() *image.Alpha {
				img := image.NewAlpha(image.Rect(0, 0, 8, 8))
				for x := 0; x < 8; x++ {
					img.SetAlpha(x, 3, color.Alpha{255})
				}
				return img
			},
			expectedBits: []struct{ x, y int }{
				{0, 3}, {1, 3}, {2, 3}, {3, 3},
				{4, 3}, {5, 3}, {6, 3}, {7, 3},
			},
		},
		{
			name: "VerticalLine",
			setupImage: func() *image.Alpha {
				img := image.NewAlpha(image.Rect(0, 0, 8, 8))
				for y := 0; y < 8; y++ {
					img.SetAlpha(3, y, color.Alpha{255})
				}
				return img
			},
			expectedBits: []struct{ x, y int }{
				{3, 0}, {3, 1}, {3, 2}, {3, 3},
				{3, 4}, {3, 5}, {3, 6}, {3, 7},
			},
		},
		{
			name: "FourCorners",
			setupImage: func() *image.Alpha {
				img := image.NewAlpha(image.Rect(0, 0, 8, 8))
				img.SetAlpha(0, 0, color.Alpha{255}) // top-left
				img.SetAlpha(7, 0, color.Alpha{255}) // top-right
				img.SetAlpha(0, 7, color.Alpha{255}) // bottom-left
				img.SetAlpha(7, 7, color.Alpha{255}) // bottom-right
				return img
			},
			expectedBits: []struct{ x, y int }{
				{0, 0}, {7, 0}, {0, 7}, {7, 7},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create glyph info and analyze it
			glyph := &GlyphInfo{
				Img:  tt.setupImage(),
				Rune: 'T', // Test rune
			}
			glyph.analyzeGlyph()

			// Test 1: Verify getBit returns true for expected pixels
			for _, pos := range tt.expectedBits {
				if !getBit(glyph.Bitmap, pos.x, pos.y) {
					t.Errorf("getBit(%d,%d) = false, want true", pos.x, pos.y)
				}
			}

			// Test 2: Verify getBit returns false for all other pixels
			setPixels := 0
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					if getBit(glyph.Bitmap, x, y) {
						setPixels++
						// Check if this pixel was expected
						found := false
						for _, pos := range tt.expectedBits {
							if pos.x == x && pos.y == y {
								found = true
								break
							}
						}
						if !found {
							t.Errorf("getBit(%d,%d) = true, but pixel not in expected list", x, y)
						}
					}
				}
			}
			if setPixels != len(tt.expectedBits) {
				t.Errorf("Expected %d set pixels, got %d", len(tt.expectedBits), setPixels)
			}

			// Test 3: Verify String() output matches getBit
			strOutput := glyph.Bitmap.String()
			t.Logf("Bitmap visualization:\n%s", strOutput)
			
			// Create our own string representation using getBit to compare
			var expectedStr string
			expectedStr += " 01234567"
			for y := 0; y < 8; y++ {
				expectedStr += fmt.Sprintf("\n%d", y)
				for x := 0; x < 8; x++ {
					if getBit(glyph.Bitmap, x, y) {
						expectedStr += "█"
					} else {
						expectedStr += "·"
					}
				}
			}
			
			if strOutput != expectedStr {
				t.Errorf("String() output doesn't match getBit representation")
				t.Logf("Expected:\n%s", expectedStr)
				t.Logf("Got:\n%s", strOutput)
			}
		})
	}
}

// TestBitOperations tests individual bit operations
func TestBitOperations(t *testing.T) {
	tests := []struct {
		name     string
		x, y     int
		expected uint64
	}{
		{"TopLeft", 0, 0, 1 << 0},
		{"TopRight", 7, 0, 1 << 7},
		{"BottomLeft", 0, 7, 1 << 56},
		{"BottomRight", 7, 7, 1 << 63},
		{"Center", 4, 4, 1 << 36},
		{"Position_2_3", 2, 3, 1 << 26},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test setting bit manually
			var bitmap GlyphBitmap
			bitmap |= 1 << (tt.y*GlyphWidth + tt.x)
			
			// Verify it equals expected value
			if uint64(bitmap) != tt.expected {
				t.Errorf("Setting bit at (%d,%d): got %064b, want %064b", 
					tt.x, tt.y, uint64(bitmap), tt.expected)
			}
			
			// Verify getBit returns true only for this position
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					expected := (x == tt.x && y == tt.y)
					if getBit(bitmap, x, y) != expected {
						t.Errorf("getBit(%d,%d) = %v, want %v", 
							x, y, getBit(bitmap, x, y), expected)
					}
				}
			}
		})
	}
}

// TestStringRepresentation verifies the String() output format
func TestStringRepresentation(t *testing.T) {
	// Create a known pattern
	var bitmap GlyphBitmap
	
	// Set specific pixels to create a recognizable pattern
	// Letter "L" shape
	for y := 0; y < 7; y++ {
		bitmap |= 1 << (y*8 + 2) // vertical line at x=2
	}
	for x := 2; x < 6; x++ {
		bitmap |= 1 << (6*8 + x) // horizontal line at y=6
	}
	
	output := bitmap.String()
	t.Logf("L-shape pattern:\n%s", output)
	
	// Verify the pattern appears correctly
	expectedPixels := []struct{ x, y int }{
		{2, 0}, {2, 1}, {2, 2}, {2, 3}, {2, 4}, {2, 5}, {2, 6},
		{3, 6}, {4, 6}, {5, 6},
	}
	
	for _, pos := range expectedPixels {
		if !getBit(bitmap, pos.x, pos.y) {
			t.Errorf("Expected pixel at (%d,%d) to be set", pos.x, pos.y)
		}
	}
}