package main

import (
	"testing"
)

// TestBitmapBitManipulation tests bitmap bit manipulation
func TestBitmapBitManipulation(t *testing.T) {
	tests := []struct {
		name string
		x, y int
		expectedBit int
	}{
		{"TopLeft", 0, 0, 63},      // analyzeGlyph would put it at bit 63
		{"TopRight", 7, 0, 56},     // analyzeGlyph would put it at bit 56
		{"BottomLeft", 0, 7, 7},    // analyzeGlyph would put it at bit 7
		{"BottomRight", 7, 7, 0},   // analyzeGlyph would put it at bit 0
		{"Center", 4, 4, 27},       // analyzeGlyph would put it at bit 27
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate what analyzeGlyph does
			var bitmap GlyphBitmap
			heightShift := GlyphHeight - 1 - tt.y
			widthShift := GlyphWidth - 1 - tt.x
			bitPos := heightShift*GlyphWidth + widthShift
			bitmap |= 1 << bitPos
			
			t.Logf("Setting pixel at (%d,%d)", tt.x, tt.y)
			t.Logf("heightShift=%d, widthShift=%d, bitPos=%d", heightShift, widthShift, bitPos)
			t.Logf("Expected bit position: %d", tt.expectedBit)
			
			if bitPos != tt.expectedBit {
				t.Errorf("Bit position mismatch: got %d, want %d", bitPos, tt.expectedBit)
			}
			
			// Test getBit at the same coordinates
			if getBit(bitmap, tt.x, tt.y) {
				t.Errorf("getBit(%d,%d) returned true, but it reads from bit %d while we set bit %d", 
					tt.x, tt.y, tt.y*GlyphWidth+tt.x, bitPos)
			}
			
			// Show what getBit is actually checking
			getBitPos := tt.y*GlyphWidth + tt.x
			t.Logf("getBit checks bit position: %d", getBitPos)
			
			// Visual representation
			t.Logf("Visual (String method):")
			t.Log(bitmap.String())
			
			// What the algorithms see
			t.Logf("What algorithms see (using getBit):")
			for y := 0; y < 8; y++ {
				line := ""
				for x := 0; x < 8; x++ {
					if getBit(bitmap, x, y) {
						line += "█"
					} else {
						line += "·"
					}
				}
				t.Log(line)
			}
		})
	}
}

// TestDiagonalBitmapRepresentation tests diagonal bitmap representation
func TestDiagonalBitmapRepresentation(t *testing.T) {
	// The diagonal bitmap from the test
	diagonal := GlyphBitmap(0x8040201008040201)
	
	t.Log("Diagonal bitmap visual (String method):")
	t.Log(diagonal.String())
	
	t.Log("\nWhat getBit sees:")
	for y := 0; y < 8; y++ {
		line := ""
		for x := 0; x < 8; x++ {
			if getBit(diagonal, x, y) {
				line += "█"
			} else {
				line += "·"
			}
		}
		t.Log(line)
	}
	
	// Check specific pixels
	t.Logf("\nChecking diagonal pixels:")
	for i := 0; i < 8; i++ {
		hasPixel := getBit(diagonal, i, i)
		t.Logf("Pixel at (%d,%d): %v", i, i, hasPixel)
	}
}

// TestBitConsistency from bitmap_consistency_test.go
func TestBitConsistency(t *testing.T) {
	// Create a test bitmap with known pattern
	// Let's set pixels at (0,0), (7,0), (0,7), (7,7) and (3,3)
	var bitmap GlyphBitmap
	
	// Set pixels using the row-major ordering that should be consistent
	bitmap |= 1 << 0  // (0,0)
	bitmap |= 1 << 7  // (7,0)
	bitmap |= 1 << 56 // (0,7)
	bitmap |= 1 << 63 // (7,7)
	bitmap |= 1 << 27 // (3,3)
	
	t.Log("Test bitmap:")
	t.Log(bitmap.String())
	
	// Test that getBit returns true for the pixels we set
	testCases := []struct {
		x, y int
		want bool
	}{
		{0, 0, true},
		{7, 0, true},
		{0, 7, true},
		{7, 7, true},
		{3, 3, true},
		{4, 4, false}, // This one we didn't set
		{1, 1, false}, // This one we didn't set
	}
	
	for _, tc := range testCases {
		got := getBit(bitmap, tc.x, tc.y)
		if got != tc.want {
			t.Errorf("getBit(%d, %d) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
	
	// Now verify String() shows pixels in the right places
	str := bitmap.String()
	lines := []string{}
	for i := 0; i < len(str); i++ {
		if str[i] == '\n' || i == len(str)-1 {
			if i > 0 && len(lines) < 8 {
				start := i - 8
				if start < 0 {
					start = 0
				}
				if i == len(str)-1 {
					i++
				}
				lines = append(lines, str[start:i])
			}
		}
	}
	
	// Verify corner pixels in string representation
	if len(lines) >= 8 {
		// Note: lines[y] is a string, so we need to convert to runes for comparison
		// Check (0,0) - top left
		line0Runes := []rune(lines[0])
		if len(line0Runes) > 0 && line0Runes[0] != '█' {
			t.Error("Expected pixel at (0,0) in string representation")
		}
		// Check (7,0) - top right
		if len(line0Runes) > 7 && line0Runes[7] != '█' {
			t.Error("Expected pixel at (7,0) in string representation")
		}
		// Check (0,7) - bottom left
		line7Runes := []rune(lines[7])
		if len(line7Runes) > 0 && line7Runes[0] != '█' {
			t.Error("Expected pixel at (0,7) in string representation")
		}
		// Check (7,7) - bottom right
		if len(line7Runes) > 7 && line7Runes[7] != '█' {
			t.Error("Expected pixel at (7,7) in string representation")
		}
	}
}

// TestCharacterBitmaps from bitmap_analysis_test.go
func TestCharacterBitmaps(t *testing.T) {
	font, err := loadFont("PxPlus_IBM_BIOS.ttf")
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	
	// Characters known to have interesting patterns
	interestingChars := []rune{
		'|', '+', '-', '/', '\\', 'O', '0', '#', '*', '@',
		'█', '▀', '▄', '▌', '▐', '░', '▒', '▓',
	}
	
	for _, ch := range interestingChars {
		glyph := analyzeGlyph(font, ch, 14, true)
		if glyph != nil {
			t.Logf("\nCharacter '%c' (U+%04X):", ch, ch)
			t.Logf("Weight: %d pixels", popcount(glyph.Bitmap))
			t.Logf("Bitmap:\n%s", glyph.Bitmap.String())
		} else {
			t.Logf("\nCharacter '%c' (U+%04X): NOT FOUND", ch, ch)
		}
	}
}

// TestSpecificMatches from matching_test.go
func TestSpecificMatches(t *testing.T) {
	font, safeFont := LoadFonts(t)
	glyphs := analyzeFont(font, safeFont)
	
	// Create lookup
	lookup := NewGlyphLookup(glyphs)
	
	// Test specific problematic matches
	tests := []struct {
		name        string
		input       GlyphBitmap
		expected    rune
		description string
	}{
		{
			name: "CirclePattern",
			input: GlyphBitmap(0x3C7EFFFFFFFF7E3C),
			expected: 'O',
			description: "Should match letter O",
		},
		{
			name: "VerticalLine",
			input: func() GlyphBitmap {
				var b GlyphBitmap
				for y := 0; y < 8; y++ {
					if y != 3 && y != 7 { // Skip the gaps in IBM BIOS font
						b |= 1 << (y*8 + 3) // Center column
					}
				}
				return b
			}(),
			expected: '|',
			description: "Should match vertical bar",
		},
		{
			name: "DiagonalSlash",
			input: GlyphBitmap(0x0102040810204080),
			expected: '/',
			description: "Should match forward slash",
		},
		{
			name: "DiagonalBackslash",
			input: GlyphBitmap(0x8040201008040201),
			expected: '\\',
			description: "Should match backslash",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test: %s - %s", tt.name, tt.description)
			t.Logf("Input bitmap:\n%s", tt.input.String())
			
			match := lookup.FindClosestGlyph(tt.input)
			t.Logf("Best match: '%c' (U+%04X)", match.Rune, match.Rune)
			t.Logf("Match bitmap:\n%s", match.Bitmap.String())
			
			if match.Rune != tt.expected {
				t.Errorf("Expected '%c' but got '%c'", tt.expected, match.Rune)
			}
			
			// Show similarity scores
			features := extractFeatures(tt.input)
			similarity := calculateSimilarity(features, match)
			t.Logf("Similarity score: %.3f", similarity)
		})
	}
}

// TestMatchAnalysis from match_analysis_test.go
func TestMatchAnalysis(t *testing.T) {
	font, safeFont := LoadFonts(t)
	glyphs := analyzeFont(font, safeFont)
	lookup := NewGlyphLookup(glyphs)
	_ = lookup // TODO: Use lookup for pattern matching
	
	// Analyze what's actually matching for key patterns
	patterns := []struct {
		name   string
		bitmap GlyphBitmap
	}{
		{"Circle", 0x3C7EFFFFFFFF7E3C},
		{"Vertical", func() GlyphBitmap {
			var b GlyphBitmap
			for y := 0; y < 8; y++ {
				b |= 1 << (y*8 + 3)
			}
			return b
		}()},
		{"Diagonal", 0x8040201008040201},
		{"Cross", 0x1818181818181818},
	}
	
	for _, p := range patterns {
		t.Logf("\n=== %s Pattern Analysis ===", p.name)
		t.Logf("Input:\n%s", p.bitmap.String())
		
		// Get top 5 matches
		features := extractFeatures(p.bitmap)
		matches := calculateSimilarityBatch(features, glyphs)
		
		t.Logf("\nTop 5 matches:")
		for i := 0; i < 5 && i < len(matches); i++ {
			match := matches[i]
			t.Logf("%d. '%c' (U+%04X) - Similarity: %.3f", 
				i+1, match.Target.Rune, match.Target.Rune, match.Similarity)
			t.Logf("   Bitmap:\n%s", match.Target.Bitmap.String())
		}
	}
}

// TestDebugSparseSimilarity from debug_sparse_test.go
func TestDebugSparseSimilarity(t *testing.T) {
	// Inverted test pattern from block (39, 21) in Mandrill image
	invertedPattern := GlyphBitmap(0x6f9ffbf6ffffff3f)
	
	// Character '▁' (bottom one-eighth block)
	bottomEighthBlock := GlyphBitmap(0xff00000000000000)
	
	// Debug the similarity calculation
	t.Logf("Inverted pattern:\n%s", invertedPattern.String())
	t.Logf("Weight: %d pixels\n", popcount(invertedPattern))
	
	t.Logf("Bottom eighth block '▁':\n%s", bottomEighthBlock.String())
	t.Logf("Weight: %d pixels\n", popcount(bottomEighthBlock))
	
	// Calculate raw Hamming distance
	xor := invertedPattern ^ bottomEighthBlock
	hammingDistance := popcount(xor)
	matchingBits := 64 - hammingDistance
	
	t.Logf("\nXOR result (differences):\n%s", xor.String())
	t.Logf("Hamming distance: %d", hammingDistance)
	t.Logf("Matching bits: %d/64", matchingBits)
	t.Logf("Similarity: %.1f%%", float64(matchingBits)/64*100)
	
	// Break down by pixel type
	onesInBoth := popcount(invertedPattern & bottomEighthBlock)
	zerosInBoth := popcount(^invertedPattern & ^bottomEighthBlock)
	
	t.Logf("\nDetailed breakdown:")
	t.Logf("Pixels ON in both: %d", onesInBoth)
	t.Logf("Pixels OFF in both: %d", zerosInBoth)
	t.Logf("Total matching: %d", onesInBoth + zerosInBoth)
	
	// This shows why sparse patterns match
	t.Logf("\nThe problem: %d matching zeros (empty pixels) count as 'similarity'", zerosInBoth)
}

// Self-validating test patterns from self_validating_test.go
type AcceptablePattern struct {
	Name        string
	Bitmap      GlyphBitmap
	Acceptable  []rune
	Description string
}

var acceptablePatterns = []AcceptablePattern{
	{
		Name: "VerticalLine",
		Bitmap: func() GlyphBitmap {
			var b GlyphBitmap
			for y := 0; y < 8; y++ {
				b |= 1 << (y*8 + 3)
			}
			return b
		}(),
		Acceptable:  []rune{'|', 'I', 'l', '1'},
		Description: "Vertical line should match pipe, I, l, or 1",
	},
	{
		Name: "HorizontalLine",
		Bitmap: func() GlyphBitmap {
			var b GlyphBitmap
			for x := 0; x < 8; x++ {
				b |= 1 << (3*8 + x)
			}
			return b
		}(),
		Acceptable:  []rune{'-', '−', '—', '_'},
		Description: "Horizontal line should match various dash characters",
	},
	{
		Name: "Cross",
		Bitmap: func() GlyphBitmap {
			var b GlyphBitmap
			// Vertical
			for y := 0; y < 8; y++ {
				b |= 1 << (y*8 + 3)
			}
			// Horizontal
			for x := 0; x < 8; x++ {
				b |= 1 << (3*8 + x)
			}
			return b
		}(),
		Acceptable:  []rune{'+', '†', '‡'},
		Description: "Cross pattern should match plus or cross characters",
	},
	{
		Name: "FullBlock",
		Bitmap: 0xFFFFFFFFFFFFFFFF,
		Acceptable:  []rune{'█', '■', '◼'},
		Description: "Full block should match block characters",
	},
	{
		Name: "Empty",
		Bitmap: 0x0000000000000000,
		Acceptable:  []rune{' ', '\u00A0'},
		Description: "Empty should match space characters",
	},
}

func TestSelfValidatingPatterns(t *testing.T) {
	font, safeFont := LoadFonts(t)
	glyphs := analyzeFont(font, safeFont)
	lookup := NewGlyphLookup(glyphs)
	
	for _, pattern := range acceptablePatterns {
		t.Run(pattern.Name, func(t *testing.T) {
			t.Logf("Pattern: %s", pattern.Description)
			t.Logf("Input bitmap:\n%s", pattern.Bitmap.String())
			
			match := lookup.FindClosestGlyph(pattern.Bitmap)
			t.Logf("Best match: '%c' (U+%04X)", match.Rune, match.Rune)
			
			// Check if match is acceptable
			acceptable := false
			for _, r := range pattern.Acceptable {
				if match.Rune == r {
					acceptable = true
					break
				}
			}
			
			if !acceptable {
				t.Errorf("Match '%c' is not in acceptable list %v", match.Rune, pattern.Acceptable)
				t.Logf("Match bitmap:\n%s", match.Bitmap.String())
				
				// Show what would have been acceptable
				t.Log("Acceptable characters:")
				for _, r := range pattern.Acceptable {
					if glyph := lookup.LookupRune(r); glyph != nil {
						t.Logf("  '%c':\n%s", r, glyph.Bitmap.String())
					} else {
						t.Logf("  '%c': not found in font", r)
					}
				}
			}
		})
	}
}