package imageutil

import (
	"image/color"
	"math"
)

// CreateGradientImage creates a horizontal gradient test image.
func CreateGradientImage(width, height int) *RGBAImage {
	img := NewRGBAImage(width, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v := uint8(255 * x / (width - 1))
			img.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return img
}

// CreateVerticalGradientImage creates a vertical gradient test image.
func CreateVerticalGradientImage(width, height int) *RGBAImage {
	img := NewRGBAImage(width, height)
	for y := 0; y < height; y++ {
		v := uint8(255 * y / (height - 1))
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return img
}

// CreateCheckerboardImage creates a checkerboard pattern for edge testing.
func CreateCheckerboardImage(width, height, squareSize int) *RGBAImage {
	img := NewRGBAImage(width, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			isWhite := ((x/squareSize)+(y/squareSize))%2 == 0
			if isWhite {
				img.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			} else {
				img.SetRGBA(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 255})
			}
		}
	}
	return img
}

// CreateSolidImage creates a solid color image.
func CreateSolidImage(width, height int, c RGB) *RGBAImage {
	img := NewRGBAImage(width, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGB(x, y, c)
		}
	}
	return img
}

// CreateColorBarsImage creates a color bars test pattern.
func CreateColorBarsImage(width, height int) *RGBAImage {
	img := NewRGBAImage(width, height)
	colors := []RGB{
		{255, 255, 255}, // White
		{255, 255, 0},   // Yellow
		{0, 255, 255},   // Cyan
		{0, 255, 0},     // Green
		{255, 0, 255},   // Magenta
		{255, 0, 0},     // Red
		{0, 0, 255},     // Blue
		{0, 0, 0},       // Black
	}

	barWidth := width / len(colors)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			colorIdx := x / barWidth
			if colorIdx >= len(colors) {
				colorIdx = len(colors) - 1
			}
			img.SetRGB(x, y, colors[colorIdx])
		}
	}
	return img
}

// CreateEdgeImage creates an image with sharp edges for testing edge detection.
func CreateEdgeImage(width, height int) *RGBAImage {
	img := NewRGBAImage(width, height)
	// Fill with gray background
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 128, G: 128, B: 128, A: 255})
		}
	}

	// Add white rectangle in center
	rx1, ry1 := width/4, height/4
	rx2, ry2 := 3*width/4, 3*height/4
	for y := ry1; y < ry2; y++ {
		for x := rx1; x < rx2; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	// Add diagonal line
	for i := 0; i < min(width, height)/2; i++ {
		img.SetRGBA(i, i, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	}

	return img
}

// CalculateMSE calculates the Mean Squared Error between two RGBA images.
func CalculateMSE(img1, img2 *RGBAImage) float64 {
	if img1.Width() != img2.Width() || img1.Height() != img2.Height() {
		return math.MaxFloat64
	}

	width, height := img1.Width(), img1.Height()
	var sumSq float64
	count := float64(width * height * 3) // 3 channels

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c1 := img1.RGBAAt(x, y)
			c2 := img2.RGBAAt(x, y)
			dr := float64(c1.R) - float64(c2.R)
			dg := float64(c1.G) - float64(c2.G)
			db := float64(c1.B) - float64(c2.B)
			sumSq += dr*dr + dg*dg + db*db
		}
	}

	return sumSq / count
}

// CalculateMSEGray calculates the Mean Squared Error between two grayscale images.
func CalculateMSEGray(img1, img2 *GrayImage) float64 {
	if img1.Width() != img2.Width() || img1.Height() != img2.Height() {
		return math.MaxFloat64
	}

	width, height := img1.Width(), img1.Height()
	var sumSq float64
	count := float64(width * height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v1 := float64(img1.GrayAt(x, y).Y)
			v2 := float64(img2.GrayAt(x, y).Y)
			d := v1 - v2
			sumSq += d * d
		}
	}

	return sumSq / count
}

// CalculateMaxDiff calculates the maximum pixel difference between two images.
func CalculateMaxDiff(img1, img2 *RGBAImage) int {
	if img1.Width() != img2.Width() || img1.Height() != img2.Height() {
		return 256
	}

	width, height := img1.Width(), img1.Height()
	maxDiff := 0

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c1 := img1.RGBAAt(x, y)
			c2 := img2.RGBAAt(x, y)
			dr := abs(int(c1.R) - int(c2.R))
			dg := abs(int(c1.G) - int(c2.G))
			db := abs(int(c1.B) - int(c2.B))
			if dr > maxDiff {
				maxDiff = dr
			}
			if dg > maxDiff {
				maxDiff = dg
			}
			if db > maxDiff {
				maxDiff = db
			}
		}
	}

	return maxDiff
}

// CalculateJaccardIndex calculates the Jaccard similarity between two binary edge maps.
// Returns a value between 0 (no overlap) and 1 (perfect overlap).
func CalculateJaccardIndex(edges1, edges2 *GrayImage) float64 {
	if edges1.Width() != edges2.Width() || edges1.Height() != edges2.Height() {
		return 0
	}

	width, height := edges1.Width(), edges1.Height()
	var intersection, union int

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			e1 := edges1.GrayAt(x, y).Y > 128
			e2 := edges2.GrayAt(x, y).Y > 128
			if e1 && e2 {
				intersection++
			}
			if e1 || e2 {
				union++
			}
		}
	}

	if union == 0 {
		return 1.0 // Both empty
	}
	return float64(intersection) / float64(union)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
