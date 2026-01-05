# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

img2ansi is a Go-based tool that converts images into ANSI art using a novel "Brown Dithering Algorithm". The project uses 2x2 pixel blocks and sophisticated Unicode character selection to create high-quality terminal art with support for multiple color palettes.

## Build Commands

```bash
# Build the main ansify command-line tool
go build ./cmd/ansify

# Build compute_tables utility (for precomputing color tables)
go build ./cmd/compute_tables

# Build compute_fonts utility (for font analysis)
go build ./cmd/compute_fonts

# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...
```

## Dependencies

- Go 1.24 or later
- `golang.org/x/image` for image processing
- For font tools: `github.com/golang/freetype`
- **Optional**: OpenCV 4 (`gocv.io/x/gocv`) - only needed for comparison tests

### Note on OpenCV

As of December 2024, the core image processing has been migrated to pure Go implementations in the `imageutil/` package. OpenCV is no longer required for normal builds. The gocv dependency is only used for comparison tests that validate the pure Go implementations against OpenCV:

```bash
# Normal build (no OpenCV required)
go build ./cmd/ansify

# Run comparison tests (requires OpenCV)
go test -tags gocv_compare ./imageutil/...
```

## The Brown Dithering Algorithm

The Brown Dithering Algorithm is a novel block-based dithering approach that converts images to ANSI art by finding the optimal representation of each 2x2 pixel block using:
- **2 colors** (foreground and background from the ANSI palette)
- **1 pattern** (one of 16 Unicode block characters)

### How It Works

For each 2x2 pixel block, the algorithm:
1. Tests all 16 Unicode block patterns (space, ▘, ▝, ▀, ▖, ▌, ▞, ▛, ▗, ▚, ▐, ▜, ▄, ▙, ▟, █)
2. For each pattern, searches for the best foreground/background color pair
3. Calculates the total error (sum of color distances for all 4 pixels)
4. Selects the pattern+colors combination with minimum error

This is essentially a constrained optimization problem: given 4 arbitrary pixel colors, find the best approximation using only 2 colors and a specific pattern.

### Example
For a 2x2 block `[red, blue, green, yellow]`, it might determine that pattern `'▛'` (three quarters) with foreground=dark_red and background=yellow gives the minimum total error.

### Perceptual Optimizations

The algorithm is specifically tuned for human perception:

1. **Edge Detection Integration**:
   - Uses Canny edge detection (thresholds 50-150) on 4x resolution intermediate image
   - Edge blocks get 50% reduced error weight (preserves sharpness)
   - Edge pixels diffuse only 50% of their error (prevents edge bleeding)
   - Results in crisp boundaries and preserved details

2. **Aspect Ratio Compensation**:
   - Default `ScaleFactor = 2.0` compensates for terminal characters being ~2x taller than wide
   - Adjustable via `-scale` flag for different terminals
   - Ensures circles remain circular, squares remain square

3. **Perceptual Color Metrics**:
   - **Redmean**: Fast approximation of human color perception
   - **LAB**: Perceptually uniform color space
   - **RGB**: Simple Euclidean (fastest but least accurate)

4. **16-Color Sweet Spot**:
   - Algorithm performs best with 16-color ANSI palette
   - Forced simplification creates stronger, more graphic shapes
   - Better pattern visibility and coherent palette
   - 256-color mode often produces "muddier" results despite higher color accuracy

## Architecture Overview

### Renderer API (v1.0+)

The library uses a `Renderer` struct that encapsulates all state for thread-safe, reusable rendering:

```go
// Create once, reuse across renders (preserves cache)
r := img2ansi.NewRenderer(
    img2ansi.WithPalette("ansi256"),
    img2ansi.WithColorMethod(img2ansi.RedmeanMethod{}),
    img2ansi.WithKdSearch(50),
)

// Render with Renderer methods
blocks := r.BrownDitherForBlocks(resized, edges)
ansi := r.RenderToAnsi(blocks)
compressed := r.CompressANSI(ansi)
```

Benefits over the old global API:
- **Thread-safe**: Multiple renderers can run concurrently
- **Cache persistence**: Block cache survives across different render sizes
- **Clean configuration**: Functional options pattern
- **Testable**: No global state to reset between tests

See `MIGRATION.md` for migrating from the old global API.

### Core Algorithm Components

1. **Image Processing** (`imageutil/` package):
   - Pure Go image processing (no OpenCV required)
   - Image loading/saving (`io.go`)
   - Resizing with various interpolation methods (`resize.go`)
   - Grayscale conversion (`convert.go`)
   - 2D convolution and sharpening (`convolve.go`)
   - Canny edge detection (`canny.go`)
   - Preprocessing pipeline (`prepare.go`):
     - `PrepareForANSI()` - all-in-one with 4x edge detection quality
     - `ResizeForANSI()` - resize only (for custom mid-pipeline processing)
     - `DetectEdges()` - edge detection only

2. **Renderer and Block Processing** (`renderer.go`, `img2ansi.go`):
   - `Renderer` struct holds palette, cache, and configuration
   - Processes images in 2x2 pixel blocks
   - Implements the Brown Dithering Algorithm with `FindBestBlockRepresentation`
   - Integrates edge detection for detail preservation
   - Block caching keyed by palette-mapped colors (size-independent)

3. **Color Management** (`palette.go`, `rgb.go`):
   - `ColorDistanceMethod` interface for extensible distance metrics
   - Built-in methods: `RGBMethod{}`, `LABMethod{}`, `RedmeanMethod{}`
   - Manages embedded palettes (ANSI 16/256, JetBrains 32)
   - KD-tree search optimization for color matching

4. **Character Selection**:
   - Current: Uses 16 Unicode block characters for 2x2 pixel blocks
   - In Development: Glyph matching system (see below)

5. **Output Generation** (`ansi.go`, `image.go`):
   - Generates compressed ANSI escape sequences
   - Supports PNG output for debugging
   - Handles terminal width constraints

### Performance Optimizations

- **KD-tree search** (`kdtree.go`): Efficient nearest-neighbor color matching
- **Block caching** (`approximatecache.go`): Caches computation results
- **Embedded palettes**: Precomputed binary palette data for faster loading
- **Pure Go image processing**: No CGO overhead from OpenCV bindings

## Important Implementation Details

### Edge Detection Integration
- Uses pure Go Canny edge detection (`imageutil/canny.go`, thresholds 50-150)
- Algorithm: Gaussian blur → Sobel gradients → Non-maximum suppression → Hysteresis
- Edge blocks get 50% reduced error diffusion to preserve sharpness
- Cache lookups use 30% reduced threshold for edge blocks
- Generates `edges.png` for debugging

### Advanced Caching System
The `ApproximateCache` uses:
- **Custom Uint256 type** (4 × uint64) for 256-bit cache keys
- **Keys represent 8 palette colors**: 4 foreground + 4 background mappings of a 2x2 block
- **Multiple patterns per key**: Different Unicode patterns can map to same palette colors
- **Error-based selection**: Evaluates all cached patterns, selects lowest error below threshold
- **Adaptive thresholds** based on edge detection (70% threshold for edge blocks)
- **Performance tracking** (hit/miss rates)

Note: This is not fuzzy key matching but exact key lookup with multiple approximate values - a clever way to cache similar visual results.

### Color Distance Methods

The `ColorDistanceMethod` interface allows extensible distance metrics:

```go
type ColorDistanceMethod interface {
    Distance(c1, c2 RGB) float64
    Name() string
}
```

Built-in implementations (available via `-colormethod` flag):
- **`RGBMethod{}`**: Simple Euclidean distance (fastest)
- **`LABMethod{}`**: CIE L*a*b* perceptually uniform space (best quality)
- **`RedmeanMethod{}`**: Fast perceptual approximation (good compromise, default)

**Custom implementations**: Can be passed to `WithColorMethod()` for specialized use cases.
- Built-in methods use precomputed `.palette` files for instant table loading
- Custom methods use **fast loading** with runtime KD-tree lookups:
  - Instant palette loading (no 30-40 second delay)
  - Uses KD-tree search at runtime instead of precomputed tables
  - Slightly slower per-pixel lookups, but block caching mitigates this
- For maximum performance with custom methods, generate a `.palette` file with `compute_tables`

### Pre-computation Tools

**`compute_tables`** generates lookup tables for O(1) color matching:
- **16.7 million entries**: Maps every possible RGB color (256³) to nearest palette color
- **All distance methods**: Separate tables for RGB, LAB, and Redmean (keyed by method name)
- **Index optimization**: Stores 1-byte palette indices instead of 3-byte RGB values
  - Reduces memory from ~50MB to ~16MB per table (3x reduction)
  - Better CPU cache performance
  - Two-step lookup: RGB → palette index → actual color
- **Compression**: Gzip + gob encoding reduces file size
- **Embedded in binary**: .palette files included via Go embed (~3MB total for all palettes)

**Regenerating palette files** (required after changing `ColorDistanceMethod` or palette format):
```bash
cd cmd/compute_tables
go build
./compute_tables ../../colordata/ansi16.json
./compute_tables ../../colordata/ansi256.json
./compute_tables ../../colordata/jetbrains32.json
```

This preprocessing converts expensive per-pixel operations into simple array lookups:
- Without: Calculate distance to 16-256 colors per pixel
- With: Single array access per pixel
- Result: Massive performance improvement for real-time conversion

The brute force approach (computing all 16.7M possibilities) combined with clever storage (index optimization) exemplifies the project's philosophy: maximum performance through preprocessing.

### How Block Search Uses Precomputed Tables

The block search algorithm has two stages that work together:

**Stage 1: Per-Pixel Color Mapping**
For each of the 4 pixels in a 2×2 block, find the closest palette color:
- With precomputed tables: O(1) array lookup
- Without: KD-tree nearest neighbor search

This produces 4 "anchor" colors - the optimal palette color for each pixel individually.

**Stage 2: Block Search**
The challenge: we can only use 2 colors (foreground + background) for all 4 pixels, but we have 4 potentially different anchors. The search finds the best 2-color approximation:

- **Small palettes (≤32 colors)**: Brute force all combinations
  - 16 patterns × 32 fg × 32 bg = 16,384 iterations (fast)

- **Large palettes (>32 colors)**: KD-tree candidate search
  - Uses anchors from Stage 1 as starting points
  - Finds N colors nearest to each anchor (default N=50)
  - Tests combinations of these candidates
  - 16 patterns × 50 fg × 50 bg = 40,000 iterations (vs 1M+ for brute force)

**Key insight**: The KD-tree candidate search *leverages* precomputed tables. The anchors from Stage 1 guide where to search in Stage 2. We're not searching the entire color space - we're searching near the known per-pixel optima.

This two-stage approach gives us:
- O(1) per-pixel mapping (precomputed)
- O(depth²) block search instead of O(colors²)
- Cache reuse across similar blocks

### Color Table Serialization

The serialization system uses multiple compression layers:

1. **Index compression**: Already covered above (3 bytes → 1 byte)

2. **Palette deduplication**: If foreground and background use identical colors, only one table is stored

3. **Custom KD-tree serialization**:
   - Each node uses only 5 bytes: `[null flag][RGB][split axis]` + children
   - Much more compact than generic serialization

4. **Two-stage compression**: Gob encoding + gzip compression

5. **Smart bundling**:
   - All three color methods (RGB, LAB, Redmean) in one file
   - Embedded directly in binary via Go's embed directive
   - No external file dependencies

The sophisticated serialization reduces the ~300MB of raw color tables to ~96MB embedded in the binary, while maintaining fast load times.

### Image Processing Pipeline

The pipeline can be used in two ways:

**Standard Pipeline** (`PrepareForANSI` - highest quality):
1. Resize to 4x target size using area interpolation
2. Convert to grayscale and run Canny edge detection at 4x resolution
3. Resize image and edges to 2x target size
4. Apply mild sharpening (3x3 convolution kernel)

**Split Pipeline** (for custom mid-processing like overlaying rivers):
```go
resized := imageutil.ResizeForANSI(img, width, height)  // Resize + sharpen
overlayRivers(resized, ...)                              // Custom modification
edges := imageutil.DetectEdges(resized)                  // Edge detection after
```

**Then for both pipelines**:
5. Apply Brown Dithering with modified Floyd-Steinberg error diffusion (`r.BrownDitherForBlocks`)
6. Generate compressed ANSI sequences (`r.RenderToAnsi`, `r.CompressANSI`)

### ANSI Output Compression
- **Run-length encoding**: Combines adjacent blocks with identical colors
- **Smart color optimization**: Full blocks only need background color, spaces only need foreground
- **Line-end resets**: Each line ends with `\x1b[0m\n` to reset terminal state
- **Significant size reduction**: Especially effective for images with uniform areas

### Auto-Sizing with MaxChars
- **Automatic dimension reduction**: If output exceeds `-maxchars` limit, progressively reduces dimensions
- **Maintains aspect ratio**: Width reduced by 2, height adjusted proportionally
- **Guarantees fit**: Ensures output always fits within terminal or system limits

### Additional Features

1. **Debug Output** (when output is .png):
   - `resized.png`: The resized input image
   - `dithered.png`: After dithering (scaled 2x for easier viewing)
   - `edges.png`: Edge detection visualization

2. **Performance Metrics**:
   - Detailed timing breakdown (initialization, computation, cache performance)
   - Cache hit/miss rates for optimization tuning
   - Available via verbose output

3. **Dynamic Quantization** (`-quantization`):
   - Pre-reduces color space before processing
   - Trades quality for speed
   - Default: 256 (no reduction)

4. **Embedded Binary Palettes**:
   - Common palettes (ansi16, ansi256, jetbrains32) embedded as binary data
   - Instant loading without JSON parsing overhead

## Common Development Tasks

```bash
# Test image conversion with default settings
./cmd/ansify/ansify -input test.png -output test.ans

# High-quality conversion (slower)
./cmd/ansify/ansify -input test.png -output test.ans -kdsearch 0 -cache_threshold 0

# Generate PNG output for debugging
./cmd/ansify/ansify -input test.png -output test.png -width 80

# Use different color palette
./cmd/ansify/ansify -input test.png -output test.ans -palette ansi256
```

## Key Files to Understand

- `renderer.go`: **Renderer API** - encapsulates all state, palette loading, configuration
- `img2ansi.go`: Brown Dithering Algorithm and block processing methods
- `imageutil/`: Pure Go image processing package (replaces gocv/OpenCV)
  - `prepare.go`: Preprocessing pipeline (`PrepareForANSI`, `ResizeForANSI`, `DetectEdges`)
  - `canny.go`: Canny edge detection implementation
  - `resize.go`: Image resizing with various interpolation methods
  - `convolve.go`: 2D convolution and sharpening
- `palette.go`: Color palette types, serialization, and table computation
- `rgb.go`: RGB/LAB color types and `ColorDistanceMethod` interface
- `approximatecache.go`: Block caching system for performance
- `cmd/ansify/ansify.go`: CLI interface and parameter handling
- `cmd/compute_tables/`: Regenerates embedded `.palette` binary files
- `MIGRATION.md`: Guide for migrating from global API to Renderer API

## Active Research: Font-Agnostic Rendering

The `cmd/compute_fonts/` directory contains active research into font-agnostic image rendering. If you're working on or interested in this research, you should read:

- `cmd/compute_fonts/CLAUDE.md`: Overview of the research lab and latest findings
- `cmd/compute_fonts/EXPERIMENT_GUIDE.md`: Guide to running experiments
- `cmd/compute_fonts/GLYPH_MATCHING_EXPERIMENTS.md`: Detailed experimental results and analysis

Key findings so far:
- 2×2 blocks still outperform 8×8 character matching
- 256 colors dramatically improve quality over 16 colors
- Simple heuristics (DominantColorSelector) are already near-optimal
- The constraint is the medium (limited characters) not the algorithms

## Glyph Matching Research Status

The `cmd/compute_fonts/` directory contains ongoing research into glyph matching that could potentially upgrade from 2x2 blocks to 8x8 character matching. While initial results show 2x2 blocks still outperform 8x8, the research continues:

### How the Glyph System Works

1. **Analyzes font glyphs** as 8x8 bitmaps (64-bit integers)
2. **Extracts features** from each glyph:
   - Pixel weight (filled pixel count)
   - Zone weights (4x4 zones)
   - Edge maps and diagonal line detection
3. **Matches image blocks to characters** using:
   - 70% shape similarity
   - 20% pattern similarity
   - 10% density similarity
4. **Optimizations**:
   - Pre-built lookup tables by zone weights
   - Special handling for diagonal characters (/, \)
   - Font safety checks (characters must exist in fallback font)

This will provide:
- **4x higher resolution** (8x8 vs 2x2 pixels per character)
- **Thousands of characters** instead of just 16 block elements
- **Better visual quality** through intelligent character selection

Example: Instead of using '▀' for a horizontal line, it could use '─' or '═' for cleaner appearance.

### Critical Bug Fix (Fixed in commit after initial development)

The glyph matching system had a **critical bit ordering bug** that made it non-functional:
- `analyzeGlyph()` used reversed bit ordering: `(GlyphHeight-1-y)*GlyphWidth + (GlyphWidth-1-x)`
- `getBit()` used standard ordering: `y*GlyphWidth + x`
- `String()` used yet another scheme: `63-y*GlyphWidth-x`

**Fix**: All three methods now use consistent row-major ordering: `y*GlyphWidth + x`

### Important Notes on Glyph Matching

1. **Font Quirks**: The IBM BIOS font has limitations:
   - '|' character has a gap in the middle (row 3 is empty)
   - '+' is offset to the left, not centered
   - Many characters don't use the full 8x8 grid
   - This means "obvious" matches may not work as expected

2. **Testing**: The system includes comprehensive bitmap tests in `bitmap_consistency_test.go` to ensure bit ordering remains consistent

3. **Not Integrated**: The glyph matching system exists but is not connected to the main img2ansi converter - it's a separate tool for analysis

4. **Computational Complexity**: Going from 2x2 to 8x8 blocks would increase computation by ~40x:
   - 16 patterns → 700+ characters
   - 4 pixels → 64 pixels per block  
   - Current optimizations (zones, caching) don't scale well to this complexity

## Critical Implementation Notes

### Floyd-Steinberg Error Diffusion Bug (Fixed)

**History**: The original implementation had a bug in `RGB.subtract()` that clamped negative values to 0:
```go
// WRONG - breaks error diffusion!
func (r RGB) subtract(other RGB) RGB {
    return RGB{
        R: uint8(math.Max(0, float64(r.R)-float64(other.R))),
        G: uint8(math.Max(0, float64(r.G)-float64(other.G))),
        B: uint8(math.Max(0, float64(r.B)-float64(other.B))),
    }
}
```

**Problem**: Floyd-Steinberg dithering requires both positive and negative errors to propagate correctly. Clamping to 0 prevented proper error diffusion.

**Solution**: Created `RGBError` type with signed integers:
```go
type RGBError struct {
    R, G, B int16
}

func (rgb RGB) subtractToError(other RGB) RGBError {
    return RGBError{
        R: int16(rgb.R) - int16(other.R),
        G: int16(rgb.G) - int16(other.G),
        B: int16(rgb.B) - int16(other.B),
    }
}
```

### Unicode Block Pattern Optimization

**Critical Performance Detail**: The quadrant checking in the hot path uses a clever bitwise trick:
```go
// PERFORMANCE: This uses a bitwise operation on the rune value
// instead of looking up quadrants. The Unicode block characters
// are specifically chosen so their codepoints encode which
// quadrants are filled. This is a critical hot path optimization.
if (bestRune & (1 << (3 - i))) != 0 {
    targetColor = fgColor
} else {
    targetColor = bgColor
}
```

This works because the Unicode block characters were carefully selected so their codepoint values encode the quadrant pattern. Do NOT replace with a lookup function - this is in the innermost loop and performance-critical.

### Terminal Color Palette Variations

**Important**: The ANSI color codes (30-37, 90-97, etc.) are interpreted differently by different terminals. The algorithm outputs standard ANSI codes, but the actual RGB values displayed depend on the terminal's color scheme. This can lead to significantly different visual results between terminals.

For example:
- Standard ANSI defines code 33 as "brown" (#AA5500)
- Some terminals may render this as orange or dark yellow
- This is not a bug in the algorithm - it's terminal-specific behavior

### ANSI Color Mapping Architecture

img2ansi uses a sophisticated color mapping system that's important to understand:

#### 1. Palette Structure
- **ANSI 16**: Basic colors (0-7) and bright colors (8-15)
- **ANSI 256**: Includes three ranges:
  - 0-15: Basic/bright colors (may differ from standalone 16-color palette!)
  - 16-231: 6×6×6 color cube (216 colors)
  - 232-255: 24-step grayscale ramp

#### 2. Color Selection Process
The Brown Dithering Algorithm selects colors through multiple stages:

1. **Image RGB → Palette RGB**: For each pixel, find the nearest color in the palette
   - Uses pre-computed lookup tables for O(1) performance
   - Three distance metrics: RGB (Euclidean), LAB (perceptual), Redmean (fast perceptual)
   - Tables map all 16.7M RGB values to palette indices

2. **Block Optimization**: For each 2×2 block, find optimal (foreground, background, pattern)
   - Tests all 16 Unicode block patterns
   - Evaluates multiple color pairs per pattern
   - Minimizes total error across all 4 pixels

3. **RGB → ANSI Code**: Convert selected RGB colors back to ANSI codes
   - Uses reverse lookup in palette data
   - Maintains separate maps for foreground/background codes

#### 3. Critical Implementation Details

**Palette Loading**: The same color can have different RGB values in different palettes:
```go
// ANSI 16 palette might define color 8 as:
ansi16[8] = RGB{85, 85, 85}   // Bright black

// ANSI 256 palette might define color 8 as:
ansi256[8] = RGB{128, 128, 128} // Different shade!
```

**ANSI Code Generation**: img2ansi generates optimal ANSI escape sequences:
- Full blocks (█) only need background color
- Spaces only need foreground color (which becomes invisible)
- Adjacent blocks with same colors share escape codes
- Uses `fgAnsi` and `bgAnsi` maps for RGB → ANSI code lookup

**Color Distance Calculation**: The choice of distance metric significantly affects output:
- **RGB**: Fast but poor for human perception (red/green seem closer than they are)
- **LAB**: Best perceptual accuracy but slower (requires color space conversion)
- **Redmean**: Good compromise - weights red channel based on overall redness

#### 4. Common Pitfalls

1. **Assuming palette consistency**: Never assume colors 0-15 are identical across palettes
2. **Direct RGB comparison**: The nearest RGB color may not be the best perceptual match
3. **Escape sequence parsing**: Must handle 256-color sequences (38;5;n) before basic codes
4. **Cache key generation**: Block appearance depends on all 4 pixel colors, not just dominant

#### 5. Performance Optimizations

- **Pre-computed tables**: 16.7M entry lookup tables eliminate per-pixel calculations
- **Embedded palettes**: Binary palette data compiled into executable
- **Compressed ANSI output**: Run-length encoding for repeated colors
- **Block caching**: Reuses calculations for visually similar blocks