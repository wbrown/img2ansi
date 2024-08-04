package main

import "gocv.io/x/gocv"

func resolveBlock(colors []string) (fgColor, bgColor string, ansiBlock rune) {
	uniqueColors := make(map[string]int)
	for _, c := range colors {
		uniqueColors[c]++
	}

	//if len(uniqueColors) > 2 && Shading {
	//	// Handle the three-or-more-color case
	//	fgColor, bgColor := chooseColorsFromNeighborhood(ditheredImg, y, x)
	//	block := getBlockFromColors(colors, fgColor, bgColor)
	//	ansiImage += fmt.Sprintf("%s[%s;%sm%s", ESC, fgColor, bgAnsi[rgbFromANSI(bgColor)], string(block))
	// We need to resolve the following scenarios:
	// 1. >1 colors are foreground, >1 are background
	// 2. Three or four different colors
	//block := mergeBlockColors(colors)
	block := colors

	// At this point, we should only have one or two colors
	dominantColors := getMostFrequentColors(block)
	fgColor = dominantColors[0]

	if len(dominantColors) > 1 {
		fgColor, bgColor = resolveColorPair(dominantColors[0],
			dominantColors[1])
		ansiBlock = getBlock(
			colors[0] == fgColor,
			colors[1] == fgColor,
			colors[2] == fgColor,
			colors[3] == fgColor,
		)
	} else {
		fgColor = dominantColors[0]
		ansiBlock = '█' // Full block
	}
	return fgColor, bgColor, ansiBlock
}

func getBlockFromColors(colors []string, fgColor, bgColor string) rune {
	return getBlock(
		colors[0] == fgColor,
		colors[1] == fgColor,
		colors[2] == fgColor,
		colors[3] == fgColor,
	)
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
	}

	firstColorIsFg := colorIsForeground(sortedColors[0])
	secondColorIsFg := colorIsForeground(sortedColors[1])
	if firstColorIsFg == secondColorIsFg {
		bgOrFg := !(firstColorIsFg && secondColorIsFg)
		secondColorCandidate := rgbFromANSI(sortedColors[1])
		_, sortedColors[1] = secondColorCandidate.rgbToANSI(bgOrFg)
	}
	return sortedColors[0], sortedColors[1]
}

func averageColor(colors []string) string {
	var r, g, b, count float64
	for _, colorStr := range colors {
		color := rgbFromANSI(colorStr)
		r += float64(color.r & 0xFF)
		g += float64(color.g & 0xFF)
		b += float64(color.b & 0xFF)
		count++
	}
	avgRgb := RGB{
		uint8(r / count),
		uint8(g / count),
		uint8(b / count),
	}
	_, ansiCode := avgRgb.rgbToANSI(true)
	return ansiCode
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
