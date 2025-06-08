package main

import (
	"image/color"
	"math"
	"math/rand"
)

// KMeansResult represents the result of k-means clustering
type KMeansResult struct {
	Centers  []color.Color
	Clusters [][]color.Color
}

// runKMeans performs k-means clustering on colors
func runKMeans(pixels []color.Color, k int, maxIterations int) KMeansResult {
	if len(pixels) == 0 || k <= 0 {
		return KMeansResult{}
	}
	
	// Initialize centers randomly from input pixels
	centers := make([]color.Color, k)
	usedIndices := make(map[int]bool)
	for i := 0; i < k; i++ {
		var idx int
		for {
			idx = rand.Intn(len(pixels))
			if !usedIndices[idx] {
				usedIndices[idx] = true
				break
			}
		}
		centers[i] = pixels[idx]
	}
	
	// Run k-means iterations
	for iter := 0; iter < maxIterations; iter++ {
		// Assign pixels to clusters
		clusters := make([][]color.Color, k)
		for _, pixel := range pixels {
			minDist := math.MaxFloat64
			bestCluster := 0
			for j, center := range centers {
				dist := colorDistance(pixel, center)
				if dist < minDist {
					minDist = dist
					bestCluster = j
				}
			}
			clusters[bestCluster] = append(clusters[bestCluster], pixel)
		}
		
		// Update centers
		converged := true
		for i := range centers {
			if len(clusters[i]) == 0 {
				continue
			}
			
			// Calculate mean of cluster
			var sumR, sumG, sumB uint32
			for _, pixel := range clusters[i] {
				r, g, b, _ := pixel.RGBA()
				sumR += r
				sumG += g
				sumB += b
			}
			count := uint32(len(clusters[i]))
			newCenter := color.RGBA{
				R: uint8((sumR / count) >> 8),
				G: uint8((sumG / count) >> 8),
				B: uint8((sumB / count) >> 8),
				A: 255,
			}
			
			// Check for convergence
			if colorDistance(centers[i], newCenter) > 0.01 {
				converged = false
			}
			centers[i] = newCenter
		}
		
		if converged {
			break
		}
	}
	
	// Build final clusters
	clusters := make([][]color.Color, k)
	for _, pixel := range pixels {
		minDist := math.MaxFloat64
		bestCluster := 0
		for j, center := range centers {
			dist := colorDistance(pixel, center)
			if dist < minDist {
				minDist = dist
				bestCluster = j
			}
		}
		clusters[bestCluster] = append(clusters[bestCluster], pixel)
	}
	
	return KMeansResult{
		Centers:  centers,
		Clusters: clusters,
	}
}

// findBestPaletteColors finds the two best palette colors using k-means
func findBestPaletteColors(pixels []color.Color, palette []color.Color) (color.Color, color.Color) {
	// Run k-means with k=2
	result := runKMeans(pixels, 2, 10)
	
	// Map cluster centers to nearest palette colors
	var fg, bg color.Color
	
	// Map first center
	minDist := math.MaxFloat64
	for _, pColor := range palette {
		dist := colorDistance(result.Centers[0], pColor)
		if dist < minDist {
			minDist = dist
			fg = pColor
		}
	}
	
	// Map second center (ensure it's different from first)
	minDist = math.MaxFloat64
	for _, pColor := range palette {
		if pColor == fg {
			continue
		}
		dist := colorDistance(result.Centers[1], pColor)
		if dist < minDist {
			minDist = dist
			bg = pColor
		}
	}
	
	// If the two colors are too similar, try to find a better contrast
	if colorDistance(fg, bg) < 30.0 { // Threshold for minimum contrast
		// Find the color that provides best contrast with fg
		maxDist := 0.0
		for _, pColor := range palette {
			if pColor == fg {
				continue
			}
			dist := colorDistance(fg, pColor)
			if dist > maxDist {
				maxDist = dist
				bg = pColor
			}
		}
	}
	
	return fg, bg
}

// findBestPaletteColorsLuminance selects colors based on luminance split
func findBestPaletteColorsLuminance(pixels []color.Color, palette []color.Color) (color.Color, color.Color) {
	// Calculate luminance for each pixel
	type pixelLum struct {
		color color.Color
		lum   float64
	}
	pixelLums := make([]pixelLum, len(pixels))
	
	for i, pixel := range pixels {
		r, g, b, _ := pixel.RGBA()
		// Use standard luminance formula
		lum := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
		pixelLums[i] = pixelLum{pixel, lum}
	}
	
	// Find median luminance
	totalLum := 0.0
	for _, pl := range pixelLums {
		totalLum += pl.lum
	}
	medianLum := totalLum / float64(len(pixelLums))
	
	// Split into dark and light groups
	var darkPixels, lightPixels []color.Color
	for _, pl := range pixelLums {
		if pl.lum < medianLum {
			darkPixels = append(darkPixels, pl.color)
		} else {
			lightPixels = append(lightPixels, pl.color)
		}
	}
	
	// Find average color for each group
	darkAvg := AverageColor(darkPixels)
	lightAvg := AverageColor(lightPixels)
	
	// Map to palette
	var darkPalette, lightPalette color.Color
	minDist := math.MaxFloat64
	for _, pColor := range palette {
		dist := colorDistance(darkAvg, pColor)
		if dist < minDist {
			minDist = dist
			darkPalette = pColor
		}
	}
	
	minDist = math.MaxFloat64
	for _, pColor := range palette {
		if pColor == darkPalette {
			continue
		}
		dist := colorDistance(lightAvg, pColor)
		if dist < minDist {
			minDist = dist
			lightPalette = pColor
		}
	}
	
	return darkPalette, lightPalette
}

