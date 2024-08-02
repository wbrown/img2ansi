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
	MaxChars     = 1048576
	Shading      = false
	Quantization = 1
	fgAnsi       = map[uint32]string{
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
	for color, code := range fgAnsi {
		if code[0] == '3' {
			bgAnsi[color] = "4" + code[1:]
		} else if code[0] == '9' {
			bgAnsi[color] = "10" + code[1:]
		}
	}
}

func rgbToANSI(r, g, b uint8, fg bool) (uint32, string) {
	color := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	colorDict := fgAnsi
	if !fg {
		colorDict = bgAnsi
	}

	minDiff := uint32(math.MaxUint32)
	var bestCode string
	var bestColor uint32

	for k, v := range colorDict {
		diff := colorDistance(color, k)
		if diff < minDiff {
			minDiff = diff
			bestCode = v
			bestColor = k
		}
	}

	return bestColor, bestCode
}

func colorDistance(c1, c2 uint32) uint32 {
	r1, g1, b1 := uint8(c1>>16), uint8(c1>>8), uint8(c1)
	r2, g2, b2 := uint8(c2>>16), uint8(c2>>8), uint8(c2)
	return uint32(math.Pow(float64(r1)-float64(r2), 2) +
		math.Pow(float64(g1)-float64(g2), 2) +
		math.Pow(float64(b1)-float64(b2), 2))
}

func detectEdges(img gocv.Mat) gocv.Mat {
	gray := gocv.NewMat()
	edges := gocv.NewMat()
	kernel := gocv.GetStructuringElement(gocv.MorphRect, image.Pt(3, 3))

	gocv.CvtColor(img, &gray, gocv.ColorRGBToGray)
	gocv.Canny(gray, &edges, 50, 150)
	gocv.Dilate(edges, &edges, kernel)

	return edges
}

func quantizeColor(c gocv.Vecb) gocv.Vecb {
	qFactor := 256 / float64(Quantization)
	return gocv.Vecb{
		uint8(math.Round(float64(c[0])/qFactor) * qFactor),
		uint8(math.Round(float64(c[1])/qFactor) * qFactor),
		uint8(math.Round(float64(c[2])/qFactor) * qFactor),
	}
}

func modifiedAtkinsonDither(img gocv.Mat, edges gocv.Mat) gocv.Mat {
	height, width := img.Rows(), img.Cols()
	newImage := gocv.NewMatWithSize(height, width, gocv.MatTypeCV8UC3)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			oldPixel := img.GetVecbAt(y, x)
			quantizedPixel := quantizeColor(oldPixel)
			r, g, b := quantizedPixel[2], quantizedPixel[1], quantizedPixel[0]

			fgColor, _ := rgbToANSI(r, g, b, true)
			bgColor, _ := rgbToANSI(r, g, b, false)
			// Check which color is closer to the original
			fgDist := colorDistance(uint32(r)<<16|uint32(g)<<8|uint32(b), fgColor)
			bgDist := colorDistance(uint32(r)<<16|uint32(g)<<8|uint32(b), bgColor)
			var newColor uint32
			if bgDist < fgDist {
				newColor = bgColor
			} else {
				newColor = fgColor
			}

			// Store full color information
			newImage.SetUCharAt(y, x*3+2, uint8(newColor>>16))
			newImage.SetUCharAt(y, x*3+1, uint8(newColor>>8))
			newImage.SetUCharAt(y, x*3, uint8(newColor&0xFF))

			if edges.GetUCharAt(y, x) > 0 { //|| colorDistance(uint32(r)<<16|uint32(g)<<8|uint32(b), newColor) < 500 {
				continue
			}

			dithError := [3]float64{
				float64(r) - float64((newColor>>16)&0xFF),
				float64(g) - float64((newColor>>8)&0xFF),
				float64(b) - float64(newColor&0xFF),
			}

			for i := 0; i < 3; i++ {
				dithError[i] /= 8
			}
			diffuseError := func(y, x int, factor float64) {
				if y >= 0 && y < height && x >= 0 && x < width {
					pixel := img.GetVecbAt(y, x)
					for i := 0; i < 3; i++ {
						newVal := uint8(math.Max(0, math.Min(255, float64(pixel[2-i])+dithError[i]*factor)))
						img.SetUCharAt(y, x*3+(2-i), newVal)
					}
				}
			}

			diffuseError(y, x+1, 1)
			diffuseError(y, x+2, 1)
			diffuseError(y+1, x, 1)
			diffuseError(y+1, x+1, 1)
			diffuseError(y+1, x-1, 1)
			diffuseError(y+2, x, 1)
		}
	}

	return newImage
}

func colorFromANSI(ansiCode string) uint32 {
	for color, code := range fgAnsi {
		if code == ansiCode {
			return color
		}
	}
	return 0 // Default to black if not found
}

func getANSICode(img gocv.Mat, y, x int) string {
	r := img.GetUCharAt(y, x*3+2)
	g := img.GetUCharAt(y, x*3+1)
	b := img.GetUCharAt(y, x*3)
	_, ansiCode := rgbToANSI(r, g, b, true)
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
					compressed.WriteString(formatANSICode(currentFg, currentBg, currentBlock, count))
				}
				currentFg, currentBg, currentBlock = fg, bg, block
				count = 1
			} else {
				count++
			}
		}
		if count > 0 {
			compressed.WriteString(formatANSICode(currentFg, currentBg, currentBlock, count))
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

func extractColors(colorCode string) (string, string) {
	colors := strings.Split(colorCode, ";")
	var fg, bg string
	for _, color := range colors {
		if strings.HasPrefix(color, "3") || strings.HasPrefix(color, "9") {
			fg = color
		} else if strings.HasPrefix(color, "4") || strings.HasPrefix(color, "10") {
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

// 		rowCt := len(line) + len(fmt.Sprintf("%s[0m\n", ESC))
//		compressed.WriteString(fmt.Sprintf("%s[0m %d\n", ESC, rowCt))

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

func averageColor(colors []string) string {
	var r, g, b, count float64
	for _, colorStr := range colors {
		color := colorFromANSI(colorStr)
		r += float64((color >> 16) & 0xFF)
		g += float64((color >> 8) & 0xFF)
		b += float64(color & 0xFF)
		count++
	}
	avgR := uint8(r / count)
	avgG := uint8(g / count)
	avgB := uint8(b / count)
	_, ansiCode := rgbToANSI(avgR, avgG, avgB, true)
	return ansiCode
}

func chooseColorsFromNeighborhood(img gocv.Mat, y, x int) (string, string) {
	neighborhoodSize := 5
	colorCounts := make(map[string]int)

	for dy := -neighborhoodSize / 2; dy <= neighborhoodSize/2; dy++ {
		for dx := -neighborhoodSize / 2; dx <= neighborhoodSize/2; dx++ {
			ny, nx := y+dy*2, x+dx*2
			if ny >= 0 && ny < img.Rows() && nx >= 0 && nx < img.Cols() {
				color := getANSICode(img, ny, nx)
				colorCounts[color]++
			}
		}
	}

	sortedColors := getMostFrequentColors(mapToSlice(colorCounts))
	if len(sortedColors) == 1 {
		return sortedColors[0], sortedColors[0]
	} else {
		return sortedColors[0], sortedColors[1]
	}
}

func getBlockFromColors(colors []string, fgColor, bgColor string) rune {
	return getBlock(
		colors[0] == fgColor,
		colors[1] == fgColor,
		colors[2] == fgColor,
		colors[3] == fgColor,
	)
}

func mapToSlice(m map[string]int) []string {
	result := make([]string, 0, len(m))
	for k, ct := range m {
		for i := 0; i < ct; i++ {
			result = append(result, k)
		}
	}
	return result
}

func imageToANSI(imagePath string) string {
	img := gocv.IMRead(imagePath, gocv.IMReadAnyColor)
	if img.Empty() {
		return fmt.Sprintf("Could not read image from %s", imagePath)
	}
	defer img.Close()

	aspectRatio := float64(img.Cols()) / float64(img.Rows())
	width := TargetWidth
	height := int(float64(width) / aspectRatio / ScaleFactor)

	for {
		resized := gocv.NewMat()
		gocv.Resize(img, &resized, image.Point{X: width * 2, Y: height * 2}, 0, 0, gocv.InterpolationLinear)

		edges := detectEdges(resized)
		ditheredImg := modifiedAtkinsonDither(resized, edges)
		// Write the dithered image to a file for debugging
		saveToPNG(ditheredImg, "dithered.png")

		ansiImage := ""

		imgHeight, imgWidth := ditheredImg.Rows(), ditheredImg.Cols()

		for y := 0; y < imgHeight; y += 2 {
			for x := 0; x < imgWidth; x += 2 {
				colors := []string{
					getANSICode(ditheredImg, y, x),
					getANSICode(ditheredImg, y, x+1),
					getANSICode(ditheredImg, y+1, x),
					getANSICode(ditheredImg, y+1, x+1),
				}

				uniqueColors := make(map[string]int)
				for _, c := range colors {
					uniqueColors[c]++
				}

				if len(uniqueColors) > 2 && Shading {
					// Handle the three-or-more-color case
					fgColor, bgColor := chooseColorsFromNeighborhood(ditheredImg, y, x)
					block := getBlockFromColors(colors, fgColor, bgColor)
					ansiImage += fmt.Sprintf("%s[%s;%sm%s", ESC, fgColor, bgAnsi[colorFromANSI(bgColor)], string(block))
				} else {
					// Handle the two-color case (or single color)
					dominantColors := getMostFrequentColors(colors)
					fgColor := dominantColors[0]
					var bgColor string
					var block rune

					if len(dominantColors) > 1 {
						bgColor = bgAnsi[colorFromANSI(dominantColors[1])]
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

					ansiImage += fmt.Sprintf("%s[%s;%sm%s", ESC, fgColor, bgColor, string(block))
				}
			}

			ansiImage += fmt.Sprintf("%s[0m\n", ESC)
		}

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

func main() {
	inputFile := flag.String("input", "", "Path to the input image file (required)")
	targetWidth := flag.Int("width", 100, "Target width of the output image")
	maxChars := flag.Int("maxchars", 1048576, "Maximum number of characters in the output")
	enableShading := flag.Bool("shading", false, "Enable shading for more detailed output")
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
	Shading = *enableShading
	Quantization = *quantization
	ScaleFactor = *scaleFactor

	if len(os.Args) < 2 {
		fmt.Println("Please provide the path to the image as an argument")
		return
	}

	// Generate ANSI art
	ansiArt := imageToANSI(*inputFile)
	compressedArt := compressANSI(ansiArt)

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
}