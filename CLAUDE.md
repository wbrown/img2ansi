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

- **OpenCV 4** is required (uses `gocv.io/x/gocv` Go bindings)
- Go 1.22.5 or later
- For font tools: `github.com/golang/freetype`

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

### Core Algorithm Components

1. **Block Processing Pipeline** (`img2ansi.go`):
   - Processes images in 2x2 pixel blocks
   - Implements the Brown Dithering Algorithm with `findBestBlockRepresentation`
   - Integrates edge detection for detail preservation
   - Uses block caching for performance

2. **Color Management** (`palette.go`, `rgb.go`):
   - Supports multiple color spaces (RGB, LAB, Redmean)
   - Manages embedded palettes (ANSI 16/256, JetBrains 32)
   - KD-tree search optimization for color matching

3. **Character Selection**:
   - Current: Uses 16 Unicode block characters for 2x2 pixel blocks
   - In Development: Glyph matching system (see below)

4. **Output Generation** (`ansi.go`, `image.go`):
   - Generates compressed ANSI escape sequences
   - Supports PNG output for debugging
   - Handles terminal width constraints

### Performance Optimizations

- **KD-tree search** (`kdtree.go`): Efficient nearest-neighbor color matching
- **Block caching** (`approximatecache.go`): Caches computation results
- **Embedded palettes**: Precomputed binary palette data for faster loading

## Important Implementation Details

### Edge Detection Integration
- Uses OpenCV's Canny edge detection (thresholds 50-150)
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
Three methods available via `-colormethod` flag:
- **RGB**: Simple Euclidean distance (fastest)
- **LAB**: CIE L*a*b* perceptually uniform space (best quality)
- **Redmean**: Fast perceptual approximation (good compromise)

### Pre-computation Tools

**`compute_tables`** generates lookup tables for O(1) color matching:
- **16.7 million entries**: Maps every possible RGB color (256³) to nearest palette color
- **All distance methods**: Separate tables for RGB, LAB, and Redmean
- **Index optimization**: Stores 1-byte palette indices instead of 3-byte RGB values
  - Reduces memory from ~50MB to ~16MB per table (3x reduction)
  - Better CPU cache performance
  - Two-step lookup: RGB → palette index → actual color
- **Compression**: Gzip + gob encoding reduces file size
- **Embedded in binary**: .palette files included via Go embed (~96MB total for all tables)

This preprocessing converts expensive per-pixel operations into simple array lookups:
- Without: Calculate distance to 16-256 colors per pixel
- With: Single array access per pixel
- Result: Massive performance improvement for real-time conversion

The brute force approach (computing all 16.7M possibilities) combined with clever storage (index optimization) exemplifies the project's philosophy: maximum performance through preprocessing.

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
1. Resize to 4x target size (better quality than direct resize)
2. Apply mild sharpening
3. Run Canny edge detection
4. Apply Brown Dithering with modified Floyd-Steinberg error diffusion
5. Generate compressed ANSI sequences

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

- `img2ansi.go`: Main algorithm implementation and block processing
- `palette.go`: Color palette management and serialization
- `cmd/ansify/ansify.go`: CLI interface and parameter handling
- `cmd/compute_fonts/fonts.go`: Glyph matching system (in development)

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