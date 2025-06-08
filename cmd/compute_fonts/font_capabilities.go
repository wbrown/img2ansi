package main

import (
	"github.com/golang/freetype/truetype"
	"unicode"
)

// BlockSupport indicates the level of block character support
type BlockSupport int

const (
	NoBlocks BlockSupport = iota
	HalfBlocks      // ▀▄▌▐
	QuarterBlocks   // ▖▗▘▙▚▛▜▝▞▟  
	FullBlocks      // All of the above
)

// FontCapabilities describes what a font can render
type FontCapabilities struct {
	Name         string
	CharacterSet []rune              // Available characters
	HasBlocks    BlockSupport        // Block character support level
	HasShading   bool                // ░▒▓ or equivalent
	HasBoxDrawing bool               // ─│┌┐└┘├┤┬┴┼
	HasDiagonals bool                // ╱╲╳
	SpecialChars map[string][]rune   // "shading" -> [░,▒,▓]
}

// Key Unicode blocks we care about
var (
	// Half blocks that IBM BIOS has
	halfBlockChars = []rune{'▀', '▄', '▌', '▐'}
	
	// Quarter blocks that IBM BIOS lacks
	quarterBlockChars = []rune{'▖', '▗', '▘', '▙', '▚', '▛', '▜', '▝', '▞', '▟'}
	
	// Shading characters
	shadingChars = []rune{'░', '▒', '▓', '█'}
	
	// Basic box drawing
	boxDrawingChars = []rune{'─', '│', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼'}
	
	// ASCII pattern characters
	asciiPatterns = []rune{'#', '%', '&', '*', '@', '.', ':', '=', '+'}
)

// AnalyzeFontCapabilities examines what a font can render
func AnalyzeFontCapabilities(font *truetype.Font, name string) *FontCapabilities {
	caps := &FontCapabilities{
		Name:         name,
		CharacterSet: make([]rune, 0),
		SpecialChars: make(map[string][]rune),
	}
	
	// Check block support
	hasAllHalf := true
	hasAllQuarter := true
	
	for _, r := range halfBlockChars {
		if font.Index(r) == 0 {
			hasAllHalf = false
			break
		}
	}
	
	for _, r := range quarterBlockChars {
		if font.Index(r) == 0 {
			hasAllQuarter = false
			break
		}
	}
	
	if hasAllQuarter && hasAllHalf {
		caps.HasBlocks = FullBlocks
	} else if hasAllHalf {
		caps.HasBlocks = HalfBlocks
	} else {
		caps.HasBlocks = NoBlocks
	}
	
	// Check shading support
	var availableShading []rune
	for _, r := range shadingChars {
		if font.Index(r) != 0 {
			availableShading = append(availableShading, r)
		}
	}
	caps.HasShading = len(availableShading) >= 3
	if len(availableShading) > 0 {
		caps.SpecialChars["shading"] = availableShading
	}
	
	// Check box drawing
	hasBoxDrawing := true
	for _, r := range boxDrawingChars {
		if font.Index(r) == 0 {
			hasBoxDrawing = false
			break
		}
	}
	caps.HasBoxDrawing = hasBoxDrawing
	
	// Collect ASCII patterns
	var availablePatterns []rune
	for _, r := range asciiPatterns {
		if font.Index(r) != 0 {
			availablePatterns = append(availablePatterns, r)
		}
	}
	if len(availablePatterns) > 0 {
		caps.SpecialChars["ascii_patterns"] = availablePatterns
	}
	
	// Build complete character set (printable characters only)
	for r := rune(0x20); r <= 0xFFFF; r++ {
		if font.Index(r) != 0 && unicode.IsPrint(r) {
			caps.CharacterSet = append(caps.CharacterSet, r)
		}
	}
	
	return caps
}

// GetPatternCharacters returns characters suitable for pattern/texture rendering
func (fc *FontCapabilities) GetPatternCharacters() []rune {
	var patterns []rune
	
	// Add shading if available
	if shading, ok := fc.SpecialChars["shading"]; ok {
		patterns = append(patterns, shading...)
	}
	
	// Add ASCII patterns
	if ascii, ok := fc.SpecialChars["ascii_patterns"]; ok {
		patterns = append(patterns, ascii...)
	}
	
	// Add block characters based on support level
	if fc.HasBlocks >= HalfBlocks {
		patterns = append(patterns, halfBlockChars...)
	}
	
	// Always include space and full block if available  
	for _, r := range []rune{' ', '█'} {
		if containsRune(fc.CharacterSet, r) {
			patterns = append(patterns, r)
		}
	}
	
	return patterns
}

func containsRune(runes []rune, r rune) bool {
	for _, chr := range runes {
		if chr == r {
			return true
		}
	}
	return false
}