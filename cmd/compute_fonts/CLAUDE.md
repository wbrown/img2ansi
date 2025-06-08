# CLAUDE.md - compute_fonts Research Lab

This directory is an **active research project** exploring font-agnostic image rendering techniques. The author has NOT given up on this vision.

## Core Research Question

Can we create a universal image-to-text rendering algorithm that works with ANY font or character set, not just Unicode block characters?

## Current Status

This is an experimental testbed where we're exploring:

1. **8×8 Glyph Matching**: Moving from 2×2 pixel blocks to 8×8 character-sized blocks
2. **Font Analysis**: Automatically detecting and classifying available characters in any font
3. **Advanced Pattern Matching**: Using visual similarity, stroke analysis, and density matching
4. **Universal Compatibility**: Making img2ansi work on terminals without Unicode support

## Key Files

### Core Implementation
- `fonts.go`: Core glyph matching system with bitmap analysis
- `brown_style.go`: Adaptation of Brown Dithering to 8×8 blocks
- Various `brown_*.go` files: Different experimental approaches to color and pattern selection

### Documentation Files
- `GLYPH_MATCHING_EXPERIMENTS.md`: **Primary research document** - detailed experiments, findings, and future directions
- `analyze_results.md`: Analysis of font enumeration and character properties
- `final_comparison.md`: Comparison of different rendering approaches
- `summary.md`: High-level summary of findings
- `DRY_REFACTORING.md`: Documentation of code cleanup efforts
- `RENDER_DUPLICATION_ANALYSIS.md`: Analysis of rendering code duplication
- `RENDER_CLEANUP_PLAN.md`: Plan for consolidating render implementations
- `CLEANUP_SUMMARY.md`: Summary of cleanup changes

## Important Context

### What We've Learned So Far

1. **2×2 blocks outperform 8×8 blocks** in current tests - but this doesn't mean we should give up
2. **Removing space/full characters** improves results by forcing actual pattern usage
3. **Simple color extraction** works better than sophisticated methods
4. **Font rendering is complex** - bit ordering, metrics, and compatibility issues abound
5. **256 colors dramatically improve quality** - more than any algorithm sophistication
6. **True exhaustive search validates heuristics** - DominantColorSelector is near-optimal for 256 colors (but NOT for 16 colors where 8×8 blocks perform much worse than 2×2)

### Why This Research Matters

While the main img2ansi project uses Unicode blocks successfully, many environments don't support them:
- Legacy terminals (VT100, etc.)
- ASCII-only systems
- Alternative character sets (PETSCII, ATASCII)
- Custom bitmap fonts

This research aims to make img2ansi truly universal.

## Development Notes

### Recent Cleanup (January 2025)

During comprehensive code cleanup:
- **Consolidated 13 brown_*.go files** into single `brown_experiments.go` with configurable strategies
- **Organized outputs** into subdirectories (outputs/ans/, outputs/png/, outputs/txt/)
- **Converted debug tools to tests** (bitmap_analysis_test.go, comparison_test.go, etc.)
- **Removed duplicate render implementations** (~400 lines of duplicate code)
- **Created unified utilities** (color_utils.go, image_utils.go, font_enumeration.go)
- **Reduced from 45+ to 39 .go files** through consolidation

Note: Some refactoring choices (like render_blocks_proper.go) were created by Claude and may not represent the author's intended direction.

### Latest Experiments (January 2025)

#### 16 vs 256 Color Palette Testing
- All 12 Brown experiments now run with both 16 and 256 color palettes
- 256 colors provide dramatically better quality (mandrill clearly recognizable)
- Performance varies by algorithm:
  - DominantColorSelector: ~350ms for both palettes
  - ExhaustiveColorSelector: Required intelligent sampling for 256 colors
  - TrueExhaustiveColorSelector: ~5 minutes for 256 colors (589,824 tests/block)

#### Key Discovery: Heuristics Are Near-Optimal (256 Colors Only)
The true exhaustive search (testing ALL 589,824 combinations per block) produces only marginally better results than the simple DominantColorSelector heuristic **when using 256 colors**. This validates that:
1. For 256 colors: The constraint is the medium (9 chars, 2 colors) not the algorithm
2. Simple heuristics that extract dominant colors work remarkably well with large palettes
3. Exhaustive search shows subtle improvements but at 1000× the computation cost

**Important**: This does NOT apply to 16 colors! With 16 colors, the 8×8 block approach produces much worse results than the main project's 2×2 Brown Dithering, regardless of algorithm used.

#### Intelligent Sampling for Large Palettes
When ExhaustiveColorSelector hit timeouts with 256 colors, we implemented intelligent sampling:
- Always include dominant colors from the block
- Sample evenly across the palette spectrum
- Test combinations with dominant colors
- This fixed the "only testing dark colors" problem

### BlockRune Architecture Integration (January 2025)

A major refactoring aligned compute_fonts with the main package's architecture:

#### The Problem
- compute_fonts was directly generating ANSI escape sequences
- Duplicated ANSI generation logic from the main package
- Mixed concerns: algorithms were both computing AND formatting output

#### The Solution
- Refactored all algorithms to produce `[][]BlockRune` arrays (same as main package)
- Removed all direct ANSI generation code (`writeANSIChar`, escape sequences)
- Now uses main package's `RenderToAnsi()` for ANSI output
- Kept the sophisticated ANSI→PNG pipeline for font-based visualization

#### Benefits
1. **Clean separation of concerns**: Algorithms produce data, not formatted output
2. **No duplication**: ANSI generation happens in one place
3. **Easy integration**: Experiments already output the right format for main package
4. **Maintained visualization**: Font-based PNG rendering still works perfectly

#### Architecture Flow
```
Algorithm → [][]BlockRune → RenderToAnsi() → ANSI file → RenderANSIToPNG() → PNG
                          ↑                              ↑
                    (main package)              (font-based rendering)
```

This refactoring makes compute_fonts a true research lab that integrates cleanly with the production code.

### The Real Challenge

Accurate font glyph rendering to PNG is tricky because:
1. Font metrics and positioning are complex
2. Bit ordering bugs plagued early implementations
3. Different fonts have different character coverage
4. Performance implications of font rendering

### Files Created During Cleanup

These files were created by Claude during DRY refactoring and may not represent the author's vision:
- `block_renderer.go`
- `color_utils.go` 
- `ansi_parser.go`
- `render_blocks_proper.go`

## Practical Recommendations

Based on our experiments, here are concrete recommendations:

### For Using This Code
1. **Run experiments with both 16 and 256 colors**: `./compute_fonts brown [experiment] 256`
2. **Use "original" (DominantColorSelector) for production**: Best quality/speed trade-off
3. **Use "true-exhaustive-full" to understand limits**: Shows theoretical maximum quality
4. **Debugging tools are separate from experiments**: See usage output for distinction

### For Algorithm Selection
- **16-color terminals**: All algorithms perform similarly poorly with 8×8 blocks (use main project's 2×2 instead)
- **256-color terminals**: DominantColorSelector achieves near-optimal results, avoid exhaustive without sampling
- **Speed critical**: Use original or optimized strategies
- **Quality critical**: True exhaustive shows what's possible (but takes 5+ minutes and only marginally improves 256-color output)

### For Future Development
1. **Focus on spatial resolution over algorithm complexity**: 2×2 > 8×8 regardless of algorithm
2. **Implement intelligent sampling for large color spaces**: Don't test sequentially
3. **Consider perceptual color metrics**: Current RGB distance has room for improvement
4. **Explore adaptive algorithms**: Use exhaustive only where it matters (edges, faces)

## Future Directions

The author envisions:
1. **Stroke-based matching**: Analyzing character topology/skeleton rather than pixels
2. **Adaptive algorithms**: Different strategies for different font types
3. **Semantic understanding**: Distinguishing between geometric patterns and meaningful symbols
4. **Performance optimization**: Making 8×8 matching feasible in real-time
5. **Hybrid approaches**: Combine 2×2 blocks where available with glyph matching fallback

## Important: This is Active Research

**Do not assume this approach has been abandoned.** The current findings show 2×2 blocks work better, but this doesn't mean 8×8 blocks can't be made to work with the right approach. The solution may require fundamentally different thinking about character matching and visual similarity.

Key insight from experiments: "Spatial resolution matters more than color depth or pattern variety" - but perhaps we haven't found the right way to leverage pattern variety yet.

**Latest validation**: True exhaustive search confirms that current heuristics are already excellent **for 256-color palettes**. For 16 colors, 8×8 blocks perform poorly compared to the main project's 2×2 approach. The challenge isn't finding better algorithms, but rather finding better representations (characters, block sizes, hybrid approaches).