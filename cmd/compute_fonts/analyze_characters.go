package main

import (
	"fmt"
	"math"
	"math/bits"
	"strings"
)

// CharacterProperties holds analyzed properties of a glyph
type CharacterProperties struct {
	Rune               rune
	Bitmap             GlyphBitmap
	Density            float64  // Percentage of pixels filled
	EdgeDensity        float64  // Percentage of edge pixels
	ConnectedComponents int      // Number of separate shapes
	HasHoles           bool     // Contains enclosed empty spaces
	Symmetry           struct {
		Horizontal float64
		Vertical   float64
		Diagonal   float64
		Rotational float64
	}
	Distribution       [4]float64 // Quadrant distribution [TL, TR, BL, BR]
	CenterOfMass       [2]float64 // X, Y coordinates (0-1 normalized)
	BoundingBox        struct {
		MinX, MinY, MaxX, MaxY int
		Width, Height          int
	}
	Orientation        string   // "horizontal", "vertical", "diagonal", "circular", "none"
	PatternType        string   // "solid", "outline", "dotted", "striped", "gradient", "complex"
	PrimaryDirection   float64  // Angle in radians (-π to π)
	AspectRatio        float64  // Width/Height of bounding box
	Compactness        float64  // How tightly packed the pixels are
	EdgeToFillRatio    float64  // Edge pixels / total pixels
	CornerPixels       int      // Pixels in the 4 corners
	CenterPixels       int      // Pixels in center 4x4 area
}

// AnalyzeCharacter performs comprehensive analysis on a glyph
func AnalyzeCharacter(info GlyphInfo) CharacterProperties {
	props := CharacterProperties{
		Rune:   info.Rune,
		Bitmap: info.Bitmap,
	}
	
	// Basic density
	totalPixels := bits.OnesCount64(uint64(info.Bitmap))
	props.Density = float64(totalPixels) / 64.0
	
	// Edge analysis
	edgeMap := detectEdges(info.Bitmap)
	edgePixels := bits.OnesCount64(uint64(edgeMap))
	props.EdgeDensity = float64(edgePixels) / 64.0
	if totalPixels > 0 {
		props.EdgeToFillRatio = float64(edgePixels) / float64(totalPixels)
	}
	
	// Connected components and holes
	props.ConnectedComponents = countConnectedComponents(info.Bitmap)
	props.HasHoles = detectHoles(info.Bitmap)
	
	// Symmetry analysis
	props.Symmetry.Horizontal = calculateHorizontalSymmetry(info.Bitmap)
	props.Symmetry.Vertical = calculateVerticalSymmetry(info.Bitmap)
	props.Symmetry.Diagonal = calculateDiagonalSymmetry(info.Bitmap)
	props.Symmetry.Rotational = calculateRotationalSymmetry(info.Bitmap)
	
	// Distribution and center of mass
	props.Distribution = calculateDistribution(info.Bitmap)
	centerX, centerY := calculateCenterOfMass(info.Bitmap)
	props.CenterOfMass = [2]float64{centerX / 8.0, centerY / 8.0}
	
	// Bounding box
	props.BoundingBox = calculateBoundingBox(info.Bitmap)
	if props.BoundingBox.Width > 0 && props.BoundingBox.Height > 0 {
		props.AspectRatio = float64(props.BoundingBox.Width) / float64(props.BoundingBox.Height)
		props.Compactness = float64(totalPixels) / float64(props.BoundingBox.Width * props.BoundingBox.Height)
	}
	
	// Corner and center analysis
	props.CornerPixels = countCornerPixels(info.Bitmap)
	props.CenterPixels = countCenterPixels(info.Bitmap)
	
	// Pattern detection
	props.Orientation = detectOrientation(info.Bitmap, props)
	props.PatternType = detectPatternType(info.Bitmap, props)
	props.PrimaryDirection = calculatePrimaryDirection(info.Bitmap)
	
	return props
}

// Calculate bounding box
func calculateBoundingBox(bitmap GlyphBitmap) struct {
	MinX, MinY, MaxX, MaxY int
	Width, Height          int
} {
	box := struct {
		MinX, MinY, MaxX, MaxY int
		Width, Height          int
	}{MinX: 8, MinY: 8, MaxX: -1, MaxY: -1}
	
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) {
				if x < box.MinX { box.MinX = x }
				if x > box.MaxX { box.MaxX = x }
				if y < box.MinY { box.MinY = y }
				if y > box.MaxY { box.MaxY = y }
			}
		}
	}
	
	if box.MaxX >= 0 {
		box.Width = box.MaxX - box.MinX + 1
		box.Height = box.MaxY - box.MinY + 1
	}
	
	return box
}

// Detect holes using flood fill from edges
func detectHoles(bitmap GlyphBitmap) bool {
	// Flood fill from outside
	visited := uint64(0)
	
	// Start flood fill from all edge pixels that are empty
	for x := 0; x < GlyphWidth; x++ {
		if !getBit(bitmap, x, 0) {
			floodFillEmpty(bitmap, x, 0, &visited)
		}
		if !getBit(bitmap, x, GlyphHeight-1) {
			floodFillEmpty(bitmap, x, GlyphHeight-1, &visited)
		}
	}
	
	for y := 1; y < GlyphHeight-1; y++ {
		if !getBit(bitmap, 0, y) {
			floodFillEmpty(bitmap, 0, y, &visited)
		}
		if !getBit(bitmap, GlyphWidth-1, y) {
			floodFillEmpty(bitmap, GlyphWidth-1, y, &visited)
		}
	}
	
	// Check if there are any empty pixels not visited (holes)
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			pos := y*GlyphWidth + x
			if !getBit(bitmap, x, y) && (visited&(1<<pos)) == 0 {
				return true
			}
		}
	}
	
	return false
}

// Flood fill empty spaces
func floodFillEmpty(bitmap GlyphBitmap, x, y int, visited *uint64) {
	if x < 0 || x >= GlyphWidth || y < 0 || y >= GlyphHeight {
		return
	}
	
	pos := y*GlyphWidth + x
	if getBit(bitmap, x, y) || (*visited&(1<<pos)) != 0 {
		return
	}
	
	*visited |= 1 << pos
	
	// 4-connectivity for empty space fill
	floodFillEmpty(bitmap, x-1, y, visited)
	floodFillEmpty(bitmap, x+1, y, visited)
	floodFillEmpty(bitmap, x, y-1, visited)
	floodFillEmpty(bitmap, x, y+1, visited)
}

// Calculate diagonal symmetry
func calculateDiagonalSymmetry(bitmap GlyphBitmap) float64 {
	matches := 0
	total := 0
	
	// Top-left to bottom-right diagonal symmetry
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if x != y { // Skip the diagonal itself
				bit1 := getBit(bitmap, x, y)
				bit2 := getBit(bitmap, y, x)
				if bit1 == bit2 {
					matches++
				}
				total++
			}
		}
	}
	
	return float64(matches) / float64(total)
}

// Calculate rotational symmetry (180 degree)
func calculateRotationalSymmetry(bitmap GlyphBitmap) float64 {
	matches := 0
	total := 0
	
	for y := 0; y < GlyphHeight/2; y++ {
		for x := 0; x < GlyphWidth; x++ {
			bit1 := getBit(bitmap, x, y)
			bit2 := getBit(bitmap, GlyphWidth-1-x, GlyphHeight-1-y)
			if bit1 == bit2 {
				matches++
			}
			total++
		}
	}
	
	return float64(matches) / float64(total)
}

// Count pixels in corners
func countCornerPixels(bitmap GlyphBitmap) int {
	count := 0
	corners := [][2]int{{0,0}, {0,1}, {1,0}, {6,0}, {7,0}, {7,1}, {0,6}, {0,7}, {1,7}, {6,7}, {7,6}, {7,7}}
	
	for _, c := range corners {
		if getBit(bitmap, c[0], c[1]) {
			count++
		}
	}
	
	return count
}

// Count pixels in center
func countCenterPixels(bitmap GlyphBitmap) int {
	count := 0
	
	for y := 2; y < 6; y++ {
		for x := 2; x < 6; x++ {
			if getBit(bitmap, x, y) {
				count++
			}
		}
	}
	
	return count
}

// Detect orientation based on properties
func detectOrientation(bitmap GlyphBitmap, props CharacterProperties) string {
	// Check for circular patterns first
	if props.CornerPixels <= 3 && props.CenterPixels >= 4 && 
	   props.Symmetry.Horizontal > 0.7 && props.Symmetry.Vertical > 0.7 {
		return "circular"
	}
	
	// Check for strong lines
	strongHorizontal := false
	strongVertical := false
	
	for y := 0; y < GlyphHeight; y++ {
		if horizontalProjection(bitmap, y) >= GlyphWidth*3/4 {
			strongHorizontal = true
			break
		}
	}
	
	for x := 0; x < GlyphWidth; x++ {
		if verticalProjection(bitmap, x) >= GlyphHeight*3/4 {
			strongVertical = true
			break
		}
	}
	
	// Check for diagonals
	diagonal := detectDiagonalLine(bitmap)
	if diagonal != DiagonalNone {
		return "diagonal"
	}
	
	if strongHorizontal && !strongVertical {
		return "horizontal"
	} else if strongVertical && !strongHorizontal {
		return "vertical"
	}
	
	return "none"
}

// Detect pattern type
func detectPatternType(bitmap GlyphBitmap, props CharacterProperties) string {
	// Solid fill
	if props.Density > 0.7 && props.EdgeToFillRatio < 0.3 {
		return "solid"
	}
	
	// Outline
	if props.HasHoles || (props.EdgeToFillRatio > 0.7 && props.Density < 0.5) {
		return "outline"
	}
	
	// Dotted/scattered
	if props.ConnectedComponents > 4 {
		return "dotted"
	}
	
	// Check for regular patterns
	if hasRegularStripes(bitmap) {
		return "striped"
	}
	
	// Gradient-like (density changes across quadrants)
	variance := calculateDistributionVariance(props.Distribution)
	if variance > 0.1 && props.Density > 0.3 && props.Density < 0.7 {
		return "gradient"
	}
	
	return "complex"
}

// Check for regular stripes
func hasRegularStripes(bitmap GlyphBitmap) bool {
	// Check horizontal stripes
	horizontalPattern := true
	for y := 0; y < GlyphHeight-2; y += 2 {
		row1 := horizontalProjection(bitmap, y)
		row2 := horizontalProjection(bitmap, y+1)
		if math.Abs(float64(row1-row2)) > 2 {
			horizontalPattern = false
			break
		}
	}
	
	// Check vertical stripes
	verticalPattern := true
	for x := 0; x < GlyphWidth-2; x += 2 {
		col1 := verticalProjection(bitmap, x)
		col2 := verticalProjection(bitmap, x+1)
		if math.Abs(float64(col1-col2)) > 2 {
			verticalPattern = false
			break
		}
	}
	
	return horizontalPattern || verticalPattern
}

// Calculate primary direction using moment analysis
func calculatePrimaryDirection(bitmap GlyphBitmap) float64 {
	// Calculate image moments
	var m00, m10, m01, m20, m02, m11 float64
	
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if getBit(bitmap, x, y) {
				m00 += 1
				m10 += float64(x)
				m01 += float64(y)
				m20 += float64(x * x)
				m02 += float64(y * y)
				m11 += float64(x * y)
			}
		}
	}
	
	if m00 == 0 {
		return 0
	}
	
	// Central moments
	xc := m10 / m00
	yc := m01 / m00
	
	mu20 := m20/m00 - xc*xc
	mu02 := m02/m00 - yc*yc
	mu11 := m11/m00 - xc*yc
	
	// Principal axis angle
	return 0.5 * math.Atan2(2*mu11, mu20-mu02)
}

// Group characters by similar properties
func GroupCharactersByType(characters []CharacterProperties) map[string][]CharacterProperties {
	groups := make(map[string][]CharacterProperties)
	
	for _, char := range characters {
		key := fmt.Sprintf("%s_%s_%.1f", char.Orientation, char.PatternType, math.Round(char.Density*10)/10)
		groups[key] = append(groups[key], char)
	}
	
	return groups
}

// Format properties for display
func (p CharacterProperties) String() string {
	var sb strings.Builder
	
	sb.WriteString(fmt.Sprintf("Character: %c (U+%04X)\n", p.Rune, p.Rune))
	sb.WriteString(fmt.Sprintf("Density: %.2f%% (Edge: %.2f%%)\n", p.Density*100, p.EdgeDensity*100))
	sb.WriteString(fmt.Sprintf("Components: %d, Has holes: %v\n", p.ConnectedComponents, p.HasHoles))
	sb.WriteString(fmt.Sprintf("Symmetry: H=%.2f V=%.2f D=%.2f R=%.2f\n", 
		p.Symmetry.Horizontal, p.Symmetry.Vertical, p.Symmetry.Diagonal, p.Symmetry.Rotational))
	sb.WriteString(fmt.Sprintf("Distribution: [%.2f %.2f %.2f %.2f]\n", 
		p.Distribution[0], p.Distribution[1], p.Distribution[2], p.Distribution[3]))
	sb.WriteString(fmt.Sprintf("Center of mass: (%.2f, %.2f)\n", p.CenterOfMass[0], p.CenterOfMass[1]))
	sb.WriteString(fmt.Sprintf("Bounding box: %dx%d at (%d,%d)\n", 
		p.BoundingBox.Width, p.BoundingBox.Height, p.BoundingBox.MinX, p.BoundingBox.MinY))
	sb.WriteString(fmt.Sprintf("Orientation: %s, Pattern: %s\n", p.Orientation, p.PatternType))
	sb.WriteString(fmt.Sprintf("Corner pixels: %d, Center pixels: %d\n", p.CornerPixels, p.CenterPixels))
	
	return sb.String()
}