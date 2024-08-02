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
	EdgeThreshold = 100
	TargetWidth   = 100
	ScaleFactor   = 3.0
	MaxChars      = 1048576
	Shading       = false
	Quantization  = 1
	BlocksToSpace = false
	fgAnsi        = map[uint32]string{
		// Original colors
		0x000000: "30", // BLACK
		0xF0524F: "31", // RED
		0x5C962C: "32", // GREEN
		0xA68A0D: "33", // YELLOW
		0x3993D4: "34", // BLUE
		0xA771BF: "35", // MAGENTA
		0x00A3A3: "36", // CYAN
		0x808080: "37", // WHITE

		// Bright colors
		0x575959: "90", // BRIGHT BLACK (dark gray)
		0xFF4050: "91", // BRIGHT RED (darker)
		0x4FC414: "92", // BRIGHT GREEN (darker)
		0xE5BF00: "93", // BRIGHT YELLOW (darker)
		0x1FB0FF: "94", // BRIGHT BLUE (darker)
		0xED7EED: "95", // BRIGHT MAGENTA (darker)
		0x00E5E5: "96", // BRIGHT CYAN (darker)
		0xFFFFFF: "97", // BRIGHT WHITE (light gray)
	}

	bgAnsi = map[uint32]string{
		// Original colors
		0x000000: "40", // BLACK
		0x772E2C: "41", // RED
		0x39511F: "42", // GREEN
		0x5C4F17: "43", // YELLOW
		0x245980: "44", // BLUE
		0x5C4069: "45", // MAGENTA
		0x154F4F: "46", // CYAN
		0x616161: "47", // WHITE

		// Bright colors
		0x424242: "100", // BRIGHT BLACK (dark gray)
		0xB82421: "101", // BRIGHT RED (darker)
		0x458500: "102", // BRIGHT GREEN (darker)
		0xA87B00: "103", // BRIGHT YELLOW (darker)
		0x1778BD: "104", // BRIGHT BLUE (darker)
		0xB247B2: "105", // BRIGHT MAGENTA (darker)
		0x006E6E: "106", // BRIGHT CYAN (darker)
		0xFFFFFF: "107", // BRIGHT WHITE (light gray)
	}

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

	unicodeToASCII = map[rune]byte{
		' ': 32, // Space
		//'▀': 223, // Upper half block
		//'▌': 221, // Left half block (approximate)
		//'▐': 222, // Right half block
		//'▄': 220, // Lower half block
		//'█': 219, // Full block
	}
)

func replaceWithASCII(s string) string {
	var result strings.Builder
	for _, r := range s {
		if ascii, ok := unicodeToASCII[r]; ok {
			result.WriteByte(ascii)
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func init() {
	//for color, code := range fgAnsi {
	//	if code[0] == '3' {
	//		bgAnsi[color] = "4" + code[1:]
	//	} else if code[0] == '9' {
	//		bgAnsi[color] = "10" + code[1:]
	//	}
	//}
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
			rgbColor := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
			fgDist := colorDistance(rgbColor, fgColor)
			bgDist := colorDistance(rgbColor, bgColor)
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

			newColorDistance := colorDistance(
				uint32(r)<<16|uint32(g)<<8|uint32(b), newColor)
			if edges.GetUCharAt(y, x) > 0 ||
				newColorDistance < uint32(EdgeThreshold) {
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
						dErr := float64(pixel[2-i]) + dithError[i]*factor
						newVal := uint8(math.Max(0,
							math.Min(255, dErr)))
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

			if BlocksToSpace && fg != "" && block == "█" && bg == "" {
				fgRGB := colorFromANSI(fg)
				// Check if fgRGB exists in the bgAnsi map
				bgMatchFg, exists := bgAnsi[fgRGB]
				if exists {
					block = " " // Full block
					bg = bgMatchFg
					fg = ""
				}
			}

			if fg != currentFg || bg != currentBg || block != currentBlock {
				if count > 0 {
					compressed.WriteString(
						formatANSICode(currentFg,
							currentBg, currentBlock, count))
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

	return replaceWithASCII(compressed.String())
}

func formatANSICode(fg, bg, block string, count int) string {
	var code strings.Builder
	code.WriteString(ESC)
	code.WriteByte('[')
	if fg != "" || bg != "" {
		if fg != "" {
			code.WriteString(fg)
		}
		if bg != "" {
			if fg != "" {
				code.WriteByte(';')
			}
			code.WriteString(bg)
		}
	}
	code.WriteByte('m')
	code.WriteString(strings.Repeat(block, count))
	return code.String()
}

func extractColors(colorCode string) (string, string) {
	colors := strings.Split(colorCode, ";")
	var fg, bg string
	for _, color := range colors {
		if strings.HasPrefix(color, "3") ||
			strings.HasPrefix(color, "9") {
			fg = color
		} else if strings.HasPrefix(color, "4") ||
			strings.HasPrefix(color, "10") {
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
		gocv.Resize(img, &resized, image.Point{X: width * 2, Y: height * 2},
			0, 0, gocv.InterpolationLinear)

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
					fgColor, bgColor := chooseColorsFromNeighborhood(
						ditheredImg, y, x)
					block := getBlockFromColors(colors, fgColor, bgColor)
					ansiImage += fmt.Sprintf("%s[%s;%sm%s",
						ESC, fgColor, bgAnsi[colorFromANSI(bgColor)],
						string(block))
				} else {
					// Handle the two-color case (or single color)
					var fgColor, bgColor string
					dominantColors := getMostFrequentColors(colors)
					for _, color := range dominantColors {
						isForeground := color[0] == '3' || color[0] == '9'
						isBackground := color[0] == '4' || color[0] == '1'
						if isBackground && bgColor == "" {
							bgColor = color
						} else if isForeground && fgColor == "" {
							fgColor = color
						} else if isForeground && bgColor == "" {
							// Convert to background color
							oldFgColor := colorFromANSI(color)
							_, bgColor = rgbToANSI(uint8(oldFgColor>>16),
								uint8(oldFgColor>>8), uint8(oldFgColor),
								false)
							for i, c := range colors {
								if c == color {
									colors[i] = bgColor
								}
							}
						} else if isBackground && fgColor == "" {
							// Convert to foreground color
							oldBgColor := colorFromANSI(color)
							_, fgColor = rgbToANSI(uint8(oldBgColor>>16),
								uint8(oldBgColor>>8),
								uint8(oldBgColor),
								true)
							for i, c := range colors {
								if c == color {
									colors[i] = fgColor
								}
							}
						} else {
							// Determine if the color is closer to the
							// foreground or background
							oldColor := colorFromANSI(color)
							fgDist := colorDistance(oldColor,
								colorFromANSI(fgColor))
							bgDist := colorDistance(oldColor,
								colorFromANSI(bgColor))
							var replacementColor string
							if fgDist < bgDist {
								replacementColor = fgColor
							} else {
								replacementColor = bgColor
							}
							// Replace the color in the list
							for i, c := range colors {
								if c == color {
									colors[i] = replacementColor
								}
							}
						}
					}

					var block rune

					dominantColors = getMostFrequentColors(colors)

					if len(dominantColors) > 1 {
						block = getBlock(
							colors[0] == fgColor,
							colors[1] == fgColor,
							colors[2] == fgColor,
							colors[3] == fgColor,
						)
					} else {
						block = '█' // Full block
					}

					if fgColor == "" {
						ansiImage += fmt.Sprintf("%s[%sm%s", ESC,
							bgColor, string(block))
					} else if bgColor == "" {
						ansiImage += fmt.Sprintf("%s[%sm%s", ESC,
							fgColor, string(block))
					} else {
						ansiImage += fmt.Sprintf("%s[%s;%sm%s", ESC,
							fgColor, bgColor, string(block))
					}
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
		colorTuples = append(colorTuples,
			colorTuple{color, count})
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
	inputFile := flag.String("input", "",
		"Path to the input image file (required)")
	targetWidth := flag.Int("width", 100,
		"Target width of the output image")
	maxChars := flag.Int("maxchars", 1048576,
		"Maximum number of characters in the output")
	enableShading := flag.Bool("shading", false,
		"Enable shading for more detailed output")
	edgeThreshold := flag.Int("edge", 100,
		"Color difference threshold for edge detection skipping")
	separatePalette := flag.Bool("separate", false,
		"Use separate palettes for foreground and background colors")
	outputFile := flag.String("output", "",
		"Path to save the output (if not specified, prints to stdout)")
	quantization := flag.Int("quantization", 256,
		"Quantization factor")
	blocksToSpace := flag.Bool("space", false,
		"Convert block characters to spaces")
	scaleFactor := flag.Float64("scale", 3.0,
		"Scale factor for the output image")
	maxLine := flag.Int("maxline", 0,
		"Maximum number of characters in a line, 0 for no limit")

	// Parse flags
	flag.Parse()

	// Validate required flags
	if *inputFile == "" {
		fmt.Println("Please provide the image using the -input flag")
		flag.PrintDefaults()
		return
	}

	// Update global variables
	TargetWidth = *targetWidth
	MaxChars = *maxChars
	Shading = *enableShading
	Quantization = *quantization
	ScaleFactor = *scaleFactor
	BlocksToSpace = *blocksToSpace
	EdgeThreshold = *edgeThreshold

	if !*separatePalette {
		bgAnsi = make(map[uint32]string)
		for color, code := range fgAnsi {
			if code[0] == '3' {
				bgAnsi[color] = "4" + code[1:]
			} else if code[0] == '9' {
				bgAnsi[color] = "10" + code[1:]
			}
		}
	}

	if len(os.Args) < 2 {
		fmt.Println("Please provide the path to the image as an argument")
		return
	}

	var ansiArt string
	var compressedArt string

	linesWithinLimit := false
	for !linesWithinLimit {
		// Generate ANSI art
		ansiArt = imageToANSI(*inputFile)
		compressedArt = compressANSI(ansiArt)
		// Count each line
		lines := strings.Split(compressedArt, "\n")
		if *maxLine == 0 {
			break
		}
		linesWithinLimit = true
		var widestLine int
		var lineWidth int
		for _, line := range lines {
			lineWidth = len([]byte(line))
			if lineWidth > widestLine {
				widestLine = lineWidth
			}
			if lineWidth > *maxLine {
				linesWithinLimit = false
				break
			}
		}
		if !linesWithinLimit {
			TargetWidth -= 2
			print(fmt.Sprintf(
				"Longest line: %d, adjusting to %d width\n",
				lineWidth, TargetWidth))
		} else {
			print(fmt.Sprintf("Longest line: %d\n", widestLine))
		}
	}

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
