package main

import (
	"fmt"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"image"
	"io/ioutil"
	"math"
	"math/bits"
	"sort"
	"strings"
	"unicode"
)

const (
	GlyphWidth  = 8
	GlyphHeight = 8
	ZoneSize    = 4
	NumZones    = (GlyphWidth * GlyphHeight) / (ZoneSize * ZoneSize)
)

var specialGlyphs = map[rune]uint32{}

type GlyphBitmap uint64

type GlyphInfo struct {
	Rune        rune
	Img         *image.Alpha
	Bounds      fixed.Rectangle26_6
	Advance     fixed.Int26_6
	Bitmap      GlyphBitmap
	Weight      uint8
	RowWeights  [GlyphHeight]byte
	EdgeMap     GlyphBitmap
	ZoneWeights [NumZones]uint8
}

type GlyphLookup struct {
	Glyphs    []GlyphInfo
	WeightMap [GlyphHeight*GlyphWidth + 1][]*GlyphInfo
	ZoneMap   [NumZones][ZoneSize*ZoneSize + 1][]*GlyphInfo
}

func NewGlyphLookup(glyphs []GlyphInfo) *GlyphLookup {
	gl := &GlyphLookup{
		Glyphs:    glyphs,
		WeightMap: [GlyphHeight*GlyphWidth + 1][]*GlyphInfo{},
		ZoneMap:   [NumZones][ZoneSize*ZoneSize + 1][]*GlyphInfo{},
	}

	for i := range glyphs {
		glyph := &gl.Glyphs[i]

		// Populate WeightMap
		gl.WeightMap[glyph.Weight] = append(gl.WeightMap[glyph.Weight], glyph)

		// Populate ZoneMap
		for zone, zoneWeight := range glyph.ZoneWeights {
			gl.ZoneMap[zone][zoneWeight] = append(
				gl.ZoneMap[zone][zoneWeight], glyph,
			)
		}

	}

	return gl
}

func (gl *GlyphLookup) LookupRune(r rune) *GlyphInfo {
	for _, glyph := range gl.Glyphs {
		if glyph.Rune == r {
			return &glyph
		}
	}
	return nil
}

func (gl *GlyphLookup) FindClosestGlyph(block GlyphBitmap) GlyphInfo {
	blockInfo := extractFeatures(block)

	candidates := gl.getCandidatesByZones(blockInfo.ZoneWeights)
	fmt.Printf("Number of candidates: %d\n", len(candidates)) // Debug print

	if len(candidates) > 50 {
		candidates = gl.filterCandidatesByWeight(candidates, blockInfo.Weight)
	}

	var bestMatch *GlyphInfo
	bestSimilarity := -1.0

	for _, glyph := range candidates {
		similarity := calculateSimilarity(blockInfo, *glyph)
		if similarity > bestSimilarity {
			bestSimilarity = similarity
			bestMatch = glyph
		}
	}

	if bestMatch == nil {
		return GlyphInfo{} // Or some default glyph
	}

	return *bestMatch
}

func (gl *GlyphLookup) getCandidatesByZones(zoneWeights [NumZones]uint8) []*GlyphInfo {
	var candidates []*GlyphInfo
	seenGlyphs := make(map[*GlyphInfo]bool)

	for zone, weight := range zoneWeights {
		for w := max(0, int(weight)-3); w <= min(
			ZoneSize*ZoneSize, int(weight)+3,
		); w++ {
			if gl.ZoneMap[zone][w] != nil {
				for _, glyph := range gl.ZoneMap[zone][w] {
					if !seenGlyphs[glyph] {
						candidates = append(candidates, glyph)
						seenGlyphs[glyph] = true
					}
				}
			}
		}
	}

	// If we still don't have candidates, fall back to overall weight
	if len(candidates) == 0 {
		totalWeight := uint8(0)
		for _, w := range zoneWeights {
			totalWeight += w
		}
		candidates = gl.getGlyphsByWeight(totalWeight)
	}

	return candidates
}

func (gl *GlyphLookup) filterCandidatesByWeight(
	candidates []*GlyphInfo,
	targetWeight uint8,
) []*GlyphInfo {
	var filtered []*GlyphInfo
	for _, glyph := range candidates {
		// Allow some tolerance
		if abs(int(glyph.Weight)-int(targetWeight)) <= 5 {
			filtered = append(filtered, glyph)
		}
	}
	return filtered
}

func (gl *GlyphLookup) getGlyphsByWeight(weight uint8) []*GlyphInfo {
	if weight >= uint8(len(gl.WeightMap)) {
		return nil
	}
	return gl.WeightMap[weight]
}

func extractFeatures(bitmap GlyphBitmap) GlyphInfo {
	return GlyphInfo{
		Bitmap:      bitmap,
		Weight:      uint8(bitmap.popCount()),
		RowWeights:  calculateRowWeights(bitmap),
		EdgeMap:     detectEdges(bitmap),
		ZoneWeights: calculateZoneWeights(bitmap),
	}
}

func calculateRowWeights(bitmap GlyphBitmap) [GlyphHeight]byte {
	var rowWeights [GlyphHeight]byte
	for y := 0; y < GlyphHeight; y++ {
		var rowWeight byte
		for x := 0; x < GlyphWidth; x++ {
			if bitmap&(1<<(y*GlyphWidth+x)) != 0 {
				rowWeight++
			}
		}
		rowWeights[y] = rowWeight
	}
	return rowWeights
}

func calculateZoneWeights(bitmap GlyphBitmap) [NumZones]uint8 {
	var weights [NumZones]uint8
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if bitmap&(1<<(y*GlyphWidth+x)) != 0 {
				zone := (y/ZoneSize)*(GlyphWidth/ZoneSize) + (x / ZoneSize)
				weights[zone]++
			}
		}
	}
	return weights
}

func detectEdges(bitmap GlyphBitmap) GlyphBitmap {
	var edgeMap GlyphBitmap
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if isEdge(bitmap, x, y) {
				edgeMap |= 1 << (y*GlyphWidth + x)
			}
		}
	}
	return edgeMap
}

func isEdge(bitmap GlyphBitmap, x, y int) bool {
	if !getBit(bitmap, x, y) {
		return false // Empty pixels are never edges
	}

	// Check immediate neighbors (8-connectivity)
	neighbors := [][2]int{
		{-1, -1}, {0, -1}, {1, -1},
		{-1, 0}, {1, 0},
		{-1, 1}, {0, 1}, {1, 1},
	}

	for _, offset := range neighbors {
		nx, ny := x+offset[0], y+offset[1]
		if nx < 0 || nx >= GlyphWidth || ny < 0 || ny >= GlyphHeight {
			return true // Treat boundary pixels as edges
		}
		if !getBit(bitmap, nx, ny) {
			return true // It's an edge if any neighbor is empty
		}
	}

	return false // Not an edge if all neighbors are filled
}

//func isEdge(bitmap GlyphBitmap, x, y int) bool {
//	center := getBit(bitmap, x, y)
//
//	// Check immediate neighbors (8-connectivity)
//	neighbors := [][2]int{
//		{-1, -1}, {0, -1}, {1, -1},
//		{-1, 0}, {1, 0},
//		{-1, 1}, {0, 1}, {1, 1},
//	}
//
//	filledNeighbors := 0
//	totalNeighbors := 0
//
//	for _, offset := range neighbors {
//		nx, ny := x+offset[0], y+offset[1]
//		if nx >= 0 && nx < GlyphWidth && ny >= 0 && ny < GlyphHeight {
//			totalNeighbors++
//			if getBit(bitmap, nx, ny) {
//				filledNeighbors++
//			}
//		}
//	}
//
//	if center {
//		// For filled pixels, it's an edge if it has any empty neighbors
//		return filledNeighbors < totalNeighbors
//	} else {
//		// For empty pixels, it's an edge if it has at least 2 filled neighbors
//		return filledNeighbors >= 2
//	}
//}

func getBit(bitmap GlyphBitmap, x, y int) bool {
	if x < 0 || x >= GlyphWidth || y < 0 || y >= GlyphHeight {
		return false
	}
	return bitmap&(1<<(y*GlyphWidth+x)) != 0
}

func calculateSimilarity(a, b GlyphInfo) float64 {
	shapeSimilarity := calculateShapeSimilarity(a.Bitmap, b.Bitmap)
	patternSimilarity := calculatePatternSimilarity(a.Bitmap, b.Bitmap)
	densitySimilarity := calculateDensitySimilarity(a.Bitmap, b.Bitmap)

	return 0.7*shapeSimilarity + 0.2*patternSimilarity + 0.1*densitySimilarity
}

const (
	DiagonalNone = iota
	DiagonalTopLeftToBottomRight
	DiagonalTopRightToBottomLeft
)

func detectDiagonalLine(bitmap GlyphBitmap) int {
	topLeft, topRight := 0, 0
	for y := 0; y < GlyphHeight; y++ {
		if getBit(bitmap, y, y) {
			topLeft++
		}
		if getBit(bitmap, GlyphWidth-1-y, y) {
			topRight++
		}
	}

	threshold := GlyphHeight * 3 / 4 // Increased threshold
	if topLeft >= threshold && topLeft > topRight*2 {
		return DiagonalTopLeftToBottomRight
	} else if topRight >= threshold && topRight > topLeft*2 {
		return DiagonalTopRightToBottomLeft
	}
	return DiagonalNone
}

func calculateShapeSimilarity(a, b GlyphBitmap) float64 {
	aDiagonal := detectDiagonalLine(a)
	bDiagonal := detectDiagonalLine(b)

	// If both are diagonal lines
	if aDiagonal != DiagonalNone && bDiagonal != DiagonalNone {
		if aDiagonal == bDiagonal {
			return 1.0 // Perfect match for same direction diagonals
		} else {
			return 0.0 // Complete mismatch for different direction diagonals
		}
	}

	// If only one is a diagonal line, or neither are
	aFeatures := extractShapeFeatures(a)
	bFeatures := extractShapeFeatures(b)

	// Calculate similarity based on features
	featureSimilarity := 1 - euclideanDistance(
		aFeatures, bFeatures,
	)/math.Sqrt(float64(len(aFeatures)))

	// If one is a diagonal and the other isn't, reduce the similarity
	if (aDiagonal != DiagonalNone && bDiagonal == DiagonalNone) ||
		(aDiagonal == DiagonalNone && bDiagonal != DiagonalNone) {
		featureSimilarity *= 0.5
	}

	return featureSimilarity
}

func extractShapeFeatures(bitmap GlyphBitmap) []float64 {
	var features []float64

	// Horizontal and vertical projections
	for i := 0; i < GlyphHeight; i++ {
		features = append(
			features, float64(horizontalProjection(bitmap, i))/GlyphWidth,
		)
	}
	for i := 0; i < GlyphWidth; i++ {
		features = append(
			features, float64(verticalProjection(bitmap, i))/GlyphHeight,
		)
	}

	// Contour features
	contour := extractContour(bitmap)
	features = append(
		features,
		float64(bits.OnesCount64(uint64(contour)))/float64(GlyphWidth*GlyphHeight),
	)

	// Centrality
	features = append(features, calculateCentrality(bitmap))

	return features
}

func calculateDiagonalOrientation(bitmap GlyphBitmap) float64 {
	topLeftSum, topRightSum := 0, 0
	totalPixels := 0

	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) {
				topLeftSum += x - y
				topRightSum += x + y
				totalPixels++
			}
		}
	}

	if totalPixels == 0 {
		return 0
	}

	topLeftAvg := float64(topLeftSum) / float64(totalPixels)
	topRightAvg := float64(topRightSum) / float64(totalPixels)

	// Normalize to [-1, 1] range
	topLeftScore := topLeftAvg / float64(GlyphWidth-1)
	topRightScore := topRightAvg / float64(GlyphWidth+GlyphHeight-2)

	// Return the score with the larger magnitude
	if math.Abs(topLeftScore) > math.Abs(topRightScore) {
		return topLeftScore
	}
	return topRightScore
}

func horizontalProjection(bitmap GlyphBitmap, row int) int {
	count := 0
	for x := 0; x < GlyphWidth; x++ {
		if getBit(bitmap, x, row) {
			count++
		}
	}
	return count
}

func verticalProjection(bitmap GlyphBitmap, col int) int {
	count := 0
	for y := 0; y < GlyphHeight; y++ {
		if getBit(bitmap, col, y) {
			count++
		}
	}
	return count
}

func extractContour(bitmap GlyphBitmap) GlyphBitmap {
	var contour GlyphBitmap
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) &&
				(x == 0 || y == 0 || x == GlyphWidth-1 || y == GlyphHeight-1 ||
					!getBit(bitmap, x-1, y) || !getBit(bitmap, x+1, y) ||
					!getBit(bitmap, x, y-1) || !getBit(bitmap, x, y+1)) {
				contour |= 1 << (y*GlyphWidth + x)
			}
		}
	}
	return contour
}

func calculateCentrality(bitmap GlyphBitmap) float64 {
	centerX, centerY := float64(GlyphWidth-1)/2, float64(GlyphHeight-1)/2
	totalDist, count := 0.0, 0
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) {
				dx, dy := float64(x)-centerX, float64(y)-centerY
				totalDist += math.Sqrt(dx*dx + dy*dy)
				count++
			}
		}
	}
	if count == 0 {
		return 0
	}
	maxDist := math.Sqrt(centerX*centerX + centerY*centerY)
	return 1 - (totalDist/float64(count))/maxDist
}

func calculatePatternSimilarity(a, b GlyphBitmap) float64 {
	aPattern := analyzePattern(a)
	bPattern := analyzePattern(b)

	return 1 - math.Abs(aPattern.horizontalFrequency-bPattern.horizontalFrequency)/2 -
		math.Abs(aPattern.verticalFrequency-bPattern.verticalFrequency)/2
}

func analyzePattern(bitmap GlyphBitmap) struct {
	horizontalFrequency float64
	verticalFrequency   float64
} {
	hChanges, vChanges := 0, 0
	for y := 0; y < GlyphHeight; y++ {
		for x := 1; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) != getBit(bitmap, x-1, y) {
				hChanges++
			}
		}
	}
	for x := 0; x < GlyphWidth; x++ {
		for y := 1; y < GlyphHeight; y++ {
			if getBit(bitmap, x, y) != getBit(bitmap, x, y-1) {
				vChanges++
			}
		}
	}
	return struct {
		horizontalFrequency float64
		verticalFrequency   float64
	}{
		horizontalFrequency: float64(hChanges) / float64(GlyphHeight*GlyphWidth),
		verticalFrequency:   float64(vChanges) / float64(GlyphHeight*GlyphWidth),
	}
}

func euclideanDistance(a, b []float64) float64 {
	sum := 0.0
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return math.Sqrt(sum)
}

func calculateLineSimilarity(a, b GlyphBitmap) float64 {
	aCount := bits.OnesCount64(uint64(a))
	bCount := bits.OnesCount64(uint64(b))

	// If both bitmaps are empty or nearly empty, they're considered similar
	if aCount <= 1 && bCount <= 1 {
		return 1.0
	}

	// If one bitmap is empty (or nearly empty) and the other isn't, they're dissimilar
	if aCount <= 1 || bCount <= 1 {
		return 0.0
	}

	aStart, aEnd := findLineEndpoints(a)
	bStart, bEnd := findLineEndpoints(b)

	aAngle := calculateAngle(aStart, aEnd)
	bAngle := calculateAngle(bStart, bEnd)

	angleDiff := math.Abs(aAngle - bAngle)
	if angleDiff > math.Pi {
		angleDiff = 2*math.Pi - angleDiff
	}

	angleSimilarity := 1 - angleDiff/math.Pi

	// Calculate length similarity
	aLength := distance(aStart, aEnd)
	bLength := distance(bStart, bEnd)
	lengthSimilarity := 1 - math.Abs(aLength-bLength)/math.Max(
		aLength, bLength,
	)

	return 0.7*angleSimilarity + 0.3*lengthSimilarity
}

func findLineEndpoints(bitmap GlyphBitmap) (start, end [2]int) {
	var points [][2]int

	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) {
				points = append(points, [2]int{x, y})
			}
		}
	}

	if len(points) < 2 {
		return [2]int{0, 0}, [2]int{0, 0} // Return same point for empty or single-pixel bitmaps
	}

	maxDist := 0.0
	for i, p1 := range points {
		for j := i + 1; j < len(points); j++ {
			p2 := points[j]
			dist := distance(p1, p2)
			if dist > maxDist {
				maxDist = dist
				start, end = p1, p2
			}
		}
	}

	return start, end
}

func calculateAngle(start, end [2]int) float64 {
	dx := float64(end[0] - start[0])
	dy := float64(end[1] - start[1])
	return math.Atan2(dy, dx)
}

func distance(p1, p2 [2]int) float64 {
	dx := float64(p2[0] - p1[0])
	dy := float64(p2[1] - p1[1])
	return math.Sqrt(dx*dx + dy*dy)
}

func calculateDensitySimilarity(a, b GlyphBitmap) float64 {
	aCount := bits.OnesCount64(uint64(a))
	bCount := bits.OnesCount64(uint64(b))

	return 1 - math.Abs(float64(aCount-bCount))/float64(GlyphWidth*GlyphHeight)
}

func calculateDistributionSimilarity(a, b GlyphBitmap) float64 {
	aDistribution := calculateDistribution(a)
	bDistribution := calculateDistribution(b)

	totalDiff := 0.0
	for i := range aDistribution {
		totalDiff += math.Abs(aDistribution[i] - bDistribution[i])
	}

	return 1 - totalDiff/4 // Normalize to [0, 1]
}

func calculateDistribution(bitmap GlyphBitmap) [4]float64 {
	var distribution [4]float64
	total := 0

	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) {
				if x < GlyphWidth/2 {
					if y < GlyphHeight/2 {
						distribution[0]++
					} else {
						distribution[2]++
					}
				} else {
					if y < GlyphHeight/2 {
						distribution[1]++
					} else {
						distribution[3]++
					}
				}
				total++
			}
		}
	}

	if total > 0 {
		for i := range distribution {
			distribution[i] /= float64(total)
		}
	}

	return distribution
}

func calculateEdgeMapSimilarity(a, b GlyphBitmap) float64 {
	structuralSimilarity := calculateStructuralSimilarity(a, b)
	directionSimilarity := calculateDirectionSimilarity(a, b)

	// Combine similarities, giving more weight to direction for diagonal lines
	return 0.3*structuralSimilarity + 0.7*directionSimilarity
}

func calculateStructuralSimilarity(a, b GlyphBitmap) float64 {
	intersection := bits.OnesCount64(uint64(a & b))
	union := bits.OnesCount64(uint64(a | b))

	if union == 0 {
		return 0.0 // Both are empty, consider them dissimilar
	}

	return float64(intersection) / float64(union)
}

func calculateDirectionSimilarity(a, b GlyphBitmap) float64 {
	aDirection := calculateDirection(a)
	bDirection := calculateDirection(b)

	// Compare directions, handling wrap-around cases
	diff := math.Abs(aDirection - bDirection)
	if diff > math.Pi {
		diff = 2*math.Pi - diff
	}

	return 1 - diff/math.Pi
}

func calculateDirection(bitmap GlyphBitmap) float64 {
	var sumX, sumY float64
	count := 0

	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) {
				sumX += float64(x - GlyphWidth/2)
				sumY += float64(y - GlyphHeight/2)
				count++
			}
		}
	}

	if count == 0 {
		return 0 // Default direction for empty bitmap
	}

	return math.Atan2(sumY, sumX)
}

//func calculateSimilarity(a, b GlyphInfo) float64 {
//	bitmapSimilarity := calculateStructuralSimilarity(a.Bitmap, b.Bitmap)
//	edgeSimilarity := calculateEdgeMapSimilarity(a.EdgeMap, b.EdgeMap)
//	zoneSimilarity := calculateZoneSimilarity(a.ZoneWeights, b.ZoneWeights)
//
//	// Adjust weights to emphasize edge similarity for diagonal lines
//	return 0.2*bitmapSimilarity + 0.6*edgeSimilarity + 0.2*zoneSimilarity
//}
//
//func calculateEdgeMapSimilarity(a, b GlyphBitmap) float64 {
//	structuralSimilarity := calculateStructuralSimilarity(a, b)
//	shapeSimilarity := calculateShapeSimilarity(a, b)
//
//	return 0.6*structuralSimilarity + 0.4*shapeSimilarity
//}
//
//func calculateStructuralSimilarity(a, b GlyphBitmap) float64 {
//	intersection := bits.OnesCount64(uint64(a & b))
//	union := bits.OnesCount64(uint64(a | b))
//
//	if union == 0 {
//		return 1.0 // Both are empty
//	}
//
//	return float64(intersection) / float64(union)
//}

func calculateShapeDescriptor(bitmap GlyphBitmap) [8]float64 {
	var descriptor [8]float64
	totalEdges := 0
	centerX, centerY := float64(GlyphWidth-1)/2, float64(GlyphHeight-1)/2

	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) {
				dx, dy := float64(x)-centerX, float64(y)-centerY
				angle := math.Atan2(dy, dx)

				// Map angle to [0, 2π) range
				if angle < 0 {
					angle += 2 * math.Pi
				}

				// Calculate sector (0 to 7)
				sector := int(8 * angle / (2 * math.Pi))

				// Ensure sector is within bounds
				// Ensure sector is always within bounds
				sector = sector % 8

				descriptor[sector]++
				totalEdges++
			}
		}
	}

	// Normalize
	if totalEdges > 0 {
		for i := range descriptor {
			descriptor[i] /= float64(totalEdges)
		}
	}

	return descriptor
}

//func calculateDistributionSimilarity(a, b GlyphBitmap) float64 {
//	aDistribution := calculateDistribution(a)
//	bDistribution := calculateDistribution(b)
//
//	totalDiff := 0.0
//	for i := range aDistribution {
//		diff := math.Abs(aDistribution[i] - bDistribution[i])
//		totalDiff += diff * diff // Use squared difference for more sensitivity
//	}
//
//	return 1.0 - math.Sqrt(totalDiff)/math.Sqrt(2.0) // Normalize to [0, 1]
//}

//func calculateDistribution(bitmap GlyphBitmap) [4]float64 {
//	var distribution [4]float64 // [top, right, bottom, left]
//	totalEdges := 0
//
//	for y := 0; y < GlyphHeight; y++ {
//		for x := 0; x < GlyphWidth; x++ {
//			if getBit(bitmap, x, y) {
//				if y < GlyphHeight/2 {
//					distribution[0]++
//				} else {
//					distribution[2]++
//				}
//				if x < GlyphWidth/2 {
//					distribution[3]++
//				} else {
//					distribution[1]++
//				}
//				totalEdges++
//			}
//		}
//	}
//
//	if totalEdges > 0 {
//		for i := range distribution {
//			distribution[i] /= float64(totalEdges)
//		}
//	}
//
//	return distribution
//}

//func calculateEdgeMapSimilarity(a, b GlyphBitmap) float64 {
//	structuralSimilarity := calculateStructuralSimilarity(a, b)
//	patternSimilarity := calculatePatternSimilarity(a, b)
//
//	return 0.6*structuralSimilarity + 0.4*patternSimilarity
//}
//
//func calculateStructuralSimilarity(a, b GlyphBitmap) float64 {
//	diffCount := bits.OnesCount64(uint64(a ^ b))
//	return 1 - float64(diffCount)/(GlyphWidth*GlyphHeight)
//}

type PatternAnalysis struct {
	horizontalFrequency float64
	verticalFrequency   float64
	diagonalFrequency   float64
}

func calculateOrientationSimilarity(a, b GlyphBitmap) float64 {
	aOrientation := calculateOrientation(a)
	bOrientation := calculateOrientation(b)
	return 1 - math.Abs(aOrientation-bOrientation)/math.Pi
}

func calculateOrientation(bitmap GlyphBitmap) float64 {
	sumX, sumY, count := 0.0, 0.0, 0.0
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) {
				sumX += float64(x)
				sumY += float64(y)
				count++
			}
		}
	}
	if count == 0 {
		return 0
	}
	centerX, centerY := sumX/count, sumY/count

	sumNum, sumDen := 0.0, 0.0
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) {
				dx, dy := float64(x)-centerX, float64(y)-centerY
				sumNum += 2 * dx * dy
				sumDen += dx*dx - dy*dy
			}
		}
	}
	return 0.5 * math.Atan2(sumNum, sumDen)
}

//func calculateDistributionSimilarity(a, b GlyphBitmap) float64 {
//	aDist := calculateDistribution(a)
//	bDist := calculateDistribution(b)
//	diff := 0.0
//	for i := range aDist {
//		diff += math.Abs(aDist[i] - bDist[i])
//	}
//	return 1 - diff/float64(GlyphWidth+GlyphHeight)
//}

//func calculateDistribution(bitmap GlyphBitmap) [GlyphWidth + GlyphHeight]float64 {
//	var dist [GlyphWidth + GlyphHeight]float64
//	for y := 0; y < GlyphHeight; y++ {
//		for x := 0; x < GlyphWidth; x++ {
//			if getBit(bitmap, x, y) {
//				dist[x]++
//				dist[GlyphWidth+y]++
//			}
//		}
//	}
//	for i := range dist {
//		dist[i] /= float64(GlyphWidth * GlyphHeight)
//	}
//	return dist
//}

//func calculateShapeSimilarity(a, b GlyphBitmap) float64 {
//	aShape := calculateShape(a)
//	bShape := calculateShape(b)
//	diff := 0.0
//	for i := range aShape {
//		for j := range aShape[i] {
//			diff += math.Abs(aShape[i][j] - bShape[i][j])
//		}
//	}
//	return 1 - diff/float64(GlyphWidth*GlyphHeight*4)
//}

func calculateShape(bitmap GlyphBitmap) [GlyphHeight][GlyphWidth]float64 {
	var shape [GlyphHeight][GlyphWidth]float64
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) {
				for dy := 0; dy < GlyphHeight; dy++ {
					for dx := 0; dx < GlyphWidth; dx++ {
						dist := math.Sqrt(float64((x-dx)*(x-dx) + (y-dy)*(y-dy)))
						shape[dy][dx] += 1 / (1 + dist)
					}
				}
			}
		}
	}
	return shape
}

func getEdgePositions(bitmap GlyphBitmap) [][2]int {
	var positions [][2]int
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) {
				positions = append(positions, [2]int{x, y})
			}
		}
	}
	return positions
}

type GlyphInfoBatchResponse struct {
	Source     GlyphInfo
	Target     GlyphInfo
	Similarity float64
}

// Sort by similarity
type BySimilarity []GlyphInfoBatchResponse

// Implement sort.Interface for BySimilarity
func (a BySimilarity) Len() int           { return len(a) }
func (a BySimilarity) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a BySimilarity) Less(i, j int) bool { return a[i].Similarity > a[j].Similarity }

func calculateSimilarityBatch(a GlyphInfo, b []GlyphInfo) BySimilarity {
	resp := make([]GlyphInfoBatchResponse, 0, len(b))

	for _, glyph := range b {
		resp = append(
			resp, GlyphInfoBatchResponse{
				Source:     a,
				Target:     glyph,
				Similarity: calculateSimilarity(a, glyph),
			},
		)
	}
	bySimilarity := BySimilarity(resp)
	sort.Sort(bySimilarity)

	return bySimilarity
}

func calculateBitmapSimilarity(a, b GlyphBitmap) float64 {
	xor := a ^ b
	differentBits := bits.OnesCount64(uint64(xor))
	return 1 - float64(differentBits)/(GlyphWidth*GlyphHeight)
}

func calculateZoneSimilarity(a, b [NumZones]uint8) float64 {
	var totalDiff float64
	for i := 0; i < NumZones; i++ {
		diff := float64(abs(int(a[i]) - int(b[i])))
		totalDiff += diff
	}
	maxPossibleDiff := float64(NumZones * ZoneSize * ZoneSize)
	return 1 - (totalDiff / maxPossibleDiff)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

var popCountTable [256]byte

func init() {
	for i := range popCountTable {
		popCountTable[i] = byte(bits.OnesCount8(uint8(i)))
	}
}

func (g GlyphBitmap) popCount() uint8 {
	return uint8(bits.OnesCount64(uint64(g)))
}

func loadFont(path string) (*truetype.Font, error) {
	fontData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	font, err := truetype.Parse(fontData)
	if err != nil {
		return nil, err
	}
	return font, nil
}

func (g GlyphBitmap) String() string {
	var sb strings.Builder

	// Add hex representation
	sb.WriteString(" ")
	for x := 0; x < GlyphWidth; x++ {
		sb.WriteString(fmt.Sprintf("%d", x))
	}
	for y := 0; y < GlyphHeight; y++ {
		sb.WriteString(fmt.Sprintf("\n%d", y))
		for x := 0; x < GlyphWidth; x++ {
			if g&(1<<(63-y*GlyphWidth-x)) != 0 {
				sb.WriteString("█")
			} else {
				sb.WriteString("·")
			}
		}
	}
	return sb.String()
}

func getSafeRunes() []rune {
	var safeRunes []rune

	// ASCII
	for r := rune(0x0000); r <= 0x007F; r++ {
		safeRunes = append(safeRunes, r)
	}

	// Latin-1 Supplement
	for r := rune(0x0080); r <= 0x00FF; r++ {
		safeRunes = append(safeRunes, r)
	}

	// Latin Extended-A
	for r := rune(0x0100); r <= 0x017F; r++ {
		safeRunes = append(safeRunes, r)
	}

	// General Punctuation
	for r := rune(0x2000); r <= 0x206F; r++ {
		safeRunes = append(safeRunes, r)
	}

	// Box Drawing Characters
	for r := rune(0x2500); r <= 0x257F; r++ {
		safeRunes = append(safeRunes, r)
	}

	// Block Elements
	for r := rune(0x2580); r <= 0x259F; r++ {
		safeRunes = append(safeRunes, r)
	}

	return safeRunes
}

func (info *GlyphInfo) analyzeGlyph() {
	img := info.Img
	var bitmap GlyphBitmap

	for y := 0; y < GlyphHeight; y++ {
		var rowWeight byte
		for x := 0; x < GlyphWidth; x++ {
			if img.AlphaAt(x, y).A > 64 {
				// Adjust bit position:
				// y counts from top to bottom (0 to GlyphHeight-1)
				// x counts from left to right (0 to GlyphWidth-1)
				// We need to reverse the bit order for each row, but
				// keep the row order as is
				heightShift := GlyphHeight - 1 - y
				widthShift := GlyphWidth - 1 - x
				bitmap |= 1 << (heightShift*GlyphWidth + widthShift)
				rowWeight++
			}
		}
		info.RowWeights[y] = rowWeight
		info.Weight += rowWeight
	}
	info.Bitmap = bitmap
	info.Weight = bitmap.popCount()
	info.RowWeights = calculateRowWeights(bitmap)
	info.EdgeMap = detectEdges(bitmap)
	info.ZoneWeights = calculateZoneWeights(bitmap)
}

func renderGlyph(ttf *truetype.Font, r rune) *GlyphInfo {
	face := truetype.NewFace(
		ttf, &truetype.Options{
			Size:    float64(GlyphHeight),
			DPI:     72,
			Hinting: font.HintingFull,
		},
	)

	img := image.NewAlpha(image.Rect(0, 0, GlyphWidth, GlyphHeight))
	d := &font.Drawer{
		Dst:  img,
		Src:  image.White,
		Face: face,
	}

	// Get glyph bounds and advance
	bounds, advance, _ := face.GlyphBounds(r)

	// Calculate horizontal centering offset
	xOffset := fixed.Int26_6((GlyphWidth*64 - advance) / 2)

	// Calculate vertical centering offset
	yOffset := face.Metrics().Ascent +
		fixed.Int26_6(GlyphHeight*64-(face.Metrics().Ascent+face.Metrics().Descent))

	// Set the drawing point
	d.Dot = fixed.Point26_6{
		X: xOffset,
		Y: yOffset,
	}

	d.DrawString(string(r))

	return &GlyphInfo{
		Rune:    r,
		Img:     img,
		Bounds:  bounds,
		Advance: advance,
	}
}

func analyzeFont(ttf *truetype.Font, safe *truetype.Font) []GlyphInfo {
	var glyphs []GlyphInfo
	//safeRunes := getSafeRunes()

	for r := rune(0); r <= 0xFFFF; r++ {
		if ttf.Index(r) != 0 && safe.Index(r) != 0 {
			// Check if the rune is printable
			if !unicode.IsPrint(r) {
				continue
			}
			glyph := renderGlyph(ttf, r)
			glyph.analyzeGlyph()
			// Remove empty glyphs that are not U+0020
			if glyph.Weight == 0 && r != 0x0020 {
				continue
			}
			glyphs = append(glyphs, *glyph)
		}
	}

	return glyphs
}

func debugPrintGlyph(glyph GlyphInfo) {
	fmt.Printf("Unicode: U+%04X\n", glyph.Rune)
	fmt.Printf("Character: %c\n", glyph.Rune)
	fmt.Printf("Weight: %d\n", glyph.Weight)
	fmt.Printf("Row Weights: %v\n", glyph.RowWeights)
	fmt.Sprintf("Hex: 0x%04X\n", uint64(glyph.Bitmap))
	fmt.Printf("Glyph Bitmap:\n%s\n", glyph.Bitmap.String())
	fmt.Println(strings.Repeat("-", 20))
}

func main() {
	font, err := loadFont("PxPlus_IBM_BIOS.ttf")
	safeFont, err := loadFont("FSD - PragmataProMono.ttf")
	if err != nil {
		panic(err)
	}
	glyphs := analyzeFont(font, safeFont)

	//glyphLookup := NewGlyphLookup(glyphs)

	// Test glyphLookup.FindClosestGlyph
	// Loop 100 times, generate a random glyph bitmap, find the closest glyph
	// and compare the weight
	//for i := 0; i < 100; i++ {
	//	// Generate a random glyph bitmap
	//	block := GlyphBitmap(0x0000000000000000)
	//	for j := 0; j < 64; j++ {
	//		if rand.Intn(2) == 1 {
	//			block |= 1 << uint(j)
	//		}
	//	}
	//
	//	// Find the closest glyph
	//	closestGlyph := glyphLookup.FindClosestGlyph(block)
	//
	//	// Show the two bitmaps
	//	println("Block:")
	//	println(block.String())
	//	println("Closest Glyph:")
	//	println(closestGlyph.Bitmap.String())
	//
	//	// Compare the weight
	//	if block.popCount() != closestGlyph.Weight {
	//		print("Weight does not match\n")
	//		println(block.String())
	//		println(closestGlyph.Bitmap.String())
	//		//panic("Weight does not match")
	//	}
	//}

	// Print debug information for each glyph
	for _, glyph := range glyphs {
		debugPrintGlyph(glyph)
	}
	print(len(glyphs), " glyphs analyzed\n")

}
