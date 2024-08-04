package main

import (
	"fmt"
	"gocv.io/x/gocv"
	"image"
	"image/color"
	"image/png"
	"os"
)

// drawBlock draws a 2x2 block of a rune with the given foreground and
// background colors at the specified position in an image. The function
// takes a pointer to an image, the x and y coordinates of the block, and
// the block character to draw.
func drawBlock(img *image.RGBA, x, y int, block BlockRune) {
	quad := getQuadrantsForRune(block.Rune)
	quadrants := [4]bool{
		quad.TopLeft,
		quad.TopRight,
		quad.BottomLeft,
		quad.BottomRight}

	for i := 0; i < 4; i++ {
		dx, dy := i%2, i/2
		var c color.RGBA
		if quadrants[i] {
			c = color.RGBA{
				R: block.FG.r,
				G: block.FG.g,
				B: block.FG.b,
				A: 255}
		} else {
			c = color.RGBA{
				block.BG.r,
				block.BG.g,
				block.BG.b,
				255}
		}
		img.Set(x+dx, y+dy, c)
	}
}

// saveBlocksToPNG saves a 2D array of BlockRune structs to a PNG file.
// The function takes a 2D array of BlockRune structs and a filename as
// strings, and returns an error if the file cannot be saved.
func saveBlocksToPNG(blocks [][]BlockRune, filename string) error {
	height, width := len(blocks)*2, len(blocks[0])*2
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	for y, row := range blocks {
		for x, block := range row {
			drawBlock(img, x*2, y*2, block)
		}
	}

	f, createErr := os.Create(filename)
	if createErr != nil {
		return createErr
	}
	defer func(f *os.File) {
		closeErr := f.Close()
		if closeErr != nil {
			println("Error closing block PNG file")
		}
	}(f)

	return png.Encode(f, img)
}

// saveToPNG saves an image to a PNG file. The function takes an image as
// a gocv.Mat and a filename as a string, and returns an error if the image
// cannot be saved.
func saveToPNG(img gocv.Mat, filename string) error {
	success := gocv.IMWrite(filename, img)
	if !success {
		return fmt.Errorf("failed to write image to file: %s", filename)
	}
	return nil
}

// prepareForANSI prepares an image for conversion to ANSI art. The function
// takes an input image, the target width and height for the output image, and
// returns the resized image and the edges detected in the image.
func prepareForANSI(img gocv.Mat, width, height int) (resized, edges gocv.Mat) {
	intermediate := gocv.NewMat()
	resized = gocv.NewMat()
	edges = gocv.NewMat()

	// Use area interpolation for downscaling to an intermediate size
	intermediateWidth := width * 4 // or another multiplier that gives results
	intermediateHeight := height * 4
	gocv.Resize(img,
		&intermediate,
		image.Point{X: intermediateWidth,
			Y: intermediateHeight},
		0, 0,
		gocv.InterpolationArea)

	// Detect edges on the intermediate image
	gray := gocv.NewMat()
	gocv.CvtColor(intermediate, &gray, gocv.ColorBGRToGray)
	gocv.Canny(gray, &edges, 50, 150) // Adjust thresholds as needed

	// Resize both the intermediate image and the edges to the final size
	resizedPoint := image.Point{X: width * 2, Y: height * 2}
	gocv.Resize(intermediate, &resized, resizedPoint, 0, 0,
		gocv.InterpolationArea)
	gocv.Resize(edges, &edges, resizedPoint, 0, 0,
		gocv.InterpolationLinear)

	// 4. Apply a very mild sharpening to the resized image
	kernel := gocv.NewMatWithSize(3, 3, gocv.MatTypeCV32F)
	kernel.SetFloatAt(0, 0, 0)
	kernel.SetFloatAt(0, 1, -0.5)
	kernel.SetFloatAt(0, 2, 0)
	kernel.SetFloatAt(1, 0, -0.5)
	kernel.SetFloatAt(1, 1, 3)
	kernel.SetFloatAt(1, 2, -0.5)
	kernel.SetFloatAt(2, 0, 0)
	kernel.SetFloatAt(2, 1, -0.5)
	kernel.SetFloatAt(2, 2, 0)

	sharpened := gocv.NewMat()
	gocv.Filter2D(resized,
		&sharpened, -1, kernel, image.Point{-1, -1},
		0, gocv.BorderDefault)
	resized = sharpened
	return resized, edges
}
