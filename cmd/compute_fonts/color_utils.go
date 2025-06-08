package main

import (
	"image/color"
)

// Note: ANSI256ToRGB and RGB24ToANSI256 have been removed in favor of
// using the main package's palette system via img2ansi.LoadPalette()
// The main package handles all RGB to ANSI conversions through its
// pre-computed palette lookup tables.


// AverageColor calculates the average of a slice of colors
func AverageColor(colors []color.Color) color.Color {
	if len(colors) == 0 {
		return color.Black
	}
	
	var sumR, sumG, sumB uint32
	for _, c := range colors {
		r, g, b, _ := c.RGBA()
		sumR += r
		sumG += g
		sumB += b
	}
	
	n := uint32(len(colors))
	return color.RGBA{
		R: uint8((sumR / n) >> 8),
		G: uint8((sumG / n) >> 8),
		B: uint8((sumB / n) >> 8),
		A: 255,
	}
}

// QuantizeColor reduces color to a lower bit depth
func QuantizeColor(c color.Color, bitsPerChannel int) color.Color {
	r, g, b, a := c.RGBA()
	
	// Convert to 8-bit
	r8 := uint8(r >> 8)
	g8 := uint8(g >> 8)
	b8 := uint8(b >> 8)
	
	// Calculate shift amount
	shift := uint(8 - bitsPerChannel)
	mask := uint8(0xFF << shift)
	
	// Quantize and restore
	r8 = (r8 & mask) | (r8 >> shift)
	g8 = (g8 & mask) | (g8 >> shift)
	b8 = (b8 & mask) | (b8 >> shift)
	
	return color.RGBA{r8, g8, b8, uint8(a >> 8)}
}

// FindClosestPaletteColor finds the closest color in a palette
func FindClosestPaletteColor(target color.Color, palette []color.Color) (color.Color, int) {
	if len(palette) == 0 {
		return color.Black, -1
	}
	
	bestIdx := 0
	bestDist := colorDistance(target, palette[0])
	
	for i := 1; i < len(palette); i++ {
		dist := colorDistance(target, palette[i])
		if dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}
	
	return palette[bestIdx], bestIdx
}

// ExtractTopColors extracts N dominant colors from a slice
func ExtractTopColors(pixels []color.Color, n int) []color.Color {
	if len(pixels) == 0 || n <= 0 {
		return nil
	}
	
	// Simple frequency-based approach
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
	
	// Extract top N colors
	result := make([]color.Color, 0, n)
	for i := 0; i < n && len(colorCounts) > 0; i++ {
		var maxCount int
		var maxKey uint32
		for key, count := range colorCounts {
			if count > maxCount {
				maxCount = count
				maxKey = key
			}
		}
		if maxCount > 0 {
			result = append(result, colorMap[maxKey])
			delete(colorCounts, maxKey)
		}
	}
	
	// Fill with black if not enough colors
	for len(result) < n {
		result = append(result, color.Black)
	}
	
	return result
}

// CalculateLuminance returns the perceptual luminance of a color
func CalculateLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	// Standard luminance formula
	return 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
}

// GetContrastRatio calculates the contrast ratio between two colors
func GetContrastRatio(c1, c2 color.Color) float64 {
	l1 := CalculateLuminance(c1) / 255.0
	l2 := CalculateLuminance(c2) / 255.0
	
	// Add small value to avoid division by zero
	l1 = l1*0.98 + 0.01
	l2 = l2*0.98 + 0.01
	
	if l1 > l2 {
		return l1 / l2
	}
	return l2 / l1
}