package main

import (
	"gocv.io/x/gocv"
	"math"
)

// RGB represents a color in the RGB color space with 8-bit channels,
// where each channel ranges from 0 to 255. The RGB color space is
// additive, meaning that colors are created by adding together the
// red, green, and blue channels.
type RGB struct {
	r, g, b uint8
}

type Uint128 struct {
	High uint64
	Low  uint64
}

type Uint256 struct {
	Highest uint64
	High    uint64
	Low     uint64
	Lowest  uint64
}

// toUint32 converts an RGB color to a 32-bit unsigned integer
func (r RGB) toUint32() uint32 {
	return uint32(r.r)<<16 | uint32(r.g)<<8 | uint32(r.b)
}

// rgbsPairToUint256 converts two pairs of RGB colors to a single Uint256
// value. The function takes two arrays of four RGB colors each and returns
// a single Uint256 value.
func rgbsPairToUint256(left [4]RGB, right [4]RGB) Uint256 {
	high := rgbsToUint128(left)
	low := rgbsToUint128(right)
	return Uint256{
		Highest: high.High,
		High:    high.Low,
		Low:     low.High,
		Lowest:  low.Low,
	}
}

// rgbsToUint128 converts an array of four RGB colors to a single Uint128
// value.
func rgbsToUint128(colors [4]RGB) Uint128 {
	return Uint128{
		High: uint64(colors[0].r)<<56 | uint64(colors[0].g)<<48 |
			uint64(colors[0].b)<<40 | uint64(colors[1].r)<<32 |
			uint64(colors[1].g)<<24 | uint64(colors[1].b)<<16 |
			uint64(colors[2].r)<<8 | uint64(colors[2].g),
		Low: uint64(colors[2].b)<<56 | uint64(colors[3].r)<<48 |
			uint64(colors[3].g)<<40 | uint64(colors[3].b)<<32,
	}
}

// rgbFromVecb converts a gocv.Vecb to an RGB color
func rgbFromVecb(color gocv.Vecb) RGB {
	return RGB{
		r: color[2],
		g: color[1],
		b: color[0],
	}
}

// rgbFromUint32 converts a 32-bit unsigned integer to an RGB color
func rgbFromUint32(color uint32) RGB {
	return RGB{
		r: uint8(color >> 16),
		g: uint8(color >> 8),
		b: uint8(color),
	}
}

// dithError calculates the error between two RGB colors in the RGB color
// space. It returns a 3-element array of floating-point numbers representing
// the error in the red, green, and blue channels, respectively.
func (r RGB) dithError(c2 RGB) [3]float64 {
	return [3]float64{
		float64(r.r) - float64(c2.r),
		float64(r.g) - float64(c2.g),
		float64(r.b) - float64(c2.b),
	}
}

// quantizeColor quantizes an RGB color by rounding each channel to the
// nearest multiple of the quantization factor. The function returns the
// quantized RGB color.
func (r RGB) quantizeColor() RGB {
	qFactor := 256 / float64(Quantization)
	return RGB{
		uint8(math.Round(float64(r.r)/qFactor) * qFactor),
		uint8(math.Round(float64(r.g)/qFactor) * qFactor),
		uint8(math.Round(float64(r.b)/qFactor) * qFactor),
	}
}

// subtract subtracts the RGB channels of another RGB color from the
// corresponding channels of the current RGB color. The function returns
// a new RGB color with the result of the subtraction.
func (r RGB) subtract(other RGB) RGB {
	return RGB{
		r: uint8(math.Max(0, float64(r.r)-float64(other.r))),
		g: uint8(math.Max(0, float64(r.g)-float64(other.g))),
		b: uint8(math.Max(0, float64(r.b)-float64(other.b))),
	}
}

// colorDistance calculates the Euclidean distance between two RGB colors
// in the RGB color space. The function returns the distance as a floating-
// point number.
func (r RGB) colorDistance(other RGB) float64 {
	dr := int(r.r) - int(other.r)
	dg := int(r.g) - int(other.g)
	db := int(r.b) - int(other.b)
	return math.Sqrt(float64(dr*dr + dg*dg + db*db))
}

const epsilon = 0.000001 // For floating-point comparisons

// colorWithDistance is a struct that contains an RGB color and its distance
// from a target color. The index field is used to ensure stable sorting.
type colorWithDistance struct {
	color    RGB
	distance float64
	index    int // To ensure stable sorting
}

// colorDistanceSlice is a slice of colorWithDistance structs
type colorDistanceSlice []colorWithDistance

func (s colorDistanceSlice) Len() int      { return len(s) }
func (s colorDistanceSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s colorDistanceSlice) Less(i, j int) bool {
	if math.Abs(s[i].distance-s[j].distance) < epsilon {
		// If distances are equal, use color components for tie-breaking
		if s[i].color.r != s[j].color.r {
			return s[i].color.r < s[j].color.r
		}
		if s[i].color.g != s[j].color.g {
			return s[i].color.g < s[j].color.g
		}
		if s[i].color.b != s[j].color.b {
			return s[i].color.b < s[j].color.b
		}
		// If colors are identical, use the index for stable sorting
		return s[i].index < s[j].index
	}
	return s[i].distance < s[j].distance
}

type sortableRGB []RGB

func (s sortableRGB) Len() int      { return len(s) }
func (s sortableRGB) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s sortableRGB) Less(i, j int) bool {
	if s[i].r != s[j].r {
		return s[i].r < s[j].r
	}
	if s[i].g != s[j].g {
		return s[i].g < s[j].g
	}
	return s[i].b < s[j].b
}
