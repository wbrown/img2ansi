package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"os"
)

// LoadImage loads an image from file with proper error handling
func LoadImage(filename string) (image.Image, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("could not open %s: %v", filename, err)
	}
	defer file.Close()
	
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("could not decode %s: %v", filename, err)
	}
	
	return img, nil
}

// ResizeImage resizes an image to the target dimensions
func ResizeImage(src image.Image, targetWidth, targetHeight int) *image.RGBA {
	bounds := src.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()
	
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	
	xRatio := float64(srcWidth) / float64(targetWidth)
	yRatio := float64(srcHeight) / float64(targetHeight)
	
	for y := 0; y < targetHeight; y++ {
		for x := 0; x < targetWidth; x++ {
			srcX := int(float64(x) * xRatio)
			srcY := int(float64(y) * yRatio)
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	
	return dst
}

// ResizeForTerminalAspect resizes an image accounting for terminal character aspect ratio
func ResizeForTerminalAspect(src image.Image, charWidth, charHeight int, aspectRatio float64) *image.RGBA {
	// First resize to square pixels
	pixelWidth := charWidth * 8
	pixelHeight := int(float64(charHeight*8) * aspectRatio)
	
	// Resize to target dimensions
	return ResizeImage(src, pixelWidth, pixelHeight)
}

// CopyImage creates a deep copy of an image
func CopyImage(src *image.RGBA) *image.RGBA {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)
	return dst
}

// Extract8x8Block extracts an 8x8 pixel block from an image
func Extract8x8Block(img image.Image, blockX, blockY int) *image.RGBA {
	block := image.NewRGBA(image.Rect(0, 0, 8, 8))
	
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			px := blockX*8 + x
			py := blockY*8 + y
			
			// Check bounds
			if px < img.Bounds().Min.X || px >= img.Bounds().Max.X ||
			   py < img.Bounds().Min.Y || py >= img.Bounds().Max.Y {
				block.Set(x, y, color.Black)
			} else {
				block.Set(x, y, img.At(px, py))
			}
		}
	}
	
	return block
}

// TileImage creates a tiled version of an image
func TileImage(src image.Image, tilesX, tilesY int) *image.RGBA {
	bounds := src.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()
	
	dst := image.NewRGBA(image.Rect(0, 0, srcWidth*tilesX, srcHeight*tilesY))
	
	for ty := 0; ty < tilesY; ty++ {
		for tx := 0; tx < tilesX; tx++ {
			dstRect := image.Rect(tx*srcWidth, ty*srcHeight, (tx+1)*srcWidth, (ty+1)*srcHeight)
			draw.Draw(dst, dstRect, src, bounds.Min, draw.Src)
		}
	}
	
	return dst
}

// ImageDimensions returns the dimensions of an image file without fully decoding
func ImageDimensions(filename string) (int, int, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, err
	}
	
	return config.Width, config.Height, nil
}

// CompareImages compares two images and returns the difference
func CompareImages(img1, img2 image.Image) (*image.RGBA, float64) {
	bounds1 := img1.Bounds()
	bounds2 := img2.Bounds()
	
	// Use the smaller bounds
	minWidth := bounds1.Dx()
	if bounds2.Dx() < minWidth {
		minWidth = bounds2.Dx()
	}
	minHeight := bounds1.Dy()
	if bounds2.Dy() < minHeight {
		minHeight = bounds2.Dy()
	}
	
	diff := image.NewRGBA(image.Rect(0, 0, minWidth, minHeight))
	totalError := 0.0
	
	for y := 0; y < minHeight; y++ {
		for x := 0; x < minWidth; x++ {
			c1 := img1.At(x+bounds1.Min.X, y+bounds1.Min.Y)
			c2 := img2.At(x+bounds2.Min.X, y+bounds2.Min.Y)
			
			r1, g1, b1, _ := c1.RGBA()
			r2, g2, b2, _ := c2.RGBA()
			
			dr := int(r1>>8) - int(r2>>8)
			dg := int(g1>>8) - int(g2>>8)
			db := int(b1>>8) - int(b2>>8)
			
			if dr < 0 { dr = -dr }
			if dg < 0 { dg = -dg }
			if db < 0 { db = -db }
			
			// Highlight differences in red
			diff.Set(x, y, color.RGBA{
				R: uint8(dr),
				G: uint8(dg),
				B: uint8(db),
				A: 255,
			})
			
			totalError += float64(dr + dg + db)
		}
	}
	
	avgError := totalError / float64(minWidth*minHeight*3)
	return diff, avgError
}