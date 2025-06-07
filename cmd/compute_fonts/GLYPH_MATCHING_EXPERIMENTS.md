# Glyph Matching Experiments

This document describes experimental approaches to improving glyph matching for the img2ansi project.

## Vision: Font-Agnostic Image Rendering

The goal is to create a system that can render images using ANY character set:
- Modern Unicode fonts with full block elements
- IBM PC BIOS with limited blocks
- DEC VT100 terminals with special graphics
- PETSCII with unique Commodore characters
- ASCII-only environments
- Custom/artistic fonts

## Background

The current glyph matching system (as of commit 2743f3d) uses a multi-factor similarity scoring approach with:
- Shape similarity based on pixel features
- Pattern analysis for texture detection
- Density matching
- Character simplicity metrics
- Codepoint commonality preferences
- Semantic bonuses for likely matches

While this approach works, it has become complex with many special cases and still struggles with fundamental matches (e.g., matching 'O' to circular patterns).

## Stroke-Based Typography Approach

### Motivation

Current issues with pixel-based matching:
1. **Over-complexity**: 10+ different metrics and special cases
2. **Font sensitivity**: Minor pixel differences cause major matching changes
3. **Non-intuitive results**: 'N' matching better than 'O' for circles
4. **Computational overhead**: Many calculations per character comparison

### Core Insight

Humans recognize characters by their structural components, not pixel-by-pixel comparison. Typography and calligraphy are fundamentally about **strokes** and their relationships.

### Typographic Fundamentals

#### 1. Basic Strokes
- **Stems**: Vertical strokes (|, l, I, 1)
- **Bars**: Horizontal strokes (-, =, _, —)
- **Diagonals**: Angled strokes (/, \, X, N)
- **Curves**: Arcs and bowls (O, C, S, U)

#### 2. Stroke Relationships
- **Junctions**: Where strokes meet
  - T-junction: Stem meets bar (T, ⊤, ┬)
  - Cross: Two strokes cross (+, x, X)
  - Corner: Two strokes meet at end (L, ┐, └)
- **Terminals**: How strokes end
  - Blunt (most characters)
  - Tapered (italics, some fonts)
  - Serif (serif fonts)
- **Spacing**: Parallel strokes (H, =, ≡)

#### 3. Topology
- **Connected components**: How many separate pieces
  - Single: Most letters (O, L, T)
  - Multiple: Punctuation (:, ;, i, j)
- **Loops/Enclosures**: Closed paths
  - Full enclosure: O, D, 0
  - Partial: C, U, G
- **Branching**: Where strokes split (Y, K)

### Proposed Algorithm

```go
// Core data structures
type StrokeType int
const (
    Vertical StrokeType = iota
    Horizontal
    DiagonalUp    // /
    DiagonalDown  // \
    CurvedCW      // Clockwise curve
    CurvedCCW     // Counter-clockwise curve
)

type Stroke struct {
    Type      StrokeType
    Start     Point
    End       Point
    Curvature float64  // 0 = straight, 1 = quarter circle
    Thickness int      // Approximate pixel width
}

type Junction struct {
    Type     JunctionType  // T, X, L, etc.
    Position Point
    Strokes  []int        // Indices of strokes meeting here
}

type CharacterSkeleton struct {
    Strokes    []Stroke
    Junctions  []Junction
    Components int         // Connected components
    Loops      int         // Closed paths
}
```

### Implementation Steps

1. **Stroke Extraction**
   - Thin the bitmap to skeleton (1-pixel wide)
   - Trace continuous paths
   - Classify each path segment

2. **Structure Analysis**
   - Find junctions (pixels with 3+ neighbors)
   - Identify loops (closed paths)
   - Count components (separate pieces)

3. **Character Matching**
   ```go
   func matchCharacter(input, candidate CharacterSkeleton) float64 {
       // Compare stroke counts by type
       strokeSimilarity := compareStrokeComposition(input, candidate)
       
       // Compare how strokes connect
       topologySimilarity := compareTopology(input, candidate)
       
       // Compare spatial arrangement
       layoutSimilarity := compareLayout(input, candidate)
       
       return 0.5*strokeSimilarity + 0.3*topologySimilarity + 0.2*layoutSimilarity
   }
   ```

### Expected Benefits

1. **Natural Grouping**: Characters with same structure match well
   - 'O', '0', 'o' all = one curved closed stroke
   - 'I', '|', 'l', '1' all = one vertical stroke
   - 'T', '⊤', '┬' all = vertical + horizontal at top

2. **Font Invariance**: Structure remains even if pixels shift
   - 'O' is still a closed curve whether it's 26 or 32 pixels
   - '+' is still two crossing strokes regardless of thickness

3. **Simplicity**: Fewer, more meaningful features
   - Instead of 20+ metrics, just strokes and topology
   - Easier to debug and understand

4. **Performance**: Potentially faster
   - Extract skeleton once per character
   - Compare structures, not pixels

### Handling Non-Calligraphic Patterns

Not all useful characters are stroke-based. Many are texture or fill patterns:

#### Pattern Types

1. **Fill Patterns**
   - Solid blocks: █ ■ ◼
   - Partial fills: ▀ ▄ ▌ ▐ ▞ ▚
   - Shading: ░ ▒ ▓ (25%, 50%, 75% density)

2. **Dot/Grid Patterns**
   - Regular grids: ⁚ ⁖ ⁘ ⁙
   - Checkerboards: ▚ ▞
   - Braille patterns: ⠿ ⡇ ⢸

3. **Geometric Shapes**
   - Filled shapes: ● ◆ ▲ ▼
   - Outlines: ○ ◇ △ ▽

#### Hybrid Approach

The solution is to classify characters first:

```go
type CharacterClass int
const (
    StrokeBased CharacterClass = iota  // Letters, numbers, punctuation
    FillBased                          // Blocks, shading
    PatternBased                       // Dots, grids, textures
)

func classifyCharacter(bitmap GlyphBitmap) CharacterClass {
    skeleton := extractSkeleton(bitmap)
    density := float64(popcount(bitmap)) / 64.0
    
    // High density with no clear skeleton = fill
    if density > 0.6 && len(skeleton.Strokes) == 0 {
        return FillBased
    }
    
    // Regular repeating pattern = texture
    if hasRegularPattern(bitmap) {
        return PatternBased
    }
    
    return StrokeBased
}

func matchCharacter(input, candidate GlyphBitmap) float64 {
    inputClass := classifyCharacter(input)
    candidateClass := classifyCharacter(candidate)
    
    // Different classes = lower similarity
    if inputClass != candidateClass {
        return 0.5 * crossClassMatch(input, candidate)
    }
    
    switch inputClass {
    case StrokeBased:
        return strokeBasedMatch(input, candidate)
    case FillBased:
        return fillBasedMatch(input, candidate)
    case PatternBased:
        return patternBasedMatch(input, candidate)
    }
}
```

#### Matching Strategies by Class

1. **Stroke-Based**: Use proposed skeleton algorithm
2. **Fill-Based**: Compare coverage, position, and shape
3. **Pattern-Based**: Use frequency analysis, regularity metrics

This ensures we use the right tool for each type of character.

### Challenges

1. **Skeleton Extraction**: Converting bitmap to clean skeleton
2. **Curve Detection**: Distinguishing curves from jagged diagonals
3. **Small Characters**: 8x8 is limited resolution for complex shapes
4. **Font Variations**: Some fonts have unusual stroke arrangements
5. **Class Boundaries**: Some characters blend categories (e.g., @ has strokes AND fill)

### Alternative Approaches to Consider

1. **Contour-based**: Match outline shapes instead of skeletons
2. **Feature Points**: Key points like corners, endpoints, junctions
3. **Hybrid**: Use strokes for initial matching, pixels for refinement

## Critical Font Constraints

### IBM BIOS Font Limitations

The IBM BIOS font we're using does NOT include the 2x2 quadrant block characters that img2ansi currently relies on:

**Missing 2x2 Blocks** (U+2596-U+259F):
- ▖ ▗ ▘ ▙ ▚ ▛ ▜ ▝ ▞ ▟ (NOT in IBM BIOS font)

**Available Block Elements**:
- ▀ (U+2580) - Upper half block
- ▄ (U+2584) - Lower half block  
- ▌ (U+258C) - Left half block
- ▐ (U+2590) - Right half block
- █ (U+2588) - Full block

**Available Shading**:
- ░ (U+2591) - Light shade (25%)
- ▒ (U+2592) - Medium shade (50%)
- ▓ (U+2593) - Dark shade (75%)

### Implications

1. **Current Algorithm Incompatible**: The Brown Dithering Algorithm's 16 patterns cannot be represented in IBM BIOS font
2. **Alternative Approaches Needed**: Must work with available blocks or use character matching
3. **8x8 Glyph Matching More Important**: Since we can't use 2x2 blocks, full character matching becomes essential

### Useful Pattern Characters for Matching

#### Fill-Based (Available in IBM BIOS):
```
█ - Full block (100% fill)
▓ - Dark shade (75% fill)
▒ - Medium shade (50% fill)
░ - Light shade (25% fill)
▀ - Upper half
▄ - Lower half
▌ - Left half
▐ - Right half
■ - Black square
□ - White square (outline)
```

#### ASCII Patterns:
```
# - Hash (grid pattern)
% - Percent (diagonal pattern)
& - Ampersand (complex pattern)
* - Asterisk (star/cross pattern)
@ - At sign (spiral pattern)
. - Period (single dot)
: - Colon (two dots)
= - Equals (horizontal lines)
```

These constraints make the glyph matching approach even more valuable, as we need to find the best available character rather than constructing patterns from 2x2 blocks.

## Font-Agnostic Architecture

### Font Capability Detection

```go
type FontCapabilities struct {
    Name           string
    CharacterSet   []rune              // Available characters
    HasBlocks      BlockSupport        // None, Half, Quarter, Full
    HasShading     bool                // ░▒▓ or equivalent
    HasBoxDrawing  bool                // ─│┌┐└┘├┤┬┴┼
    HasDiagonals   bool                // ╱╲╳
    SpecialChars   map[string][]rune   // "shading" -> [░,▒,▓]
}

type BlockSupport int
const (
    NoBlocks BlockSupport = iota
    HalfBlocks      // ▀▄▌▐
    QuarterBlocks   // ▖▗▘▙▚▛▜▝▞▟
    CustomBlocks    // PETSCII-style
)
```

### Adaptive Rendering Strategy

```go
func selectRenderingStrategy(font FontCapabilities) RenderStrategy {
    if font.HasBlocks == QuarterBlocks {
        return BrownDitheringStrategy{} // Current 2x2 approach
    } else if font.HasBlocks == HalfBlocks {
        return HalfBlockStrategy{}      // 2x1 or 1x2 patterns
    } else {
        return GlyphMatchingStrategy{}  // Full character matching
    }
}
```

### Font Profiles

#### Unicode (Modern)
- Full 2x2 blocks available
- Extensive box drawing
- Rich symbol set
- Current algorithm works perfectly

#### IBM PC BIOS (CP437)
- Half blocks only (▀▄▌▐)
- Shading characters (░▒▓)
- Box drawing subset
- Need glyph matching or half-block mode

#### DEC VT100
- Special graphics mode
- Limited to 32 special characters
- Focus on line drawing
- Need custom mapping

#### PETSCII
- Completely different character set
- Unique block patterns
- No Unicode mapping
- Need custom renderer

#### ASCII-Only
- Just 95 printable characters
- Must use density/pattern matching
- Characters like .,:;*#@
- Ultimate fallback mode

### Universal Character Selection

Instead of hardcoding Unicode blocks, the system should:

1. **Analyze available characters** in the target font
2. **Classify them** by visual properties
3. **Build optimal mapping** for that specific font
4. **Select best matches** from what's available

```go
func buildFontMapping(font FontCapabilities) CharacterMapper {
    mapper := CharacterMapper{}
    
    // Classify all available characters
    for _, char := range font.CharacterSet {
        bitmap := renderCharacter(font, char)
        class := classifyCharacter(bitmap)
        properties := extractProperties(bitmap)
        
        mapper.Add(char, class, properties)
    }
    
    // Build optimal selections for common patterns
    mapper.BuildPatternMap()
    
    return mapper
}
```

This approach means:
- **PETSCII**: Uses its unique graphics characters
- **ASCII**: Falls back to density patterns (., :, *, #, @)
- **VT100**: Uses special graphics characters
- **Unicode**: Takes full advantage of all blocks

## Next Steps

1. Create font capability detection system
2. Build font profile database
3. Implement adaptive rendering strategies
4. Test with multiple character sets (IBM BIOS, PETSCII, ASCII)
5. Design font-specific optimizations
6. Create "font packs" for popular terminals

## Experimental Results (2025-01-07)

### 8x8 Glyph Matching vs Brown Dithering

We implemented a basic 8x8 glyph matching system and compared it to Brown Dithering with 2x2 blocks. Key findings:

#### Setup
- **Test image**: Standard Mandrill/Baboon test image
- **Target output**: 80×40 characters (matching Brown Dithering examples)
- **Color support**: Basic two-color extraction per 8×8 block (256-color ANSI palette)
- **Character set**: 252 characters including symbols, Greek letters, and available block elements

#### Results

1. **IBM BIOS Font Limitations Confirmed**
   - Only 6 of 16 Unicode 2x2 block characters available:
     - Found: Space, ▀, ▌, ▐, ▄, █
     - Missing: ▘, ▝, ▖, ▗, ▞, ▚, ▛, ▜, ▙, ▟
   - This makes Brown Dithering incompatible with IBM BIOS font

2. **Character Usage Patterns**
   - @ symbol dominated (49% of output) - matches many medium-density patterns
   - $ (15%), _ (8%), ▓ (8%), ~ (5%) were next most common
   - Block characters (▀▌▐▄█) were used but not dominant
   - Semantic characters (letters, symbols) create visual distraction

3. **Quality Comparison**
   - Brown Dithering is dramatically superior despite using only 16 colors vs our 256
   - 8x8 blocks are too coarse (4× less spatial resolution than 2×2)
   - Semantic interference from letters/symbols degrades perception
   - Simple two-color extraction is inferior to joint pattern+color optimization
   - **Unfair advantage**: We used 256 colors while Brown Dithering used only 16!

4. **Perceptual Issues at 8x8 Scale**
   - **Semantic distraction**: Characters like ☻ (smiley), @ (at), $ (dollar) pull attention
   - **Pattern mismatch**: 8×8 is large enough for complex patterns that trigger meaning processing
   - **Context matters**: What works at 2×2 (pure geometry) fails at 8×8 (semantic meaning emerges)

### Key Insights

1. **Resolution matters more than character variety**
   - 2×2 blocks with 16 patterns > 8×8 blocks with 252 characters
   - Finer spatial resolution trumps pattern complexity
   - This holds true even when giving 8×8 a 16× color advantage (256 vs 16 colors)

2. **Joint optimization is crucial**
   - Brown Dithering optimizes pattern AND color together
   - Our approach separated pattern finding from color selection
   - Joint optimization produces far superior results

3. **Semantic-free patterns are essential**
   - Pure geometric patterns (blocks) don't trigger semantic processing
   - Letters and symbols are inherently distracting
   - Restricting to non-semantic characters helps but isn't sufficient

4. **Font constraints shape algorithm choice**
   - IBM BIOS font can't support Brown Dithering's 2×2 approach
   - Different fonts need different strategies
   - Universal algorithm must adapt to available characters

### Implications for Glyph Matching

1. **8×8 may be too large** for pattern matching
   - Consider 4×4 blocks as compromise
   - Or use multiple 2×2 blocks to approximate larger patterns

2. **Character selection critical**
   - Prefer geometric/abstract characters
   - Avoid semantic symbols where possible
   - Consider font-specific restricted sets

3. **Need better color optimization**
   - Current two-color extraction is too simple
   - Should test multiple color pairs per pattern
   - Consider error diffusion between blocks

4. **Hybrid approaches may work better**
   - Use 2×2 blocks where available
   - Fall back to glyph matching for fonts without blocks
   - Adapt block size to font capabilities

## Latest Developments (2025-01-07 continued)

### Hamming Distance and Sparse Pattern Issues

We discovered that simple Hamming distance (bit matching) fails badly with sparse patterns:
- Sparse patterns match other sparse patterns just by being "mostly empty"
- Example: An inverted pattern from block (39,21) matched '▁' with 59% similarity despite being visually different
- The problem: Hamming distance counts matching zeros (empty space) as positively as matching ones (filled pixels)

### When Hamming Distance Works

Hamming distance is effective when:
1. **Patterns have similar density** - Comparing patterns with ~50% fill avoids the sparse matching problem
2. **Noise is random and uniform** - Works well for sensor noise, transmission errors
3. **All bits are equally important** - No semantic meaning to pixel positions
4. **Patterns are not sparse or dense** - Extreme densities (mostly empty or mostly full) cause false matches

For our use case with noisy, dithered images containing many sparse patterns, Hamming distance alone is inadequate.

### Brown-Style Joint Optimization for 8×8

We successfully implemented Brown Dithering concepts for 8×8 blocks:

#### Implementation Details

1. **Joint Pattern+Color Optimization**
   - For each 8×8 block, extract dominant colors
   - For each character in restricted set, calculate error with those colors
   - Test both normal and inverse orientations (swapping fg/bg)
   - Select character+color combination with minimum total error

2. **Restricted Character Set**
   ```
   Density gradient: ' ' (0%), '░' (25%), '▒' (50%), '▓' (75%), '█' (100%)
   Half blocks: '▀' (top), '▄' (bottom), '▌' (left), '▐' (right)
   ```

3. **Error Calculation**
   - For each pixel in 8×8 block:
     - If character bitmap has bit set → use foreground color
     - Otherwise → use background color
     - Calculate RGB distance between actual and rendered pixel
   - Sum errors across all 64 pixels

#### Results

Character usage distribution on Mandrill image (80×40 characters):
- Space (0%): 23.3%
- Light shade (25%): 4.7%
- Medium shade (50%): 2.0%
- Dark shade (75%): 4.1%
- Full block (100%): 12.1%
- Upper half: 17.9%
- Lower half: 13.3%
- Left half: 13.2%
- Right half: 9.4%

The distribution shows good usage across all density levels, indicating the joint optimization is effectively selecting appropriate patterns.

### Key Improvements Over Previous Attempts

1. **No Semantic Distraction**
   - Removed all letters, numbers, and recognizable symbols
   - Using only geometric density patterns prevents semantic processing interference

2. **True Joint Optimization**
   - Previous attempts selected character first, then found colors
   - Brown-style approach evaluates character+color together
   - Results in much better visual fidelity

3. **Inverse Pattern Support**
   - Testing both normal and inverted fg/bg doubles effective pattern vocabulary
   - Especially important with limited character sets

### Lessons Learned

1. **Simplicity Wins**
   - Complex multi-factor similarity scoring was replaced with simple pixel error
   - Direct error measurement outperforms abstract similarity metrics

2. **Context is Critical**
   - Best character depends on available colors in the block
   - Cannot separate pattern selection from color selection
   - Joint optimization is essential

3. **Density Patterns Superior to Shapes**
   - For image rendering, density gradients work better than trying to match shapes
   - Geometric patterns avoid semantic interference
   - Simple patterns compose better than complex ones

### Comparison with Original Brown Dithering

While we successfully applied Brown Dithering concepts to 8×8:
- Original 2×2 Brown Dithering still produces superior results
- 4× higher spatial resolution (2×2 vs 8×8) is the key advantage
- Even with 16× color disadvantage (16 vs 256 colors), 2×2 wins

This reinforces that **spatial resolution matters more than color depth or pattern variety** for high-quality image rendering.

### Future Directions

1. **Adaptive Block Sizes**
   - Use 2×2 where supported (Unicode fonts)
   - Fall back to 4×4 or 8×8 for limited fonts
   - Mix block sizes based on image content

2. **Error Diffusion Between Blocks**
   - Current implementation treats each block independently
   - Adding error diffusion could improve overall quality
   - Need to adapt Floyd-Steinberg for block-based approach

3. **Perceptual Color Spaces**
   - Current RGB distance is perceptually non-uniform
   - Implementing LAB or other perceptual spaces could improve color selection
   - Already supported in main img2ansi, just needs integration