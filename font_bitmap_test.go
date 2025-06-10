package img2ansi

import (
	"os"
	"testing"
)

// TestGlyphBitmapBitOperations tests basic bit operations on GlyphBitmap
func TestGlyphBitmapBitOperations(t *testing.T) {
	var bitmap GlyphBitmap
	
	// Test setting bits
	bitmap.setBit(0, 0, true)
	if !bitmap.getBit(0, 0) {
		t.Error("Expected bit at (0,0) to be set")
	}
	
	bitmap.setBit(7, 7, true)
	if !bitmap.getBit(7, 7) {
		t.Error("Expected bit at (7,7) to be set")
	}
	
	// Test clearing bits
	bitmap.setBit(0, 0, false)
	if bitmap.getBit(0, 0) {
		t.Error("Expected bit at (0,0) to be clear")
	}
	
	// Test out of bounds
	bitmap.setBit(8, 8, true)
	if bitmap.getBit(8, 8) {
		t.Error("Out of bounds bit should return false")
	}
}

// TestFontBitmapsRendering tests rendering blocks with fonts
func TestFontBitmapsRendering(t *testing.T) {
	// Create a mock FontBitmaps for testing
	fb := &FontBitmaps{
		glyphs:   make(map[rune]GlyphBitmap),
		fallback: make(map[rune]GlyphBitmap),
		name:     "test",
	}
	
	// Add some test glyphs
	// Full block (all bits set)
	fb.glyphs['█'] = ^GlyphBitmap(0)
	
	// Half block (top half set)
	var halfBlock GlyphBitmap
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			halfBlock.setBit(x, y, true)
		}
	}
	fb.glyphs['▀'] = halfBlock
	
	// Create test blocks
	blocks := [][]BlockRune{
		{
			{Rune: '█', FG: RGB{255, 0, 0}, BG: RGB{0, 0, 0}},
			{Rune: '▀', FG: RGB{0, 255, 0}, BG: RGB{0, 0, 255}},
		},
		{
			{Rune: ' ', FG: RGB{255, 255, 255}, BG: RGB{128, 128, 128}},
			{Rune: 'A', FG: RGB{255, 255, 0}, BG: RGB{0, 0, 0}},
		},
	}
	
	// Test rendering at scale 1
	img := fb.RenderBlocks(blocks, 1)
	if img.Bounds().Dx() != 16 || img.Bounds().Dy() != 16 {
		t.Errorf("Expected 16x16 image, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
	
	// Test rendering at scale 2
	img2 := fb.RenderBlocks(blocks, 2)
	if img2.Bounds().Dx() != 32 || img2.Bounds().Dy() != 32 {
		t.Errorf("Expected 32x32 image, got %dx%d", img2.Bounds().Dx(), img2.Bounds().Dy())
	}
	
	// Check some pixel colors
	// Top-left should be red (full block with red foreground)
	c := img.At(0, 0)
	r, g, b, _ := c.RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Errorf("Expected red pixel at (0,0), got R:%d G:%d B:%d", r>>8, g>>8, b>>8)
	}
	
	// Bottom-left should be gray (space with gray background)
	c = img.At(0, 8)
	r, g, b, _ = c.RGBA()
	if r>>8 != 128 || g>>8 != 128 || b>>8 != 128 {
		t.Errorf("Expected gray pixel at (0,8), got R:%d G:%d B:%d", r>>8, g>>8, b>>8)
	}
}

// TestSaveBlocksWithFontRendering tests the integration with SaveBlocksToPNGWithOptions
func TestSaveBlocksWithFontRendering(t *testing.T) {
	// Create test blocks
	blocks := [][]BlockRune{
		{
			{Rune: '█', FG: RGB{255, 0, 0}, BG: RGB{0, 0, 0}},
			{Rune: '▀', FG: RGB{0, 255, 0}, BG: RGB{0, 0, 255}},
			{Rune: '▄', FG: RGB{255, 255, 0}, BG: RGB{255, 0, 255}},
		},
	}
	
	// Test geometric rendering (existing behavior)
	opts := RenderOptions{
		UseFont:     false,
		TargetWidth: 6,
		TargetHeight: 2,
		ScaleFactor: 1.0,
	}
	
	tmpfile := "test_geometric.png"
	defer os.Remove(tmpfile)
	
	err := SaveBlocksToPNGWithOptions(blocks, tmpfile, opts)
	if err != nil {
		t.Fatalf("Failed to save with geometric rendering: %v", err)
	}
	
	// Verify file was created
	if _, err := os.Stat(tmpfile); os.IsNotExist(err) {
		t.Error("Geometric rendered file was not created")
	}
	
	// Test font rendering (would need actual fonts for full test)
	// This test just verifies the code path works
	fb := &FontBitmaps{
		glyphs:   make(map[rune]GlyphBitmap),
		fallback: make(map[rune]GlyphBitmap),
		name:     "test",
	}
	
	// Add test glyph
	fb.glyphs['█'] = ^GlyphBitmap(0) // Full block
	
	opts2 := RenderOptions{
		UseFont:     true,
		FontBitmaps: fb,
		Scale:       2,
	}
	
	tmpfile2 := "test_font.png"
	defer os.Remove(tmpfile2)
	
	err = SaveBlocksToPNGWithOptions(blocks, tmpfile2, opts2)
	if err != nil {
		t.Fatalf("Failed to save with font rendering: %v", err)
	}
	
	// Verify file was created
	if _, err := os.Stat(tmpfile2); os.IsNotExist(err) {
		t.Error("Font rendered file was not created")
	}
}

// TestRgbToColor tests the RGB to color.Color conversion
func TestRgbToColor(t *testing.T) {
	rgb := RGB{R: 128, G: 64, B: 192}
	c := rgbToColor(rgb)
	
	r, g, b, a := c.RGBA()
	// RGBA returns 16-bit values, so we need to shift
	if r>>8 != 128 || g>>8 != 64 || b>>8 != 192 || a>>8 != 255 {
		t.Errorf("RGB conversion failed: got R:%d G:%d B:%d A:%d", r>>8, g>>8, b>>8, a>>8)
	}
}