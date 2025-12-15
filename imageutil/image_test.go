package imageutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewRGBAImage(t *testing.T) {
	img := NewRGBAImage(100, 50)
	if img.Width() != 100 {
		t.Errorf("Expected width 100, got %d", img.Width())
	}
	if img.Height() != 50 {
		t.Errorf("Expected height 50, got %d", img.Height())
	}
}

func TestRGBAImageGetSetRGB(t *testing.T) {
	img := NewRGBAImage(10, 10)
	c := RGB{R: 100, G: 150, B: 200}
	img.SetRGB(5, 5, c)

	got := img.GetRGB(5, 5)
	if got != c {
		t.Errorf("Expected %v, got %v", c, got)
	}
}

func TestRGBAImageClone(t *testing.T) {
	img := NewRGBAImage(10, 10)
	img.SetRGB(5, 5, RGB{R: 255, G: 0, B: 0})

	clone := img.Clone()
	if clone.GetRGB(5, 5) != img.GetRGB(5, 5) {
		t.Error("Clone should have same pixel values")
	}

	// Modify clone, original should be unchanged
	clone.SetRGB(5, 5, RGB{R: 0, G: 255, B: 0})
	if img.GetRGB(5, 5).G != 0 {
		t.Error("Modifying clone should not affect original")
	}
}

func TestNewGrayImage(t *testing.T) {
	img := NewGrayImage(100, 50)
	if img.Width() != 100 {
		t.Errorf("Expected width 100, got %d", img.Width())
	}
	if img.Height() != 50 {
		t.Errorf("Expected height 50, got %d", img.Height())
	}
}

func TestGrayImageGetSetGray(t *testing.T) {
	img := NewGrayImage(10, 10)
	img.Gray.Pix[5*img.Stride+5] = 128

	got := img.GrayAt(5, 5).Y
	if got != 128 {
		t.Errorf("Expected 128, got %d", got)
	}
}

func TestToGrayscale(t *testing.T) {
	// Test with known values
	img := NewRGBAImage(1, 1)
	img.SetRGB(0, 0, RGB{R: 255, G: 255, B: 255})

	gray := ToGrayscale(img)
	v := gray.GrayAt(0, 0).Y

	// White should produce white (255)
	if v != 255 {
		t.Errorf("White pixel should convert to 255, got %d", v)
	}

	// Test black
	img.SetRGB(0, 0, RGB{R: 0, G: 0, B: 0})
	gray = ToGrayscale(img)
	v = gray.GrayAt(0, 0).Y
	if v != 0 {
		t.Errorf("Black pixel should convert to 0, got %d", v)
	}

	// Test red (0.299 * 255 = 76.245)
	img.SetRGB(0, 0, RGB{R: 255, G: 0, B: 0})
	gray = ToGrayscale(img)
	v = gray.GrayAt(0, 0).Y
	if v < 75 || v > 77 {
		t.Errorf("Red pixel should convert to ~76, got %d", v)
	}
}

func TestResize(t *testing.T) {
	img := CreateGradientImage(100, 100)

	// Downscale
	resized := Resize(img, 50, 50, InterpolationArea)
	if resized.Width() != 50 || resized.Height() != 50 {
		t.Errorf("Expected 50x50, got %dx%d", resized.Width(), resized.Height())
	}

	// Upscale
	resized = Resize(img, 200, 200, InterpolationLinear)
	if resized.Width() != 200 || resized.Height() != 200 {
		t.Errorf("Expected 200x200, got %dx%d", resized.Width(), resized.Height())
	}
}

func TestConvolve(t *testing.T) {
	img := CreateGradientImage(10, 10)

	// Test identity kernel (should produce same image)
	identity := NewKernel([][]float64{
		{0, 0, 0},
		{0, 1, 0},
		{0, 0, 0},
	})
	result := Convolve(img, identity)

	// Check center pixels (avoid borders)
	for y := 1; y < 9; y++ {
		for x := 1; x < 9; x++ {
			c1 := img.GetRGB(x, y)
			c2 := result.GetRGB(x, y)
			if c1 != c2 {
				t.Errorf("Identity kernel should preserve pixels at (%d,%d): %v != %v", x, y, c1, c2)
			}
		}
	}
}

func TestSharpen(t *testing.T) {
	img := CreateEdgeImage(100, 100)
	sharpened := Sharpen(img)

	if sharpened.Width() != img.Width() || sharpened.Height() != img.Height() {
		t.Error("Sharpened image should have same dimensions")
	}
}

func TestCanny(t *testing.T) {
	// Create image with clear edges
	img := CreateEdgeImage(100, 100)
	gray := ToGrayscale(img)
	edges := CannyDefault(gray)

	if edges.Width() != img.Width() || edges.Height() != img.Height() {
		t.Error("Edge image should have same dimensions")
	}

	// Check that some edges were detected
	edgeCount := 0
	for y := 0; y < edges.Height(); y++ {
		for x := 0; x < edges.Width(); x++ {
			if edges.GrayAt(x, y).Y > 128 {
				edgeCount++
			}
		}
	}

	if edgeCount == 0 {
		t.Error("Canny should detect some edges in edge test image")
	}
}

func TestLoadSaveImage(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create test image
	img := CreateColorBarsImage(64, 64)

	// Save to PNG
	pngPath := filepath.Join(tmpDir, "test.png")
	err := SaveImage(img.RGBA, pngPath)
	if err != nil {
		t.Fatalf("Failed to save PNG: %v", err)
	}

	// Load back
	loaded, err := LoadImage(pngPath)
	if err != nil {
		t.Fatalf("Failed to load PNG: %v", err)
	}

	// PNG should be lossless
	mse := CalculateMSE(img, loaded)
	if mse > 0.01 {
		t.Errorf("PNG should be lossless, MSE=%f", mse)
	}
}

func TestCalculateMSE(t *testing.T) {
	img1 := NewRGBAImage(10, 10)
	img2 := NewRGBAImage(10, 10)

	// Same images should have MSE of 0
	mse := CalculateMSE(img1, img2)
	if mse != 0 {
		t.Errorf("Identical images should have MSE=0, got %f", mse)
	}

	// Different images
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img1.SetRGB(x, y, RGB{R: 0, G: 0, B: 0})
			img2.SetRGB(x, y, RGB{R: 10, G: 10, B: 10})
		}
	}
	mse = CalculateMSE(img1, img2)
	expected := 100.0 // 10^2 = 100
	if mse != expected {
		t.Errorf("Expected MSE=%f, got %f", expected, mse)
	}
}

func TestCalculateJaccardIndex(t *testing.T) {
	edges1 := NewGrayImage(10, 10)
	edges2 := NewGrayImage(10, 10)

	// No edges - should be 1 (both empty)
	j := CalculateJaccardIndex(edges1, edges2)
	if j != 1.0 {
		t.Errorf("Empty images should have Jaccard=1, got %f", j)
	}

	// Same edges
	for x := 0; x < 5; x++ {
		edges1.Gray.Pix[5*edges1.Stride+x] = 255
		edges2.Gray.Pix[5*edges2.Stride+x] = 255
	}
	j = CalculateJaccardIndex(edges1, edges2)
	if j != 1.0 {
		t.Errorf("Identical edges should have Jaccard=1, got %f", j)
	}

	// No overlap
	edges2 = NewGrayImage(10, 10)
	for x := 5; x < 10; x++ {
		edges2.Gray.Pix[5*edges2.Stride+x] = 255
	}
	j = CalculateJaccardIndex(edges1, edges2)
	if j != 0.0 {
		t.Errorf("Non-overlapping edges should have Jaccard=0, got %f", j)
	}
}

// TestSaveTestImages saves test images to testdata directory for visual inspection.
// Run with: go test -run TestSaveTestImages -v
func TestSaveTestImages(t *testing.T) {
	if os.Getenv("SAVE_TEST_IMAGES") != "1" {
		t.Skip("Set SAVE_TEST_IMAGES=1 to generate test images")
	}

	testdataDir := "../testdata"
	os.MkdirAll(testdataDir, 0755)

	// Gradient
	gradient := CreateGradientImage(256, 256)
	SaveImage(gradient.RGBA, filepath.Join(testdataDir, "gradient.png"))

	// Vertical gradient
	vgradient := CreateVerticalGradientImage(256, 256)
	SaveImage(vgradient.RGBA, filepath.Join(testdataDir, "vgradient.png"))

	// Checkerboard
	checker := CreateCheckerboardImage(256, 256, 32)
	SaveImage(checker.RGBA, filepath.Join(testdataDir, "checkerboard.png"))

	// Color bars
	bars := CreateColorBarsImage(256, 256)
	SaveImage(bars.RGBA, filepath.Join(testdataDir, "colorbars.png"))

	// Edge test
	edges := CreateEdgeImage(256, 256)
	SaveImage(edges.RGBA, filepath.Join(testdataDir, "edges.png"))

	t.Log("Test images saved to testdata/")
}
