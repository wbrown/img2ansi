package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	
	"github.com/wbrown/img2ansi"
)

// TestBlockCharacterRendering tests that Unicode block characters render correctly
func TestBlockCharacterRendering(t *testing.T) {
	// Create a small test image
	img := image.NewRGBA(image.Rect(0, 0, 80, 80))
	
	testCases := []struct {
		name string
		char rune
		fg   img2ansi.RGB
		bg   img2ansi.RGB
		x, y int
	}{
		{"space", ' ', img2ansi.RGB{255, 0, 0}, img2ansi.RGB{0, 0, 0}, 0, 0},
		{"full block", '█', img2ansi.RGB{255, 0, 0}, img2ansi.RGB{0, 0, 0}, 1, 0},
		{"upper half", '▀', img2ansi.RGB{255, 0, 0}, img2ansi.RGB{0, 255, 0}, 2, 0},
		{"lower half", '▄', img2ansi.RGB{255, 0, 0}, img2ansi.RGB{0, 255, 0}, 3, 0},
		{"left half", '▌', img2ansi.RGB{255, 0, 0}, img2ansi.RGB{0, 0, 255}, 4, 0},
		{"right half", '▐', img2ansi.RGB{255, 0, 0}, img2ansi.RGB{0, 0, 255}, 5, 0},
		{"light shade", '░', img2ansi.RGB{255, 255, 255}, img2ansi.RGB{0, 0, 0}, 6, 0},
		{"medium shade", '▒', img2ansi.RGB{255, 255, 255}, img2ansi.RGB{0, 0, 0}, 7, 0},
		{"dark shade", '▓', img2ansi.RGB{255, 255, 255}, img2ansi.RGB{0, 0, 0}, 8, 0},
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
	
	fg := img2ansi.RGB{255, 255, 255}
	bg := img2ansi.RGB{0, 0, 0}
	
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
	fg := img2ansi.RGB{255, 255, 255}
	bg := img2ansi.RGB{0, 0, 0}
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

// TestPaletteMapping tests that our palette loading matches expected color values
func TestPaletteMapping(t *testing.T) {
	// Load 256-color palette
	fgTable256, _, err := img2ansi.LoadPalette("ansi256")
	if err != nil {
		t.Fatalf("Failed to load ansi256 palette: %v", err)
	}
	
	t.Logf("AnsiData length: %d", len(fgTable256.AnsiData))
	
	// Test specific known colors
	testCases := []struct {
		ansiCode int
		expected img2ansi.RGB
		name     string
	}{
		{0, img2ansi.RGB{0, 0, 0}, "Black"},
		{15, img2ansi.RGB{255, 255, 255}, "White"},
		{196, img2ansi.RGB{255, 0, 0}, "Bright Red"},
		{21, img2ansi.RGB{0, 0, 255}, "Blue"},
		{46, img2ansi.RGB{0, 255, 0}, "Green"},
		{226, img2ansi.RGB{255, 255, 0}, "Yellow"},
	}
	
	// Check if palette is properly sorted by ANSI code
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("ANSI_%d_%s", tc.ansiCode, tc.name), func(t *testing.T) {
			if tc.ansiCode >= len(fgTable256.AnsiData) {
				t.Skipf("ANSI code %d out of range (palette has %d entries)", 
					tc.ansiCode, len(fgTable256.AnsiData))
			}
			
			entry := fgTable256.AnsiData[tc.ansiCode]
			rgb := img2ansi.RGB{
				R: uint8((entry.Key >> 16) & 0xFF),
				G: uint8((entry.Key >> 8) & 0xFF),
				B: uint8(entry.Key & 0xFF),
			}
			
			// Check if the Value field contains the expected ANSI code
			var parsedCode int
			if _, err := fmt.Sscanf(entry.Value, "38;5;%d", &parsedCode); err == nil {
				if parsedCode != tc.ansiCode {
					t.Errorf("Entry at index %d has Value %s, expected code %d",
						tc.ansiCode, entry.Value, tc.ansiCode)
				}
			}
			
			// Check color
			if rgb != tc.expected {
				t.Errorf("ANSI code %d: got RGB(%d,%d,%d), expected RGB(%d,%d,%d)",
					tc.ansiCode, rgb.R, rgb.G, rgb.B, 
					tc.expected.R, tc.expected.G, tc.expected.B)
			}
		})
	}
	
	// Test our ansi256ToRGBForParsing function
	loadPaletteOnce()
	
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Parse_ANSI_%d", tc.ansiCode), func(t *testing.T) {
			rgb := ansi256ToRGBForParsing(tc.ansiCode)
			if rgb != tc.expected {
				t.Errorf("ansi256ToRGBForParsing(%d): got RGB(%d,%d,%d), expected RGB(%d,%d,%d)",
					tc.ansiCode, rgb.R, rgb.G, rgb.B,
					tc.expected.R, tc.expected.G, tc.expected.B)
			}
		})
	}
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