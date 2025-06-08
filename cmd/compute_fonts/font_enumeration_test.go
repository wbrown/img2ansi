package main

import (
	"testing"
)

// TestFontCharacterEnumeration tests that we can enumerate and categorize font characters
func TestFontCharacterEnumeration(t *testing.T) {
	// Load fonts
	font, err := loadFont("PxPlus_IBM_BIOS.ttf")
	if err != nil {
		t.Skipf("Font not available: %v", err)
	}
	
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	if safeFont == nil {
		safeFont = font
	}
	
	glyphs := analyzeFont(font, safeFont)
	
	// Test that we found expected characters
	expectedChars := []rune{' ', '░', '▒', '▓', '█', '▀', '▄', '▌', '▐'}
	foundCount := 0
	
	for _, expected := range expectedChars {
		found := false
		for _, glyph := range glyphs {
			if glyph.Rune == expected {
				found = true
				foundCount++
				break
			}
		}
		if !found {
			t.Errorf("Expected character %c (U+%04X) not found in font", expected, expected)
		}
	}
	
	t.Logf("Found %d/%d expected block characters", foundCount, len(expectedChars))
}

// TestFontDensityGradient tests finding characters suitable for density gradients
func TestFontDensityGradient(t *testing.T) {
	font, err := loadFont("PxPlus_IBM_BIOS.ttf")
	if err != nil {
		t.Skipf("Font not available: %v", err)
	}
	
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	if safeFont == nil {
		safeFont = font
	}
	
	glyphs := analyzeFont(font, safeFont)
	_ = NewGlyphLookup(glyphs) // lookup created but not used in this test
	
	// Find characters at different density levels
	densityLevels := []struct {
		name        string
		minDensity  float64
		maxDensity  float64
		expectChars []rune
	}{
		{"empty", 0.0, 0.1, []rune{' '}},
		{"light", 0.2, 0.3, []rune{'░', '.', '·'}},
		{"medium", 0.4, 0.6, []rune{'▒', '+', '#'}},
		{"heavy", 0.7, 0.8, []rune{'▓', '@'}},
		{"full", 0.9, 1.0, []rune{'█', '■'}},
	}
	
	for _, level := range densityLevels {
		t.Run(level.name, func(t *testing.T) {
			found := 0
			for _, glyph := range glyphs {
				density := float64(glyph.Weight) / 64.0
				if density >= level.minDensity && density <= level.maxDensity {
					found++
					
					// Check if it's one of the expected chars
					expected := false
					for _, exp := range level.expectChars {
						if glyph.Rune == exp {
							expected = true
							break
						}
					}
					
					if testing.Verbose() && expected {
						t.Logf("Found expected %c at density %.2f", glyph.Rune, density)
					}
				}
			}
			
			if found == 0 {
				t.Errorf("No characters found in density range %.1f-%.1f", 
					level.minDensity, level.maxDensity)
			} else {
				t.Logf("Found %d characters in density range", found)
			}
		})
	}
}

// TestFontEdgeCharacters tests finding characters with strong edges
func TestFontEdgeCharacters(t *testing.T) {
	font, err := loadFont("PxPlus_IBM_BIOS.ttf")
	if err != nil {
		t.Skipf("Font not available: %v", err)
	}
	
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	if safeFont == nil {
		safeFont = font
	}
	
	glyphs := analyzeFont(font, safeFont)
	
	// Characters we expect to have strong edges
	edgeChars := []rune{'|', '-', '+', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼'}
	
	for _, char := range edgeChars {
		found := false
		for _, glyph := range glyphs {
			if glyph.Rune == char {
				found = true
				// Count edge pixels
				edgeCount := 0
				for i := 0; i < 64; i++ {
					if glyph.EdgeMap&(1<<i) != 0 {
						edgeCount++
					}
				}
				
				if edgeCount < 4 {
					t.Errorf("Character %c has too few edge pixels: %d", char, edgeCount)
				} else if testing.Verbose() {
					t.Logf("Character %c has %d edge pixels", char, edgeCount)
				}
				break
			}
		}
		
		if !found && testing.Verbose() {
			t.Logf("Edge character %c not found in font", char)
		}
	}
}

// TestFontZoneWeights tests zone weight calculations
func TestFontZoneWeights(t *testing.T) {
	font, err := loadFont("PxPlus_IBM_BIOS.ttf")
	if err != nil {
		t.Skipf("Font not available: %v", err)
	}
	
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	if safeFont == nil {
		safeFont = font
	}
	
	glyphs := analyzeFont(font, safeFont)
	
	// Test specific characters with known patterns
	testCases := []struct {
		char        rune
		expectZones []int // Expected non-zero zones (0-3)
	}{
		{'▘', []int{0}},      // Upper left quadrant
		{'▝', []int{1}},      // Upper right quadrant
		{'▖', []int{2}},      // Lower left quadrant
		{'▗', []int{3}},      // Lower right quadrant
		{'▀', []int{0, 1}},   // Upper half
		{'▄', []int{2, 3}},   // Lower half
		{'█', []int{0, 1, 2, 3}}, // Full block
	}
	
	for _, tc := range testCases {
		t.Run(string(tc.char), func(t *testing.T) {
			for _, glyph := range glyphs {
				if glyph.Rune == tc.char {
					// Check that expected zones have weight
					for _, zone := range tc.expectZones {
						if glyph.ZoneWeights[zone] == 0 {
							t.Errorf("Expected zone %d to have weight for %c", zone, tc.char)
						}
					}
					
					// Check that other zones are empty (for these specific patterns)
					for zone := 0; zone < 4; zone++ {
						expected := false
						for _, exp := range tc.expectZones {
							if zone == exp {
								expected = true
								break
							}
						}
						
						if !expected && glyph.ZoneWeights[zone] > 12 {
							t.Errorf("Zone %d unexpectedly heavy (%d) for %c", 
								zone, glyph.ZoneWeights[zone], tc.char)
						}
					}
					return
				}
			}
			t.Errorf("Character %c not found in font", tc.char)
		})
	}
}