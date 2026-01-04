package img2ansi

import (
	"math"
	"sync"

	"github.com/wbrown/img2ansi/imageutil"
)

// RGB represents a color in the RGB color space with 8-bit channels,
// where each channel ranges from 0 to 255. The RGB color space is
// additive, meaning that colors are created by adding together the
// red, green, and blue channels.
type RGB struct {
	R, G, B uint8
}

type LAB struct {
	L float64
	A float64
	B float64
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

var (
	sRGBToLinearLookup [256]float64
	linearToSRGBLookup [1024]uint8
	labInitOnce        sync.Once
)

// toUint32 converts an RGB color to a 32-bit unsigned integer
func (rgb RGB) toUint32() uint32 {
	return uint32(rgb.R)<<16 | uint32(rgb.G)<<8 | uint32(rgb.B)
}

// rgbsPairToUint256 converts two pairs of RGB colors to a single Uint256
// Value. The function takes two arrays of four RGB colors each and returns
// a single Uint256 Value.
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
// Value.
func rgbsToUint128(colors [4]RGB) Uint128 {
	return Uint128{
		High: uint64(colors[0].R)<<56 | uint64(colors[0].G)<<48 |
			uint64(colors[0].B)<<40 | uint64(colors[1].R)<<32 |
			uint64(colors[1].G)<<24 | uint64(colors[1].B)<<16 |
			uint64(colors[2].R)<<8 | uint64(colors[2].G),
		Low: uint64(colors[2].B)<<56 | uint64(colors[3].R)<<48 |
			uint64(colors[3].G)<<40 | uint64(colors[3].B)<<32,
	}
}

// rgbFromImageutil converts an imageutil.RGB to the local RGB type.
func rgbFromImageutil(color imageutil.RGB) RGB {
	return RGB{
		R: color.R,
		G: color.G,
		B: color.B,
	}
}

// toImageutil converts a local RGB to imageutil.RGB.
func (rgb RGB) toImageutil() imageutil.RGB {
	return imageutil.RGB{
		R: rgb.R,
		G: rgb.G,
		B: rgb.B,
	}
}

// rgbFromUint32 converts a 32-bit unsigned integer to an RGB color
func rgbFromUint32(color uint32) RGB {
	return RGB{
		R: uint8(color >> 16),
		G: uint8(color >> 8),
		B: uint8(color),
	}
}

// dithError calculates the error between two RGB colors in the RGB color
// space. It returns a 3-element array of floating-point numbers representing
// the error in the red, green, and blue channels, respectively.
func (rgb RGB) dithError(c2 RGB) [3]float64 {
	return [3]float64{
		float64(rgb.R) - float64(c2.R),
		float64(rgb.G) - float64(c2.G),
		float64(rgb.B) - float64(c2.B),
	}
}

// quantizeColor quantizes an RGB color by rounding each channel to the
// nearest multiple of the quantization factor. The function returns the
// quantized RGB color.
func (rgb RGB) quantizeColor(quantization int) RGB {
	qFactor := 256 / float64(quantization)
	return RGB{
		uint8(math.Round(float64(rgb.R)/qFactor) * qFactor),
		uint8(math.Round(float64(rgb.G)/qFactor) * qFactor),
		uint8(math.Round(float64(rgb.B)/qFactor) * qFactor),
	}
}

// RGBError represents signed color errors for dithering
type RGBError struct {
	R, G, B int16
}

// subtractToError calculates the signed error between two RGB colors
// for use in error diffusion dithering
func (rgb RGB) subtractToError(other RGB) RGBError {
	return RGBError{
		R: int16(rgb.R) - int16(other.R),
		G: int16(rgb.G) - int16(other.G),
		B: int16(rgb.B) - int16(other.B),
	}
}

func initLab() {
	for i := 0; i < 256; i++ {
		f := float64(i) / 255.0
		if f > 0.04045 {
			sRGBToLinearLookup[i] = math.Pow((f+0.055)/1.055, 2.4)
		} else {
			sRGBToLinearLookup[i] = f / 12.92
		}
	}

	for i := 0; i < 1024; i++ {
		f := float64(i) / 1023.0
		if f > 0.0031308 {
			linearToSRGBLookup[i] = uint8(math.Min(255, math.Round(255*(1.055*math.Pow(f, 1/2.4)-0.055))))
		} else {
			linearToSRGBLookup[i] = uint8(math.Min(255, math.Round(f*12.92*255)))
		}
	}
}

// Convert RGB to CIE L*a*B*
// toLab converts RGB to CIE L*a*B*
func (rgb RGB) toLab() LAB {
	labInitOnce.Do(initLab)
	r := sRGBToLinearLookup[rgb.R]
	g := sRGBToLinearLookup[rgb.G]
	b := sRGBToLinearLookup[rgb.B]

	x := r*0.4124564 + g*0.3575761 + b*0.1804375
	y := r*0.2126729 + g*0.7151522 + b*0.0721750
	z := r*0.0193339 + g*0.1191920 + b*0.9503041

	x /= 0.95047
	y /= 1.00000
	z /= 1.08883

	fx := labf(x)
	fy := labf(y)
	fz := labf(z)

	return LAB{
		L: 116.0*fy - 16.0,
		A: 500.0 * (fx - fy),
		B: 200.0 * (fy - fz),
	}
}

func labf(t float64) float64 {
	if t > 0.008856 {
		return math.Pow(t, 1.0/3.0)
	}
	return 7.787*t + 16.0/116.0
}

func (lab LAB) toRGB() RGB {
	labInitOnce.Do(initLab)
	y := (lab.L + 16.0) / 116.0
	x := lab.A/500.0 + y
	z := y - lab.B/200.0

	x = labfInv(x) * 0.95047
	y = labfInv(y) * 1.00000
	z = labfInv(z) * 1.08883

	r := x*3.2404542 - y*1.5371385 - z*0.4985314
	g := -x*0.9692660 + y*1.8760108 + z*0.0415560
	b := x*0.0556434 - y*0.2040259 + z*1.0572252

	return RGB{
		R: linearToSRGBLookup[int(math.Min(math.Max(r, 0), 1)*1023)],
		G: linearToSRGBLookup[int(math.Min(math.Max(g, 0), 1)*1023)],
		B: linearToSRGBLookup[int(math.Min(math.Max(b, 0), 1)*1023)],
	}
}

func labfInv(t float64) float64 {
	if t > 0.206893 {
		return t * t * t
	}
	return (t - 16.0/116.0) / 7.787
}

// ColorDistanceMethod is an interface for different color distance calculation methods.
// This allows users to implement custom distance metrics optimized for their use case.
type ColorDistanceMethod interface {
	// Distance calculates the perceptual distance between two colors.
	Distance(c1, c2 RGB) float64
	// Name returns the name of this distance method for debugging/display.
	Name() string
}

// RGBMethod calculates simple Euclidean distance in RGB space.
// Fast but not perceptually accurate.
type RGBMethod struct{}

func (m RGBMethod) Distance(c1, c2 RGB) float64 {
	dr := int(c1.R) - int(c2.R)
	dg := int(c1.G) - int(c2.G)
	db := int(c1.B) - int(c2.B)
	return math.Sqrt(float64(dr*dr + dg*dg + db*db))
}

func (m RGBMethod) Name() string { return "RGB" }

// LABMethod calculates CIE76 distance in L*a*b* perceptually uniform color space.
// Most accurate but slower due to color space conversion.
type LABMethod struct{}

func (m LABMethod) Distance(c1, c2 RGB) float64 {
	lab1 := c1.toLab()
	lab2 := c2.toLab()

	return math.Sqrt(
		math.Pow(lab2.L-lab1.L, 2) +
			math.Pow(lab2.A-lab1.A, 2) +
			math.Pow(lab2.B-lab1.B, 2))
}

func (m LABMethod) Name() string { return "LAB" }

// RedmeanMethod uses weighted Euclidean distance as fast perceptual approximation.
// Good balance between speed and perceptual accuracy.
type RedmeanMethod struct{}

func (m RedmeanMethod) Distance(c1, c2 RGB) float64 {
	rmean := (int(c1.R) + int(c2.R)) / 2
	dr := int(c1.R) - int(c2.R)
	dg := int(c1.G) - int(c2.G)
	db := int(c1.B) - int(c2.B)

	return math.Sqrt(float64(
		(((512 + rmean) * dr * dr) >> 8) +
			4*dg*dg +
			(((767 - rmean) * db * db) >> 8)))
}

func (m RedmeanMethod) Name() string { return "Redmean" }

// ColorDistance is a convenience method for use in KD-tree building and palette generation.
// It defaults to Redmean distance. For runtime color matching, use ColorDistanceMethod interface instead.
func (rgb RGB) ColorDistance(other RGB) float64 {
	return RedmeanMethod{}.Distance(rgb, other)
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
		if s[i].color.R != s[j].color.R {
			return s[i].color.R < s[j].color.R
		}
		if s[i].color.G != s[j].color.G {
			return s[i].color.G < s[j].color.G
		}
		if s[i].color.B != s[j].color.B {
			return s[i].color.B < s[j].color.B
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
	if s[i].R != s[j].R {
		return s[i].R < s[j].R
	}
	if s[i].G != s[j].G {
		return s[i].G < s[j].G
	}
	return s[i].B < s[j].B
}
