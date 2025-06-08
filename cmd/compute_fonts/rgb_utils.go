package main

import (
	"image"
	"image/color"
	"github.com/wbrown/img2ansi"
)

// colorToRGB converts color.Color to img2ansi.RGB
func colorToRGB(c color.Color) img2ansi.RGB {
	r, g, b, _ := c.RGBA()
	return img2ansi.RGB{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
	}
}

// rgbToColor converts img2ansi.RGB to color.Color
func rgbToColor(rgb img2ansi.RGB) color.Color {
	return color.RGBA{
		R: rgb.R,
		G: rgb.G,
		B: rgb.B,
		A: 255,
	}
}

// colorsToRGBs converts a slice of color.Color to img2ansi.RGB
func colorsToRGBs(colors []color.Color) []img2ansi.RGB {
	rgbs := make([]img2ansi.RGB, len(colors))
	for i, c := range colors {
		rgbs[i] = colorToRGB(c)
	}
	return rgbs
}

// rgbsToColors converts a slice of img2ansi.RGB to color.Color
func rgbsToColors(rgbs []img2ansi.RGB) []color.Color {
	colors := make([]color.Color, len(rgbs))
	for i, rgb := range rgbs {
		colors[i] = rgbToColor(rgb)
	}
	return colors
}

// extractRGBsFromImage extracts RGB values from image pixels
func extractRGBsFromImage(img image.Image, x, y, width, height int) []img2ansi.RGB {
	pixels := make([]img2ansi.RGB, width*height)
	idx := 0
	for py := 0; py < height; py++ {
		for px := 0; px < width; px++ {
			pixels[idx] = colorToRGB(img.At(x+px, y+py))
			idx++
		}
	}
	return pixels
}

// setImagePixel sets a pixel in an RGBA image using RGB
func setImagePixel(img *image.RGBA, x, y int, rgb img2ansi.RGB) {
	img.Set(x, y, rgbToColor(rgb))
}