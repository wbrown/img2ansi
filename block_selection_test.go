package img2ansi

import (
	"fmt"
	"testing"
)

func TestBlockSelection(t *testing.T) {
	// Load the ansi16 palette with LAB
	CurrentColorDistanceMethod = MethodLAB
	_, _, err := LoadPalette("colordata/ansi16.json")
	if err != nil {
		t.Fatalf("Failed to load palette: %v", err)
	}

	fmt.Println("Testing mixed color blocks:")
	
	// Test a block that should use brown
	testBlocks := []struct {
		name  string
		block [4]RGB
	}{
		{
			"Brown gradient",
			[4]RGB{
				{170, 85, 0},   // Pure brown
				{200, 100, 20}, // Lighter
				{140, 70, 0},   // Darker
				{180, 90, 10},  // Medium
			},
		},
		{
			"Brown/Gray mix",
			[4]RGB{
				{170, 85, 0},    // Brown
				{128, 128, 128}, // Gray
				{170, 85, 0},    // Brown
				{128, 128, 128}, // Gray
			},
		},
		{
			"Natural fur colors",
			[4]RGB{
				{160, 120, 80},
				{180, 140, 100},
				{140, 100, 60},
				{200, 160, 120},
			},
		},
		{
			"High contrast with brown",
			[4]RGB{
				{170, 85, 0},    // Brown
				{255, 255, 255}, // White
				{170, 85, 0},    // Brown
				{0, 0, 0},       // Black
			},
		},
	}

	for _, test := range testBlocks {
		bestRune, fgColor, bgColor := FindBestBlockRepresentation(test.block, false)
		fmt.Printf("\n%s:\n", test.name)
		fmt.Printf("  Input: ")
		for _, c := range test.block {
			fmt.Printf("RGB(%d,%d,%d) ", c.R, c.G, c.B)
		}
		fmt.Printf("\n  Result: Rune '%c', FG: RGB(%d,%d,%d), BG: RGB(%d,%d,%d)\n",
			bestRune, fgColor.R, fgColor.G, fgColor.B, bgColor.R, bgColor.G, bgColor.B)
		
		// Show which color codes these map to
		fgCode := "?"
		bgCode := "?"
		fgAnsi.Iterate(func(key, value interface{}) {
			if rgbFromUint32(key.(uint32)) == fgColor {
				fgCode = value.(string)
			}
		})
		bgAnsi.Iterate(func(key, value interface{}) {
			if rgbFromUint32(key.(uint32)) == bgColor {
				bgCode = value.(string)
			}
		})
		fmt.Printf("  ANSI codes: FG=%s, BG=%s\n", fgCode, bgCode)
	}

	// Count how many times each color is used
	fmt.Println("\nTesting color frequency in block selection:")
	colorCounts := make(map[RGB]int)
	
	// Generate many random-ish blocks
	for r := 0; r < 256; r += 32 {
		for g := 0; g < 256; g += 32 {
			for b := 0; b < 256; b += 32 {
				block := [4]RGB{
					{uint8(r), uint8(g), uint8(b)},
					{uint8((r + 16) % 256), uint8((g + 16) % 256), uint8((b + 16) % 256)},
					{uint8((r + 32) % 256), uint8((g + 32) % 256), uint8((b + 32) % 256)},
					{uint8((r + 48) % 256), uint8((g + 48) % 256), uint8((b + 48) % 256)},
				}
				
				_, fgColor, bgColor := FindBestBlockRepresentation(block, false)
				colorCounts[fgColor]++
				colorCounts[bgColor]++
			}
		}
	}
	
	fmt.Println("\nColor usage frequency:")
	for color, count := range colorCounts {
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