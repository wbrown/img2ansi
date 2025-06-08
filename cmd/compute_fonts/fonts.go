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
	"os"
	"sort"
	"strconv"
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
	// fmt.Printf("Number of candidates: %d\n", len(candidates)) // Debug print

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
		if math.Abs(float64(glyph.Weight)-float64(targetWeight)) <= 5 {
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

// FindClosestGlyphRestricted finds the closest matching glyph from a restricted set
func (gl *GlyphLookup) FindClosestGlyphRestricted(block GlyphBitmap, allowedChars map[rune]bool) GlyphInfo {
	blockInfo := extractFeatures(block)
	
	// Get all candidates but filter by allowed chars
	candidates := gl.getCandidatesByZones(blockInfo.ZoneWeights)
	
	var restrictedCandidates []*GlyphInfo
	for _, glyph := range candidates {
		if allowedChars[glyph.Rune] {
			restrictedCandidates = append(restrictedCandidates, glyph)
		}
	}
	
	// If no candidates in restricted set, check all allowed chars
	if len(restrictedCandidates) == 0 {
		for _, glyph := range gl.Glyphs {
			if allowedChars[glyph.Rune] {
				restrictedCandidates = append(restrictedCandidates, &glyph)
			}
		}
	}
	
	var bestMatch *GlyphInfo
	bestSimilarity := -1.0
	
	for _, glyph := range restrictedCandidates {
		similarity := calculateSimilarity(blockInfo, *glyph)
		if similarity > bestSimilarity {
			bestSimilarity = similarity
			bestMatch = glyph
		}
	}
	
	if bestMatch == nil {
		// Return space if no match found
		return GlyphInfo{Rune: ' '}
	}
	
	return *bestMatch
}

// Match represents a glyph match with its similarity score
type Match struct {
	Rune       rune
	Similarity float64
}

// FindTopNGlyphsRestricted finds the top N matching glyphs from a restricted set
func (gl *GlyphLookup) FindTopNGlyphsRestricted(block GlyphBitmap, allowedChars map[rune]bool, n int) []Match {
	blockInfo := extractFeatures(block)
	
	// Get all candidates but filter by allowed chars
	candidates := gl.getCandidatesByZones(blockInfo.ZoneWeights)
	
	var restrictedCandidates []*GlyphInfo
	for _, glyph := range candidates {
		if allowedChars[glyph.Rune] {
			restrictedCandidates = append(restrictedCandidates, glyph)
		}
	}
	
	// If no candidates in restricted set, check all allowed chars
	if len(restrictedCandidates) == 0 {
		for _, glyph := range gl.Glyphs {
			if allowedChars[glyph.Rune] {
				restrictedCandidates = append(restrictedCandidates, &glyph)
			}
		}
	}
	
	// Calculate similarities for all candidates
	matches := make([]Match, 0, len(restrictedCandidates))
	for _, glyph := range restrictedCandidates {
		similarity := calculateSimilarity(blockInfo, *glyph)
		matches = append(matches, Match{
			Rune:       glyph.Rune,
			Similarity: similarity,
		})
	}
	
	// Sort by similarity (highest first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Similarity > matches[j].Similarity
	})
	
	// Return top N
	if len(matches) > n {
		matches = matches[:n]
	}
	
	return matches
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

// popcount counts the number of set bits in a GlyphBitmap
func popcount(bitmap GlyphBitmap) int {
	count := 0
	for i := 0; i < 64; i++ {
		if (bitmap & (1 << i)) != 0 {
			count++
		}
	}
	return count
}

// analyzeGlyph is a helper function for tests that creates and analyzes a glyph
func analyzeGlyph(font *truetype.Font, r rune, size float64, analyze bool) *GlyphInfo {
	if font.Index(r) == 0 {
		return nil
	}
	glyph := renderGlyph(font, r)
	if glyph == nil {
		return nil
	}
	if analyze {
		glyph.analyzeGlyph()
	}
	return glyph
}

func calculateSimilarity(a, b GlyphInfo) float64 {
	// Simple Hamming distance - count matching bits
	matching := ^(a.Bitmap ^ b.Bitmap)  // XOR then NOT gives matching bits
	return float64(bits.OnesCount64(uint64(matching))) / 64.0
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
	// Special handling for circular patterns
	aCircular := isLikelyCircular(a)
	bCircular := isLikelyCircular(b)
	
	if aCircular && bCircular {
		// Both circular - compare based on size and center alignment
		return compareCircularPatterns(a, b)
	} else if aCircular != bCircular {
		// One circular, one not - reduce similarity significantly
		// This helps ensure circular patterns match circular characters
		return calculateGeneralShapeSimilarity(a, b) * 0.7
	}
	
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

	// If only one is a diagonal line, reduce similarity
	if (aDiagonal != DiagonalNone && bDiagonal == DiagonalNone) ||
		(aDiagonal == DiagonalNone && bDiagonal != DiagonalNone) {
		return calculateGeneralShapeSimilarity(a, b) * 0.5
	}

	return calculateGeneralShapeSimilarity(a, b)
}

// General shape similarity calculation
func calculateGeneralShapeSimilarity(a, b GlyphBitmap) float64 {
	aFeatures := extractShapeFeatures(a)
	bFeatures := extractShapeFeatures(b)

	// Calculate similarity based on features
	return 1 - euclideanDistance(aFeatures, bFeatures)/math.Sqrt(float64(len(aFeatures)))
}

// Compare two circular patterns
func compareCircularPatterns(a, b GlyphBitmap) float64 {
	// Compare radius (approximate by counting pixels)
	aCount := float64(bits.OnesCount64(uint64(a)))
	bCount := float64(bits.OnesCount64(uint64(b)))
	
	sizeSimilarity := 1.0 - math.Abs(aCount-bCount)/(aCount+bCount)
	
	// Compare center of mass
	aCenterX, aCenterY := calculateCenterOfMass(a)
	bCenterX, bCenterY := calculateCenterOfMass(b)
	
	centerDistance := math.Sqrt(math.Pow(aCenterX-bCenterX, 2) + math.Pow(aCenterY-bCenterY, 2))
	centerSimilarity := 1.0 - centerDistance/math.Sqrt(float64(GlyphWidth*GlyphWidth+GlyphHeight*GlyphHeight))
	
	// Compare symmetry
	aSymmetry := (calculateHorizontalSymmetry(a) + calculateVerticalSymmetry(a)) / 2
	bSymmetry := (calculateHorizontalSymmetry(b) + calculateVerticalSymmetry(b)) / 2
	symmetrySimilarity := 1.0 - math.Abs(aSymmetry-bSymmetry)
	
	return 0.4*sizeSimilarity + 0.3*centerSimilarity + 0.3*symmetrySimilarity
}

// Calculate center of mass
func calculateCenterOfMass(bitmap GlyphBitmap) (float64, float64) {
	var sumX, sumY float64
	count := 0
	
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
		return float64(GlyphWidth)/2, float64(GlyphHeight)/2
	}
	
	return sumX/float64(count), sumY/float64(count)
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
		diff := math.Abs(float64(a[i]) - float64(b[i]))
		totalDiff += diff
	}
	maxPossibleDiff := float64(NumZones * ZoneSize * ZoneSize)
	return 1 - (totalDiff / maxPossibleDiff)
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
			if g&(1<<(y*GlyphWidth+x)) != 0 {
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
				// Use standard row-major bit ordering:
				// bit position = y * width + x
				// This ensures bit 0 = (0,0) and bit 63 = (7,7)
				bitmap |= 1 << (y*GlyphWidth + x)
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

// Calculate character simplicity based on bitmap properties
func calculateCharacterSimplicity(bitmap GlyphBitmap) float64 {
	// Connected components (fewer = simpler)
	components := float64(countConnectedComponents(bitmap))
	if components == 0 {
		components = 1
	}
	
	// Edge-to-fill ratio (lower = simpler solid shapes)
	edges := float64(bits.OnesCount64(uint64(detectEdges(bitmap))))
	fills := float64(bits.OnesCount64(uint64(bitmap)))
	if fills == 0 {
		return 0.5 // Empty bitmap is somewhat simple
	}
	edgeRatio := edges / fills
	
	// Symmetry (more symmetric = simpler)
	symmetry := calculateSymmetryScore(bitmap)
	
	// Combine metrics
	return (1.0 / components) * symmetry * (2.0 - edgeRatio)
}

// Measure how coherent/organized a pattern is
func calculatePatternCoherence(bitmap GlyphBitmap) float64 {
	// Compactness (pixels clustered vs scattered)
	compactness := calculateCompactness(bitmap)
	
	// Directional consistency
	directionality := calculateDirectionalConsistency(bitmap)
	
	// Distribution variance
	distribution := calculateDistribution(bitmap)
	variance := calculateDistributionVariance(distribution)
	
	return compactness * directionality / (1.0 + variance)
}

// Use Unicode codepoint as proxy for commonality
func getCodepointCommonality(r rune) float64 {
	// Remove ASCII bias - all characters are equal
	return 1.0
}

// Boost characters that match key structural features
func calculateStructuralBonus(input, candidate GlyphBitmap) float64 {
	// Check if both have same primary orientation
	inputOrientation := detectPrimaryOrientation(input)
	candidateOrientation := detectPrimaryOrientation(candidate)
	
	if inputOrientation == candidateOrientation && inputOrientation != OrientationNone {
		return 1.03 // Reduced from 1.1 to prevent overriding better shape matches
	}
	return 1.0
}

// Give semantic bonus to likely character matches based on pattern type
func calculateSemanticBonus(input GlyphBitmap, candidateRune rune) float64 {
	// Check if input looks circular
	if isLikelyCircular(input) {
		// Boost O, 0, and other circular characters
		switch candidateRune {
		case 'O', 'o', '0', 'Q', 'C', 'G', '@':
			return 1.05
		}
	}
	
	// Check for diagonal patterns
	diagonal := detectDiagonalLine(input)
	if diagonal == DiagonalTopLeftToBottomRight && candidateRune == '\\' {
		return 1.05
	} else if diagonal == DiagonalTopRightToBottomLeft && candidateRune == '/' {
		return 1.05
	}
	
	// Check for line patterns
	orientation := detectPrimaryOrientation(input)
	if orientation == OrientationHorizontal && (candidateRune == '-' || candidateRune == '_' || candidateRune == '=' || candidateRune == '—') {
		return 1.05
	} else if orientation == OrientationVertical && (candidateRune == '|' || candidateRune == 'I' || candidateRune == 'l' || candidateRune == '1') {
		return 1.05
	}
	
	// Check for cross/plus patterns
	if isCrossPattern(input) && (candidateRune == '+' || candidateRune == 'x' || candidateRune == 'X' || candidateRune == '*') {
		return 1.05
	}
	
	return 1.0
}

// Check if pattern looks like a cross/plus
func isCrossPattern(bitmap GlyphBitmap) bool {
	// Check for strong central vertical and horizontal lines
	hasVertical := false
	hasHorizontal := false
	
	// Check middle column
	middleCol := 0
	for y := 0; y < GlyphHeight; y++ {
		if getBit(bitmap, GlyphWidth/2, y) {
			middleCol++
		}
	}
	hasVertical = middleCol >= GlyphHeight*2/3
	
	// Check middle row
	middleRow := 0
	for x := 0; x < GlyphWidth; x++ {
		if getBit(bitmap, x, GlyphHeight/2) {
			middleRow++
		}
	}
	hasHorizontal = middleRow >= GlyphWidth*2/3
	
	return hasVertical && hasHorizontal
}

// Count connected components using flood fill
func countConnectedComponents(bitmap GlyphBitmap) int {
	visited := uint64(0)
	components := 0
	
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			pos := y*GlyphWidth + x
			if getBit(bitmap, x, y) && (visited&(1<<pos)) == 0 {
				components++
				// Flood fill this component
				floodFill(bitmap, x, y, &visited)
			}
		}
	}
	
	return components
}

// Flood fill helper
func floodFill(bitmap GlyphBitmap, x, y int, visited *uint64) {
	if x < 0 || x >= GlyphWidth || y < 0 || y >= GlyphHeight {
		return
	}
	
	pos := y*GlyphWidth + x
	if !getBit(bitmap, x, y) || (*visited&(1<<pos)) != 0 {
		return
	}
	
	*visited |= 1 << pos
	
	// 8-connectivity
	floodFill(bitmap, x-1, y, visited)
	floodFill(bitmap, x+1, y, visited)
	floodFill(bitmap, x, y-1, visited)
	floodFill(bitmap, x, y+1, visited)
	floodFill(bitmap, x-1, y-1, visited)
	floodFill(bitmap, x+1, y-1, visited)
	floodFill(bitmap, x-1, y+1, visited)
	floodFill(bitmap, x+1, y+1, visited)
}

// Calculate symmetry score (0-1)
func calculateSymmetryScore(bitmap GlyphBitmap) float64 {
	horizontalSym := calculateHorizontalSymmetry(bitmap)
	verticalSym := calculateVerticalSymmetry(bitmap)
	
	// Return the better symmetry
	if horizontalSym > verticalSym {
		return horizontalSym
	}
	return verticalSym
}

func calculateHorizontalSymmetry(bitmap GlyphBitmap) float64 {
	matches := 0
	total := 0
	
	for y := 0; y < GlyphHeight/2; y++ {
		for x := 0; x < GlyphWidth; x++ {
			topBit := getBit(bitmap, x, y)
			bottomBit := getBit(bitmap, x, GlyphHeight-1-y)
			if topBit == bottomBit {
				matches++
			}
			total++
		}
	}
	
	return float64(matches) / float64(total)
}

func calculateVerticalSymmetry(bitmap GlyphBitmap) float64 {
	matches := 0
	total := 0
	
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth/2; x++ {
			leftBit := getBit(bitmap, x, y)
			rightBit := getBit(bitmap, GlyphWidth-1-x, y)
			if leftBit == rightBit {
				matches++
			}
			total++
		}
	}
	
	return float64(matches) / float64(total)
}

// Calculate how compact/clustered the pixels are
func calculateCompactness(bitmap GlyphBitmap) float64 {
	if bitmap == 0 {
		return 1.0 // Empty is perfectly compact
	}
	
	// Find bounding box
	minX, minY := GlyphWidth, GlyphHeight
	maxX, maxY := -1, -1
	
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) {
				if x < minX { minX = x }
				if x > maxX { maxX = x }
				if y < minY { minY = y }
				if y > maxY { maxY = y }
			}
		}
	}
	
	if maxX < 0 {
		return 1.0 // No pixels
	}
	
	// Calculate density within bounding box
	boxWidth := maxX - minX + 1
	boxHeight := maxY - minY + 1
	boxArea := boxWidth * boxHeight
	
	pixelCount := bits.OnesCount64(uint64(bitmap))
	
	return float64(pixelCount) / float64(boxArea)
}

// Calculate directional consistency
func calculateDirectionalConsistency(bitmap GlyphBitmap) float64 {
	if bitmap == 0 {
		return 1.0
	}
	
	// Calculate gradient directions for each pixel
	var dx, dy float64
	count := 0
	
	for y := 1; y < GlyphHeight-1; y++ {
		for x := 1; x < GlyphWidth-1; x++ {
			if getBit(bitmap, x, y) {
				// Simple gradient
				if getBit(bitmap, x+1, y) != getBit(bitmap, x-1, y) {
					if getBit(bitmap, x+1, y) {
						dx += 1
					} else {
						dx -= 1
					}
				}
				if getBit(bitmap, x, y+1) != getBit(bitmap, x, y-1) {
					if getBit(bitmap, x, y+1) {
						dy += 1
					} else {
						dy -= 1
					}
				}
				count++
			}
		}
	}
	
	if count == 0 {
		return 1.0
	}
	
	// Normalize and calculate consistency
	dx /= float64(count)
	dy /= float64(count)
	
	magnitude := math.Sqrt(dx*dx + dy*dy)
	
	// Higher magnitude means more consistent direction
	return math.Min(magnitude, 1.0)
}

// Calculate variance of distribution
func calculateDistributionVariance(dist [4]float64) float64 {
	mean := (dist[0] + dist[1] + dist[2] + dist[3]) / 4.0
	
	variance := 0.0
	for i := 0; i < 4; i++ {
		diff := dist[i] - mean
		variance += diff * diff
	}
	
	return variance / 4.0
}

// Orientation detection
const (
	OrientationNone = iota
	OrientationHorizontal
	OrientationVertical
	OrientationDiagonal
)

// Check if a bitmap is likely a circular or curved pattern
func isLikelyCircular(bitmap GlyphBitmap) bool {
	// Count corner pixels vs center pixels
	cornerCount := 0
	centerCount := 0
	
	// Check corners (should be empty for circles)
	corners := [][2]int{{0,0}, {0,1}, {1,0}, {6,0}, {7,0}, {7,1}, {0,6}, {0,7}, {1,7}, {6,7}, {7,6}, {7,7}}
	for _, c := range corners {
		if getBit(bitmap, c[0], c[1]) {
			cornerCount++
		}
	}
	
	// Check center area (should have pixels for circles)
	for y := 2; y < 6; y++ {
		for x := 2; x < 6; x++ {
			if getBit(bitmap, x, y) {
				centerCount++
			}
		}
	}
	
	// High symmetry score
	horizontalSym := calculateHorizontalSymmetry(bitmap)
	verticalSym := calculateVerticalSymmetry(bitmap)
	
	// Circles have high symmetry, few corner pixels
	// For hollow circles, we need to check if there's a ring pattern
	// A completely hollow circle might have 0 center pixels
	totalPixels := bits.OnesCount64(uint64(bitmap))
	
	// Check if this is likely a line pattern (all pixels in one row or column)
	isVerticalLine := true
	isHorizontalLine := true
	for y := 0; y < GlyphHeight; y++ {
		rowCount := horizontalProjection(bitmap, y)
		if rowCount > 0 && rowCount < GlyphWidth*3/4 {
			isHorizontalLine = false
		}
	}
	for x := 0; x < GlyphWidth; x++ {
		colCount := verticalProjection(bitmap, x)
		if colCount > 0 && colCount < GlyphHeight*3/4 {
			isVerticalLine = false
		}
	}
	
	// If it's a pure line, it's not circular
	if isVerticalLine || isHorizontalLine {
		return false
	}
	
	// Check for ring pattern - pixels around the edge but not in center
	isRingPattern := totalPixels >= 16 && totalPixels <= 32 && centerCount <= 4
	
	// Original checks for partially filled circles
	isHollow := centerCount >= 2 && centerCount <= 8
	isFilled := centerCount >= 8
	
	// Relax symmetry requirements for real fonts
	// Many font O characters are slightly asymmetric
	avgSymmetry := (horizontalSym + verticalSym) / 2
	
	return cornerCount <= 3 && (isRingPattern || isHollow || isFilled) && avgSymmetry > 0.65
}

func detectPrimaryOrientation(bitmap GlyphBitmap) int {
	// First check if this might be a circular/curved pattern
	if isLikelyCircular(bitmap) {
		return OrientationNone // Circles have no primary orientation
	}
	
	// Check for strong horizontal lines (consecutive rows)
	horizontalStrength := 0
	consecutiveHorizontal := 0
	for y := 0; y < GlyphHeight; y++ {
		rowCount := horizontalProjection(bitmap, y)
		if rowCount >= GlyphWidth*3/4 {
			consecutiveHorizontal++
			if consecutiveHorizontal > horizontalStrength {
				horizontalStrength = consecutiveHorizontal
			}
		} else {
			consecutiveHorizontal = 0
		}
	}
	
	// Check for strong vertical lines (consecutive columns)
	verticalStrength := 0
	consecutiveVertical := 0
	for x := 0; x < GlyphWidth; x++ {
		colCount := verticalProjection(bitmap, x)
		if colCount >= GlyphHeight*3/4 {
			consecutiveVertical++
			if consecutiveVertical > verticalStrength {
				verticalStrength = consecutiveVertical
			}
		} else {
			consecutiveVertical = 0
		}
	}
	
	// Check for diagonal
	diagonalType := detectDiagonalLine(bitmap)
	
	if diagonalType != DiagonalNone {
		return OrientationDiagonal
	} else if horizontalStrength >= 2 && horizontalStrength > verticalStrength {
		// Require at least 2 consecutive strong rows for horizontal
		return OrientationHorizontal
	} else if verticalStrength >= 2 {
		// Require at least 2 consecutive strong columns for vertical
		return OrientationVertical
	}
	
	return OrientationNone
}

// debugRenderGlyph shows step-by-step rendering of a glyph
func debugRenderGlyph(ttf *truetype.Font, r rune) {
	fmt.Printf("\n=== Debug rendering of '%c' (U+%04X) ===\n", r, r)
	
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
	fmt.Printf("Glyph bounds: %v\n", bounds)
	fmt.Printf("Advance: %v\n", advance)
	
	// Calculate horizontal centering offset
	xOffset := fixed.Int26_6((GlyphWidth*64 - advance) / 2)
	
	// Calculate vertical centering offset
	yOffset := face.Metrics().Ascent +
		fixed.Int26_6(GlyphHeight*64-(face.Metrics().Ascent+face.Metrics().Descent))
	
	fmt.Printf("Face metrics - Ascent: %v, Descent: %v\n", face.Metrics().Ascent, face.Metrics().Descent)
	fmt.Printf("X offset: %v, Y offset: %v\n", xOffset, yOffset)
	
	// Set the drawing point
	d.Dot = fixed.Point26_6{
		X: xOffset,
		Y: yOffset,
	}
	
	d.DrawString(string(r))
	
	// Show the raw image
	fmt.Println("\nRaw image pixels (Alpha values):")
	for y := 0; y < GlyphHeight; y++ {
		fmt.Printf("Row %d: ", y)
		for x := 0; x < GlyphWidth; x++ {
			alpha := img.AlphaAt(x, y).A
			fmt.Printf("%3d ", alpha)
		}
		fmt.Println()
	}
	
	// Show which pixels would be set
	fmt.Println("\nThresholded (>64):")
	var bitmap GlyphBitmap
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if img.AlphaAt(x, y).A > 64 {
				fmt.Print("█")
				bitmap |= 1 << (y*GlyphWidth + x)
			} else {
				fmt.Print("·")
			}
		}
		fmt.Println()
	}
	
	fmt.Printf("\nResulting bitmap: %064b\n", uint64(bitmap))
}

func renderGlyphSimple(ttf *truetype.Font, r rune) GlyphBitmap {
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
	_, advance, _ := face.GlyphBounds(r)
	
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
	
	// Convert to bitmap
	var bitmap GlyphBitmap
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if img.AlphaAt(x, y).A > 64 {
				bitmap |= 1 << (y*GlyphWidth + x)
			}
		}
	}
	
	return bitmap
}

func runAllExperiments() {
	experiments := []struct {
		name        string
		configName  string
	}{
		{"style", "original"},
		{"quantized", "quantized"},
		{"diffusion", "diffusion"},
		{"smart-diffusion", "smart"},
		{"kmeans-diffusion", "kmeans"},
		{"compare-diffusion", "compare"},
		{"contrast-diffusion", "contrast"},
		{"no-space", "no-space"},
		{"patterns-only", "patterns"},
		{"optimized", "optimized"},
		{"exhaustive", "exhaustive"},
		{"true-exhaustive-full", "true-exhaustive-full"},
	}
	
	paletteSizes := []int{16, 256}
	
	fmt.Println("Running all experiments with both 16 and 256 color palettes...")
	fmt.Println("=" + strings.Repeat("=", 60))
	
	successCount := 0
	failureCount := 0
	totalExperiments := len(experiments) * len(paletteSizes)
	currentExperiment := 0
	
	for _, paletteSize := range paletteSizes {
		fmt.Printf("\n=== %d COLOR PALETTE ===\n", paletteSize)
		
		for _, exp := range experiments {
			currentExperiment++
			fmt.Printf("\n[%d/%d] Running %s (%d colors)...\n", currentExperiment, totalExperiments, exp.name, paletteSize)
			fmt.Println("-" + strings.Repeat("-", 40))
			
			err := RunBrownExperimentWithPalette(exp.configName, paletteSize)
			if err != nil {
				fmt.Printf("ERROR: %s (%d colors) failed: %v\n", exp.name, paletteSize, err)
				failureCount++
			} else {
				fmt.Printf("SUCCESS: %s (%d colors) completed\n", exp.name, paletteSize)
				successCount++
			}
		}
	}
	
	fmt.Println("\n" + "=" + strings.Repeat("=", 60))
	fmt.Printf("All experiments completed: %d successful, %d failed\n", successCount, failureCount)
	
	// Optionally render all results
	fmt.Println("\nRendering all results to PNG...")
	RenderAllToPNG()
}

func printUsage() {
	fmt.Println("Usage: compute_fonts [command]")
	fmt.Println("\nExperiment Commands:")
	fmt.Println("  style              - Original Brown dithering using dominant color extraction")
	fmt.Println("  quantized          - Brown dithering with pre-quantized color space")
	fmt.Println("  diffusion          - Brown dithering with Floyd-Steinberg error diffusion")
	fmt.Println("  smart-diffusion    - Enhanced error diffusion with edge detection")
	fmt.Println("  kmeans-diffusion   - Error diffusion using k-means color clustering")
	fmt.Println("  compare-diffusion  - Compare different diffusion strategies")
	fmt.Println("  contrast-diffusion - Maximize contrast between foreground/background")
	fmt.Println("  no-space           - Force pattern usage by excluding space character")
	fmt.Println("  patterns-only      - Exclude both space and full block characters")
	fmt.Println("  optimized          - Optimized Brown dithering with caching")
	fmt.Println("  exhaustive         - Test all possible color pairs (slow but thorough)")
	fmt.Println("  true-exhaustive-full - TRUE exhaustive on mandrill (first 100 blocks, VERY slow)")
	fmt.Println("  all                - Run all experiments sequentially (both 16 & 256 colors)")
	fmt.Println("  brown [name] [palette] - Run specific Brown experiment (palette: 16 or 256)")
	fmt.Println("\n=== DEBUGGING & ANALYSIS TOOLS ===")
	fmt.Println("These are not experiments but tools for understanding the algorithms:")
	fmt.Println("\nExhaustive Algorithm Analysis:")
	fmt.Println("  true-exhaustive    - Demo of true exhaustive (10 blocks only)")
	fmt.Println("  debug-brown        - Debug color selection on a single block")
	fmt.Println("  compare-exhaustive - Compare current vs true exhaustive approaches")
	fmt.Println("  visual-exhaustive  - Visual comparison of exhaustive methods on actual block")
	fmt.Println("  stress-exhaustive  - Stress test to find differences between methods")
	fmt.Println("  visual-output-exhaustive - Generate test pattern comparisons")
	fmt.Println("\nRendering Commands:")
	fmt.Println("  render             - Render all ANSI files to PNG using font rendering")
	fmt.Println("  render-unified     - Render using unified rendering pipeline")
}

func main() {
	// Run test based on command line argument
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "style":
			RunBrownExperiment("original")
		case "quantized":
			RunBrownExperiment("quantized")
		case "diffusion":
			RunBrownExperiment("diffusion")
		case "smart-diffusion":
			RunBrownExperiment("smart")
		case "kmeans-diffusion":
			RunBrownExperiment("kmeans")
		case "compare-diffusion":
			RunBrownExperiment("compare")
		case "contrast-diffusion":
			RunBrownExperiment("contrast")
		case "no-space":
			RunBrownExperiment("no-space")
		case "patterns-only":
			RunBrownExperiment("patterns")
		case "optimized":
			RunBrownExperiment("optimized")
		case "exhaustive":
			RunBrownExperiment("exhaustive")
		case "true-exhaustive-full":
			RunBrownExperiment("true-exhaustive-full")
		case "all":
			runAllExperiments()
		case "render":
			RenderAllToPNG()
		case "render-unified":
			RenderAllToPNGUnified()
		case "debug-brown":
			DebugBrownDithering()
		case "compare-exhaustive":
			CompareExhaustiveApproaches()
		case "visual-exhaustive":
			if err := VisualExhaustiveComparison(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "stress-exhaustive":
			if err := ExhaustiveStressTest(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "visual-output-exhaustive":
			if err := ExhaustiveVisualOutput(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "test256":
			fmt.Println("Run 'go test -v -run TestPaletteMapping' to test palette mapping")
		case "debug-palette":
			if err := DebugPalette(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "test-color-render":
			if err := TestColorRender(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "debug-ansi-colors":
			if err := DebugANSIColors(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "debug-specific-colors":
			if err := DebugSpecificColors(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "debug-first-line":
			if err := DebugFirstLine(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "test-single-char":
			if err := TestSingleChar(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "debug-render-process":
			if err := DebugRenderProcess(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "debug-ansi-parser":
			if err := DebugANSIParser(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "true-exhaustive":
			// Run true exhaustive search demonstration
			resized, err := LoadMandrillImage()
			if err != nil {
				fmt.Printf("Error loading image: %v\n", err)
				return
			}
			lookup := LoadFontGlyphs()
			if err := TrueExhaustiveBrownDithering(resized, lookup); err != nil {
				fmt.Printf("Error running true exhaustive: %v\n", err)
			}
		case "brown":
			// New consolidated brown experiments
			if len(os.Args) > 2 {
				// Check if third argument is palette size
				if len(os.Args) > 3 && (os.Args[3] == "256" || os.Args[3] == "16") {
					paletteSize, _ := strconv.Atoi(os.Args[3])
					RunBrownExperimentWithPalette(os.Args[2], paletteSize)
				} else {
					RunBrownExperiment(os.Args[2])
				}
			} else {
				fmt.Println("Brown experiments: original, exhaustive, diffusion, optimized, patterns")
				fmt.Println("Usage: compute_fonts brown [experiment-name] [palette-size]")
				fmt.Println("  palette-size: 16 (default) or 256")
			}
		default:
			printUsage()
		}
	} else {
		printUsage()
	}
}
