package imageutil

// PrepareForANSI prepares an image for conversion to ANSI art.
// This is the pure Go equivalent of the gocv-based prepareForANSI function.
//
// The function:
// 1. Resizes to 4x target size (for higher quality intermediate processing)
// 2. Detects edges using Canny edge detection (thresholds 50-150)
// 3. Resizes both image and edges to 2x target size (for 2x2 block processing)
// 4. Applies mild sharpening
//
// Parameters:
//   - img: The input image
//   - width: Target width in blocks (final pixel width will be width*2)
//   - height: Target height in blocks (final pixel height will be height*2)
//
// Returns:
//   - resized: The processed image at (width*2 x height*2)
//   - edges: Binary edge map at (width*2 x height*2)
func PrepareForANSI(img *RGBAImage, width, height int) (resized *RGBAImage, edges *GrayImage) {
	// Step 1: Resize to 4x target size using area interpolation
	intermediateWidth := width * 4
	intermediateHeight := height * 4
	intermediate := Resize(img, intermediateWidth, intermediateHeight, InterpolationArea)

	// Step 2: Edge detection on intermediate image
	gray := ToGrayscale(intermediate)
	edgesFull := CannyDefault(gray) // Uses thresholds 50, 150

	// Step 3: Resize both intermediate and edges to 2x target size
	resizedWidth := width * 2
	resizedHeight := height * 2
	resized = Resize(intermediate, resizedWidth, resizedHeight, InterpolationArea)
	edges = ResizeGray(edgesFull, resizedWidth, resizedHeight, InterpolationLinear)

	// Step 4: Apply mild sharpening
	resized = Sharpen(resized)

	return resized, edges
}
