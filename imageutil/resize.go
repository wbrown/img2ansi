package imageutil

import (
	"image"

	"golang.org/x/image/draw"
)

// Interpolation specifies the interpolation method for resizing.
type Interpolation int

const (
	// InterpolationArea uses Catmull-Rom for high-quality downscaling.
	// This is the closest equivalent to OpenCV's INTER_AREA.
	InterpolationArea Interpolation = iota

	// InterpolationLinear uses bilinear interpolation.
	// Equivalent to OpenCV's INTER_LINEAR.
	InterpolationLinear

	// InterpolationNearest uses nearest-neighbor interpolation.
	// Fastest but lowest quality.
	InterpolationNearest
)

// Resize resizes an RGBA image to the specified dimensions using the
// given interpolation method.
func Resize(img *RGBAImage, width, height int, interp Interpolation) *RGBAImage {
	dst := NewRGBAImage(width, height)
	dstRect := image.Rect(0, 0, width, height)

	var scaler draw.Scaler
	switch interp {
	case InterpolationArea:
		// CatmullRom provides high quality for both up and down scaling
		scaler = draw.CatmullRom
	case InterpolationLinear:
		scaler = draw.BiLinear
	case InterpolationNearest:
		scaler = draw.NearestNeighbor
	default:
		scaler = draw.CatmullRom
	}

	scaler.Scale(dst.RGBA, dstRect, img.RGBA, img.Bounds(), draw.Over, nil)
	return dst
}

// ResizeGray resizes a grayscale image to the specified dimensions.
func ResizeGray(img *GrayImage, width, height int, interp Interpolation) *GrayImage {
	dst := NewGrayImage(width, height)
	dstRect := image.Rect(0, 0, width, height)

	var scaler draw.Scaler
	switch interp {
	case InterpolationArea:
		scaler = draw.CatmullRom
	case InterpolationLinear:
		scaler = draw.BiLinear
	case InterpolationNearest:
		scaler = draw.NearestNeighbor
	default:
		scaler = draw.CatmullRom
	}

	scaler.Scale(dst.Gray, dstRect, img.Gray, img.Bounds(), draw.Over, nil)
	return dst
}

// ResizeToWidth resizes an image to the specified width while maintaining
// aspect ratio.
func ResizeToWidth(img *RGBAImage, width int, interp Interpolation) *RGBAImage {
	aspectRatio := float64(img.Width()) / float64(img.Height())
	height := int(float64(width) / aspectRatio)
	return Resize(img, width, height, interp)
}

// ResizeToHeight resizes an image to the specified height while maintaining
// aspect ratio.
func ResizeToHeight(img *RGBAImage, height int, interp Interpolation) *RGBAImage {
	aspectRatio := float64(img.Width()) / float64(img.Height())
	width := int(float64(height) * aspectRatio)
	return Resize(img, width, height, interp)
}
