package img2ansi

import (
	"fmt"
	"testing"
)

func TestBlockSelection(t *testing.T) {
	t.Parallel()

	// Create Renderer with LAB color method
	r := NewRenderer(
		WithColorMethod(LABMethod{}),
		WithPalette("colordata/ansi16.json"),
	)

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
		bestRune, fgColor, bgColor := r.FindBestBlockRepresentation(test.block, false)
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
		r.fgAnsi.Iterate(func(key, value interface{}) {
			if rgbFromUint32(key.(uint32)) == fgColor {
				fgCode = value.(string)
			}
		})
		r.bgAnsi.Iterate(func(key, value interface{}) {
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
	for red := 0; red < 256; red += 32 {
		for green := 0; green < 256; green += 32 {
			for blue := 0; blue < 256; blue += 32 {
				block := [4]RGB{
					{uint8(red), uint8(green), uint8(blue)},
					{uint8((red + 16) % 256), uint8((green + 16) % 256), uint8((blue + 16) % 256)},
					{uint8((red + 32) % 256), uint8((green + 32) % 256), uint8((blue + 32) % 256)},
					{uint8((red + 48) % 256), uint8((green + 48) % 256), uint8((blue + 48) % 256)},
				}

				_, fgColor, bgColor := r.FindBestBlockRepresentation(block, false)
				colorCounts[fgColor]++
				colorCounts[bgColor]++
			}
		}
	}
	
	fmt.Println("\nColor usage frequency:")
	for color, count := range colorCounts {
		if count > 0 {
			code := "?"
			r.fgAnsi.Iterate(func(key, value interface{}) {
				if rgbFromUint32(key.(uint32)) == color {
					code = value.(string)
				}
			})
			fmt.Printf("  RGB(%d,%d,%d) [code %s]: used %d times\n",
				color.R, color.G, color.B, code, count)
		}
	}
}