package main

import (
	"math"
)

// ApproximateCache is a map of Uint256 to lookupEntry
// that is used to store approximate matches for a given
// block of 4 RGB values. Approximate matches are performed
// by comparing the error of a given match to a threshold
// value.
//
// The key of the map is a Uint256, which is a 256-bit
// unsigned integer that is used to represent the foreground
// and background colors of a block of 4 RGB values.
//
// There may be multiple matches for a given key, so the
// value of the map is a lookupEntry, which is a struct
// that contains a slice of Match structs.
type ApproximateCache map[Uint256]lookupEntry

// Match is a struct that contains the rune, foreground
// color, background color, and error of a match. The error
// is a float64 value that represents the difference between
// the actual block of 4 RGB values and the pair of foreground
// and background colors encoded in the key as an Uint256.
type Match struct {
	Rune  rune
	FG    RGB
	BG    RGB
	Error float64
}

type lookupEntry struct {
	Matches []Match
}

// AddEntry adds a new entry to the cache. The entry is
// represented by a key, which is a Uint256, and a Match
// struct that contains the rune, foreground color, background
// color, and error of the match.
func (cache ApproximateCache) addEntry(
	k Uint256,
	r rune,
	fg RGB,
	bg RGB,
	block [4]RGB,
	isEdge bool,
) {
	newMatch := Match{
		Rune: r,
		FG:   fg,
		BG:   bg,
		Error: calculateBlockError(
			block,
			getQuadrantsForRune(r),
			fg,
			bg,
			isEdge,
		),
	}
	if entry, exists := lookupTable[k]; exists {
		// Create a new slice with the appended match
		updatedMatches := append(entry.Matches, newMatch)
		// Update the map with the new slice
		lookupTable[k] = lookupEntry{Matches: updatedMatches}
	} else {
		lookupTable[k] = lookupEntry{Matches: []Match{newMatch}}
	}
}

// GetEntry retrieves an entry from the cache. The entry is
// represented by a key, which is a Uint256, and a block of
// 4 RGB values. The function returns the rune, foreground
// color, background color, and a boolean value indicating
// whether the entry was found in the cache.
//
// There may be multiple matches for a given key, so the
// function returns the match with the lowest error value.
func (cache ApproximateCache) getEntry(
	k Uint256,
	block [4]RGB,
	isEdge bool,
) (rune, RGB, RGB, bool) {
	baseThreshold := cacheThreshold
	if isEdge {
		baseThreshold *= 0.7
	}
	lowestError := math.MaxFloat64
	var bestMatch *Match = nil
	if entry, exists := lookupTable[k]; exists {
		for _, match := range entry.Matches {
			// Recalculate error for this match
			matchError := calculateBlockError(block,
				getQuadrantsForRune(match.Rune), match.FG, match.BG, isEdge)
			if matchError < baseThreshold {
				if matchError < lowestError {
					lowestError = matchError
					bestMatch = &match
				}
			}
		}
		if bestMatch != nil {
			lookupHits++
			return bestMatch.Rune, bestMatch.FG, bestMatch.BG, true
		}
	}
	lookupMisses++
	return 0, RGB{}, RGB{}, false
}
