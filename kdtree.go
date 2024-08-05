package main

import (
	"container/heap"
	"math"
	"sort"
)

type ColorNode struct {
	Color       RGB
	Left, Right *ColorNode
	SplitAxis   int
}

func buildKDTree(colors []RGB, depth int, maxDepth int) *ColorNode {
	if len(colors) == 0 || depth >= maxDepth {
		return nil
	}

	// Choose splitting axis based on the dimension with the largest variance
	axis := chooseSplitAxis(colors)

	// Sort colors along the chosen axis
	sort.Slice(colors, func(i, j int) bool {
		return getColorComponent(colors[i], axis) < getColorComponent(colors[j], axis)
	})

	median := len(colors) / 2
	return &ColorNode{
		Color:     colors[median],
		Left:      buildKDTree(colors[:median], depth+1, maxDepth),
		Right:     buildKDTree(colors[median+1:], depth+1, maxDepth),
		SplitAxis: axis,
	}
}

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

func nearestNeighbor(root *ColorNode, target RGB, best RGB, bestDist float64, depth int) (RGB, float64) {
	if root == nil {
		return best, bestDist
	}

	dist := root.Color.colorDistance(target)
	if dist < bestDist {
		best = root.Color
		bestDist = dist
	}

	axis := depth % 3
	var next, other *ColorNode
	switch axis {
	case 0:
		if target.r < root.Color.r {
			next, other = root.Left, root.Right
		} else {
			next, other = root.Right, root.Left
		}
	case 1:
		if target.g < root.Color.g {
			next, other = root.Left, root.Right
		} else {
			next, other = root.Right, root.Left
		}
	default:
		if target.b < root.Color.b {
			next, other = root.Left, root.Right
		} else {
			next, other = root.Right, root.Left
		}
	}

	best, bestDist = nearestNeighbor(next, target, best, bestDist, depth+1)

	// Check if we need to search the other branch
	var axisDistance float64
	switch axis {
	case 0:
		axisDistance = float64(target.r - root.Color.r)
	case 1:
		axisDistance = float64(target.g - root.Color.g)
	default:
		axisDistance = float64(target.b - root.Color.b)
	}
	if axisDistance*axisDistance < bestDist {
		best, bestDist = nearestNeighbor(other, target, best, bestDist, depth+1)
	}

	return best, bestDist
}

// ColorDistance is a helper struct to keep track of colors and their distances
type ColorDistance struct {
	color    RGB
	distance float64
}

// PriorityQueue implements heap.Interface and holds ColorDistance items
type PriorityQueue []ColorDistance

func (pq PriorityQueue) Len() int { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].distance > pq[j].distance // Max heap
}
func (pq PriorityQueue) Swap(i, j int) { pq[i], pq[j] = pq[j], pq[i] }

func (pq *PriorityQueue) Push(x interface{}) {
	*pq = append(*pq, x.(ColorDistance))
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

func kNearestNeighbors(root *ColorNode, target RGB, k int) []RGB {
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

	search(root, 0)

	result := make([]RGB, k)
	for i := k - 1; i >= 0; i-- {
		result[i] = heap.Pop(&pq).(ColorDistance).color
	}

	return result
}
