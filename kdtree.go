package main

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

// buildKDTree constructs a KD-tree from a list of RGB colors. The function
// takes a list of colors, the current depth, and the maximum depth of the
// tree as arguments, and returns the root node of the KD-tree.
func buildKDTree(colors []RGB, depth int, maxDepth int) *ColorNode {
	if len(colors) == 0 || depth >= maxDepth {
		return nil
	}

	// Choose splitting axis based on the dimension with the largest variance
	axis := chooseSplitAxis(colors)

	// Sort colors along the chosen axis
	sort.Slice(colors, func(i, j int) bool {
		return getColorComponent(colors[i], axis) <
			getColorComponent(colors[j], axis)
	})

	median := len(colors) / 2
	return &ColorNode{
		Color:     colors[median],
		Left:      buildKDTree(colors[:median], depth+1, maxDepth),
		Right:     buildKDTree(colors[median+1:], depth+1, maxDepth),
		SplitAxis: axis,
	}
}

// chooseSplitAxis selects the axis along which to split the colors in a
// KD-tree. The function takes a list of RGB colors as input and returns
// the index of the axis with the largest variance.
func chooseSplitAxis(colors []RGB) int {
	var varR, varG, varB float64
	var meanR, meanG, meanB float64

	for _, c := range colors {
		meanR += float64(c.r)
		meanG += float64(c.g)
		meanB += float64(c.b)
	}
	meanR /= float64(len(colors))
	meanG /= float64(len(colors))
	meanB /= float64(len(colors))

	for _, c := range colors {
		varR += math.Pow(float64(c.r)-meanR, 2)
		varG += math.Pow(float64(c.g)-meanG, 2)
		varB += math.Pow(float64(c.b)-meanB, 2)
	}

	if varR > varG && varR > varB {
		return 0 // R axis
	} else if varG > varB {
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
		return color.r
	case 1:
		return color.g
	default:
		return color.b
	}
}

// getCandidateColors finds the k-nearest neighbors of the colors in a block
// using a KD-tree. The function takes a block of colors, the depth of the
// search, and the number of neighbors to find as input, and returns a slice
// of colors sorted by distance.
func (node *ColorNode) getCandidateColors(
	block [4]RGB,
	depth int,
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
		nearest := node.kNearestNeighbors(color, depth)
		for _, c := range nearest {
			if _, seen := seenColors[c]; !seen {
				distance := color.colorDistance(c)
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
	target RGB, best RGB, bestDist float64, depth int) (RGB, float64) {
	if node == nil {
		return best, bestDist
	}

	dist := node.Color.colorDistance(target)
	if dist < bestDist {
		best = node.Color
		bestDist = dist
	}

	axis := depth % 3
	var next, other *ColorNode
	switch axis {
	case 0:
		if target.r < node.Color.r {
			next, other = node.Left, node.Right
		} else {
			next, other = node.Right, node.Left
		}
	case 1:
		if target.g < node.Color.g {
			next, other = node.Left, node.Right
		} else {
			next, other = node.Right, node.Left
		}
	default:
		if target.b < node.Color.b {
			next, other = node.Left, node.Right
		} else {
			next, other = node.Right, node.Left
		}
	}

	best, bestDist = next.nearestNeighbor(target, best, bestDist, depth+1)

	// Check if we need to search the other branch
	var axisDistance float64
	switch axis {
	case 0:
		axisDistance = float64(target.r - node.Color.r)
	case 1:
		axisDistance = float64(target.g - node.Color.g)
	default:
		axisDistance = float64(target.b - node.Color.b)
	}
	if axisDistance*axisDistance < bestDist {
		best, bestDist = other.nearestNeighbor(
			target, best, bestDist, depth+1)
	}

	return best, bestDist
}

// ColorDistance is a helper struct to keep track of colors and their
// distances
type ColorDistance struct {
	color    RGB
	distance float64
}

// kNearestNeighbors finds the k-nearest neighbors of a target color in a
// KD-tree. The function takes the root node of the KD-tree, the target color,
// and the number of neighbors to find as input, and returns a slice of colors
// sorted by distance.
func (node *ColorNode) kNearestNeighbors(target RGB, k int) []RGB {
	pq := make(PriorityQueue, 0)
	heap.Init(&pq)

	var search func(*ColorNode, int)
	search = func(node *ColorNode, depth int) {
		if node == nil {
			return
		}

		dist := node.Color.colorDistance(target)

		if pq.Len() < k {
			heap.Push(&pq, ColorDistance{node.Color, dist})
		} else if dist < pq[0].distance {
			heap.Pop(&pq)
			heap.Push(&pq, ColorDistance{node.Color, dist})
		}

		axis := depth % 3
		var firstChild, secondChild *ColorNode
		var axisDist float64

		switch axis {
		case 0:
			axisDist = float64(target.r) - float64(node.Color.r)
			if axisDist < 0 {
				firstChild, secondChild = node.Left, node.Right
			} else {
				firstChild, secondChild = node.Right, node.Left
			}
		case 1:
			axisDist = float64(target.g) - float64(node.Color.g)
			if axisDist < 0 {
				firstChild, secondChild = node.Left, node.Right
			} else {
				firstChild, secondChild = node.Right, node.Left
			}
		case 2:
			axisDist = float64(target.b) - float64(node.Color.b)
			if axisDist < 0 {
				firstChild, secondChild = node.Left, node.Right
			} else {
				firstChild, secondChild = node.Right, node.Left
			}
		}

		search(firstChild, depth+1)

		if pq.Len() < k || axisDist*axisDist < pq[0].distance {
			search(secondChild, depth+1)
		}
	}

	search(node, 0)

	result := make([]RGB, k)
	for i := k - 1; i >= 0; i-- {
		result[i] = heap.Pop(&pq).(ColorDistance).color
	}

	return result
}
