package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
)

// ===== Output Path Management =====

// OutputPath returns the appropriate output path for a given filename
// It automatically routes files to the outputs subdirectory based on extension
func OutputPath(filename string) string {
	// Ensure outputs directory exists
	os.MkdirAll("outputs/ans", 0755)
	os.MkdirAll("outputs/png", 0755)
	os.MkdirAll("outputs/txt", 0755)
	
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".ans":
		return filepath.Join("outputs/ans", filename)
	case ".png":
		return filepath.Join("outputs/png", filename)
	case ".txt":
		return filepath.Join("outputs/txt", filename)
	default:
		// For other files, just use current directory
		return filename
	}
}

// EnsureOutputDirs creates all output directories if they don't exist
func EnsureOutputDirs() {
	os.MkdirAll("outputs/ans", 0755)
	os.MkdirAll("outputs/png", 0755)
	os.MkdirAll("outputs/txt", 0755)
}

// ===== Color Palette Utilities =====

// Get16ColorPalette returns the standard 16 ANSI colors
// DEPRECATED: Use img2ansi.LoadPalette("ansi16") instead
// This function is kept only for legacy compatibility and should not be used in new code
func Get16ColorPalette() []color.Color {
	return []color.Color{
		color.RGBA{0, 0, 0, 255},       // 0: Black
		color.RGBA{170, 0, 0, 255},     // 1: Red  
		color.RGBA{0, 170, 0, 255},     // 2: Green
		color.RGBA{170, 85, 0, 255},    // 3: Brown/Yellow
		color.RGBA{0, 0, 170, 255},     // 4: Blue
		color.RGBA{170, 0, 170, 255},   // 5: Magenta
		color.RGBA{0, 170, 170, 255},   // 6: Cyan
		color.RGBA{170, 170, 170, 255}, // 7: White/Gray
		color.RGBA{85, 85, 85, 255},    // 8: Bright Black/Dark Gray
		color.RGBA{255, 85, 85, 255},   // 9: Bright Red
		color.RGBA{85, 255, 85, 255},   // 10: Bright Green  
		color.RGBA{255, 255, 85, 255},  // 11: Bright Yellow
		color.RGBA{85, 85, 255, 255},   // 12: Bright Blue
		color.RGBA{255, 85, 255, 255},  // 13: Bright Magenta
		color.RGBA{85, 255, 255, 255},  // 14: Bright Cyan
		color.RGBA{255, 255, 255, 255}, // 15: Bright White
	}
}

// GetAnsi16FgCode returns the ANSI foreground code for a 16-color index
func GetAnsi16FgCode(index int) string {
	if index < 8 {
		return fmt.Sprintf("%d", 30+index)
	}
	return fmt.Sprintf("%d", 90+(index-8))
}

// GetAnsi16BgCode returns the ANSI background code for a 16-color index  
func GetAnsi16BgCode(index int) string {
	if index < 8 {
		return fmt.Sprintf("%d", 40+index)
	}
	return fmt.Sprintf("%d", 100+(index-8))
}

// ===== Image Processing Utilities =====

// AtkinsonDither applies Atkinson dithering to an image
func AtkinsonDither(img image.Image) *image.Gray {
	bounds := img.Bounds()
	result := image.NewGray(bounds)
	
	// Create grayscale version
	gray := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			oldColor := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			gray.Set(x, y, oldColor)
		}
	}
	
	// Apply Atkinson dithering
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			oldPixel := gray.GrayAt(x, y).Y
			newPixel := uint8(0)
			if oldPixel > 128 {
				newPixel = 255
			}
			result.Set(x, y, color.Gray{newPixel})
			
			// Distribute error (Atkinson uses 6/8 of error)
			error := int(oldPixel) - int(newPixel)
			distributeError := error * 6 / 8
			
			// Atkinson distribution pattern
			if x+1 < bounds.Max.X {
				applyError(gray, x+1, y, distributeError/6)
			}
			if x+2 < bounds.Max.X {
				applyError(gray, x+2, y, distributeError/6)
			}
			if y+1 < bounds.Max.Y {
				if x-1 >= bounds.Min.X {
					applyError(gray, x-1, y+1, distributeError/6)
				}
				applyError(gray, x, y+1, distributeError/6)
				if x+1 < bounds.Max.X {
					applyError(gray, x+1, y+1, distributeError/6)
				}
			}
			if y+2 < bounds.Max.Y {
				applyError(gray, x, y+2, distributeError/6)
			}
		}
	}
	
	return result
}

func applyError(img *image.Gray, x, y, errorVal int) {
	oldVal := img.GrayAt(x, y).Y
	newVal := int(oldVal) + errorVal
	if newVal < 0 {
		newVal = 0
	} else if newVal > 255 {
		newVal = 255
	}
	img.Set(x, y, color.Gray{uint8(newVal)})
}

// applyThreshold applies a threshold to create binary image
func applyThreshold(img image.Image, threshold int) *image.Gray {
	bounds := img.Bounds()
	result := image.NewGray(bounds)
	
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			if gray.Y > uint8(threshold) {
				result.Set(x, y, color.White)
			} else {
				result.Set(x, y, color.Black)
			}
		}
	}
	
	return result
}

// Segment8x8 segments an image into 8x8 blocks represented as bitmaps
func Segment8x8(img image.Image) [][]GlyphBitmap {
	bounds := img.Bounds()
	height := bounds.Dy() / 8
	width := bounds.Dx() / 8
	
	blocks := make([][]GlyphBitmap, height)
	for y := 0; y < height; y++ {
		blocks[y] = make([]GlyphBitmap, width)
		for x := 0; x < width; x++ {
			var bitmap GlyphBitmap
			for py := 0; py < 8; py++ {
				for px := 0; px < 8; px++ {
					pixX := bounds.Min.X + x*8 + px
					pixY := bounds.Min.Y + y*8 + py
					
					// Convert to grayscale and check if > 128
					gray := color.GrayModel.Convert(img.At(pixX, pixY)).(color.Gray)
					if gray.Y > 128 {
						bitmap |= 1 << (py*8 + px)
					}
				}
			}
			blocks[y][x] = bitmap
		}
	}
	
	return blocks
}

