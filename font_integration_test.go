package img2ansi

import (
	"os"
	"testing"
)

// TestLoadRealFont tests loading the actual IBM BIOS font
func TestLoadRealFont(t *testing.T) {
	// Try to load the IBM BIOS font
	fonts, err := LoadFontBitmaps("fonts/PxPlus_IBM_BIOS.ttf", "")
	if err != nil {
		t.Fatalf("Failed to load IBM BIOS font: %v", err)
	}
	
	// Check that we loaded some glyphs
	if len(fonts.glyphs) == 0 {
		t.Error("No glyphs were loaded from font")
	}
	
	// Check for specific characters
	testChars := []rune{'A', '█', '▀', '▄', ' '}
	for _, char := range testChars {
		if _, exists := fonts.glyphs[char]; !exists {
			t.Errorf("Expected character '%c' not found in font", char)
		}
	}
	
	// Test rendering with real font
	blocks := [][]BlockRune{
		{
			{Rune: 'H', FG: RGB{255, 255, 255}, BG: RGB{0, 0, 0}},
			{Rune: 'i', FG: RGB{255, 255, 255}, BG: RGB{0, 0, 0}},
			{Rune: '!', FG: RGB{255, 255, 255}, BG: RGB{0, 0, 0}},
		},
		{
			{Rune: '█', FG: RGB{255, 0, 0}, BG: RGB{0, 0, 0}},
			{Rune: '▀', FG: RGB{0, 255, 0}, BG: RGB{0, 0, 255}},
			{Rune: '▄', FG: RGB{255, 255, 0}, BG: RGB{255, 0, 255}},
		},
	}
	
	// Render at scale 2
	img := fonts.RenderBlocks(blocks, 2)
	if img == nil {
		t.Fatal("RenderBlocks returned nil")
	}
	
	// Save test output
	opts := RenderOptions{
		UseFont:     true,
		FontBitmaps: fonts,
		Scale:       2,
	}
	
	tmpfile := "test_real_font_output.png"
	defer os.Remove(tmpfile)
	
	err = SaveBlocksToPNGWithOptions(blocks, tmpfile, opts)
	if err != nil {
		t.Fatalf("Failed to save with real font: %v", err)
	}
	
	// Verify file exists and has content
	info, err := os.Stat(tmpfile)
	if err != nil {
		t.Fatalf("Output file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Output file is empty")
	}
}

// TestMissingFont tests behavior when font file doesn't exist
func TestMissingFont(t *testing.T) {
	_, err := LoadFontBitmaps("nonexistent.ttf", "")
	if err == nil {
		t.Error("Expected error when loading non-existent font")
	}
}