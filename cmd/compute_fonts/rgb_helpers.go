package main

import (
	"github.com/wbrown/img2ansi"
	"image"
	"math"
)

// extractBlockPixelsRGB extracts an 8x8 block of pixels as RGB values
func extractBlockPixelsRGB(img image.Image, blockX, blockY int) []img2ansi.RGB {
	pixels := make([]img2ansi.RGB, 64)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			pixX := blockX*8 + x
			pixY := blockY*8 + y
			pixels[y*8+x] = colorToRGB(img.At(pixX, pixY))
		}
	}
	return pixels
}

// extractBlockColorsRGB extracts two most common colors from RGB array
func extractBlockColorsRGB(pixels []img2ansi.RGB) []img2ansi.RGB {
	// Use ExtractTopColors with conversion
	colorPixels := rgbsToColors(pixels)
	colors := ExtractTopColors(colorPixels, 2)
	
	// Convert back to RGB
	rgbs := make([]img2ansi.RGB, len(colors))
	for i, c := range colors {
		rgbs[i] = colorToRGB(c)
	}
	
	// Ensure we have at least 2 colors
	for len(rgbs) < 2 {
		rgbs = append(rgbs, img2ansi.RGB{R: 0, G: 0, B: 0}) // Black
	}
	
	return rgbs
}

// findNearestPaletteColorRGB finds the nearest palette color using RGB
func findNearestPaletteColorRGB(target img2ansi.RGB, palette []img2ansi.RGB) img2ansi.RGB {
	if len(palette) == 0 {
		return target
	}
	
	minDist := target.ColorDistance(palette[0])
	nearest := palette[0]
	
	for _, c := range palette[1:] {
		dist := target.ColorDistance(c)
		if dist < minDist {
			minDist = dist
			nearest = c
		}
	}
	
	return nearest
}

// Note: getPaletteColorsRGB has been removed in favor of using
// the main package's palette system via img2ansi.LoadPalette()

// calculateGlyphBlockErrorRGB calculates error with RGB types
func calculateGlyphBlockErrorRGB(pixels []img2ansi.RGB, glyphBitmap GlyphBitmap, fg, bg img2ansi.RGB) float64 {
	var totalError float64
	
	for i := 0; i < 64; i++ {
		x := i % 8
		y := i / 8
		
		// Check if this pixel is set in the glyph
		var glyphColor img2ansi.RGB
		if getBit(glyphBitmap, x, y) {
			glyphColor = fg
		} else {
			glyphColor = bg
		}
		
		// Calculate error vs actual pixel
		totalError += pixels[i].ColorDistance(glyphColor)
	}
	
	return totalError
}

// kmeansColorsRGB performs k-means clustering on RGB values
func kmeansColorsRGB(pixels []img2ansi.RGB, k int) []img2ansi.RGB {
	// Convert to color.Color for kmeans (temporary)
	colors := rgbsToColors(pixels)
	centers := kmeansColors(colors, k)
	
	// Convert back to RGB
	rgbCenters := make([]img2ansi.RGB, len(centers))
	for i, c := range centers {
		rgbCenters[i] = colorToRGB(c)
	}
	
	return rgbCenters
}

// findKNearestColorsRGB finds k nearest colors in palette to target
func findKNearestColorsRGB(target img2ansi.RGB, palette []img2ansi.RGB, k int) []img2ansi.RGB {
	if k >= len(palette) {
		return palette
	}
	
	// Calculate distances
	type colorDist struct {
		color img2ansi.RGB
		dist  float64
	}
	
	distances := make([]colorDist, len(palette))
	for i, c := range palette {
		distances[i] = colorDist{c, target.ColorDistance(c)}
	}
	
	// Sort by distance
	for i := 0; i < len(distances)-1; i++ {
		for j := i + 1; j < len(distances); j++ {
			if distances[j].dist < distances[i].dist {
				distances[i], distances[j] = distances[j], distances[i]
			}
		}
	}
	
	// Return k nearest
	result := make([]img2ansi.RGB, k)
	for i := 0; i < k; i++ {
		result[i] = distances[i].color
	}
	
	return result
}

// findContrastingColorRGB finds a color in palette with good contrast to the target
func findContrastingColorRGB(target img2ansi.RGB, palette []img2ansi.RGB) img2ansi.RGB {
	// Calculate luminance of target
	targetLum := calculateLuminanceRGB(target)
	
	// Find color with maximum luminance difference
	var bestColor img2ansi.RGB
	maxDiff := 0.0
	
	for _, c := range palette {
		lum := calculateLuminanceRGB(c)
		diff := math.Abs(targetLum - lum)
		if diff > maxDiff {
			maxDiff = diff
			bestColor = c
		}
	}
	
	return bestColor
}

// calculateLuminanceRGB calculates relative luminance of RGB color
func calculateLuminanceRGB(rgb img2ansi.RGB) float64 {
	// Using standard luminance formula
	return 0.299*float64(rgb.R) + 0.587*float64(rgb.G) + 0.114*float64(rgb.B)
}

// calculateContrastRGB calculates contrast between two RGB colors
func calculateContrastRGB(c1, c2 img2ansi.RGB) float64 {
	// Simple RGB distance for contrast
	dr := float64(c1.R) - float64(c2.R)
	dg := float64(c1.G) - float64(c2.G)
	db := float64(c1.B) - float64(c2.B)
	
	return math.Sqrt(dr*dr + dg*dg + db*db) / 255.0 * 100.0
}

// findMaxContrastPairRGB finds the pair of colors with maximum contrast
func findMaxContrastPairRGB(palette []img2ansi.RGB) (img2ansi.RGB, img2ansi.RGB) {
	var bestFg, bestBg img2ansi.RGB
	maxContrast := 0.0
	
	for i := 0; i < len(palette); i++ {
		for j := i + 1; j < len(palette); j++ {
			contrast := calculateContrastRGB(palette[i], palette[j])
			if contrast > maxContrast {
				maxContrast = contrast
				bestFg = palette[i]
				bestBg = palette[j]
			}
		}
	}
	
	return bestFg, bestBg
}

// quantizeColorsKMeansRGB performs k-means quantization on RGB values
func quantizeColorsKMeansRGB(pixels []img2ansi.RGB, levels int) []img2ansi.RGB {
	// Use k-means for quantization
	return kmeansColorsRGB(pixels, levels)
}