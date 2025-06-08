package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// TestBlockCharacterRendering tests that Unicode block characters render correctly
func TestBlockCharacterRendering(t *testing.T) {
	// Create a small test image
	img := image.NewRGBA(image.Rect(0, 0, 80, 80))
	
	testCases := []struct {
		name string
		char rune
		fg   color.RGBA
		bg   color.RGBA
		x, y int
	}{
		{"space", ' ', color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 0, 255}, 0, 0},
		{"full block", '█', color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 0, 255}, 1, 0},
		{"upper half", '▀', color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}, 2, 0},
		{"lower half", '▄', color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}, 3, 0},
		{"left half", '▌', color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}, 4, 0},
		{"right half", '▐', color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}, 5, 0},
		{"light shade", '░', color.RGBA{255, 255, 255, 255}, color.RGBA{0, 0, 0, 255}, 6, 0},
		{"medium shade", '▒', color.RGBA{255, 255, 255, 255}, color.RGBA{0, 0, 0, 255}, 7, 0},
		{"dark shade", '▓', color.RGBA{255, 255, 255, 255}, color.RGBA{0, 0, 0, 255}, 8, 0},
	}
	
	// Create renderer once for all tests
	renderer := NewBlockRenderer()
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			renderer.SetColors(tc.fg, tc.bg)
			renderer.RenderCharacter(img, tc.char, tc.x*8, tc.y*8)
			
			// Verify at least one pixel was set
			hasColor := false
			startX := tc.x * 8
			startY := tc.y * 8
			
			for dy := 0; dy < 8; dy++ {
				for dx := 0; dx < 8; dx++ {
					c := img.At(startX+dx, startY+dy)
					if c != (color.RGBA{0, 0, 0, 0}) {
						hasColor = true
						break
					}
				}
			}
			
			if !hasColor && tc.char != ' ' {
				t.Errorf("Character '%c' rendered no pixels", tc.char)
			}
		})
	}
	
	// Save test output for visual inspection
	if testing.Verbose() {
		out, err := os.Create(OutputPath("test_blocks_render.png"))
		if err == nil {
			defer out.Close()
			saveImage(out, img)
			t.Log("Saved test output to test_blocks_render.png")
		}
	}
}

// TestANSIToPNGRendering tests the full ANSI to PNG pipeline
func TestANSIToPNGRendering(t *testing.T) {
	// Skip if no test file available
	testFile := OutputPath("test_simple.ans")
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test_simple.ans not found")
	}
	
	outputFile := OutputPath("test_render_output.png")
	err := RenderANSIToPNGUnified(testFile, outputFile)
	if err != nil {
		t.Fatalf("Failed to render ANSI to PNG: %v", err)
	}
	
	// Verify output exists
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("Output PNG file was not created")
	}
}

// TestGlyphRendering tests the unified glyph-based renderer
func TestGlyphRendering(t *testing.T) {
	// Load fonts
	font, err := loadFont("PxPlus_IBM_BIOS.ttf")
	if err != nil {
		t.Skipf("Font not available: %v", err)
	}
	
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	if safeFont == nil {
		safeFont = font
	}
	
	glyphs := analyzeFont(font, safeFont)
	lookup := NewGlyphLookup(glyphs)
	
	// Create renderer
	renderer := NewGlyphRenderer(lookup, 1)
	
	// Test image
	width := 16 * 8
	height := 4 * 8
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	
	// Test various characters
	testChars := []rune{' ', '█', '▀', '▄', '▌', '▐', '░', '▒', '▓', 'A', 'B', '1', '2', '#', '@', '■'}
	
	fg := color.RGBA{255, 255, 255, 255}
	bg := color.RGBA{0, 0, 0, 255}
	
	for i, char := range testChars {
		x := (i % 16) * 8
		y := (i / 16) * 8
		renderer.RenderCharacter(img, char, x, y, fg, bg)
	}
	
	// Verify rendering produced output
	hasPixels := false
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y && !hasPixels; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if img.At(x, y) != (color.RGBA{0, 0, 0, 0}) {
				hasPixels = true
				break
			}
		}
	}
	
	if !hasPixels {
		t.Error("Glyph renderer produced no output")
	}
	
	// Save for visual inspection
	if testing.Verbose() {
		out, err := os.Create(OutputPath("test_glyph_render.png"))
		if err == nil {
			defer out.Close()
			saveImage(out, img)
			t.Log("Saved glyph test output")
		}
	}
}

// TestANSIColorParsing tests ANSI escape sequence parsing
func TestANSIColorParsing(t *testing.T) {
	testCases := []struct {
		name     string
		sequence string
		wantFg   int
		wantBg   int
	}{
		{"foreground 16", "\x1b[38;5;1m", 1, -1},
		{"background 16", "\x1b[48;5;2m", -1, 2},
		{"both colors", "\x1b[38;5;15;48;5;0m", 15, 0},
		{"reset", "\x1b[0m", -1, -1},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This would test the ANSI parsing logic
			// Implementation depends on how parsing is structured
		})
	}
}

// Benchmark rendering performance
func BenchmarkBlockRendering(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 640, 320))
	fg := color.RGBA{255, 255, 255, 255}
	bg := color.RGBA{0, 0, 0, 255}
	renderer := NewBlockRenderer()
	renderer.SetColors(fg, bg)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderer.RenderCharacter(img, '▒', (i%80)*8, (i/80%40)*8)
	}
}

// Helper to save images in tests
func saveImage(out *os.File, img image.Image) error {
	return png.Encode(out, img)
}

// TestRenderingComparison tests rendering comparison (simplified - only uses unified method)
func TestRenderingComparison(t *testing.T) {
	testFile := OutputPath("mandrill_brown_8x8.ans")

	if _, err := os.Stat(testFile); err != nil {
		t.Skipf("Test file %s not found", testFile)
		return
	}

	// Only use unified method now
	t.Log("Rendering with unified method...")
	outputFile := OutputPath("test_unified_method.png")
	if err := RenderANSIToPNG(testFile, outputFile); err != nil {
		t.Errorf("Unified method error: %v", err)
	}

	t.Logf("Rendered to %s", outputFile)
}