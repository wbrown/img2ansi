package main

import (
	"image"
	"image/color"
	"math"
	"sort"
	"github.com/wbrown/img2ansi"
)

// OptimizedColorSelector uses k-means with nearest palette colors
type OptimizedColorSelector struct {
	K int
}

func (o *OptimizedColorSelector) SelectColors(pixels []img2ansi.RGB, palette []img2ansi.RGB) []ColorPair {
	// Run k-means to find dominant colors
	dominantColors := kmeansColorsRGB(pixels, o.K)
	
	// For each dominant color, find nearest palette colors
	candidateColors := make(map[img2ansi.RGB]bool)
	for _, dominant := range dominantColors {
		// Find k nearest palette colors
		nearest := findKNearestColorsRGB(dominant, palette, o.K)
		for _, c := range nearest {
			candidateColors[c] = true
		}
	}
	
	// Convert to slice
	var candidates []img2ansi.RGB
	for c := range candidateColors {
		candidates = append(candidates, c)
	}
	
	// Generate pairs from candidates
	var pairs []ColorPair
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			pairs = append(pairs, ColorPair{candidates[i], candidates[j]})
			pairs = append(pairs, ColorPair{candidates[j], candidates[i]})
		}
	}
	
	return pairs
}

// FrequencyColorSelector selects the most frequent palette colors in the block
type FrequencyColorSelector struct {
	TopN int
}

func (f *FrequencyColorSelector) SelectColors(pixels []img2ansi.RGB, palette []img2ansi.RGB) []ColorPair {
	// Map each pixel to its nearest palette color
	paletteCounts := make(map[img2ansi.RGB]int)
	for _, pixel := range pixels {
		nearest := findNearestPaletteColorRGB(pixel, palette)
		paletteCounts[nearest]++
	}
	
	// Sort by frequency
	type colorFreq struct {
		color img2ansi.RGB
		count int
	}
	var frequencies []colorFreq
	for c, count := range paletteCounts {
		frequencies = append(frequencies, colorFreq{c, count})
	}
	sort.Slice(frequencies, func(i, j int) bool {
		return frequencies[i].count > frequencies[j].count
	})
	
	// Take top N
	topColors := make([]img2ansi.RGB, 0, f.TopN)
	for i := 0; i < f.TopN && i < len(frequencies); i++ {
		topColors = append(topColors, frequencies[i].color)
	}
	
	// Generate pairs
	var pairs []ColorPair
	for i := 0; i < len(topColors); i++ {
		for j := i + 1; j < len(topColors); j++ {
			pairs = append(pairs, ColorPair{topColors[i], topColors[j]})
			pairs = append(pairs, ColorPair{topColors[j], topColors[i]})
		}
	}
	
	// If not enough pairs, add some defaults
	if len(pairs) == 0 && len(topColors) > 0 {
		// Find a contrasting color
		contrast := findContrastingColorRGB(topColors[0], palette)
		pairs = append(pairs, ColorPair{topColors[0], contrast})
		pairs = append(pairs, ColorPair{contrast, topColors[0]})
	}
	
	return pairs
}

// ContrastColorSelector ensures high contrast between fg and bg
type ContrastColorSelector struct {
	MinContrast float64
}

func (c *ContrastColorSelector) SelectColors(pixels []img2ansi.RGB, palette []img2ansi.RGB) []ColorPair {
	// First extract dominant colors as candidates
	dominantColors := extractBlockColorsRGB(pixels)
	
	var pairs []ColorPair
	
	// For each dominant color, find high-contrast pairs
	for _, dominant := range dominantColors {
		fg := findNearestPaletteColorRGB(dominant, palette)
		
		// Find colors with sufficient contrast
		for _, bg := range palette {
			if calculateContrastRGB(fg, bg) >= c.MinContrast {
				pairs = append(pairs, ColorPair{fg, bg})
			}
		}
	}
	
	// If no high-contrast pairs found, fall back to maximum contrast
	if len(pairs) == 0 {
		fg, bg := findMaxContrastPairRGB(palette)
		pairs = append(pairs, ColorPair{fg, bg})
		pairs = append(pairs, ColorPair{bg, fg})
	}
	
	return pairs
}

// QuantizedColorSelector pre-quantizes the block before matching
type QuantizedColorSelector struct {
	Levels int
}

func (q *QuantizedColorSelector) SelectColors(pixels []img2ansi.RGB, palette []img2ansi.RGB) []ColorPair {
	// Quantize block pixels to N levels
	quantized := quantizeColorsKMeansRGB(pixels, q.Levels)
	
	// Map quantized colors to palette
	paletteColors := make(map[img2ansi.RGB]bool)
	for _, qc := range quantized {
		pc := findNearestPaletteColorRGB(qc, palette)
		paletteColors[pc] = true
	}
	
	// Convert to slice
	var uniqueColors []img2ansi.RGB
	for c := range paletteColors {
		uniqueColors = append(uniqueColors, c)
	}
	
	// Generate pairs
	var pairs []ColorPair
	for i := 0; i < len(uniqueColors); i++ {
		for j := i + 1; j < len(uniqueColors); j++ {
			pairs = append(pairs, ColorPair{uniqueColors[i], uniqueColors[j]})
			pairs = append(pairs, ColorPair{uniqueColors[j], uniqueColors[i]})
		}
	}
	
	return pairs
}

// Helper functions for color selection
// Note: These have been replaced by RGB versions in rgb_helpers.go

// kmeansColors performs k-means clustering on colors
func kmeansColors(pixels []color.Color, k int) []color.Color {
	if k <= 0 || len(pixels) == 0 {
		return []color.Color{}
	}
	
	// Initialize centers randomly from pixels
	centers := make([]color.Color, k)
	for i := 0; i < k; i++ {
		centers[i] = pixels[i*len(pixels)/k]
	}
	
	// K-means iterations
	for iter := 0; iter < 10; iter++ {
		// Assign pixels to clusters
		clusters := make([][]color.Color, k)
		for _, pixel := range pixels {
			nearest := 0
			minDist := math.MaxFloat64
			for j, center := range centers {
				dist := colorDistance(pixel, center)
				if dist < minDist {
					minDist = dist
					nearest = j
				}
			}
			clusters[nearest] = append(clusters[nearest], pixel)
		}
		
		// Update centers
		for i, cluster := range clusters {
			if len(cluster) > 0 {
				centers[i] = AverageColor(cluster)
			}
		}
	}
	
	return centers
}


// Color error calculation for diffusion
func calculateColorError(actual, target color.Color) (float64, float64, float64) {
	r1, g1, b1, _ := actual.RGBA()
	r2, g2, b2, _ := target.RGBA()
	
	// Convert to signed values for error
	errorR := float64(r1) - float64(r2)
	errorG := float64(g1) - float64(g2)
	errorB := float64(b1) - float64(b2)
	
	// Scale down from 16-bit to 8-bit range
	return errorR / 256.0, errorG / 256.0, errorB / 256.0
}

// distributeErrorDiffusion applies Floyd-Steinberg error distribution
func distributeErrorDiffusion(img *image.RGBA, x, y int, errorR, errorG, errorB, strength float64) {
	bounds := img.Bounds()
	
	// Floyd-Steinberg distribution matrix
	// X   7/16
	// 3/16 5/16 1/16
	
	distribute := func(dx, dy int, factor float64) {
		nx, ny := x+dx, y+dy
		if nx >= bounds.Min.X && nx < bounds.Max.X && ny >= bounds.Min.Y && ny < bounds.Max.Y {
			c := img.RGBAAt(nx, ny)
			img.SetRGBA(nx, ny, color.RGBA{
				R: clampUint8(float64(c.R) + errorR*factor*strength),
				G: clampUint8(float64(c.G) + errorG*factor*strength),
				B: clampUint8(float64(c.B) + errorB*factor*strength),
				A: c.A,
			})
		}
	}
	
	distribute(1, 0, 7.0/16.0)   // Right
	distribute(-1, 1, 3.0/16.0)  // Bottom-left
	distribute(0, 1, 5.0/16.0)   // Bottom
	distribute(1, 1, 1.0/16.0)   // Bottom-right
}

func clampUint8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// TrueExhaustiveColorSelector returns ALL possible color combinations
type TrueExhaustiveColorSelector struct{}

func (t *TrueExhaustiveColorSelector) SelectColors(pixels []img2ansi.RGB, palette []img2ansi.RGB) []ColorPair {
	// Return every possible combination of foreground and background colors
	// This is the key difference - we don't analyze the pixels at all,
	// we just return every possible pairing for the algorithm to test
	pairs := make([]ColorPair, 0, len(palette)*len(palette))
	
	for _, fg := range palette {
		for _, bg := range palette {
			pairs = append(pairs, ColorPair{Fg: fg, Bg: bg})
		}
	}
	
	return pairs
}