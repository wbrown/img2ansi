// Package imageutil provides pure Go image processing utilities
// as a replacement for gocv (OpenCV) dependencies.
package imageutil

import (
	"image"
	"image/color"
)

// RGB represents a color in the RGB color space with 8-bit channels.
type RGB struct {
	R, G, B uint8
}

// ToColor converts RGB to color.RGBA for use with standard library.
func (rgb RGB) ToColor() color.RGBA {
	return color.RGBA{R: rgb.R, G: rgb.G, B: rgb.B, A: 255}
}

// RGBFromColor converts a color.Color to RGB.
func RGBFromColor(c color.Color) RGB {
	r, g, b, _ := c.RGBA()
	return RGB{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
	}
}

// RGBAImage wraps image.RGBA with convenience methods for pixel access.
type RGBAImage struct {
	*image.RGBA
}

// NewRGBAImage creates a new RGBAImage with the specified dimensions.
func NewRGBAImage(width, height int) *RGBAImage {
	return &RGBAImage{
		RGBA: image.NewRGBA(image.Rect(0, 0, width, height)),
	}
}

// RGBAImageFromImage converts any image.Image to RGBAImage.
func RGBAImageFromImage(img image.Image) *RGBAImage {
	bounds := img.Bounds()
	rgba := NewRGBAImage(bounds.Dx(), bounds.Dy())

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			rgba.Set(x-bounds.Min.X, y-bounds.Min.Y, img.At(x, y))
		}
	}
	return rgba
}

// Width returns the image width.
func (img *RGBAImage) Width() int {
	return img.Bounds().Dx()
}

// Height returns the image height.
func (img *RGBAImage) Height() int {
	return img.Bounds().Dy()
}

// GetRGB returns the RGB value at (x, y).
func (img *RGBAImage) GetRGB(x, y int) RGB {
	c := img.RGBAAt(x, y)
	return RGB{R: c.R, G: c.G, B: c.B}
}

// SetRGB sets the RGB value at (x, y).
func (img *RGBAImage) SetRGB(x, y int, c RGB) {
	img.SetRGBA(x, y, color.RGBA{R: c.R, G: c.G, B: c.B, A: 255})
}

// Clone creates a deep copy of the image.
func (img *RGBAImage) Clone() *RGBAImage {
	clone := NewRGBAImage(img.Width(), img.Height())
	copy(clone.Pix, img.Pix)
	return clone
}

// GrayImage wraps image.Gray for single-channel images (e.g., edge maps).
type GrayImage struct {
	*image.Gray
}

// NewGrayImage creates a new GrayImage with the specified dimensions.
func NewGrayImage(width, height int) *GrayImage {
	return &GrayImage{
		Gray: image.NewGray(image.Rect(0, 0, width, height)),
	}
}

// GrayImageFromImage converts any image.Image to GrayImage.
func GrayImageFromImage(img image.Image) *GrayImage {
	bounds := img.Bounds()
	gray := NewGrayImage(bounds.Dx(), bounds.Dy())

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray.Set(x-bounds.Min.X, y-bounds.Min.Y, img.At(x, y))
		}
	}
	return gray
}

// Width returns the image width.
func (img *GrayImage) Width() int {
	return img.Bounds().Dx()
}

// Height returns the image height.
func (img *GrayImage) Height() int {
	return img.Bounds().Dy()
}

// GetGray returns the grayscale value at (x, y).
func (img *GrayImage) GetGray(x, y int) uint8 {
	return img.GrayAt(x, y).Y
}

// SetGrayValue sets the grayscale value at (x, y).
func (img *GrayImage) SetGrayValue(x, y int, v uint8) {
	img.Gray.SetGray(x, y, color.Gray{Y: v})
}

// Clone creates a deep copy of the image.
func (img *GrayImage) Clone() *GrayImage {
	clone := NewGrayImage(img.Width(), img.Height())
	copy(clone.Pix, img.Pix)
	return clone
}
