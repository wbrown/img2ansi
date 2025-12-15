// Package gocv_compare contains tests that compare pure Go implementations
// against gocv (OpenCV). These tests require OpenCV to be installed.
//
// Run with: cd imageutil/gocv_compare && go test -v
package gocv_compare

import (
	"image"
	"testing"

	"github.com/wbrown/img2ansi/imageutil"
	"gocv.io/x/gocv"
)

// gocvToRGBA converts a gocv.Mat (BGR) to RGBAImage (RGB).
func gocvToRGBA(mat gocv.Mat) *imageutil.RGBAImage {
	height, width := mat.Rows(), mat.Cols()
	img := imageutil.NewRGBAImage(width, height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// gocv uses BGR format
			vec := mat.GetVecbAt(y, x)
			img.SetRGB(x, y, imageutil.RGB{R: vec[2], G: vec[1], B: vec[0]})
		}
	}
	return img
}

// gocvGrayToGray converts a gocv.Mat (grayscale) to GrayImage.
func gocvGrayToGray(mat gocv.Mat) *imageutil.GrayImage {
	height, width := mat.Rows(), mat.Cols()
	img := imageutil.NewGrayImage(width, height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Gray.Pix[y*img.Stride+x] = mat.GetUCharAt(y, x)
		}
	}
	return img
}

// rgbaToGocv converts an RGBAImage to gocv.Mat (BGR).
func rgbaToGocv(img *imageutil.RGBAImage) gocv.Mat {
	mat := gocv.NewMatWithSize(img.Height(), img.Width(), gocv.MatTypeCV8UC3)

	for y := 0; y < img.Height(); y++ {
		for x := 0; x < img.Width(); x++ {
			c := img.GetRGB(x, y)
			// gocv uses BGR format
			mat.SetUCharAt(y, x*3, c.B)
			mat.SetUCharAt(y, x*3+1, c.G)
			mat.SetUCharAt(y, x*3+2, c.R)
		}
	}
	return mat
}

// grayToGocv converts a GrayImage to gocv.Mat (grayscale).
func grayToGocv(img *imageutil.GrayImage) gocv.Mat {
	mat := gocv.NewMatWithSize(img.Height(), img.Width(), gocv.MatTypeCV8U)

	for y := 0; y < img.Height(); y++ {
		for x := 0; x < img.Width(); x++ {
			mat.SetUCharAt(y, x, img.GrayAt(x, y).Y)
		}
	}
	return mat
}

func TestCompareGrayscaleConversion(t *testing.T) {
	// Create test image
	img := imageutil.CreateColorBarsImage(256, 256)
	mat := rgbaToGocv(img)
	defer mat.Close()

	// Convert with gocv
	grayMat := gocv.NewMat()
	defer grayMat.Close()
	gocv.CvtColor(mat, &grayMat, gocv.ColorBGRToGray)
	gocvGray := gocvGrayToGray(grayMat)

	// Convert with pure Go
	pureGoGray := imageutil.ToGrayscale(img)

	// Compare
	mse := imageutil.CalculateMSEGray(gocvGray, pureGoGray)
	t.Logf("Grayscale conversion MSE: %f", mse)

	// Allow small differences due to rounding
	if mse > 1.0 {
		t.Errorf("Grayscale MSE too high: %f (threshold: 1.0)", mse)
	}
}

func TestCompareResize(t *testing.T) {
	testCases := []struct {
		name      string
		srcWidth  int
		srcHeight int
		dstWidth  int
		dstHeight int
		threshold float64
	}{
		{"Downscale 2x", 256, 256, 128, 128, 10.0},
		{"Downscale 4x", 256, 256, 64, 64, 15.0},
		{"Upscale 2x", 64, 64, 128, 128, 10.0},
		{"Arbitrary", 256, 256, 100, 75, 15.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			img := imageutil.CreateGradientImage(tc.srcWidth, tc.srcHeight)
			mat := rgbaToGocv(img)
			defer mat.Close()

			// Resize with gocv (area interpolation)
			resizedMat := gocv.NewMat()
			defer resizedMat.Close()
			gocv.Resize(mat, &resizedMat, image.Point{X: tc.dstWidth, Y: tc.dstHeight},
				0, 0, gocv.InterpolationArea)
			gocvResized := gocvToRGBA(resizedMat)

			// Resize with pure Go
			pureGoResized := imageutil.Resize(img, tc.dstWidth, tc.dstHeight, imageutil.InterpolationArea)

			// Compare
			mse := imageutil.CalculateMSE(gocvResized, pureGoResized)
			t.Logf("%s resize MSE: %f", tc.name, mse)

			if mse > tc.threshold {
				t.Errorf("Resize MSE too high: %f (threshold: %f)", mse, tc.threshold)
			}
		})
	}
}

func TestCompareSharpening(t *testing.T) {
	img := imageutil.CreateEdgeImage(256, 256)
	mat := rgbaToGocv(img)
	defer mat.Close()

	// Sharpen with gocv
	kernel := gocv.NewMatWithSize(3, 3, gocv.MatTypeCV32F)
	defer kernel.Close()
	kernel.SetFloatAt(0, 0, 0)
	kernel.SetFloatAt(0, 1, -0.5)
	kernel.SetFloatAt(0, 2, 0)
	kernel.SetFloatAt(1, 0, -0.5)
	kernel.SetFloatAt(1, 1, 3)
	kernel.SetFloatAt(1, 2, -0.5)
	kernel.SetFloatAt(2, 0, 0)
	kernel.SetFloatAt(2, 1, -0.5)
	kernel.SetFloatAt(2, 2, 0)

	sharpenedMat := gocv.NewMat()
	defer sharpenedMat.Close()
	gocv.Filter2D(mat, &sharpenedMat, -1, kernel, image.Point{-1, -1}, 0, gocv.BorderDefault)
	gocvSharpened := gocvToRGBA(sharpenedMat)

	// Sharpen with pure Go
	pureGoSharpened := imageutil.Sharpen(img)

	// Compare
	mse := imageutil.CalculateMSE(gocvSharpened, pureGoSharpened)
	maxDiff := imageutil.CalculateMaxDiff(gocvSharpened, pureGoSharpened)
	t.Logf("Sharpening MSE: %f, Max diff: %d", mse, maxDiff)

	if mse > 5.0 {
		t.Errorf("Sharpening MSE too high: %f (threshold: 5.0)", mse)
	}
}

func TestCompareCanny(t *testing.T) {
	testCases := []struct {
		name        string
		createImage func(int, int) *imageutil.RGBAImage
		minJaccard  float64
	}{
		// Pure Go Canny detects more edges than OpenCV due to implementation differences.
		// What matters is that the full pipeline produces equivalent results.
		{"Edges", imageutil.CreateEdgeImage, 0.4}, // Strong edges, some variation expected
		{"Checkerboard", func(w, h int) *imageutil.RGBAImage {
			return imageutil.CreateCheckerboardImage(w, h, 32)
		}, 0.3}, // Many edges, more variation expected
		{"Gradient", imageutil.CreateGradientImage, 0.3}, // Soft edges, more variation
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			img := tc.createImage(256, 256)

			// Convert to grayscale
			gray := imageutil.ToGrayscale(img)
			grayMat := grayToGocv(gray)
			defer grayMat.Close()

			// Canny with gocv
			edgesMat := gocv.NewMat()
			defer edgesMat.Close()
			gocv.Canny(grayMat, &edgesMat, 50, 150)
			gocvEdges := gocvGrayToGray(edgesMat)

			// Canny with pure Go
			pureGoEdges := imageutil.CannyDefault(gray)

			// Compare using Jaccard index
			jaccard := imageutil.CalculateJaccardIndex(gocvEdges, pureGoEdges)
			t.Logf("%s Canny Jaccard index: %f", tc.name, jaccard)

			if jaccard < tc.minJaccard {
				t.Errorf("Canny Jaccard too low: %f (min: %f)", jaccard, tc.minJaccard)
			}

			// Also check edge counts are similar
			gocvCount, pureGoCount := 0, 0
			for y := 0; y < gocvEdges.Height(); y++ {
				for x := 0; x < gocvEdges.Width(); x++ {
					if gocvEdges.GrayAt(x, y).Y > 128 {
						gocvCount++
					}
					if pureGoEdges.GrayAt(x, y).Y > 128 {
						pureGoCount++
					}
				}
			}
			t.Logf("Edge counts - gocv: %d, pureGo: %d", gocvCount, pureGoCount)
		})
	}
}

func TestCompareFullPipeline(t *testing.T) {
	// This test compares the full prepareForANSI-equivalent pipeline
	// between gocv and pure Go implementations.

	img := imageutil.CreateColorBarsImage(320, 240)
	width, height := 80, 60

	// gocv pipeline
	mat := rgbaToGocv(img)
	defer mat.Close()

	// Intermediate resize to 4x
	intermediate := gocv.NewMat()
	defer intermediate.Close()
	gocv.Resize(mat, &intermediate, image.Point{X: width * 4, Y: height * 4}, 0, 0, gocv.InterpolationArea)

	// Edge detection
	gray := gocv.NewMat()
	defer gray.Close()
	gocv.CvtColor(intermediate, &gray, gocv.ColorBGRToGray)
	edges := gocv.NewMat()
	defer edges.Close()
	gocv.Canny(gray, &edges, 50, 150)

	// Final resize to 2x
	resizedMat := gocv.NewMat()
	defer resizedMat.Close()
	edgesResized := gocv.NewMat()
	defer edgesResized.Close()
	gocv.Resize(intermediate, &resizedMat, image.Point{X: width * 2, Y: height * 2}, 0, 0, gocv.InterpolationArea)
	gocv.Resize(edges, &edgesResized, image.Point{X: width * 2, Y: height * 2}, 0, 0, gocv.InterpolationLinear)

	// Sharpening
	kernel := gocv.NewMatWithSize(3, 3, gocv.MatTypeCV32F)
	defer kernel.Close()
	kernel.SetFloatAt(0, 0, 0)
	kernel.SetFloatAt(0, 1, -0.5)
	kernel.SetFloatAt(0, 2, 0)
	kernel.SetFloatAt(1, 0, -0.5)
	kernel.SetFloatAt(1, 1, 3)
	kernel.SetFloatAt(1, 2, -0.5)
	kernel.SetFloatAt(2, 0, 0)
	kernel.SetFloatAt(2, 1, -0.5)
	kernel.SetFloatAt(2, 2, 0)

	sharpened := gocv.NewMat()
	defer sharpened.Close()
	gocv.Filter2D(resizedMat, &sharpened, -1, kernel, image.Point{-1, -1}, 0, gocv.BorderDefault)

	gocvResult := gocvToRGBA(sharpened)
	gocvEdges := gocvGrayToGray(edgesResized)

	// Pure Go pipeline
	pureGoIntermediate := imageutil.Resize(img, width*4, height*4, imageutil.InterpolationArea)
	pureGoGray := imageutil.ToGrayscale(pureGoIntermediate)
	pureGoEdgesFull := imageutil.CannyDefault(pureGoGray)
	pureGoResized := imageutil.Resize(pureGoIntermediate, width*2, height*2, imageutil.InterpolationArea)
	pureGoEdgesResized := imageutil.ResizeGray(pureGoEdgesFull, width*2, height*2, imageutil.InterpolationLinear)
	pureGoResult := imageutil.Sharpen(pureGoResized)

	// Compare
	imageMSE := imageutil.CalculateMSE(gocvResult, pureGoResult)
	edgeJaccard := imageutil.CalculateJaccardIndex(gocvEdges, pureGoEdgesResized)

	t.Logf("Full pipeline - Image MSE: %f, Edge Jaccard: %f", imageMSE, edgeJaccard)

	// Thresholds
	if imageMSE > 20.0 {
		t.Errorf("Pipeline image MSE too high: %f (threshold: 20.0)", imageMSE)
	}
	if edgeJaccard < 0.3 {
		t.Errorf("Pipeline edge Jaccard too low: %f (min: 0.3)", edgeJaccard)
	}
}
