package img2ansi

import (
	"container/heap"
	"math"
	"sort"
)

// ColorNode represents a node in a KD-tree that stores RGB colors. Each node
// contains a color, a left child, a right child, and the axis along which the
// colors are split.
type ColorNode struct {
	Color       RGB
	Left, Right *ColorNode
	SplitAxis   int
}

// buildKDTree constructs a KD-tree from a list of RGB colors, optimized for
// ANSI color matching. This implementation is particularly effective for
// ANSI art because:
//
//   - It preserves subtle shade differences crucial in limited color palettes
//     like ANSI.
//   - It maintains determinism, ensuring consistent output across runs, which
//     is important for reproducible ANSI art generation.
//   - It balances the tree effectively without over-optimizing, which works
//     well with the relatively small ANSI color space.
//   - It handles the discrete nature of ANSI colors well by carefully
//     managing axis splitting and duplicate color values.
//
// The function takes a list of colors and returns the root node of the
// KD-tree. The resulting tree structure allows for efficient
// nearest-neighbor searches in the ANSI color space, crucial for mapping
// arbitrary RGB colors to the closest ANSI representation.
//
// Every input color becomes a node: median splitting consumes one element
// per level, so recursion terminates on empty slices. (An earlier version
// capped recursion at log2(n)+1, which silently DROPPED colors whenever
// duplicate component values skewed the median — the ansi16 tree shipped
// without pure black in it, so no search, however correct, could return
// black. TestBuildKDTreeContainsAllColors pins this.)
func buildKDTree(colors []RGB) *ColorNode {
	if len(colors) == 0 {
		return nil
	}

	// Choose splitting axis based on the dimension with the largest range
	// This preserves the actual distribution of colors in the dataset,
	// helping to maintain subtle shade differences
	axis := chooseSplitAxis(colors)

	// Sort colors deterministically. This ensures consistency between
	// runs while minimally altering the tree structure
	sort.Slice(colors, func(i, j int) bool {
		iComp := getColorComponent(colors[i], axis)
		jComp := getColorComponent(colors[j], axis)
		if iComp != jComp {
			return iComp < jComp
		}
		// Tie-breakers: ensure full determinism even with equal axis values
		if colors[i].R != colors[j].R {
			return colors[i].R < colors[j].R
		}
		if colors[i].G != colors[j].G {
			return colors[i].G < colors[j].G
		}
		return colors[i].B < colors[j].B
	})

	median := len(colors) / 2

	// Handle duplicate values at the median
	// This prevents arbitrary splitting of identical colors, which could
	// cause inconsistencies
	for median < len(colors)-1 && getColorComponent(colors[median], axis) ==
		getColorComponent(colors[median+1], axis) {
		median++
	}

	// Create and return the node, recursively building left and right subtrees
	// Using the median as split point generally creates a balanced tree
	return &ColorNode{
		Color:     colors[median],
		Left:      buildKDTree(colors[:median]),
		Right:     buildKDTree(colors[median+1:]),
		SplitAxis: axis,
	}
}

// chooseSplitAxis selects the axis with the largest color range
// This helps to preserve the color variations in the dataset
func chooseSplitAxis(colors []RGB) int {
	minR, maxR := colors[0].R, colors[0].R
	minG, maxG := colors[0].G, colors[0].G
	minB, maxB := colors[0].B, colors[0].B

	// Find the min and max values for each color component
	for _, c := range colors {
		minR = min(minR, c.R)
		maxR = max(maxR, c.R)
		minG = min(minG, c.G)
		maxG = max(maxG, c.G)
		minB = min(minB, c.B)
		maxB = max(maxB, c.B)
	}

	// Calculate the range for each color component
	rangeR := maxR - minR
	rangeG := maxG - minG
	rangeB := maxB - minB

	// Return the axis with the largest range
	// This ensures we split along the axis with the most color variation
	if rangeR >= rangeG && rangeR >= rangeB {
		return 0 // R axis
	} else if rangeG >= rangeB {
		return 1 // G axis
	}
	return 2 // B axis
}

// getColorComponent returns the color component of an RGB color along the
// specified axis. The function takes an RGB color and an axis index as input
// and returns the corresponding color component.
func getColorComponent(color RGB, axis int) uint8 {
	switch axis {
	case 0:
		return color.R
	case 1:
		return color.G
	default:
		return color.B
	}
}

// getCandidateColors finds the k-nearest neighbors of the colors in a block
// using a KD-tree. The function takes a block of colors, the depth of the
// search, and the number of neighbors to find as input, and returns a slice
// of colors sorted by distance.
func (node *ColorNode) getCandidateColors(
	block [4]RGB,
	depth int,
	method ColorDistanceMethod,
) colorDistanceSlice {
	// Find initial candidates using KD-tree and sort by distance
	var candidateColors colorDistanceSlice
	seenColors := make(map[RGB]bool)

	// Process block colors in a consistent order
	sortedBlockColors := make(sortableRGB, len(block))
	for i, c := range block {
		sortedBlockColors[i] = c
	}
	sort.Sort(sortedBlockColors)

	for _, color := range sortedBlockColors {
		nearest := node.kNearestNeighbors(color, depth, method)
		for _, c := range nearest {
			if _, seen := seenColors[c]; !seen {
				distance := method.Distance(color, c)
				candidateColors = append(candidateColors,
					colorWithDistance{
						c,
						distance,
						len(candidateColors)})
				seenColors[c] = true
			}
		}
	}

	return candidateColors
}

// nearestNeighbor finds the nearest neighbor of a target color in a KD-tree.
// The function takes the root node of the KD-tree, the target color, the best
// color found so far, the best distance found so far, and the depth of the
// search as input, and returns the nearest neighbor and the distance to it.
func (node *ColorNode) nearestNeighbor(
	target RGB, best RGB, bestDist float64, depth int, method ColorDistanceMethod) (RGB, float64) {
	if node == nil {
		return best, bestDist
	}

	dist := method.Distance(node.Color, target)
	if dist < bestDist {
		best = node.Color
		bestDist = dist
	}

	// The tree is built on per-node largest-range axes, so the stored
	// SplitAxis is authoritative. (An earlier version of this search
	// walked with depth%3 axes, computed the plane distance with
	// wrapping uint8 subtraction, and pruned squared-RGB units against
	// linear method distances; the embedded tables built through it
	// mapped pure black to #555555.)
	axis := node.SplitAxis
	targetComp := float64(getColorComponent(target, axis))
	nodeComp := float64(getColorComponent(node.Color, axis))

	next, other := node.Right, node.Left
	if targetComp < nodeComp {
		next, other = node.Left, node.Right
	}

	best, bestDist = next.nearestNeighbor(target, best, bestDist, depth+1, method)

	// Search the other branch unless every color beyond the splitting
	// plane is provably farther than the best found. The plane distance
	// is in RGB units; planePruneFactor converts it to a lower bound in
	// the method's units (factor 0 = no safe bound, always search).
	planeDist := math.Abs(targetComp - nodeComp)
	if planeDist*planePruneFactor(method, axis) < bestDist {
		best, bestDist = other.nearestNeighbor(
			target, best, bestDist, depth+1, method)
	}

	return best, bestDist
}

// planePruneFactor returns f such that f*|Δaxis| is a lower bound on
// method.Distance between two colors differing by at least Δaxis on the
// given RGB axis. RGBMethod is Euclidean in RGB, so the bound is exact.
// Redmean's per-axis weights have known floors: (512+rmean)/256 ≥ 2 for
// R, exactly 4 for G, (767-rmean)/256 ≥ 2 for B. LAB and custom methods
// have no safe bound in RGB space, so they never prune — palettes are a
// few hundred colors at most, making the full traversal cheap.
func planePruneFactor(method ColorDistanceMethod, axis int) float64 {
	switch method.(type) {
	case RGBMethod:
		return 1
	case RedmeanMethod:
		if axis == 1 {
			return 2
		}
		return math.Sqrt2
	default:
		return 0
	}
}

// ColorDistance pairs a color with its distance to a query point.
type ColorDistance struct {
	color    RGB
	distance float64
}

// kNearestNeighbors finds the k-nearest neighbors of a target color in a
// KD-tree. The function takes the root node of the KD-tree, the target color,
// and the number of neighbors to find as input, and returns a slice of colors
// sorted by distance.
func (node *ColorNode) kNearestNeighbors(target RGB, k int, method ColorDistanceMethod) []RGB {
	allColors := node.getAllColors()
	if len(allColors) <= k {
		return allColors
	}

	pq := make(PriorityQueue, 0, k)
	heap.Init(&pq)

	for _, color := range allColors {
		dist := method.Distance(color, target)
		if pq.Len() < k {
			heap.Push(&pq, ColorDistance{color, dist})
		} else if dist < pq[0].distance {
			heap.Pop(&pq)
			heap.Push(&pq, ColorDistance{color, dist})
		}
	}

	result := make([]RGB, k)
	for i := k - 1; i >= 0; i-- {
		result[i] = heap.Pop(&pq).(ColorDistance).color
	}

	return result
}

func (node *ColorNode) getAllColors() []RGB {
	if node == nil {
		return nil
	}
	colors := []RGB{node.Color}
	colors = append(colors, node.Left.getAllColors()...)
	colors = append(colors, node.Right.getAllColors()...)
	return colors
}

func (node *ColorNode) Serialize() []byte {
	if node == nil {
		return []byte{0} // Null node
	}

	data := []byte{1} // Non-null node
	data = append(data, node.Color.R, node.Color.G, node.Color.B)
	data = append(data, byte(node.SplitAxis))
	data = append(data, node.Left.Serialize()...)
	data = append(data, node.Right.Serialize()...)

	return data
}

func DeserializeKDTree(data []byte) (*ColorNode, []byte) {
	if len(data) == 0 || data[0] == 0 {
		return nil, data[1:]
	}

	node := &ColorNode{
		Color:     RGB{data[1], data[2], data[3]},
		SplitAxis: int(data[4]),
	}

	node.Left, data = DeserializeKDTree(data[5:])
	node.Right, data = DeserializeKDTree(data)

	return node, data
}
