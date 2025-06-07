package main

import (
	"image/color"
	"math"
)

// colorDistance calculates simple RGB distance between two colors
func colorDistance(c1, c2 color.Color) float64 {
	r1, g1, b1, _ := c1.RGBA()
	r2, g2, b2, _ := c2.RGBA()

	dr := float64(r1) - float64(r2)
	dg := float64(g1) - float64(g2)
	db := float64(b1) - float64(b2)

	return math.Sqrt(dr*dr + dg*dg + db*db)
}

// rgbToANSI256 converts RGB to ANSI 256 color code
func rgbToANSI256(r, g, b uint8) int {
	// Map to 6x6x6 color cube (16-231)
	if r == g && g == b {
		// Grayscale (232-255)
		if r < 8 {
			return 16 // Black
		} else if r > 247 {
			return 231 // White  
		} else {
			return 232 + int((r-8)/10)
		}
	}

	// Color cube
	rIdx := int(r) * 5 / 255
	gIdx := int(g) * 5 / 255
	bIdx := int(b) * 5 / 255

	return 16 + 36*rIdx + 6*gIdx + bIdx
}

// calculateBlockError calculates total pixel error for a character
func calculateBlockError(pixels []color.Color, charBitmap GlyphBitmap, fg, bg color.Color) float64 {
	var totalError float64

	for i := 0; i < 64; i++ {
		y := i / 8
		x := i % 8

		// What color does the character show at this position?
		var charColor color.Color
		if getBit(charBitmap, x, y) {
			charColor = fg
		} else {
			charColor = bg
		}

		// Calculate error vs actual pixel
		totalError += colorDistance(pixels[i], charColor)
	}

	return totalError
}

// extractBlockColors extracts two most common colors from pixel array
func extractBlockColors(pixels []color.Color) []color.Color {
	colorCounts := make(map[uint32]int)
	colorMap := make(map[uint32]color.Color)

	for _, c := range pixels {
		r, g, b, _ := c.RGBA()
		// Quantize to reduce color space
		r = (r >> 10) << 10
		g = (g >> 10) << 10
		b = (b >> 10) << 10
		key := (r << 16) | (g << 8) | b
		colorCounts[key]++
		colorMap[key] = color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
	}

	// Find two most common
	var colors []color.Color
	for i := 0; i < 2; i++ {
		var maxCount int
		var maxKey uint32
		for key, count := range colorCounts {
			if count > maxCount {
				maxCount = count
				maxKey = key
			}
		}
		if maxCount > 0 {
			colors = append(colors, colorMap[maxKey])
			delete(colorCounts, maxKey)
		}
	}

	// Ensure we have at least 2 colors
	for len(colors) < 2 {
		colors = append(colors, color.Black)
	}

	return colors
}