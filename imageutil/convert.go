package imageutil

import "image/color"

// ToGrayscale converts an RGBA image to grayscale using the standard
// luminance formula: Y = 0.299*R + 0.587*G + 0.114*B
// This matches the BT.601 standard used by OpenCV's COLOR_BGR2GRAY.
func ToGrayscale(img *RGBAImage) *GrayImage {
	width, height := img.Width(), img.Height()
	gray := NewGrayImage(width, height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := img.RGBAAt(x, y)
			// Standard luminance formula (BT.601)
			// Using integer math for speed, scaled by 256
			lum := (299*int(c.R) + 587*int(c.G) + 114*int(c.B) + 500) / 1000
			if lum > 255 {
				lum = 255
			}
			gray.Gray.SetGray(x, y, color.Gray{Y: uint8(lum)})
		}
	}

	return gray
}

// ToGrayscaleFloat converts an RGBA image to grayscale, returning
// floating-point values in the range [0, 255] for higher precision.
func ToGrayscaleFloat(img *RGBAImage) [][]float64 {
	width, height := img.Width(), img.Height()
	gray := make([][]float64, height)

	for y := 0; y < height; y++ {
		gray[y] = make([]float64, width)
		for x := 0; x < width; x++ {
			c := img.RGBAAt(x, y)
			gray[y][x] = 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
		}
	}

	return gray
}

// GrayscaleToRGBA converts a grayscale image back to RGBA.
func GrayscaleToRGBA(gray *GrayImage) *RGBAImage {
	width, height := gray.Width(), gray.Height()
	rgba := NewRGBAImage(width, height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v := gray.GrayAt(x, y).Y
			rgba.SetRGB(x, y, RGB{R: v, G: v, B: v})
		}
	}

	return rgba
}
