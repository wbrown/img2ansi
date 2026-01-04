# img2ansi v1.0 Migration Guide

This guide covers migrating from the global function API to the new `Renderer`-based API.

## Why Migrate?

The old API used global state, which caused problems:
- Not thread-safe (concurrent renders could interfere)
- No way to have multiple configurations
- Cache couldn't be preserved across different render sizes
- Hard to test in isolation

The new `Renderer` API:
- Encapsulates all state in a struct
- Thread-safe concurrent rendering
- Cache persists across renders with different dimensions
- Clean functional options pattern

## Quick Migration

### Before (Global API)

```go
import (
    "github.com/wbrown/img2ansi"
    "github.com/wbrown/img2ansi/imageutil"
)

// Set global configuration
img2ansi.KdSearch = 50
img2ansi.CurrentColorDistanceMethod = img2ansi.MethodRedmean
img2ansi.ScaleFactor = 2.0

// Load palette into global state
_, _, err := img2ansi.LoadPalette("ansi256")
if err != nil {
    return err
}

// Render using global functions
resized, edges := imageutil.PrepareForANSI(img, width, height)
blocks := img2ansi.BrownDitherForBlocks(resized, edges)
ansi := img2ansi.RenderToAnsi(blocks)
compressed := img2ansi.CompressANSI(ansi)
```

### After (Renderer API)

```go
import (
    "github.com/wbrown/img2ansi"
    "github.com/wbrown/img2ansi/imageutil"
)

// Create Renderer once (preserves cache across renders)
r := img2ansi.NewRenderer(
    img2ansi.WithPalette("ansi256"),
    img2ansi.WithColorMethod(img2ansi.RedmeanMethod{}),
    img2ansi.WithKdSearch(50),
)

// Render using Renderer methods
resized, edges := imageutil.PrepareForANSI(img, width, height)
blocks := r.BrownDitherForBlocks(resized, edges)
ansi := r.RenderToAnsi(blocks)
compressed := r.CompressANSI(ansi)
```

## Renderer Options

```go
r := img2ansi.NewRenderer(
    img2ansi.WithPalette("ansi256"),           // Palette: "ansi16", "ansi256", "jetbrains32", or path
    img2ansi.WithColorMethod(img2ansi.LABMethod{}),  // LABMethod{}, RedmeanMethod{}, RGBMethod{}
    img2ansi.WithKdSearch(50),                 // KD-tree search depth (0 = use precomputed tables)
    img2ansi.WithCacheThreshold(40.0),         // Error threshold for cache hits
)
```

## Custom Mid-Pipeline Processing

If you need to modify the image after resizing but before edge detection (e.g., overlaying rivers, annotations), use the split pipeline functions:

### Before (Wasteful)

```go
// Edge detection happens too early, must redo it
resized, _ := imageutil.PrepareForANSI(img, width, height)
overlayRivers(resized, h, data)
gray := imageutil.ToGrayscale(resized)
edges := imageutil.CannyDefault(gray)  // Redundant edge detection
```

### After (Split Pipeline)

```go
// Resize without edge detection
resized := imageutil.ResizeForANSI(img, width, height)

// Custom modifications
overlayRivers(resized, h, data)

// Edge detection after modifications
edges := imageutil.DetectEdges(resized)

// Continue with dithering
blocks := r.BrownDitherForBlocks(resized, edges)
```

## Pipeline Functions

| Function | Purpose | When to Use |
|----------|---------|-------------|
| `PrepareForANSI(img, w, h)` | Resize + edge detect (4x quality) | Standard rendering, no mid-pipeline changes |
| `ResizeForANSI(img, w, h)` | Resize + sharpen only | When you need to modify image before edge detection |
| `DetectEdges(img)` | Canny edge detection | After custom modifications to resized image |

## Resizable TUI Example

For TUI applications with resizable viewports, create the Renderer once and reuse it:

```go
type MapView struct {
    renderer *img2ansi.Renderer
    image    *imageutil.RGBAImage  // Cached source image
}

func NewMapView() *MapView {
    return &MapView{
        renderer: img2ansi.NewRenderer(
            img2ansi.WithPalette("ansi256"),
            img2ansi.WithColorMethod(img2ansi.RedmeanMethod{}),
        ),
    }
}

func (v *MapView) Render(width, height int) string {
    // Cache is preserved across different sizes!
    resized := imageutil.ResizeForANSI(v.image, width, height)
    edges := imageutil.DetectEdges(resized)
    blocks := v.renderer.BrownDitherForBlocks(resized, edges)
    ansi := v.renderer.RenderToAnsi(blocks)
    return v.renderer.CompressANSI(ansi)
}
```

## Cache Behavior

The block cache is keyed by **palette-mapped colors**, not image dimensions. This means:

- Rendering the same image at different sizes benefits from cached color decisions
- Similar color patterns across different images will hit the cache
- Cache is only invalidated when you change the palette or color method

```go
// All these renders share the same cache:
r.BrownDitherForBlocks(resize(img, 80, 40), edges1)   // Misses, populates cache
r.BrownDitherForBlocks(resize(img, 120, 60), edges2)  // Partial hits from similar colors
r.BrownDitherForBlocks(resize(img, 80, 40), edges3)   // More hits
```

## API Reference

### Renderer Methods

```go
// Core rendering
func (r *Renderer) BrownDitherForBlocks(img *imageutil.RGBAImage, edges *imageutil.GrayImage) [][]BlockRune
func (r *Renderer) RenderToAnsi(blocks [][]BlockRune) string
func (r *Renderer) CompressANSI(ansi string) string

// Statistics
func (r *Renderer) CacheStats() (hits, misses int, hitRate float64)
func (r *Renderer) ResetStats()
func (r *Renderer) GetBestBlockTime() time.Duration

// Palette management
func (r *Renderer) LoadPalette(path string) error
```

### Color Distance Methods

```go
img2ansi.RGBMethod{}     // Fast, simple Euclidean distance
img2ansi.RedmeanMethod{} // Fast perceptual approximation (recommended)
img2ansi.LABMethod{}     // Accurate perceptual distance (slower)
```

**Custom methods**: Implement the `ColorDistanceMethod` interface:

```go
type MyPerceptualMethod struct{}

func (m MyPerceptualMethod) Distance(c1, c2 img2ansi.RGB) float64 {
    // Your custom distance calculation
}

func (m MyPerceptualMethod) Name() string {
    return "MyPerceptual"
}

// Use it - loads instantly, no precomputation needed!
r := img2ansi.NewRenderer(
    img2ansi.WithColorMethod(MyPerceptualMethod{}),
    img2ansi.WithPalette("ansi256"),
)
```

Custom methods use **fast loading** - the Renderer builds only the KD-tree structure
(instant) and uses runtime KD-tree lookups instead of precomputed tables. This means:
- No 30-40 second initialization delay
- Slightly slower per-pixel lookups (KD-tree search vs. table lookup)
- Caching still works normally, so similar blocks are fast after the first lookup

## Removed Global Variables

The following globals have been removed. Use `Renderer` options instead:

| Old Global | New Equivalent |
|------------|----------------|
| `img2ansi.KdSearch` | `WithKdSearch(n)` |
| `img2ansi.CurrentColorDistanceMethod` | `WithColorMethod(m)` |
| `img2ansi.ScaleFactor` | `WithScaleFactor(f)` |
| `img2ansi.CacheThreshold` | `WithCacheThreshold(t)` |
| `img2ansi.TargetWidth` | Pass to render functions |
| `img2ansi.MaxChars` | `WithMaxChars(n)` |
| `img2ansi.LoadPalette()` | `WithPalette()` or `r.LoadPalette()` |
| `img2ansi.LookupHits/Misses` | `r.CacheStats()` |
