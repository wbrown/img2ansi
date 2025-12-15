package imageutil

import "math"

// Canny performs Canny edge detection on a grayscale image.
// lowThreshold and highThreshold control edge sensitivity.
// Typical values: lowThreshold=50, highThreshold=150.
func Canny(gray *GrayImage, lowThreshold, highThreshold float64) *GrayImage {
	width, height := gray.Width(), gray.Height()

	// Step 1: Gaussian blur to reduce noise
	blurred := GaussianBlurGray(gray)

	// Step 2: Compute Sobel gradients
	gx, gy := sobelGradients(blurred)

	// Step 3: Compute magnitude and direction
	magnitude := make([][]float64, height)
	direction := make([][]float64, height)
	for y := 0; y < height; y++ {
		magnitude[y] = make([]float64, width)
		direction[y] = make([]float64, width)
		for x := 0; x < width; x++ {
			magnitude[y][x] = math.Sqrt(gx[y][x]*gx[y][x] + gy[y][x]*gy[y][x])
			direction[y][x] = math.Atan2(gy[y][x], gx[y][x])
		}
	}

	// Step 4: Non-maximum suppression
	suppressed := nonMaxSuppression(magnitude, direction, width, height)

	// Step 5: Double threshold
	strong, weak := doubleThreshold(suppressed, lowThreshold, highThreshold, width, height)

	// Step 6: Edge tracking by hysteresis
	edges := hysteresis(strong, weak, width, height)

	return edges
}

// CannyDefault performs Canny edge detection with default thresholds (50, 150).
// These match the values used in img2ansi.
func CannyDefault(gray *GrayImage) *GrayImage {
	return Canny(gray, 50, 150)
}

// sobelGradients computes horizontal and vertical Sobel gradients.
func sobelGradients(img *GrayImage) (gx, gy [][]float64) {
	width, height := img.Width(), img.Height()

	// Sobel kernels
	sobelX := NewKernel([][]float64{
		{-1, 0, 1},
		{-2, 0, 2},
		{-1, 0, 1},
	})
	sobelY := NewKernel([][]float64{
		{-1, -2, -1},
		{0, 0, 0},
		{1, 2, 1},
	})

	// Convert to float for processing
	gray := make([][]float64, height)
	for y := 0; y < height; y++ {
		gray[y] = make([]float64, width)
		for x := 0; x < width; x++ {
			gray[y][x] = float64(img.GrayAt(x, y).Y)
		}
	}

	gx = ConvolveGrayFloat(gray, sobelX)
	gy = ConvolveGrayFloat(gray, sobelY)

	return gx, gy
}

// nonMaxSuppression performs non-maximum suppression on edge magnitudes.
// Only keeps pixels that are local maxima along the gradient direction.
func nonMaxSuppression(magnitude, direction [][]float64, width, height int) [][]float64 {
	suppressed := make([][]float64, height)
	for y := 0; y < height; y++ {
		suppressed[y] = make([]float64, width)
	}

	for y := 1; y < height-1; y++ {
		for x := 1; x < width-1; x++ {
			angle := direction[y][x]
			mag := magnitude[y][x]

			// Quantize angle to 4 directions: 0, 45, 90, 135 degrees
			// Normalize angle to [0, 180)
			angle = angle * 180.0 / math.Pi
			if angle < 0 {
				angle += 180
			}

			var q, r float64

			// Direction 0 (horizontal)
			if (angle >= 0 && angle < 22.5) || (angle >= 157.5 && angle <= 180) {
				q = magnitude[y][x+1]
				r = magnitude[y][x-1]
			} else if angle >= 22.5 && angle < 67.5 {
				// Direction 45 (diagonal)
				q = magnitude[y+1][x+1]
				r = magnitude[y-1][x-1]
			} else if angle >= 67.5 && angle < 112.5 {
				// Direction 90 (vertical)
				q = magnitude[y+1][x]
				r = magnitude[y-1][x]
			} else {
				// Direction 135 (other diagonal)
				q = magnitude[y+1][x-1]
				r = magnitude[y-1][x+1]
			}

			// Keep pixel if it's a local maximum
			if mag >= q && mag >= r {
				suppressed[y][x] = mag
			}
		}
	}

	return suppressed
}

// doubleThreshold classifies edges as strong or weak based on thresholds.
func doubleThreshold(suppressed [][]float64, low, high float64, width, height int) (strong, weak [][]bool) {
	strong = make([][]bool, height)
	weak = make([][]bool, height)

	for y := 0; y < height; y++ {
		strong[y] = make([]bool, width)
		weak[y] = make([]bool, width)
		for x := 0; x < width; x++ {
			val := suppressed[y][x]
			if val >= high {
				strong[y][x] = true
			} else if val >= low {
				weak[y][x] = true
			}
		}
	}

	return strong, weak
}

// hysteresis performs edge tracking by hysteresis.
// Weak edges are kept only if they are connected to strong edges.
func hysteresis(strong, weak [][]bool, width, height int) *GrayImage {
	edges := NewGrayImage(width, height)

	// First pass: mark all strong edges
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if strong[y][x] {
				edges.Gray.Pix[y*edges.Stride+x] = 255
			}
		}
	}

	// Second pass: connect weak edges to strong edges using flood fill
	// We iterate multiple times until no more changes occur
	changed := true
	for changed {
		changed = false
		for y := 1; y < height-1; y++ {
			for x := 1; x < width-1; x++ {
				if weak[y][x] && edges.Gray.Pix[y*edges.Stride+x] == 0 {
					// Check if any 8-connected neighbor is an edge
					hasStrongNeighbor := false
					for dy := -1; dy <= 1; dy++ {
						for dx := -1; dx <= 1; dx++ {
							if edges.Gray.Pix[(y+dy)*edges.Stride+(x+dx)] == 255 {
								hasStrongNeighbor = true
								break
							}
						}
						if hasStrongNeighbor {
							break
						}
					}
					if hasStrongNeighbor {
						edges.Gray.Pix[y*edges.Stride+x] = 255
						changed = true
					}
				}
			}
		}
	}

	return edges
}

// SobelX computes the horizontal Sobel gradient.
func SobelX(gray *GrayImage) *GrayImage {
	sobelX := NewKernel([][]float64{
		{-1, 0, 1},
		{-2, 0, 2},
		{-1, 0, 1},
	})
	return ConvolveGray(gray, sobelX)
}

// SobelY computes the vertical Sobel gradient.
func SobelY(gray *GrayImage) *GrayImage {
	sobelY := NewKernel([][]float64{
		{-1, -2, -1},
		{0, 0, 0},
		{1, 2, 1},
	})
	return ConvolveGray(gray, sobelY)
}

// SobelMagnitude computes the gradient magnitude from Sobel operators.
func SobelMagnitude(gray *GrayImage) *GrayImage {
	width, height := gray.Width(), gray.Height()
	result := NewGrayImage(width, height)

	// Convert to float for processing
	grayFloat := make([][]float64, height)
	for y := 0; y < height; y++ {
		grayFloat[y] = make([]float64, width)
		for x := 0; x < width; x++ {
			grayFloat[y][x] = float64(gray.GrayAt(x, y).Y)
		}
	}

	sobelX := NewKernel([][]float64{
		{-1, 0, 1},
		{-2, 0, 2},
		{-1, 0, 1},
	})
	sobelY := NewKernel([][]float64{
		{-1, -2, -1},
		{0, 0, 0},
		{1, 2, 1},
	})

	gx := ConvolveGrayFloat(grayFloat, sobelX)
	gy := ConvolveGrayFloat(grayFloat, sobelY)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			mag := math.Sqrt(gx[y][x]*gx[y][x] + gy[y][x]*gy[y][x])
			result.Gray.Pix[y*result.Stride+x] = clampUint8(mag)
		}
	}

	return result
}
