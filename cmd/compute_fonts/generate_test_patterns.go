package main

import (
	"fmt"
)

// ExtractedPattern represents a glyph extracted from the font
type ExtractedPattern struct {
	Rune        rune
	Bitmap      GlyphBitmap
	Description string
	Category    string
}

// GenerateTestPatternsFromFont extracts actual glyphs to use as test cases
func GenerateTestPatternsFromFont() map[string][]ExtractedPattern {
	// Load fonts
	font, err := loadFont("PxPlus_IBM_BIOS.ttf")
	if err != nil {
		panic(err)
	}
	
	safeFont, err := loadFont("FSD - PragmataProMono.ttf")
	if err != nil {
		panic(err)
	}
	
	// Get all glyphs
	glyphs := analyzeFont(font, safeFont)
	
	// Categorize patterns we want to test
	patterns := map[string][]ExtractedPattern{
		"circles": extractCircularPatterns(glyphs),
		"diagonal_lines": extractDiagonalPatterns(glyphs),
		"straight_lines": extractStraightLinePatterns(glyphs),
		"crosses": extractCrossPatterns(glyphs),
		"blocks": extractBlockPatterns(glyphs),
		"shading": extractShadingPatterns(glyphs),
	}
	
	return patterns
}

func extractCircularPatterns(glyphs []GlyphInfo) []ExtractedPattern {
	candidates := []rune{'O', 'o', '0', 'Q', 'C', 'G', '@', '●', '○', 'D'}
	return extractByRunes(glyphs, candidates, "circular")
}

func extractDiagonalPatterns(glyphs []GlyphInfo) []ExtractedPattern {
	candidates := []rune{'/', '\\', 'X', 'x', '%', 'Z', 'z', 'N'}
	return extractByRunes(glyphs, candidates, "diagonal")
}

func extractStraightLinePatterns(glyphs []GlyphInfo) []ExtractedPattern {
	candidates := []rune{'-', '_', '=', '—', '|', 'I', 'l', '1', '!', '[', ']'}
	return extractByRunes(glyphs, candidates, "straight")
}

func extractCrossPatterns(glyphs []GlyphInfo) []ExtractedPattern {
	candidates := []rune{'+', 'x', 'X', '*', '†', '‡', 't', 'T'}
	return extractByRunes(glyphs, candidates, "cross")
}

func extractBlockPatterns(glyphs []GlyphInfo) []ExtractedPattern {
	candidates := []rune{'█', '■', '▀', '▄', '▌', '▐', '▪', '▫', '□'}
	return extractByRunes(glyphs, candidates, "block")
}

func extractShadingPatterns(glyphs []GlyphInfo) []ExtractedPattern {
	candidates := []rune{'░', '▒', '▓', '#', '%', '@', '&'}
	return extractByRunes(glyphs, candidates, "shading")
}

func extractByRunes(glyphs []GlyphInfo, runes []rune, category string) []ExtractedPattern {
	var patterns []ExtractedPattern
	
	for _, r := range runes {
		for _, g := range glyphs {
			if g.Rune == r {
				patterns = append(patterns, ExtractedPattern{
					Rune:        r,
					Bitmap:      g.Bitmap,
					Description: fmt.Sprintf("%c (U+%04X)", r, r),
					Category:    category,
				})
				break
			}
		}
	}
	
	return patterns
}

// GenerateSelfValidatingTests creates test cases that validate consistency
func GenerateSelfValidatingTests(patterns map[string][]ExtractedPattern) {
	fmt.Println("// Auto-generated self-validating test patterns")
	fmt.Println("// Each pattern should match all acceptable alternatives")
	fmt.Println()
	
	for category, patterns := range patterns {
		if len(patterns) == 0 {
			continue
		}
		
		fmt.Printf("// %s patterns (%d variants)\n", category, len(patterns))
		fmt.Printf("var %sPatterns = []AcceptablePattern{\n", category)
		
		for _, p := range patterns {
			fmt.Printf("\t{\n")
			fmt.Printf("\t\tname: %q,\n", p.Description)
			fmt.Printf("\t\trune: '%c',\n", p.Rune)
			fmt.Printf("\t\tbitmap: 0x%016X,\n", p.Bitmap)
			
			// Find all runes in this category
			acceptableRunes := []rune{}
			for _, other := range patterns {
				acceptableRunes = append(acceptableRunes, other.Rune)
			}
			
			fmt.Printf("\t\tacceptable: []rune{")
			for i, r := range acceptableRunes {
				if i > 0 {
					fmt.Printf(", ")
				}
				fmt.Printf("'%c'", r)
			}
			fmt.Printf("},\n")
			fmt.Printf("\t},\n")
		}
		fmt.Printf("}\n\n")
	}
}

// ValidatePatternConsistency checks if patterns match each other as expected
func ValidatePatternConsistency(lookup *GlyphLookup) {
	patterns := GenerateTestPatternsFromFont()
	
	fmt.Println("\nPattern Consistency Validation")
	fmt.Println("==============================")
	
	for category, catPatterns := range patterns {
		fmt.Printf("\n%s:\n", category)
		
		// Build acceptable set
		acceptableRunes := make(map[rune]bool)
		for _, p := range catPatterns {
			acceptableRunes[p.Rune] = true
		}
		
		// Test each pattern
		failures := 0
		for _, pattern := range catPatterns {
			match := lookup.FindClosestGlyph(pattern.Bitmap)
			
			if !acceptableRunes[match.Rune] {
				failures++
				fmt.Printf("  ❌ %c -> %c (expected one of: ", pattern.Rune, match.Rune)
				for r := range acceptableRunes {
					fmt.Printf("%c ", r)
				}
				fmt.Printf(")\n")
				
				// Show why it matched wrong
				similarity := calculateSimilarity(
					extractFeatures(pattern.Bitmap),
					*lookup.LookupRune(match.Rune),
				)
				fmt.Printf("     Similarity: %.3f\n", similarity)
			} else if match.Rune != pattern.Rune {
				fmt.Printf("  ⚠️  %c -> %c (acceptable but not self-match)\n", 
					pattern.Rune, match.Rune)
			} else {
				fmt.Printf("  ✅ %c -> %c (self-match)\n", pattern.Rune, match.Rune)
			}
		}
		
		if failures > 0 {
			fmt.Printf("  Category coherence: %.1f%%\n", 
				100.0 * float64(len(catPatterns)-failures) / float64(len(catPatterns)))
		}
	}
}

// GenerateCrossMatchMatrix shows how patterns match to each other
func GenerateCrossMatchMatrix(lookup *GlyphLookup, runes []rune) {
	fmt.Printf("\nCross-Match Matrix:\n")
	fmt.Printf("   ")
	for _, r := range runes {
		fmt.Printf(" %c ", r)
	}
	fmt.Println()
	
	for _, r1 := range runes {
		fmt.Printf("%c: ", r1)
		glyph1 := lookup.LookupRune(r1)
		if glyph1 == nil {
			fmt.Printf(" [not found]\n")
			continue
		}
		
		for _, r2 := range runes {
			glyph2 := lookup.LookupRune(r2)
			if glyph2 == nil {
				fmt.Printf(" ? ")
				continue
			}
			
			similarity := calculateSimilarity(*glyph1, *glyph2)
			if similarity > 0.8 {
				fmt.Printf(" ● ") // High similarity
			} else if similarity > 0.6 {
				fmt.Printf(" ◐ ") // Medium similarity
			} else {
				fmt.Printf(" ○ ") // Low similarity
			}
		}
		fmt.Println()
	}
}

// TestAcceptableMatches validates that acceptable alternatives actually match each other
func TestAcceptableMatches(patterns []ExtractedPattern, lookup *GlyphLookup) error {
	// For each pattern in the acceptable set
	for _, p1 := range patterns {
		match := lookup.FindClosestGlyph(p1.Bitmap)
		
		// Check if it matches something in the acceptable set
		found := false
		for _, p2 := range patterns {
			if match.Rune == p2.Rune {
				found = true
				break
			}
		}
		
		if !found {
			// Find which pattern it did match
			actualGlyph := lookup.LookupRune(match.Rune)
			similarity := calculateSimilarity(
				extractFeatures(p1.Bitmap),
				*actualGlyph,
			)
			
			return fmt.Errorf("pattern %c matched %c (similarity %.3f) outside acceptable set",
				p1.Rune, match.Rune, similarity)
		}
	}
	
	return nil
}