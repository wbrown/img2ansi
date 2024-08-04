package main

import (
	"flag"
	"fmt"
	"gocv.io/x/gocv"
	"image"
	_ "image/png"
	"math"
	"os"
	"slices"
	"strings"
)

const (
	ESC = "\u001b"
)

var (
	TargetWidth  = 100
	ScaleFactor  = 3.0
	EdgeDistance = uint32(1250)
	MaxChars     = 1048576
	Quantization = 1
	fgAnsi       = map[uint32]string{
		//	// Original colors (more likely to be selected)
		0x000000: "30", // BLACK
		0xEB5156: "31", // RED
		0x69953D: "32", // GREEN
		0xA28B2F: "33", // YELLOW
		0x5291CF: "34", // BLUE
		0x9F73BA: "35", // MAGENTA
		0x48A0A2: "36", // CYAN
		0x808080: "37", // WHITE
		// Additional colors (less likely to be selected)
		0x4D4D4D: "90", // BRIGHT BLACK (dark gray)
		0xEF5357: "91", // BRIGHT RED (darker)
		0x70C13E: "92", // BRIGHT GREEN (darker)
		0xE3C23C: "93", // BRIGHT YELLOW (darker)
		0x54AFF9: "94", // BRIGHT BLUE (darker)
		0xDF84E7: "95", // BRIGHT MAGENTA (darker)
		0x67E0E1: "96", // BRIGHT CYAN (darker)
		0xC0C0C0: "97", // BRIGHT WHITE (light gray)
	}
	// Below are measured from the terminal using a color picker
	ansiOverrides = map[uint32]string{
		0x000000: "30", // BLACK
		0xB93018: "31", // RED
		0x52BF37: "32", // GREEN
		0xFFFC7F: "33", // YELLOW
		0x0D23BF: "34", // BLUE
		0xBA3FC0: "35", // MAGENTA
		0x53C2C5: "36", // CYAN
		0xC8C7C7: "37", // WHITE
		0x686868: "90", // BRIGHT BLACK (dark gray)
		0xF1776D: "91", // BRIGHT RED (darker)
		0x8DF67A: "92", // BRIGHT GREEN (darker)
		0xFEFC7F: "93", // BRIGHT YELLOW (darker)
		0x6A71F6: "94", // BRIGHT BLUE (darker)
		0xF07FF8: "95", // BRIGHT MAGENTA (darker)
		0x8EFAFD: "96", // BRIGHT CYAN (darker)
		0xFFFFFF: "97", // BRIGHT WHITE (light gray)
	}

	// Original colors
	//0x000000: "30", // BLACK
	//0xF0524F: "31", // RED
	//0x5C962C: "32", // GREEN
	//0xA68A0D: "33", // YELLOW
	//0x3993D4: "34", // BLUE
	//0xA771BF: "35", // MAGENTA
	//0x00A3A3: "36", // CYAN
	//0x808080: "37", // WHITE

	// Bright colors
	//0x575959: "90", // BRIGHT BLACK (dark gray)
	//0xFF4050: "91", // BRIGHT RED (darker)
	//0x4FC414: "92", // BRIGHT GREEN (darker)
	//0xE5BF00: "93", // BRIGHT YELLOW (darker)
	//0x1FB0FF: "94", // BRIGHT BLUE (darker)
	//0xED7EED: "95", // BRIGHT MAGENTA (darker)
	//0x00E5E5: "96", // BRIGHT CYAN (darker)
	//0xFFFFFF: "97", // BRIGHT WHITE (light gray)

	bgAnsi = make(map[uint32]string)

	blocks = []rune{
		' ', // 0000 - Empty space
		'▘', // 0001 - Quadrant upper left
		'▝', // 0010 - Quadrant upper right
		'▀', // 0011 - Upper half block
		'▖', // 0100 - Quadrant lower left
		'▌', // 0101 - Left half block
		'▞', // 0110 - Quadrant diagonal upper right and lower left
		'▛', // 0111 - Three quadrants: upper left, upper right, lower left
		'▗', // 1000 - Quadrant lower right
		'▚', // 1001 - Quadrant diagonal upper left and lower right
		'▐', // 1010 - Right half block
		'▜', // 1011 - Three quadrants: upper left, upper right, lower right
		'▄', // 1100 - Lower half block
		'▙', // 1101 - Three quadrants: upper left, lower left, lower right
		'▟', // 1110 - Three quadrants: upper right, lower left, lower right
		'█', // 1111 - Full block
	}
)

func init() {
	newAnsi := fgAnsi
	for color, code := range ansiOverrides {
		for origColor, origCode := range fgAnsi {
			if origCode == code {
				delete(newAnsi, origColor)
				newAnsi[color] = code
			}
		}
	}
	fgAnsi = newAnsi
	// If bgaAnsi is empty, populate it
	if len(bgAnsi) == 0 {
		for color, code := range fgAnsi {
			if code[0] == '3' {
				bgAnsi[color] = "4" + code[1:]
			} else if code[0] == '9' {
				bgAnsi[color] = "10" + code[1:]
			}
		}
	}
}

// RGB represents a color in the RGB color space with 8-bit channels,
// where each channel ranges from 0 to 255. The RGB color space is
// additive, meaning that colors are created by adding together the
// red, green, and blue channels.

type RGB struct {
	r, g, b uint8
}

// rgbFromANSI converts an ANSI color code to an RGB color, using the
// provided color dictionaries. The function returns the RGB color
// corresponding to the ANSI color code. It does not handle the case
// where the ANSI code is not found in the dictionary.
func rgbFromANSI(ansiCode string) RGB {
	var table map[uint32]string
	if colorIsForeground(ansiCode) {
		table = fgAnsi
	} else {
		table = bgAnsi
	}
	for color, code := range table {
		if code == ansiCode {
			return rgbFromUint32(color)
		}
	}
	return RGB{}
}

// toUint32 converts an RGB color to a 32-bit unsigned integer
func (rgb RGB) toUint32() uint32 {
	return uint32(rgb.r)<<16 | uint32(rgb.g)<<8 | uint32(rgb.b)
}

// rgbFromVecb converts a gocv.Vecb to an RGB color
func rgbFromVecb(color gocv.Vecb) RGB {
	return RGB{
		r: color[2],
		g: color[1],
		b: color[0],
	}
}

// rgbFromUint32 converts a 32-bit unsigned integer to an RGB color
func rgbFromUint32(color uint32) RGB {
	return RGB{
		r: uint8(color >> 16),
		g: uint8(color >> 8),
		b: uint8(color),
	}
}

// rgbToANSI converts an RGB color to the closest ANSI color code based on
// the provided color dictionary. If fg is true, the function will use the
// foreground color dictionary; otherwise, it will use the background color
// dictionary. The function returns the RGB color, and the closest ANSI
// color code.
func (rgb RGB) rgbToANSI(fg bool) (bestColor RGB, bestCode string) {
	colorDict := fgAnsi
	if !fg {
		colorDict = bgAnsi
	}

	minDiff := uint32(math.MaxUint32)

	for k, v := range colorDict {
		diff := rgb.colorDistance(rgbFromUint32(k))
		if diff < minDiff {
			minDiff = diff
			bestCode = v
			bestColor = rgbFromUint32(k)
		}
	}

	return bestColor, bestCode
}

// colorDistance calculates the Euclidean distance between two RGB colors
// in the RGB color space. The function returns the squared distance between
// the two colors as a 32-bit unsigned integer.
func (rgb RGB) colorDistance(c2 RGB) uint32 {
	return uint32(math.Pow(float64(rgb.r)-float64(c2.r), 2) +
		math.Pow(float64(rgb.g)-float64(c2.g), 2) +
		math.Pow(float64(rgb.b)-float64(c2.b), 2))
}

// dithError calculates the error between two RGB colors in the RGB color
// space. It returns a 3-element array of floating-point numbers representing
// the error in the red, green, and blue channels, respectively.
func (rgb RGB) dithError(c2 RGB) [3]float64 {
	return [3]float64{
		float64(rgb.r) - float64(c2.r),
		float64(rgb.g) - float64(c2.g),
		float64(rgb.b) - float64(c2.b),
	}
}

// detectEdges applies edge detection to an image using the Canny edge
// detection algorithm. The function returns a binary image with edges
// detected.
//func detectEdges(img gocv.Mat) gocv.Mat {
//	gray := gocv.NewMat()
//	edges := gocv.NewMat()
//
//	gocv.CvtColor(img, &gray, gocv.ColorRGBToGray)
//	gocv.GaussianBlur(gray, &gray, image.Pt(3, 3), 0, 0, gocv.BorderDefault)
//	gocv.Canny(gray, &edges, 75, 200)
//
//	// Optional: Apply a very subtle dilation if needed
//	kernel := gocv.GetStructuringElement(gocv.MorphRect, image.Pt(2, 2))
//	gocv.Dilate(edges, &edges, kernel)
//
//	return edges
//}

func detectEdges(img gocv.Mat) gocv.Mat {
	gray := gocv.NewMat()
	edges := gocv.NewMat()
	inverted := gocv.NewMat()

	gocv.CvtColor(img, &gray, gocv.ColorRGBToGray)
	gocv.AdaptiveThreshold(gray, &edges,
		255,
		gocv.AdaptiveThresholdGaussian,
		gocv.ThresholdBinary, 9, 2)
	gocv.BitwiseNot(edges, &inverted)

	return inverted
}

func thinEdges(edges *gocv.Mat) {
	kernel := gocv.GetStructuringElement(gocv.MorphCross, image.Pt(3, 3))
	gocv.Erode(*edges, edges, kernel)
}

// quantizeColor quantizes an RGB color by rounding each channel to the
// nearest multiple of the quantization factor. The function returns the
// quantized RGB color.
func (rgb RGB) quantizeColor() RGB {
	qFactor := 256 / float64(Quantization)
	return RGB{
		uint8(math.Round(float64(rgb.r)/qFactor) * qFactor),
		uint8(math.Round(float64(rgb.g)/qFactor) * qFactor),
		uint8(math.Round(float64(rgb.b)/qFactor) * qFactor),
	}
}

// modifiedAtkinsonDither applies the modified Atkinson dithering algorithm
// to an image. The function takes an input image and a binary image with
// edges detected. It returns a new image with the Atkinson dithering
// algorithm applied, with colors quantized to the nearest ANSI color.
func modifiedAtkinsonDither(img gocv.Mat, edges gocv.Mat) gocv.Mat {
	height, width := img.Rows(), img.Cols()
	newImage := gocv.NewMatWithSize(height, width, gocv.MatTypeCV8UC3)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			oldPixel := rgbFromVecb(img.GetVecbAt(y, x))
			quantizedPixel := oldPixel.quantizeColor()
			newColor, _ := quantizedPixel.rgbToANSI(true)
			// Store full color information
			newImage.SetUCharAt(y, x*3+2, newColor.r)
			newImage.SetUCharAt(y, x*3+1, newColor.g)
			newImage.SetUCharAt(y, x*3, newColor.b)

			edgeStrength := float64(edges.GetUCharAt(y, x)) / 255.0
			colorDiff := quantizedPixel.colorDistance(newColor)

			// Adjust dithering based on edge strength
			if edgeStrength > 0.5 || colorDiff < EdgeDistance {
				continue
			}

			dithError := oldPixel.dithError(newColor)

			// Scale error diffusion based on edge strength
			errorScale := 1.0 - edgeStrength
			for i := 0; i < 3; i++ {
				dithError[i] *= errorScale
			}

			// Divide the error by 8 and distribute it to neighboring pixels
			for i := 0; i < 3; i++ {
				dithError[i] /= 8
			}

			diffuseError := func(y, x int, factor float64) {
				if y >= 0 && y < height && x >= 0 && x < width {
					pixel := img.GetVecbAt(y, x)
					for i := 0; i < 3; i++ {
						chanDithErr := float64(pixel[2-i]) +
							dithError[i]*factor
						newVal := uint8(math.Max(0,
							math.Min(255, chanDithErr)))
						img.SetUCharAt(y, x*3+(2-i), newVal)
					}
				}
			}

			// Adjust error diffusion pattern based on edge strength
			diffuseError(y, x+1, 1-edgeStrength*0.5)
			diffuseError(y, x+2, 1-edgeStrength*0.7)
			diffuseError(y+1, x, 1-edgeStrength*0.5)
			diffuseError(y+1, x+1, 1-edgeStrength*0.7)
			diffuseError(y+1, x-1, 1-edgeStrength*0.7)
			diffuseError(y+2, x, 1-edgeStrength*0.9)
		}
	}

	return newImage
}

// getANSICode retrieves the ANSI color code for a pixel in an image. The
// function takes an image and the y and x coordinates of the pixel and
// returns the ANSI color code as a string. This assumes that the image
// only has ANSI color pixels.
func getANSICode(img gocv.Mat, y, x int) string {
	rgb := RGB{img.GetUCharAt(y, x*3+2),
		img.GetUCharAt(y, x*3+1),
		img.GetUCharAt(y, x*3)}
	_, ansiCode := rgb.rgbToANSI(true)
	return ansiCode
}

func compressANSI(ansiImage string) string {
	var compressed strings.Builder
	var currentFg, currentBg, currentBlock string
	var count int

	lines := strings.Split(ansiImage, "\n")
	for _, line := range lines {
		segments := strings.Split(line, "\u001b[")
		for _, segment := range segments {
			if segment == "" {
				continue
			}
			parts := strings.SplitN(segment, "m", 2)
			if len(parts) != 2 {
				continue
			}
			colorCode, block := parts[0], parts[1]
			fg, bg := extractColors(colorCode)

			if fg != currentFg || bg != currentBg || block != currentBlock {
				if count > 0 {
					compressed.WriteString(
						formatANSICode(currentFg, currentBg, currentBlock, count))
				}
				currentFg, currentBg, currentBlock = fg, bg, block
				count = 1
			} else {
				count++
			}
		}
		if count > 0 {
			compressed.WriteString(
				formatANSICode(currentFg, currentBg, currentBlock, count))
		}
		compressed.WriteString(fmt.Sprintf("%s[0m\n", ESC))
		count = 0
		currentFg, currentBg = "", "" // Reset colors at end of line
	}

	return compressed.String()
}

func formatANSICode(fg, bg, block string, count int) string {
	var code strings.Builder
	code.WriteString(ESC)
	code.WriteByte('[')
	if fg != "" {
		code.WriteString(fg)
		if bg != "" {
			code.WriteByte(';')
		}
	}
	if bg != "" {
		code.WriteString(bg)
	}
	code.WriteByte('m')
	code.WriteString(strings.Repeat(block, count))
	return code.String()
}

// extractColors extracts the foreground and background color codes from
// an ANSI color code. The function takes an ANSI color code as a string
// and returns the foreground and background color codes as strings.
func extractColors(colorCode string) (fg string, bg string) {
	colors := strings.Split(colorCode, ";")
	for _, color := range colors {
		if colorIsForeground(color) {
			fg = color
		} else if colorIsBackground(color) {
			bg = color
		}
	}
	return fg, bg
}

func saveToPNG(img gocv.Mat, filename string) error {
	success := gocv.IMWrite(filename, img)
	if !success {
		return fmt.Errorf("failed to write image to file: %s", filename)
	}
	return nil
}

// getBlock returns the Unicode block character corresponding to the
// specified configuration of filled quadrants. The function takes four
// boolean values representing the filled quadrants in the following order:
// upper left, upper right, lower left, lower right. It returns the Unicode
// block character corresponding to the filled quadrants.
func getBlock(upperLeft, upperRight, lowerLeft, lowerRight bool) rune {
	index := 0
	if upperLeft {
		index |= 1
	}
	if upperRight {
		index |= 2
	}
	if lowerLeft {
		index |= 4
	}
	if lowerRight {
		index |= 8
	}
	return blocks[index]
}

// colorIsForeground returns true if the ANSI color code corresponds to a
// foreground color, and false otherwise. The function takes an ANSI color
// code as a string and returns true if the color is a foreground color, and
// false if it is a background color.
func colorIsForeground(color string) bool {
	return strings.HasPrefix(color, "3") ||
		strings.HasPrefix(color, "9")
}

// colorIsBackground returns true if the ANSI color code corresponds to a
// background color, and false otherwise. The function takes an ANSI color
// code as a string and returns true if the color is a background color, and
// false if it is a foreground color.
func colorIsBackground(color string) bool {
	return strings.HasPrefix(color, "4") ||
		strings.HasPrefix(color, "10")
}

// resolveColorPair resolves a pair of colors to ensure that the foreground
// and background colors are correctly assigned. The function takes two
// ANSI color codes as strings, and returns the colors mapped to foreground
// and background codes, respectively.
func resolveColorPair(first, second string) (fg string, bg string) {
	fg = first
	bg = second
	firstColorIsFg := colorIsForeground(fg)
	secondColorIsFg := colorIsForeground(bg)
	if firstColorIsFg == secondColorIsFg {
		bgOrFg := !(firstColorIsFg && secondColorIsFg)
		bgColorCandidate := rgbFromANSI(second)
		_, bg = bgColorCandidate.rgbToANSI(bgOrFg)
	}
	if colorIsForeground(fg) && colorIsBackground(bg) {
		return fg, bg
	} else if colorIsForeground(bg) && colorIsBackground(fg) {
		return bg, fg
	} else if colorIsForeground(fg) {
		return fg, bg
	} else if colorIsForeground(second) {
		return fg, bg
	}
	return fg, bg
}

// simpleResolveBlock resolves a block of colors to a single foreground
// and background color, and a corresponding Unicode block character. The
// function takes a slice of four ANSI color codes as strings, and returns
// the foreground color, background color, and Unicode block character
// corresponding to the block of colors.
//
// It does not handle the case where the block of colors has more than two
// distinct colors.
func simpleResolveBlock(colors []string) (fgColor, bgColor string, ansiBlock rune) {
	// Handle the two-color case (or single color)
	dominantColors := getMostFrequentColors(colors)
	fgColor = dominantColors[0]
	var block rune

	if len(dominantColors) > 1 {
		bgCandidate := rgbFromANSI(dominantColors[1])
		bgColor = bgAnsi[bgCandidate.toUint32()]
		block = getBlock(
			colors[0] == fgColor,
			colors[1] == fgColor,
			colors[2] == fgColor,
			colors[3] == fgColor,
		)
	} else {
		bgColor = "40" // Default to black background for single color
		block = '█'    // Full block
	}
	return fgColor, bgColor, block
}

// renderAnsi renders an image to ANSI art using the provided image. The
// function takes an image as a gocv.Mat and returns the rendered ANSI art
// as a string.
//
// The function renders the image by iterating over the pixels in the image
// and converting each pixel to an ANSI color code. It then merges the
// colors in each block of four pixels to determine the foreground and
// background colors, and the corresponding Unicode block character. The
// function then constructs the ANSI escape sequences to render the image
// in the terminal.
func renderAnsi(img gocv.Mat) (ansiImage string) {
	imgHeight, imgWidth := img.Rows(), img.Cols()

	for y := 0; y < imgHeight; y += 2 {
		for x := 0; x < imgWidth; x += 2 {
			colors := []string{
				getANSICode(img, y, x),
				getANSICode(img, y, x+1),
				getANSICode(img, y+1, x),
				getANSICode(img, y+1, x+1),
			}

			uniqueColors := make(map[string]int)
			for _, c := range colors {
				uniqueColors[c]++
			}

			var fgColor, bgColor string
			var ansiStr string
			var ansiBlock rune
			if len(uniqueColors) > 2 {
				// Handle the three-or-more-color case
				newColors := mergeBlockColors(colors)
				fgColor, bgColor, ansiBlock = simpleResolveBlock(newColors)
				ansiStr = string(ansiBlock)
			} else {
				fgColor, bgColor, ansiBlock = simpleResolveBlock(colors)
				ansiStr = string(ansiBlock)
			}
			ansiImage += fmt.Sprintf("%s[%s;%sm%s",
				ESC, fgColor, bgColor, ansiStr)
		}

		ansiImage += fmt.Sprintf("%s[0m\n", ESC)
	}
	return ansiImage
}

func prepareForANSI(img gocv.Mat, width, height int) (resized, edges gocv.Mat) {
	intermediate := gocv.NewMat()
	resized = gocv.NewMat()
	edges = gocv.NewMat()

	// 1. Use area interpolation for downscaling to an intermediate size
	intermediateWidth := width * 4 // or another multiplier that gives good results
	intermediateHeight := height * 4
	gocv.Resize(img, &intermediate, image.Point{X: intermediateWidth, Y: intermediateHeight}, 0, 0, gocv.InterpolationArea)

	// 2. Detect edges on the intermediate image
	gray := gocv.NewMat()
	gocv.CvtColor(intermediate, &gray, gocv.ColorBGRToGray)
	gocv.Canny(gray, &edges, 50, 150) // Adjust these thresholds as needed

	// 3. Resize both the intermediate image and the edges to the final size
	gocv.Resize(intermediate, &resized, image.Point{X: width * 2, Y: height * 2}, 0, 0, gocv.InterpolationArea)
	gocv.Resize(edges, &edges, image.Point{X: width * 2, Y: height * 2}, 0, 0, gocv.InterpolationLinear)

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
	gocv.Filter2D(resized, &sharpened, -1, kernel, image.Point{-1, -1}, 0, gocv.BorderDefault)
	resized = sharpened
	return resized, edges
}

func imageToANSI(imagePath string) string {
	img := gocv.IMRead(imagePath, gocv.IMReadAnyColor)
	if img.Empty() {
		return fmt.Sprintf("Could not read image from %s", imagePath)
	}
	defer func(img *gocv.Mat) {
		err := img.Close()
		if err != nil {
			fmt.Println("Error closing image")
		}
	}(&img)

	aspectRatio := float64(img.Cols()) / float64(img.Rows())
	width := TargetWidth
	height := int(float64(width) / aspectRatio / ScaleFactor)

	for {
		resized, edges := prepareForANSI(img, width, height)
		//blurred := gocv.NewMat()
		//gocv.GaussianBlur(resized, &blurred, image.Point{1, 1}, 0, 0, gocv.BorderDefault)
		//resized = blurred
		//gocv.Resize(img,
		//	&resized,
		//	image.Point{X: width * 2, Y: height * 2},
		//	0, 0,
		//	gocv.InterpolationLinear)

		//edges := detectEdges(resized)
		ditheredImg := modifiedAtkinsonDither(resized, edges)
		// Write the scaled image to a file for debugging
		if err := saveToPNG(resized, "resized.png"); err != nil {
			fmt.Println(err)
		}

		// Write the dithered image to a file for debugging
		if err := saveToPNG(ditheredImg, "dithered.png"); err != nil {
			fmt.Println(err)
		}
		// Write the edges image to a file for debugging
		if err := saveToPNG(edges, "edges.png"); err != nil {
			fmt.Println(err)
		}

		ansiImage := renderAnsi(ditheredImg)
		if len(ansiImage) <= MaxChars {
			return ansiImage
		}

		width -= 2
		height = int(float64(width) / aspectRatio / 2)
		if width < 10 {
			return "Image too large to fit within character limit"
		}
	}
}

func getMostFrequentColors(colors []string) []string {
	colorCount := make(map[string]int)
	for _, color := range colors {
		colorCount[color]++
	}
	// Sort the colors by frequency -- first create tuples
	type colorTuple struct {
		color string
		count int
	}
	colorTuples := make([]colorTuple, 0, len(colorCount))
	for color, count := range colorCount {
		colorTuples = append(colorTuples, colorTuple{color, count})
	}
	// Sort the tuples by count
	slices.SortFunc(colorTuples, func(a colorTuple, j colorTuple) int {
		return j.count - a.count
	})

	// Extract the colors from the sorted tuples
	dominantColors := make([]string, 0, len(colorTuples))
	for _, tuple := range colorTuples {
		dominantColors = append(dominantColors, tuple.color)
	}

	return dominantColors
}

func mergeColors(block []string) []string {
	for idx := 0; idx < len(block)-1; idx++ {
		firstColor := rgbFromANSI(block[idx])
		secondColor := rgbFromANSI(block[idx+1])
		if firstColor == secondColor {
			block[idx+1] = block[idx]
		}
	}
	return block
}

func mergeBlockColors(block []string) []string {
	// We need to scan our frequent colors,  determine if there are
	// colors that are the same, and merge them
	rgbColors := make([]RGB, 0, len(block))
	for idx := 0; idx < len(block); idx++ {
		rgbColors = append(rgbColors, rgbFromANSI(block[idx]))
	}
	// Now that we've mapped to actual RGB colors, we can determine
	// if we have two or more colors that are the same
	for idx := 0; idx < len(rgbColors); idx++ {
		for jdx := idx + 1; jdx < len(rgbColors); jdx++ {
			if jdx == idx {
				continue
			}
			if rgbColors[idx].toUint32() == rgbColors[jdx].toUint32() {
				block[jdx] = block[idx]
			} else {
				block[jdx] = block[jdx]
			}
		}
	}

	block = mergeColors(block)
	freqColors := getMostFrequentColors(block)
	if len(freqColors) == 1 {
		return block
	} else {
		block = mergeColors(block)
		freqColors = getMostFrequentColors(block)
		if len(freqColors) == 1 {
			return block
		}
		if len(freqColors) == 2 {
			if colorIsForeground(freqColors[0]) && colorIsBackground(freqColors[1]) {
				return block
			} else if colorIsForeground(freqColors[1]) && colorIsBackground(freqColors[0]) {
				return block
			}
		}

		freqColors = getMostFrequentColors(block)

		fg, bg := resolveColorPair(freqColors[0], freqColors[1])
		for idx := 0; idx < len(block); idx++ {
			if block[idx] == freqColors[0] {
				block[idx] = fg
			} else if block[idx] == freqColors[1] {
				block[idx] = bg
			}
		}
		block = mergeColors(block)
		freqColors = getMostFrequentColors(block)
		if len(freqColors) < 3 {
			return block
		} else {
			fg, bg = resolveColorPair(freqColors[0], freqColors[1])
			// We still have three or more colors, so we need to perform
			// nearest color match to either fg or bg
			for idx := 0; idx < len(block); idx++ {
				currAnsi := block[idx]
				currColor := rgbFromANSI(currAnsi)
				if currAnsi == fg || currAnsi == bg {
					continue
				}
				// Determine if currAnsi is closer to fg or bg
				fgColor := rgbFromANSI(fg)
				bgColor := rgbFromANSI(bg)
				fgDist := fgColor.colorDistance(currColor)
				bgDist := bgColor.colorDistance(currColor)
				if fgDist < bgDist {
					block[idx] = fg
				} else {
					block[idx] = bg
				}
			}
		}
	}
	return block
}

func printAnsiTable() {
	// Header
	fgColors := make([]uint32, 0, len(fgAnsi))
	for color := range fgAnsi {
		fgColors = append(fgColors, color)
	}
	bgColors := make([]uint32, 0, len(bgAnsi))
	for color := range bgAnsi {
		bgColors = append(bgColors, color)
	}
	fmt.Printf("%17s", " ")
	for _, fg := range fgColors {
		fmt.Printf(" %6x (%3s) ", fg, fgAnsi[fg])
	}
	fmt.Println()
	for _, bg := range bgColors {
		fmt.Printf("   %6x (%3s) ", bg, bgAnsi[bg])

		for _, fg := range fgColors {
			fmt.Printf("    %s[%s;%sm %3s %3s %s[0m ", ESC, fgAnsi[fg], bgAnsi[bg], fgAnsi[fg], bgAnsi[bg], ESC)
		}
		fmt.Println()
	}
}

func main() {
	inputFile := flag.String("input", "", "Path to the input image file (required)")
	targetWidth := flag.Int("width", 100, "Target width of the output image")
	maxChars := flag.Int("maxchars", 1048576, "Maximum number of characters in the output")
	edgeDistance := flag.Uint("edge", 1250, "Edge distance")
	outputFile := flag.String("output", "", "Path to save the output (if not specified, prints to stdout)")
	quantization := flag.Int("quantization", 256, "Quantization factor")
	scaleFactor := flag.Float64("scale", 3.0, "Scale factor for the output image")

	// Parse flags
	flag.Parse()

	// Validate required flags
	if *inputFile == "" {
		fmt.Println("Please provide the path to the input image using the -input flag")
		flag.PrintDefaults()
		return
	}

	// Update global variables
	TargetWidth = *targetWidth
	MaxChars = *maxChars
	Quantization = *quantization
	ScaleFactor = *scaleFactor
	EdgeDistance = uint32(*edgeDistance)

	if len(os.Args) < 2 {
		fmt.Println("Please provide the path to the image as an argument")
		return
	}

	// Generate ANSI art
	ansiArt := imageToANSI(*inputFile)
	compressedArt := compressANSI(ansiArt)
	//compressedArt := ansiArt

	// Output result
	if *outputFile != "" {
		err := os.WriteFile(*outputFile, []byte(compressedArt), 0644)
		if err != nil {
			fmt.Printf("Error writing to file: %v\n", err)
			return
		}
		fmt.Printf("Output written to %s\n", *outputFile)
	} else {
		fmt.Print(compressedArt)
	}

	fmt.Printf("Total string length: %d\n", len(ansiArt))
	fmt.Printf("Compressed string length: %d\n", len(compressedArt))
	printAnsiTable()
}
