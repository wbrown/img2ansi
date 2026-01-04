package imageutil

// PrepareForANSI prepares an image for conversion to ANSI art.
// This is the highest-quality preparation pipeline that detects edges
// at 4x resolution before downscaling.
//
// The function:
// 1. Resizes to 4x target size (for higher quality intermediate processing)
// 2. Detects edges using Canny edge detection (thresholds 50-150)
// 3. Resizes both image and edges to 2x target size (for 2x2 block processing)
// 4. Applies mild sharpening
//
// For custom mid-pipeline processing (e.g., overlaying rivers), use
// ResizeForANSI followed by DetectEdges instead.
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

// ResizeForANSI resizes an image for ANSI art conversion without edge detection.
// Use this when you need to modify the image after resizing (e.g., overlay rivers)
// and then call DetectEdges separately.
//
// The function:
// 1. Resizes to 2x target size (for 2x2 block processing)
// 2. Applies mild sharpening
//
// Parameters:
//   - img: The input image
//   - width: Target width in blocks (final pixel width will be width*2)
//   - height: Target height in blocks (final pixel height will be height*2)
//
// Returns:
//   - resized: The processed image at (width*2 x height*2)
func ResizeForANSI(img *RGBAImage, width, height int) *RGBAImage {
	resizedWidth := width * 2
	resizedHeight := height * 2
	resized := Resize(img, resizedWidth, resizedHeight, InterpolationArea)
	return Sharpen(resized)
}

// DetectEdges performs Canny edge detection on an image.
// Use this after ResizeForANSI and any custom image modifications
// to generate the edge map needed for dithering.
//
// Parameters:
//   - img: The input image (typically output of ResizeForANSI)
//
// Returns:
//   - edges: Binary edge map at the same size as input
func DetectEdges(img *RGBAImage) *GrayImage {
	gray := ToGrayscale(img)
	return CannyDefault(gray)
}
