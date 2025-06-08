package main

import (
	"testing"
)

// TestIBMBIOSPatterns analyzes the IBM BIOS font pattern distribution
func TestIBMBIOSPatterns(t *testing.T) {
	// Load fonts
	IBMFont, err := loadFont("PxPlus_IBM_BIOS.ttf")
	if err != nil {
		t.Skip("IBM BIOS font not available")
	}
	
	safeFont, err := loadFont("FSD - PragmataProMono.ttf") 
	if err != nil {
		safeFont = IBMFont
	}
	
	// Analyze font
	glyphs := analyzeFont(IBMFont, safeFont)
	options := EnumerationOptions{
		IncludeAlphanumeric: true,
		IncludeSemantic:     true,
		GeometricOnly:       false,
	}
	categories := EnumerateFontCharacters(glyphs, options)
	
	// Important characters to track
	importantChars := map[rune]string{
		' ':  "Space",
		'░':  "Light shade 25%",
		'▒':  "Medium shade 50%", 
		'▓':  "Dark shade 75%",
		'█':  "Full block 100%",
		'▀':  "Upper half block",
		'▄':  "Lower half block",
		'▌':  "Left half block",
		'▐':  "Right half block",
		'■':  "Black square",
		'▪':  "Small black square",
		'□':  "White square",
		'▫':  "Small white square",
		'○':  "White circle outline",
		'#':  "Hash pattern",
		'%':  "Percent diagonal",
		'&':  "Ampersand complex",
		'*':  "Asterisk star",
		'@':  "At sign spiral",
		'.':  "Single dot",
		':':  "Two dots vertical",
		'=':  "Double horizontal line",
		'+':  "Plus cross",
		'-':  "Horizontal line",
		'_':  "Low horizontal line",
		'|':  "Vertical line",
		'/':  "Forward slash",
		'\\': "Backslash",
		'X':  "Diagonal cross",
		'x':  "Small diagonal cross",
	}
	
	// Collect density distribution
	
	var densityChars []DensityChar
	
	for _, cat := range categories {
		for _, char := range cat.Characters {
			desc := ""
			if d, ok := importantChars[char.Rune]; ok {
				desc = d
			}
			
			densityChars = append(densityChars, DensityChar{
				Char:        char.Properties,
				Rune:        char.Rune,
				Density:     char.Properties.Density,
				Description: desc,
			})
		}
	}
	
	// Sort by density
	for i := 0; i < len(densityChars)-1; i++ {
		for j := i + 1; j < len(densityChars); j++ {
			if densityChars[j].Density < densityChars[i].Density {
				densityChars[i], densityChars[j] = densityChars[j], densityChars[i]
			}
		}
	}
	
	// Report findings
	t.Log("IBM BIOS Font Pattern Characters by Density")
	t.Log("===========================================")
	
	// Density ranges
	ranges := []struct {
		name string
		min  float64
		max  float64
	}{
		{"Empty (0%)", 0.0, 0.01},
		{"Minimal (1-10%)", 0.01, 0.10},
		{"Light (10-30%)", 0.10, 0.30},
		{"Medium-Light (30-45%)", 0.30, 0.45},
		{"Medium (45-55%)", 0.45, 0.55},
		{"Medium-Dark (55-70%)", 0.55, 0.70},
		{"Dark (70-90%)", 0.70, 0.90},
		{"Near-Full (90-99%)", 0.90, 0.99},
		{"Full (100%)", 0.99, 1.01},
	}
	
	for _, r := range ranges {
		var chars []DensityChar
		for _, dc := range densityChars {
			if dc.Density >= r.min && dc.Density < r.max {
				chars = append(chars, dc)
			}
		}
		
		if len(chars) > 0 {
			t.Logf("\n%s:", r.name)
			count := 0
			for _, dc := range chars {
				if dc.Rune >= 0x20 && dc.Rune != 0x7F {
					desc := ""
					if dc.Description != "" {
						desc = " - " + dc.Description
					}
					t.Logf("  '%c' (%.1f%%)%s", dc.Rune, dc.Density*100, desc)
					count++
					if count >= 10 && len(chars) > 10 {
						t.Logf("  ... and %d more", len(chars)-10)
						break
					}
				}
			}
		}
	}
	
	// Pattern analysis
	t.Log("\nPattern Type Distribution:")
	patternTypes := make(map[string]int)
	for _, dc := range densityChars {
		patternTypes[dc.Char.PatternType]++
	}
	
	for ptype, count := range patternTypes {
		if ptype != "" {
			t.Logf("  %s: %d characters", ptype, count)
		}
	}
	
	// Find best characters for gradients
	t.Log("\nBest gradient sequence (by density):")
	gradientChars := findBestGradient(densityChars, 9)
	for i, dc := range gradientChars {
		t.Logf("  %d. '%c' (%.1f%%)", i+1, dc.Rune, dc.Density*100)
	}
}

// TestFontDensityDistribution tests the density distribution of font characters
func TestFontDensityDistribution(t *testing.T) {
	fonts := []struct {
		name string
		file string
	}{
		{"IBM BIOS", "PxPlus_IBM_BIOS.ttf"},
		{"IBM BIOS 2x", "Px437_IBM_BIOS-2x.ttf"},
		{"IBM BIOS 2y", "Px437_IBM_BIOS-2y.ttf"},
	}
	
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	
	for _, f := range fonts {
		t.Run(f.name, func(t *testing.T) {
			font, err := loadFont(f.file)
			if err != nil {
				t.Skip("Font not available")
			}
			
			if safeFont == nil {
				safeFont = font
			}
			
			glyphs := analyzeFont(font, safeFont)
			
			// Create density histogram
			histogram := make([]int, 11) // 0-10%, 10-20%, ..., 90-100%
			
			for _, glyph := range glyphs {
				density := float64(glyph.Weight) / 64.0
				bucket := int(density * 10)
				if bucket >= 10 {
					bucket = 10
				}
				histogram[bucket]++
			}
			
			t.Logf("Density distribution for %s:", f.name)
			for i, count := range histogram {
				if i < 10 {
					t.Logf("  %d-%d%%: %d characters", i*10, (i+1)*10, count)
				} else {
					t.Logf("  100%%: %d characters", count)
				}
			}
		})
	}
}

// Helper function to find best gradient sequence (moved to global scope for testing)
type DensityChar struct {
	Char        CharacterProperties
	Rune        rune
	Density     float64
	Description string
}

func findBestGradient(chars []DensityChar, steps int) []DensityChar {
	if len(chars) == 0 || steps <= 0 {
		return nil
	}
	
	// Already sorted by density
	result := make([]DensityChar, 0, steps)
	
	// Add first (lowest density)
	result = append(result, chars[0])
	
	// Find evenly spaced characters
	targetStep := 1.0 / float64(steps-1)
	
	for i := 1; i < steps-1; i++ {
		targetDensity := float64(i) * targetStep
		
		// Find closest character
		bestIdx := 0
		bestDiff := 1.0
		
		for j, dc := range chars {
			diff := targetDensity - dc.Density
			if diff < 0 {
				diff = -diff
			}
			if diff < bestDiff {
				bestDiff = diff
				bestIdx = j
			}
		}
		
		// Don't add duplicates
		duplicate := false
		for _, existing := range result {
			if existing.Rune == chars[bestIdx].Rune {
				duplicate = true
				break
			}
		}
		
		if !duplicate {
			result = append(result, chars[bestIdx])
		}
	}
	
	// Add last (highest density)
	if len(chars) > 0 {
		result = append(result, chars[len(chars)-1])
	}
	
	return result
}