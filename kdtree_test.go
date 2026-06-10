package img2ansi

import (
	"math"
	"math/rand"
	"testing"
)

// nearestByLinearScan is the ground-truth oracle: exact nearest palette
// color by exhaustive scan.
func nearestByLinearScan(colors []RGB, target RGB, method ColorDistanceMethod) (RGB, float64) {
	best, bestDist := colors[0], method.Distance(target, colors[0])
	for _, c := range colors[1:] {
		if d := method.Distance(target, c); d < bestDist {
			best, bestDist = c, d
		}
	}
	return best, bestDist
}

// oracleQueries builds the query set used to validate tree and tables:
// every palette color, a coarse RGB lattice, and seeded random colors.
func oracleQueries(palette []RGB) []RGB {
	queries := append([]RGB{}, palette...)
	for _, r := range []uint8{0, 85, 170, 255} {
		for _, g := range []uint8{0, 85, 170, 255} {
			for _, b := range []uint8{0, 85, 170, 255} {
				queries = append(queries, RGB{r, g, b})
			}
		}
	}
	rng := rand.New(rand.NewSource(437))
	for i := 0; i < 2000; i++ {
		queries = append(queries, RGB{
			uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256))})
	}
	return queries
}

// TestNearestNeighborMatchesLinearScan locks the KD-tree search to the
// exact result for every embedded palette and color method. This is the
// regression test for the traversal bugs that shipped wrong tables
// (depth%3 axes against largest-range splits, wrapping uint8 plane
// distance, squared-vs-linear pruning units): under those bugs the tree
// returned #555555 for a pure black query against ansi16.
func TestNearestNeighborMatchesLinearScan(t *testing.T) {
	palettes := []string{"ansi16", "ansi16bbs", "ansi256", "jetbrains32"}
	methods := []ColorDistanceMethod{RGBMethod{}, LABMethod{}, RedmeanMethod{}}

	for _, pal := range palettes {
		for _, method := range methods {
			r := NewRenderer(WithColorMethod(method), WithPalette(pal))
			mismatches := 0
			for _, q := range oracleQueries(r.fgColors) {
				_, wantDist := nearestByLinearScan(r.fgColors, q, method)
				_, gotDist := r.fgTree.nearestNeighbor(
					q, r.fgTree.Color, math.MaxFloat64, 0, method)
				// Distance equality, not color equality: distinct
				// palette entries may tie exactly.
				if math.Abs(gotDist-wantDist) > 1e-9 {
					mismatches++
					if mismatches <= 3 {
						t.Errorf("%s/%s: query %v: tree dist %.4f, exact %.4f",
							pal, method.Name(), q, gotDist, wantDist)
					}
				}
			}
			if mismatches > 0 {
				t.Errorf("%s/%s: %d mismatches against linear scan",
					pal, method.Name(), mismatches)
			}
		}
	}
}

// TestBuildKDTreeContainsAllColors pins the tree-construction bug that
// shipped a 15-color ansi16 tree: a log2(n)+1 depth cap silently
// discarded the remainder slice whenever duplicate component values
// skewed the median, and pure black — first in every sort order — was
// the color that fell off. Every palette color must be a node.
func TestBuildKDTreeContainsAllColors(t *testing.T) {
	for _, pal := range []string{"ansi16", "ansi16bbs", "ansi256", "jetbrains32"} {
		r := NewRenderer(WithPalette(pal))
		tree := buildKDTree(append([]RGB{}, r.fgColors...))
		nodes := tree.getAllColors()
		if len(nodes) != len(r.fgColors) {
			t.Errorf("%s: tree has %d nodes for %d palette colors",
				pal, len(nodes), len(r.fgColors))
		}
		inTree := make(map[RGB]bool, len(nodes))
		for _, c := range nodes {
			inTree[c] = true
		}
		for _, c := range r.fgColors {
			if !inTree[c] {
				t.Errorf("%s: color %v missing from tree", pal, c)
			}
		}
	}
}

// TestNearestNeighborBlackRegression pins the original symptom: pure
// black must resolve to pure black in ansi16 under every method.
func TestNearestNeighborBlackRegression(t *testing.T) {
	for _, method := range []ColorDistanceMethod{RGBMethod{}, LABMethod{}, RedmeanMethod{}} {
		r := NewRenderer(WithColorMethod(method), WithPalette("ansi16"))
		got, _ := r.fgTree.nearestNeighbor(
			RGB{0, 0, 0}, r.fgTree.Color, math.MaxFloat64, 0, method)
		if got != (RGB{0, 0, 0}) {
			t.Errorf("%s: tree maps pure black to %v", method.Name(), got)
		}
	}
}
