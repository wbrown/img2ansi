package img2ansi

import (
	"math"
)

// ApproximateCache is a map of Uint256 to lookupEntry
// that is used to store approximate matches for a given
// block of 4 RGB values. Approximate matches are performed
// by comparing the error of a given match to a threshold
// Value.
//
// The Key of the map is a Uint256, which is a 256-bit
// unsigned integer that is used to represent the foreground
// and background colors of a block of 4 RGB values.
//
// There may be multiple matches for a given Key, so the
// Value of the map is a lookupEntry, which is a struct
// that contains a slice of Match structs.
type ApproximateCache map[Uint256]lookupEntry

// Match is a struct that contains the rune, foreground
// color, background color, and error of a match. The error
// is a float64 Value that represents the difference between
// the actual block of 4 RGB values and the pair of foreground
// and background colors encoded in the Key as an Uint256.
type Match struct {
	Rune  rune
	FG    RGB
	BG    RGB
	Error float64
}

type lookupEntry struct {
	Matches []Match
}

// addCacheEntry adds a new entry to the renderer's lookup cache. The entry is
// represented by a key, which is a Uint256, and a Match struct that contains
// the rune, foreground color, background color, and error of the match.
func (r *Renderer) addCacheEntry(
	k Uint256,
	rune rune,
	fg RGB,
	bg RGB,
	block [4]RGB,
	isEdge bool,
) {
	r.lookupMisses++
	newMatch := Match{
		Rune:  rune,
		FG:    fg,
		BG:    bg,
		Error: 0, // Error will be calculated when retrieved
	}
	if entry, exists := r.lookupTable[k]; exists {
		// Create a new slice with the appended match
		updatedMatches := append(entry.Matches, newMatch)
		// Update the map with the new slice
		r.lookupTable[k] = lookupEntry{Matches: updatedMatches}
	} else {
		r.lookupTable[k] = lookupEntry{Matches: []Match{newMatch}}
	}
}

// getCacheEntry retrieves an entry from the renderer's lookup cache. The entry
// is represented by a key, which is a Uint256, and a block of 4 RGB values.
// The function returns the rune, foreground color, background color, and a
// boolean value indicating whether the entry was found in the cache.
//
// There may be multiple matches for a given key, so the function evaluates
// all cached patterns and returns the match with the lowest error below the
// cache threshold.
func (r *Renderer) getCacheEntry(
	k Uint256,
	block [4]RGB,
	isEdge bool,
) (rune, RGB, RGB, bool) {
	baseThreshold := r.CacheThreshold
	if isEdge {
		baseThreshold *= 0.7
	}
	lowestError := math.MaxFloat64
	var bestMatch *Match = nil
	if entry, exists := r.lookupTable[k]; exists {
		for i := range entry.Matches {
			match := &entry.Matches[i]
			// Calculate error using the renderer's color method
			quad := getQuadrantsForRune(match.Rune)
			error := r.calculateBlockError(block, quad, match.FG, match.BG, isEdge)

			if error < lowestError && error < baseThreshold {
				lowestError = error
				bestMatch = match
			}
		}
		if bestMatch != nil {
			r.lookupHits++
			return bestMatch.Rune, bestMatch.FG, bestMatch.BG, true
		}
	}
	return 0, RGB{}, RGB{}, false
}
