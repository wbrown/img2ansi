package img2ansi

import (
	"fmt"
	"testing"
)

func TestColorMatching(t *testing.T) {
	// Load the ansi16 palette
	_, _, err := LoadPalette("colordata/ansi16.json")
	if err != nil {
		t.Fatalf("Failed to load palette: %v", err)
	}

	// Test specific colors that should map to browns, oranges, etc
	testCases := []struct {
		name     string
		input    RGB
		expected string // Expected ANSI color name
	}{
		{"Pure Brown", RGB{170, 85, 0}, "brown (#AA5500)"},
		{"Dark Orange", RGB{200, 100, 0}, "brown (#AA5500)"},
		{"Light Brown", RGB{180, 120, 60}, "brown (#AA5500)"},
		{"Pure Red", RGB{255, 0, 0}, "bright red (#FF5555)"},
		{"Pure Cyan", RGB{0, 255, 255}, "bright cyan (#55FFFF)"},
		{"Medium Gray", RGB{128, 128, 128}, "white (#AAAAAA)"},
		{"Dark Gray", RGB{64, 64, 64}, "bright black (#555555)"},
	}

	fmt.Println("Testing color matching with LAB method:")
	CurrentColorDistanceMethod = MethodLAB
	for _, tc := range testCases {
		// Find closest color in palette
		closest := (*fgClosestColor)[tc.input.toUint32()]
		fmt.Printf("  %s RGB(%d,%d,%d) -> RGB(%d,%d,%d)\n",
			tc.name, tc.input.R, tc.input.G, tc.input.B,
			closest.R, closest.G, closest.B)
	}

	fmt.Println("\nTesting color matching with RGB method:")
	CurrentColorDistanceMethod = MethodRGB
	// Reload palette for RGB method
	_, _, err = LoadPalette("colordata/ansi16.json")
	if err != nil {
		t.Fatalf("Failed to reload palette: %v", err)
	}
	
	for _, tc := range testCases {
		// Find closest color in palette
		closest := (*fgClosestColor)[tc.input.toUint32()]
		fmt.Printf("  %s RGB(%d,%d,%d) -> RGB(%d,%d,%d)\n",
			tc.name, tc.input.R, tc.input.G, tc.input.B,
			closest.R, closest.G, closest.B)
	}

	// Test block selection
	fmt.Println("\nTesting block color selection:")
	// A block with brownish colors
	brownBlock := [4]RGB{
		{180, 120, 60},
		{170, 110, 50},
		{190, 130, 70},
		{160, 100, 40},
	}
	
	CurrentColorDistanceMethod = MethodLAB
	bestRune, fgColor, bgColor := findBestBlockRepresentation(brownBlock, false)
	fmt.Printf("  Brown block -> Rune: %c, FG: RGB(%d,%d,%d), BG: RGB(%d,%d,%d)\n",
		bestRune, fgColor.R, fgColor.G, fgColor.B, bgColor.R, bgColor.G, bgColor.B)

	// A block with high contrast
	contrastBlock := [4]RGB{
		{255, 255, 255},
		{0, 0, 0},
		{255, 255, 255},
		{0, 0, 0},
	}
	
	bestRune, fgColor, bgColor = findBestBlockRepresentation(contrastBlock, false)
	fmt.Printf("  Contrast block -> Rune: %c, FG: RGB(%d,%d,%d), BG: RGB(%d,%d,%d)\n",
		bestRune, fgColor.R, fgColor.G, fgColor.B, bgColor.R, bgColor.G, bgColor.B)

	// Print the actual palette colors
	fmt.Println("\nANSI 16 Palette colors:")
	fgAnsi.Iterate(func(key, value interface{}) {
		color := rgbFromUint32(key.(uint32))
		code := value.(string)
		fmt.Printf("  Code %s: RGB(%d,%d,%d) = #%02X%02X%02X\n",
			code, color.R, color.G, color.B, color.R, color.G, color.B)
	})

	// Test some mandrill-like colors
	fmt.Println("\nTesting mandrill face colors:")
	mandrillColors := []RGB{
		{180, 140, 100}, // Brownish fur
		{200, 160, 120}, // Lighter brown
		{100, 80, 60},   // Darker brown
		{220, 180, 140}, // Very light brown
		{80, 120, 180},  // Blueish (cheek)
		{200, 60, 60},   // Reddish (nose)
	}

	for i, color := range mandrillColors {
		// Test as a uniform block
		block := [4]RGB{color, color, color, color}
		bestRune, fgColor, bgColor := findBestBlockRepresentation(block, false)
		fmt.Printf("  Color %d RGB(%d,%d,%d) -> Rune: %c, FG: RGB(%d,%d,%d), BG: RGB(%d,%d,%d)\n",
			i, color.R, color.G, color.B, bestRune, 
			fgColor.R, fgColor.G, fgColor.B,
			bgColor.R, bgColor.G, bgColor.B)
	}
}