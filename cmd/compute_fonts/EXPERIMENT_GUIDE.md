# Experiment Guide for compute_fonts

This guide documents how to create and run experiments in the compute_fonts research environment.

## Overview

The compute_fonts directory is a research lab for exploring font-agnostic image rendering. Experiments test different approaches to converting images into text using various character sets, color strategies, and rendering techniques.

### Key Discovery

Through extensive testing including true exhaustive search (testing ALL possible combinations), we've validated that simple heuristics like DominantColorSelector are already near-optimal **when using 256 colors**. The constraint is the medium itself (limited characters and colors), not the algorithm sophistication.

**Critical caveat**: This finding does NOT apply to 16-color palettes! With 16 colors, the 8×8 block approach produces much worse results than the main project's 2×2 Brown Dithering algorithm, regardless of which color selection strategy is used. The 8×8 approach only becomes competitive with 256 colors.

This finding helps focus future work on the right problems: spatial resolution, character sets, and perceptual optimizations rather than complex search algorithms.

## Experiment Structure

### 1. Using the Consolidated Brown System

The preferred way to create experiments is through the consolidated Brown dithering system:

```go
// Define a new configuration
var MyExperimentConfig = BrownDitheringConfig{
    ColorStrategy:      ColorStrategyKMeans,
    ColorSearchDepth:   16,
    CharacterSet:       CharSetPatternsOnly,
    EnableDiffusion:    true,
    DiffusionStrength:  0.75,
    PaletteSize:        16,
    OutputFile:         OutputPath("my_experiment.ans"),
    OutputPNG:          OutputPath("my_experiment.png"),
    ShowProgress:       true,
    DebugMode:          true,
}

// Run it
err := BrownDithering(img, lookup, MyExperimentConfig)
```

### 2. Adding New Color Selection Strategies

To add a new color selection strategy:

1. Add to the enum in `brown_experiments.go`:
```go
const (
    // ... existing strategies ...
    ColorStrategyMyNew ColorSelectionStrategy = iota
)
```

2. Implement the ColorSelector interface in `color_selectors.go`:
```go
type MyNewColorSelector struct {
    // parameters
}

func (m *MyNewColorSelector) SelectColors(pixels []img2ansi.RGB, palette []img2ansi.RGB) []ColorPair {
    // Your implementation
    // Return candidate color pairs to test
}
```

3. Add to createColorSelector():
```go
case ColorStrategyMyNew:
    return &MyNewColorSelector{/* params */}
```

#### Existing Color Strategies

- **DominantColorSelector**: Extracts 2 most common colors (fastest, near-optimal for 256 colors)
- **ExhaustiveColorSelector**: Tests many color pairs (with intelligent sampling for 256)
- **TrueExhaustiveColorSelector**: Tests ALL color×character combinations
- **KMeansColorSelector**: Uses k-means clustering to find colors
- **OptimizedColorSelector**: K-means + nearest palette colors
- **FrequencyColorSelector**: Most frequent palette colors in block
- **ContrastColorSelector**: Maximizes contrast between fg/bg
- **QuantizedColorSelector**: Pre-quantizes pixels before matching

### 3. Adding New Character Sets

To experiment with different character sets:

```go
case CharSetMyCustom:
    return []rune{'╱', '╲', '╳', '┃', '━', '┏', '┓', '┗', '┛'}
```

## Experiment Patterns

### Pattern 1: Parameter Sweep

Test how a parameter affects quality:

```go
func TestDiffusionStrengthSweep() {
    strengths := []float64{0.0, 0.25, 0.5, 0.75, 1.0}
    
    for _, strength := range strengths {
        config := DefaultBrownConfig()
        config.EnableDiffusion = true
        config.DiffusionStrength = strength
        config.OutputFile = OutputPath(fmt.Sprintf("diffusion_%.2f.ans", strength))
        
        BrownDithering(img, lookup, config)
    }
}
```

### Pattern 2: Strategy Comparison

Compare different strategies on the same image:

```go
func CompareColorStrategies() {
    strategies := []struct {
        name     string
        strategy ColorSelectionStrategy
    }{
        {"dominant", ColorStrategyDominant},
        {"kmeans", ColorStrategyKMeans},
        {"contrast", ColorStrategyContrast},
    }
    
    for _, s := range strategies {
        config := DefaultBrownConfig()
        config.ColorStrategy = s.strategy
        config.OutputFile = OutputPath(fmt.Sprintf("%s.ans", s.name))
        
        start := time.Now()
        BrownDithering(img, lookup, config)
        fmt.Printf("%s: %v\n", s.name, time.Since(start))
    }
}
```

### Pattern 3: Quality Metrics

Track quality metrics across experiments:

```go
type ExperimentResult struct {
    Config      BrownDitheringConfig
    Runtime     time.Duration
    CharUsage   map[rune]int
    AvgError    float64
    CacheHitRate float64
}

func RunExperimentWithMetrics(config BrownDitheringConfig) ExperimentResult {
    start := time.Now()
    
    // Run experiment
    err := BrownDithering(img, lookup, config)
    
    // Collect metrics
    result := ExperimentResult{
        Config:  config,
        Runtime: time.Since(start),
        // ... collect other metrics
    }
    
    return result
}
```

### Pattern 4: A/B Testing

Test specific hypotheses:

```go
func TestContrastImportance() {
    // Hypothesis: High contrast thresholds improve readability
    
    baseConfig := DefaultBrownConfig()
    baseConfig.ColorStrategy = ColorStrategyContrast
    
    // Test A: Low contrast threshold
    configA := baseConfig
    configA.ContrastThreshold = 10.0
    configA.OutputFile = OutputPath("contrast_low.ans")
    
    // Test B: High contrast threshold  
    configB := baseConfig
    configB.ContrastThreshold = 50.0
    configB.OutputFile = OutputPath("contrast_high.ans")
    
    BrownDithering(img, lookup, configA)
    BrownDithering(img, lookup, configB)
    
    // Compare results...
}
```

## Running Experiments

### Command Line

```bash
# Run named experiment with default 16 colors
./compute_fonts brown original

# Run with 256 color palette
./compute_fonts brown original 256

# Run all experiments (both 16 and 256 colors)
./compute_fonts all

# Run custom experiment (add to RunBrownExperiment)
./compute_fonts brown my-experiment

# Render all ANSI files to PNG
./compute_fonts render
```

### Available Experiments

1. **original** - DominantColorSelector (fastest, near-optimal for 256 colors)
2. **exhaustive** - Tests many color pairs (uses intelligent sampling for 256)
3. **true-exhaustive-full** - Tests ALL combinations (5+ minutes for 256 colors!)
4. **diffusion** - With Floyd-Steinberg error diffusion
5. **optimized** - K-means with caching
6. **quantized** - Pre-quantizes color space
7. **smart** - Enhanced diffusion with edge detection
8. **kmeans** - K-means clustering for colors
9. **compare** - Frequency-based selection
10. **contrast** - Maximizes foreground/background contrast
11. **no-space** - Excludes space character
12. **patterns** - Excludes both space and full block

### Batch Processing

The default "all" command runs both 16 and 256 color versions:

```go
func RunAllExperiments() {
    experiments := []string{
        "original", "exhaustive", "diffusion",
        "optimized", "patterns", // ... etc
    }
    
    paletteSizes := []int{16, 256}
    
    for _, paletteSize := range paletteSizes {
        for _, exp := range experiments {
            fmt.Printf("\n=== Running %s (%d colors) ===\n", exp, paletteSize)
            RunBrownExperimentWithPalette(exp, paletteSize)
        }
    }
}
```

### Understanding Performance Numbers

When comparing experiments:
- **16 colors**: Fast regardless of algorithm (small search space)
- **256 colors**: Performance varies dramatically by strategy
  - Dominant/Optimized: ~350-500ms (heuristics scale well)
  - Exhaustive with sampling: ~800ms-5s (depends on MaxPairs)
  - True Exhaustive: 5+ minutes (tests all 589,824 combinations/block)

## Analyzing Results

### Visual Comparison

1. Generate HTML comparison:
```go
func GenerateComparisonHTML(experiments []string) {
    // Create HTML with side-by-side images
}
```

2. Use outputs/ directory structure:
```
outputs/
├── ans/     # ANSI output files
├── png/     # Rendered PNGs for visual comparison
└── txt/     # Metrics and logs
```

### Quantitative Analysis

Track metrics in a CSV:

```go
func LogExperimentMetrics(result ExperimentResult) {
    file, _ := os.OpenFile(OutputPath("metrics.csv"), os.O_APPEND|os.O_CREATE, 0644)
    defer file.Close()
    
    fmt.Fprintf(file, "%s,%s,%v,%f\n", 
        result.Config.ColorStrategy,
        result.Runtime,
        result.AvgError,
    )
}
```

## Best Practices

1. **Always use OutputPath()** - Keeps outputs organized
2. **Document hypotheses** - What are you testing and why?
3. **Control variables** - Change one parameter at a time
4. **Use consistent test images** - mandrill_original.png is the standard
5. **Track metrics** - Runtime, quality, character usage
6. **Visual inspection** - Always generate PNGs for comparison
7. **Reproducibility** - Document exact configurations

## Common Experiments

### 1. Character Set Exploration
- Test ASCII-only vs Unicode blocks
- Custom symbol sets for specific aesthetics
- Density gradients with different characters
- Patterns-only (no space/full block) for forced detail

### 2. Color Strategy Optimization
- **Palette size impact (16 vs 256)** - Most dramatic quality improvement
- Color extraction methods (dominant, k-means, contrast, etc.)
- Perceptual vs mathematical color distance
- True exhaustive search to validate heuristics

### 3. Error Diffusion Tuning
- Different diffusion patterns
- Variable strength by image region
- Interaction with color strategies
- Edge-aware diffusion weights

### 4. Performance Optimization
- Caching effectiveness
- Parallel processing opportunities
- Early termination strategies
- Intelligent sampling for large search spaces

## Adding Results to Documentation

When you discover something significant:

1. Update GLYPH_MATCHING_EXPERIMENTS.md with findings
2. Add configuration to named configs in brown_experiments.go
3. Include comparison images in documentation
4. Document in git commit message

## Example: Complete Experiment

```go
// Hypothesis: Combining k-means color selection with pattern-only 
// characters produces better contrast than the original approach

func TestKMeansPatternsHypothesis() {
    // Control: Original approach
    control := BrownConfigOriginal
    control.OutputFile = OutputPath("control.ans")
    
    // Test: K-means + patterns
    test := BrownDitheringConfig{
        ColorStrategy:    ColorStrategyKMeans,
        ColorSearchDepth: 8,
        CharacterSet:     CharSetPatternsOnly,
        PaletteSize:      16,
        OutputFile:       OutputPath("kmeans_patterns.ans"),
        OutputPNG:        OutputPath("kmeans_patterns.png"),
    }
    
    // Run both
    fmt.Println("Running control...")
    err1 := BrownDithering(img, lookup, control)
    
    fmt.Println("Running test...")  
    err2 := BrownDithering(img, lookup, test)
    
    // Generate comparison
    GenerateComparison("control.png", "kmeans_patterns.png", 
                      "kmeans_patterns_comparison.html")
    
    // Log results
    fmt.Println("Results saved. View comparison.html to evaluate hypothesis.")
}
```

## Palette Size Testing

### 16 vs 256 Color Experiments

One of the most impactful discoveries: palette size matters more than algorithm sophistication.

```bash
# Run experiment with 16 colors (default)
./compute_fonts brown original

# Run same experiment with 256 colors
./compute_fonts brown original 256

# Compare the outputs
open outputs/png/mandrill_original.png outputs/png/mandrill_original_256.png
```

### Key Findings

1. **Dramatic Quality Improvement**
   - 256 colors make the mandrill clearly recognizable
   - 16 colors force creative pattern use but limit detail
   - Color depth > algorithm complexity for quality

2. **Performance Implications**
   - DominantColorSelector: ~350ms for both palettes
   - ExhaustiveColorSelector: Requires intelligent sampling for 256
   - TrueExhaustive: 5+ minutes for 256 colors (589,824 tests/block)

3. **Heuristics Are Near-Optimal (256 Colors Only)**
   - Simple dominant color extraction performs excellently with 256 colors
   - True exhaustive search shows only subtle improvements over heuristics
   - For 16 colors: 8×8 blocks perform poorly vs main project's 2×2 approach
   - The constraint is the medium (8×8 blocks, 9 chars) not the algorithm

### Intelligent Sampling for Large Palettes

When using ExhaustiveColorSelector with 256 colors:

```go
// Problem: Testing first 100 color pairs sequentially only tests dark colors
// Solution: Intelligent sampling across the palette

func (e *ExhaustiveColorSelector) SelectColors(pixels []img2ansi.RGB, palette []img2ansi.RGB) []ColorPair {
    if e.MaxPairs > 0 && len(palette) > 16 {
        // Always include dominant colors
        colors := extractBlockColorsRGB(pixels)
        fg := findNearestPaletteColorRGB(colors[0], palette)
        bg := findNearestPaletteColorRGB(colors[1], palette)
        pairs = append(pairs, ColorPair{fg, bg}, ColorPair{bg, fg})
        
        // Sample evenly across palette
        step := len(palette) / (e.MaxPairs / 4)
        for i := 0; i < len(palette); i += step {
            // Test combinations with dominant colors
            pairs = append(pairs, ColorPair{palette[i], bg})
            pairs = append(pairs, ColorPair{fg, palette[i]})
        }
    }
    // ... rest of implementation
}
```

## Future Experiment Ideas

1. **Adaptive Strategies**: Different strategies for different image regions
   - Use exhaustive for faces/important areas
   - Heuristics for uniform regions
   
2. **Multi-pass Rendering**: Refine output through multiple passes
   - Initial pass with heuristics
   - Refinement pass on high-error blocks
   
3. **Perceptual Optimization**: Use perceptual color spaces
   - LAB distance instead of RGB
   - May close gap between heuristic and exhaustive
   
4. **Animation Support**: Temporal coherence for video conversion
   - Consistent palette across frames
   - Motion-aware block selection
   
5. **Style Transfer**: Match the aesthetic of specific terminals/eras
   - Simulate CRT phosphor limitations
   - Terminal-specific color profiles

Remember: The goal is to understand the fundamental limits and possibilities of character-based image representation. Every experiment should teach us something about this problem space.