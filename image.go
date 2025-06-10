package img2ansi

import (
	"fmt"
	"gocv.io/x/gocv"
	"image"
	"image/color"
	"image/png"
	"os"
)

// RenderOptions controls PNG rendering behavior
type RenderOptions struct {
	UseFont      bool         // Use font rendering instead of geometric
	FontBitmaps  *FontBitmaps // Pre-loaded font bitmaps (nil = use geometric)
	Scale        int          // Font scaling factor (1 = 8x8, 2 = 16x16, etc.)
	TargetWidth  int          // Target width in pixels (0 = auto)
	TargetHeight int          // Target height in pixels (0 = auto)
	ScaleFactor  float64      // Scale factor for geometric mode
}

// drawBlock draws a 2x2 block of a rune with the given foreground and
// background colors at the specified position in an image. The function
// takes a pointer to an image, the x and y coordinates of the block, and
// the block character to draw.
func drawBlock(img *gocv.Mat, x, y int, block BlockRune) {
	quad := getQuadrantsForRune(block.Rune)
	quadrants := [4]bool{
		quad.TopLeft,
		quad.TopRight,
		quad.BottomLeft,
		quad.BottomRight}

	for i := 0; i < 4; i++ {
		dx, dy := i%2, i/2
		var r, g, b uint8
		if quadrants[i] {
			r, g, b = block.FG.R, block.FG.G, block.FG.B
		} else {
			r, g, b = block.BG.R, block.BG.G, block.BG.B
		}
		img.SetUCharAt(y+dy, (x+dx)*3, b)
		img.SetUCharAt(y+dy, (x+dx)*3+1, g)
		img.SetUCharAt(y+dy, (x+dx)*3+2, r)
	}
}

// saveBlocksToPNG saves a 2D array of BlockRune structs to a PNG file.
// The function takes a 2D array of BlockRune structs and a filename as
// strings, and returns an error if the file cannot be saved.
func saveBlocksToPNG(
	blocks [][]BlockRune,
	filename string,
	targetWidth,
	targetHeight int,
	scaleFactor float64,
) error {
	blockHeight, blockWidth := len(blocks), len(blocks[0])

	var outputWidth, outputHeight int
	if targetWidth == 0 && targetHeight == 0 {
		// Unscaled mode: each block is 2x2 pixels
		outputWidth = blockWidth * 2
		outputHeight = blockHeight * 2
	} else {
		// Scaled mode
		outputWidth = targetWidth
		if outputWidth == 0 {
			outputWidth = blockWidth * 2
		}
		outputHeight = targetHeight
		if outputHeight == 0 {
			outputHeight = int(float64(blockHeight) * 2 * scaleFactor)
		}
	}

	// Create the output image
	img := gocv.NewMatWithSize(outputHeight, outputWidth, gocv.MatTypeCV8UC3)
	defer img.Close()

	scaleX := float64(outputWidth) / float64(blockWidth*2)
	scaleY := float64(outputHeight) / float64(blockHeight*2)

	for y := 0; y < outputHeight; y++ {
		for x := 0; x < outputWidth; x++ {
			blockX := int(float64(x) / scaleX / 2)
			blockY := int(float64(y) / scaleY / 2)

			if blockX >= blockWidth {
				blockX = blockWidth - 1
			}
			if blockY >= blockHeight {
				blockY = blockHeight - 1
			}

			block := blocks[blockY][blockX]
			quad := getQuadrantsForRune(block.Rune)

			quadX := int(float64(x)/scaleX) % 2
			quadY := int(float64(y)/scaleY) % 2

			var r, g, b uint8
			if isQuadrantActive(quad, quadX, quadY) {
				r, g, b = block.FG.R, block.FG.G, block.FG.B
			} else {
				r, g, b = block.BG.R, block.BG.G, block.BG.B
			}

			img.SetUCharAt(y, x*3, b)
			img.SetUCharAt(y, x*3+1, g)
			img.SetUCharAt(y, x*3+2, r)
		}
	}

	// Convert gocv.Mat to image.Image
	bounds := image.Rect(0, 0, outputWidth, outputHeight)
	rgbaImg := image.NewRGBA(bounds)
	for y := 0; y < outputHeight; y++ {
		for x := 0; x < outputWidth; x++ {
			b := img.GetUCharAt(y, x*3)
			g := img.GetUCharAt(y, x*3+1)
			r := img.GetUCharAt(y, x*3+2)
			rgbaImg.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	// Save the image
	f, createErr := os.Create(filename)
	if createErr != nil {
		return createErr
	}
	defer f.Close()

	return png.Encode(f, rgbaImg)
}

// SaveBlocksToPNGWithOptions saves blocks to PNG with rendering options
func SaveBlocksToPNGWithOptions(blocks [][]BlockRune, filename string, opts RenderOptions) error {
	// Use font rendering if requested and available
	if opts.UseFont && opts.FontBitmaps != nil {
		// Font-based rendering
		if opts.Scale < 1 {
			opts.Scale = 1
		}
		
		img := opts.FontBitmaps.RenderBlocks(blocks, opts.Scale)
		
		// Save the image
		f, err := os.Create(filename)
		if err != nil {
			return err
		}
		defer f.Close()
		
		return png.Encode(f, img)
	}
	
	// Fall back to geometric rendering
	return saveBlocksToPNG(blocks, filename, opts.TargetWidth, opts.TargetHeight, opts.ScaleFactor)
}

// isQuadrantActive returns true if the specified quadrant is active in the
// given block rune, and false otherwise.
func isQuadrantActive(quad Quadrants, x, y int) bool {
	switch {
	case x == 0 && y == 0:
		return quad.TopLeft
	case x == 1 && y == 0:
		return quad.TopRight
	case x == 0 && y == 1:
		return quad.BottomLeft
	case x == 1 && y == 1:
		return quad.BottomRight
	}
	return false
}

func drawScaledBlock(img *gocv.Mat, x, y int, block BlockRune, scale int) {
	quad := getQuadrantsForRune(block.Rune)
	quadrants := [4]bool{
		quad.TopLeft,
		quad.TopRight,
		quad.BottomLeft,
		quad.BottomRight}

	halfScale := scale / 2

	for qy := 0; qy < 2; qy++ {
		for qx := 0; qx < 2; qx++ {
			var r, g, b uint8
			if quadrants[qy*2+qx] {
				r, g, b = block.FG.R, block.FG.G, block.FG.B
			} else {
				r, g, b = block.BG.R, block.BG.G, block.BG.B
			}

			// Fill the quadrant with the color
			for dy := 0; dy < halfScale; dy++ {
				for dx := 0; dx < halfScale; dx++ {
					px := x + qx*halfScale + dx
					py := y + qy*halfScale + dy
					img.SetUCharAt(py, px*3, b)
					img.SetUCharAt(py, px*3+1, g)
					img.SetUCharAt(py, px*3+2, r)
				}
			}
		}
	}
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
//
// It uses area interpolation for downscaling to an intermediate size, detects
// edges on the intermediate image, and resizes both the intermediate image
// and the edges to the final size. It also applies a very mild sharpening to
// the resized image.
func prepareForANSI(
	img gocv.Mat,
	width,
	height int,
) (
	resized, edges gocv.Mat,
) {
	intermediate := gocv.NewMat()
	resized = gocv.NewMat()
	edges = gocv.NewMat()

	// Use area interpolation for downscaling to an intermediate size
	intermediateWidth := width * 4
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
