package imageutil

import (
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/tiff" // Register TIFF decoder
)

// LoadImage loads an image from the specified path.
// Supports PNG, JPEG, GIF, and TIFF formats.
func LoadImage(path string) (*RGBAImage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open image: %w", err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	return RGBAImageFromImage(img), nil
}

// SaveImage saves an image to the specified path.
// Format is determined by file extension (png, jpg/jpeg, gif).
func SaveImage(img image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return png.Encode(f, img)
	case ".jpg", ".jpeg":
		return jpeg.Encode(f, img, &jpeg.Options{Quality: 95})
	case ".gif":
		return gif.Encode(f, img, nil)
	default:
		// Default to PNG
		return png.Encode(f, img)
	}
}

// SavePNG saves an image as PNG to the specified path.
func SavePNG(img image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	return png.Encode(f, img)
}

// SaveGrayImage saves a grayscale image to the specified path.
func SaveGrayImage(img *GrayImage, path string) error {
	return SaveImage(img.Gray, path)
}
