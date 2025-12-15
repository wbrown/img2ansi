package imageutil

import "math"

// Kernel represents a convolution kernel.
type Kernel struct {
	Values [][]float64
	Width  int
	Height int
}

// NewKernel creates a new kernel from a 2D slice.
func NewKernel(values [][]float64) *Kernel {
	height := len(values)
	width := 0
	if height > 0 {
		width = len(values[0])
	}
	return &Kernel{
		Values: values,
		Width:  width,
		Height: height,
	}
}

// SharpeningKernel returns the mild sharpening kernel used in img2ansi.
// This matches the kernel from image.go:229-238.
func SharpeningKernel() *Kernel {
	return NewKernel([][]float64{
		{0, -0.5, 0},
		{-0.5, 3, -0.5},
		{0, -0.5, 0},
	})
}

// GaussianKernel3x3 returns a 3x3 Gaussian blur kernel.
func GaussianKernel3x3() *Kernel {
	return NewKernel([][]float64{
		{1.0 / 16, 2.0 / 16, 1.0 / 16},
		{2.0 / 16, 4.0 / 16, 2.0 / 16},
		{1.0 / 16, 2.0 / 16, 1.0 / 16},
	})
}

// GaussianKernel5x5 returns a 5x5 Gaussian blur kernel with sigma ~1.4.
func GaussianKernel5x5() *Kernel {
	// Approximation of Gaussian with sigma = 1.4
	return NewKernel([][]float64{
		{2.0 / 159, 4.0 / 159, 5.0 / 159, 4.0 / 159, 2.0 / 159},
		{4.0 / 159, 9.0 / 159, 12.0 / 159, 9.0 / 159, 4.0 / 159},
		{5.0 / 159, 12.0 / 159, 15.0 / 159, 12.0 / 159, 5.0 / 159},
		{4.0 / 159, 9.0 / 159, 12.0 / 159, 9.0 / 159, 4.0 / 159},
		{2.0 / 159, 4.0 / 159, 5.0 / 159, 4.0 / 159, 2.0 / 159},
	})
}

// Convolve applies a convolution kernel to an RGBA image.
// Border pixels are handled by replicating edge values.
func Convolve(img *RGBAImage, kernel *Kernel) *RGBAImage {
	width, height := img.Width(), img.Height()
	dst := NewRGBAImage(width, height)

	halfKW := kernel.Width / 2
	halfKH := kernel.Height / 2

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var sumR, sumG, sumB float64

			for ky := 0; ky < kernel.Height; ky++ {
				for kx := 0; kx < kernel.Width; kx++ {
					// Source pixel coordinates with border replication
					sx := clampInt(x+kx-halfKW, 0, width-1)
					sy := clampInt(y+ky-halfKH, 0, height-1)

					c := img.RGBAAt(sx, sy)
					k := kernel.Values[ky][kx]

					sumR += float64(c.R) * k
					sumG += float64(c.G) * k
					sumB += float64(c.B) * k
				}
			}

			// Clamp to [0, 255]
			dst.SetRGB(x, y, RGB{
				R: clampUint8(sumR),
				G: clampUint8(sumG),
				B: clampUint8(sumB),
			})
		}
	}

	return dst
}

// ConvolveGray applies a convolution kernel to a grayscale image.
func ConvolveGray(img *GrayImage, kernel *Kernel) *GrayImage {
	width, height := img.Width(), img.Height()
	dst := NewGrayImage(width, height)

	halfKW := kernel.Width / 2
	halfKH := kernel.Height / 2

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var sum float64

			for ky := 0; ky < kernel.Height; ky++ {
				for kx := 0; kx < kernel.Width; kx++ {
					sx := clampInt(x+kx-halfKW, 0, width-1)
					sy := clampInt(y+ky-halfKH, 0, height-1)

					v := img.GrayAt(sx, sy).Y
					k := kernel.Values[ky][kx]

					sum += float64(v) * k
				}
			}

			dst.Gray.Pix[y*dst.Stride+x] = clampUint8(sum)
		}
	}

	return dst
}

// ConvolveGrayFloat applies a convolution kernel to a grayscale float image.
// Returns float values without clamping.
func ConvolveGrayFloat(img [][]float64, kernel *Kernel) [][]float64 {
	height := len(img)
	if height == 0 {
		return nil
	}
	width := len(img[0])

	dst := make([][]float64, height)
	for y := 0; y < height; y++ {
		dst[y] = make([]float64, width)
	}

	halfKW := kernel.Width / 2
	halfKH := kernel.Height / 2

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var sum float64

			for ky := 0; ky < kernel.Height; ky++ {
				for kx := 0; kx < kernel.Width; kx++ {
					sx := clampInt(x+kx-halfKW, 0, width-1)
					sy := clampInt(y+ky-halfKH, 0, height-1)

					sum += img[sy][sx] * kernel.Values[ky][kx]
				}
			}

			dst[y][x] = sum
		}
	}

	return dst
}

// Sharpen applies a mild sharpening filter to an RGBA image.
// This uses the same kernel as the original img2ansi implementation.
func Sharpen(img *RGBAImage) *RGBAImage {
	return Convolve(img, SharpeningKernel())
}

// GaussianBlur applies a Gaussian blur to an RGBA image.
func GaussianBlur(img *RGBAImage) *RGBAImage {
	return Convolve(img, GaussianKernel5x5())
}

// GaussianBlurGray applies a Gaussian blur to a grayscale image.
func GaussianBlurGray(img *GrayImage) *GrayImage {
	return ConvolveGray(img, GaussianKernel5x5())
}

// clampInt clamps an integer to the given range.
func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// clampUint8 clamps a float64 to [0, 255] and converts to uint8.
func clampUint8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(math.Round(v))
}
