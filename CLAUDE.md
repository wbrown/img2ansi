# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

The constitution below is adapted from `wbrown/janus-datalog` and carries the
same force here. Most of its examples are not hypothetical: they are documented
failures from sessions in this repository.

## The Gate

The repository's standard gate, in full:

```bash
go test ./...      # main module: img2ansi + imageutil
gofmt -l .         # must print NOTHING
go vet .
```

- `cmd/ansify`, `cmd/compute_tables`, and `cmd/compute_glyphs` are **separate
  modules**: root `go test ./...` does not touch them. When you change them,
  build them (`cd cmd/<tool> && go build .`).
- The main-package suite is slow (90–300s: palette table computation, the
  quality harness). Use generous **tool-call timeouts** (600000ms) and wait.
  **Do NOT add `-timeout` to `go test` commands, or use `-timeout 0`.** Use the
  default. Timeouts mean WAIT or TEST SMALLER SUBSETS, never COMMIT ANYWAY.
- A non-empty `gofmt -l` is a failed gate, exactly like a red test.

## Architectural Authority

**The user owns all architectural decisions. Claude implements them.**

Before making ANY of these decisions, ASK:
- Introducing new patterns (globals, parallel code paths, abstractions)
- Changing existing patterns (options → globals, new ways to reach the palette)
- Adding new cross-cutting concerns (configuration, logging, caching)
- Deviating from established conventions for any reason

**If you're unsure whether something is an "architectural decision":**
- Would it affect multiple files/packages?
- Would it change how components interact?
- Would it require other code to change to accommodate it?
- Are you thinking "I'll ask forgiveness later"?

**Then ASK first.**

**Red flags that indicate you're overstepping:**
- "This is just temporary/experimental"
- "I'll refactor this later"
- "It's faster to do it this way"
- "It's simpler/easier this way" (when deviating from a plan or established pattern)
- Making a choice between multiple valid approaches without consulting

**Bugs do not authorize design changes:**
- Discovering a bug does not authorize you to change the agreed design. Report
  it and ask.
- If something we agreed on doesn't work, STOP and ask. Do not substitute
  alternatives.
- If you're about to do something different from what was discussed, ASK FIRST.

**Case study (this repository)**: on discovering that the KD-tree nearest-color
search was broken, a session built a *parallel* exact-scan path for new code and
left the broken tree (and every table built from it) in production, filed as a
"known issue." That forked "nearest color" into two truths. The correct action
was the one eventually demanded: fix the tree, regenerate the tables, delete the
fork.

**The user's job**: Set direction, make architectural choices, review designs.
**Your job**: Implement, follow patterns, propose options (not make choices).

## When Tests Fail

**Failing tests are information, not obstacles.**

When tests fail after you make a change:
1. Understand WHY the test is failing
2. Report the failure to the user with context
3. Ask how they want to proceed

**NEVER change architecture or add code just to make tests pass.** A failing
test may mean the change has unintended consequences, the approach is wrong,
the expectations need updating, or the feature isn't ready. All of these are
decisions for the user.

A test that encodes a *measured result* is different from a test that encodes
an *invariant*. When a measurement shifts because an upstream defect was fixed,
the assertion may legitimately change — but say so explicitly, show both
numbers, and get agreement. Do not quietly loosen an assertion to get green.

## The Baseline Is Green — Never Blame Pre-Existing Conditions

**Every gate passes before any work session starts. This is an invariant.**
Any gate that fails during or after your work was caused by your work — either
a change you made, or a stricter check you chose to run. There is no third
possibility. Therefore:

- **NEVER attribute a failure to "pre-existing conditions."** The phrasing
  "pre-existing / not my code / I didn't touch that" is **forbidden** — it is
  blame-deflection. In this repository a session dismissed `gofmt -l` output as
  "pre-existing alignment drift, not my code — leaving it." Wrong twice: some
  of the drift had passed through that session's own commits, and in a gofmt'd
  language *nobody owns whitespace* — the tree has one format. The fix took one
  command. Run it.
- **NEVER run experiments to "prove" a failure is pre-existing.** No
  `git stash`, no revert-and-rerun A/B. Causation is determined by reading the
  diff and the failure. (`git stash` is banned outright — see below.)
- If you choose to run a stricter check than the standard gate, anything it
  finds is yours to fully resolve — or don't run it.

When a gate is red there is exactly one fork: **fix it, or report it and ask.**
There is no "investigate whose fault it is" step.

## When Asked to Revert

**Revert IMMEDIATELY. Do not defer.** Do not explain first, do not read files
to "understand context," do not make any other changes first. Deferred reverts
bury the recoverable state in context; if compaction happens, the original
code may be unrecoverable.

## Principles Are Upstream, Not a Final Filter

Every rule in this file is a **discipline that generates your actions**, not a
token to suppress at the last checkpoint. If you find yourself routing a rule
through a filter instead of letting it shape the action, you have already lost
it. The recurring instances:

- **"No helpers" is not "don't type `helper` in a name."** Name every piece of
  code for what it does. If the word *helper* occurs to you at all — in a name,
  a comment, prose, a commit message — you have not done the naming, and that
  code needs a second look. The word surfacing IS the violation, wherever it
  surfaces. Acceptable names describe action or role: `nearestFgColor`,
  `calibrationProbes`, `measureConverterArms`, a fixture builder, a constructor.
  (`t.Helper()` is Go's API name, not yours; calling it is fine.)

- **Never destroy state you cannot get back.** `git clean` and `git stash` are
  **banned outright, for any reason**. Do not `rm` files you did not create
  this turn; to remove something, move it aside or ask. When in doubt, move,
  don't delete.

- **Green gates commit-prep.** `git add`, updating docs to say "fixed",
  refreshing numbers — these come *after* a green gate, never in the same
  breath as the run that would tell you whether the work is correct.

- **Diagnose by reading, not by ritual — and never patch around tooling.** A
  failed Edit means your `old_string` was stale: Read the file again and Edit
  again. In this repository a session responded to Edit failures by patching
  source files with `python3` heredocs — which bypassed change tracking,
  corrupted a file when a string assertion half-matched, and made the edits
  unreviewable. Read-then-Edit, every time. Scripted rewrites of source files
  are not an escape hatch from tool friction.

- **One result at a time when actions depend on each other.** Fan out tool
  calls only for truly independent work.

## Fix the Root Cause — Extend, Don't Avoid

When a component can't handle a valid input, **extend or fix the component**.
Never:
- Return an error to refuse the work ("not supported")
- Route around it ("I'll add a separate path that avoids the broken one")
- Pass through unchanged
- Blame the caller

These are avoidance disguised as engineering judgment. Workarounds metastasize:
the KD-tree workaround (above) created two sources of truth for "nearest
color"; the gofmt dodge left the tree dirty for the next session to dodge
again. The moment you can name the root cause, the root cause is the work.

The one legitimate layering exception must be **explicit and principled**, not
an evasion: e.g. this codebase *synthesizes* missing quadrant glyphs for
preview rendering (the preview renderer is our medium and can draw anything)
while **forbidding** synthesized glyphs from expanding a search alphabet (the
target's medium is what it is — `GenuineGlyph`, never `GetGlyph`, for
derivation). Search space must equal what the medium can display. When you
make a layering argument, write the rule down and pin it with a test.

## Measure, Don't Theorize

This repository has a referee: the quality harness in
`diffusion_quality_test.go` (blurred-LAB ΔE, cross-converter arms, comparison
renders). Performance and quality claims come from it, not from reasoning.

**Rules**:
- Never claim a quality or performance effect without harness numbers (or a
  profile, for CPU). Never fabricate per-operation costs — measure.
- **A comparison must isolate one variable.** A dither-vs-matcher comparison
  conflated representation with error diffusion and reported a wrong
  conclusion until a diffusion-ablated control arm was added — at the user's
  insistence. When arms differ in more than the thing you're measuring, add
  the control arm before drawing any conclusion.
- **A validation that cannot fail in the way you might be wrong validates
  nothing.** Documented instances here: a rasterizer "calibration" scored
  perfect on `█`/`▀` while mis-rendering 225 of 233 glyphs (block edges sit on
  cell boundaries — the probe couldn't see half-pixel phase error); a
  bit-identical regeneration check "validated" glyph data that was identically
  wrong in both pipelines (89 runes of missing-glyph boxes); 16 passing tests
  exercised a black-to-#555555 table defect zero times. Before trusting a
  check, ask: *if I were wrong in the most likely way, would this fail?*
- When measurements contradict a theory, the theory is wrong — measure again.

## Attribution

- Commits and PRs are authored as the repository owner
  (`Wes Brown <wesbrown18@gmail.com>`), never as Claude.
- No "Generated with Claude" trailers, no model identifiers, no AI attribution
  in commit messages, PR bodies, code comments, or any committed artifact.

## Go Implementation Guidelines

Write idiomatic Go, not Java-in-Go.

### CRITICAL: No Global Configuration State
**NEVER use package-level variables for configuration.** This codebase already
paid to remove its global API (v1.0.0): configuration lives on the `Renderer`
via functional options (`WithPalette`, `WithBBSMode`, ...). New configuration
goes there. Fields the search reads (like `blocks`) are always-initialized
attributes, not nil-checked overrides behind accessors.

### CRITICAL: Stop Creating V2 Versions
**NEVER create V2 versions of functions/interfaces.** Fix the original. If you
need different behavior, add a parameter or option. Parallel implementations
are how this repo got three glyph bit orderings and two nearest-color paths.

### CRITICAL: No "Helpers"
**NEVER name files, functions, or packages `helper`, `helpers`, `utils`,
`common`, `misc`, or `shared`.** Never use "helper" in comments or prose to
describe code. Every function does something specific — name it for what it
does. A junk-drawer name signals "secondary, not important," which is exactly
where parallel implementations and untested code hide. If you can't name it,
you don't understand it well enough to write it.

**DO**: simple functions; methods on the type whose data they use; interfaces
only for actual polymorphism (`BlockConverter`, `ColorDistanceMethod`); small
focused files named for their contents; explicit errors.
**DON'T**: Manager/Service/Controller/Factory types; abstraction layers
without a second implementation; getter/setters; dependency injection
ceremony.

## Testing Strategy

**Tests are not optional.** Implementation without tests is incomplete
implementation. "It compiles" proves syntax; tests prove behavior.

**Workflow (mandatory)**: implement → write tests (happy path, edge cases,
error cases) → run them → verify PASS in the output → only then commit.

**NEVER**:
- ❌ Declare work done before tests pass — actually read the PASS lines
- ❌ Assume a failure is a "test problem" — it reveals a real bug until proven otherwise
- ❌ Use `t.Skip` to hide known bugs or unimplemented behavior
- ❌ Leave scratch `*_test.go` probes in the tree — a probe that taught you
  something becomes a real named regression test, or it gets removed before
  commit
- ❌ Commit because tests are slow — the suite's slowest tests (palette
  computation, the harness) are the ones guarding correctness
- ❌ Write assertions calibrated to broken data — when a measured baseline
  moves because you fixed its input, re-derive the assertion from the
  invariant, not from the old number

**ALWAYS**:
- ✅ Pin every bug you fix with a regression test that fails on the old code
  (`TestBlocksIndexEncodesQuadrants`, `TestNearestNeighborBlackRegression`,
  `TestBlockGlyphsCoverDitherOutput` exist for exactly this reason)
- ✅ Validate algorithms against a ground-truth oracle where one exists
  (`TestNearestNeighborMatchesLinearScan` is the pattern: exhaustive scan as
  the referee for the clever structure)
- ✅ Ask whether your tests exercise the code path you changed — passing
  tests that all route around the defect are worse than no tests, because
  they manufacture confidence
- ✅ Wait for slow tests; raise the tool-call timeout, not your risk tolerance

## Project Overview

img2ansi is a Go-based tool that converts images into ANSI art using a novel "Brown Dithering Algorithm". The project uses 2x2 pixel blocks and sophisticated Unicode character selection to create high-quality terminal art with support for multiple color palettes.

## Build Commands

```bash
# Build the main ansify command-line tool
go build ./cmd/ansify

# Build compute_tables utility (for precomputing color tables)
go build ./cmd/compute_tables

# Build compute_glyphs utility (font glyph extraction for research)
cd cmd/compute_glyphs && go build .

# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# The tree must be gofmt-clean; this must print nothing.
# Run it before committing.
gofmt -l .
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

Glyph matching research (8x8 character cells instead of 2x2 quadrant
blocks) now lives on `main`:

- `docs/glyph-research/README.md`: Lab overview — current status, the
  rasterization-calibration story, and the roadmap. **Read this first.**
- `docs/glyph-research/GLYPH_MATCHING_EXPERIMENTS.md`: The detailed
  experiment log from the original `font-analysis` research branch.
- `glyph.go` / `cp437.go`: `GlyphBitmap` infrastructure, `.glyphs` and
  ROM font loading, font-based rendering of `BlockRune` output, and
  font-derived dither alphabets (`WithBlocksFromFont` = Blocks ∩ font;
  the general form of `WithBBSMode`, whose `BBSBlocks` is exactly the
  CP437 derivation). Alphabet derivation uses `GenuineGlyph`, never
  `GetGlyph` — synthesized glyphs render previews, they do not expand a
  target's alphabet.
- `converter.go`: `BlockConverter` — the slot a glyph matcher implements
  (`Convert` + `SourcePixelsPerCell`). The `Renderer` is the reference
  implementation; the harness scores any set of converters on a common
  cell grid against the same reference (see Measuring Diffusion
  Quality), with an 8×8 mean-color baseline as the floor to beat.
- `cmd/compute_glyphs/`: Glyph extraction tool with self-calibrating
  TTF rasterization (keeps the freetype dependency out of the library).

The historical experiment code (the multi-factor similarity scorer,
color selector experiments) remains on the `font-analysis` branch; the
similarity scorer was deliberately not ported — see the lab README for
the ideal-mask/popcount formulation that should replace it.

Key findings so far:
- The 2×2-vs-8×8 question decomposes into two factors, and the harness
  carries a diffusion-ablated control arm to keep them apart.
  **Representation**: 2×2 quadrants and exhaustive 8×8 glyph matching
  are near parity — the matcher ties or wins on most images at 256
  colors, consistent with the original research. **Diffusion**: worth
  2–4× on the blurred-ΔE metric, growing with palette size, and the
  full dither's entire practical lead — the matcher does not have it
  yet. See the standings tables in `docs/glyph-research/README.md`.
- Simple heuristics (DominantColorSelector) are near-optimal at 256
  colors — validated by true exhaustive search. The constraint is the
  medium, not the algorithms.
- The most promising direction is hybrid cells: glyphs for high-detail
  low-color regions, quadrant dithering elsewhere, refereed by the
  blurred-LAB harness in `diffusion_quality_test.go`.

### Glyph Bitmaps: Hard-Won Lessons

1. **One canonical bit ordering.** Three incompatible orderings
   coexisted in early implementations. The layout is row-major, LSB =
   top-left (bit `y*8+x` = pixel `(x,y)`), implemented only in
   `GlyphBitmap.Bit`/`SetBit` and locked by `TestGlyphBitmapOrdering`.
   ROM dumps are MSB-left per row; only `LoadROMFont` does that swap.

2. **Never trust font metrics at 8px — calibrate.** Rasterizing the TTF
   recreation with metrics-derived baselines can pass blunt checks while
   being wrong: a half-pixel baseline error still renders '█' and '▀'
   perfectly (their edges sit on cell boundaries) yet smears every
   single-pixel stroke across two rows. `compute_glyphs` searches
   (ppem, baseline, x-offset) for zero coverage *ambiguity* — on the
   design grid every cell is ~0% or ~100% inked. Its `-compare` flag
   verifies regeneration is bit-identical to the embedded data.

3. **Font quirks are real, not bugs.** Two independent rasterization
   approaches produce bit-identical glyphs, confirming: '|' genuinely
   has a gap at row 3 (CP437 broken-bar tradition), '+' genuinely sits
   left of center (column 7 is the spacing column). "Obvious" matches
   may not work as expected — that is the font, not the loader.

4. **A font cannot say no — check coverage explicitly.** `DrawString`
   renders the missing-glyph box for unmapped runes (an inverse '?' in
   PxPlus IBM BIOS). 89 runes of the research set — including all 10
   quadrant-only blocks the dither emits — were once embedded as
   identical '?' boxes that `GetGlyph` reported as present, drawing
   question marks all over rendered output. `compute_glyphs` now skips
   unmapped runes, the library synthesizes the 16 geometric quadrant
   blocks when absent (`synthesizeBlockGlyphs`), and
   `TestBlockGlyphsCoverDitherOutput` pins all 16 dither runes to their
   exact quadrant geometry.

5. **The matcher exists but is not wired into the CLI.**
   `GlyphMatcher` (`glyphmatch.go`) implements the ideal-mask +
   weighted-Hamming (XOR/popcount) search as a `BlockConverter` — it
   replaced the retired 70/20/10 similarity scorer, and
   `TestGlyphMatcherExactGlyph` pins exact glyph reproduction. The
   harness scores it against the quadrant dither and the mean-color
   floor (`TestConverterArms`); at 16 colors the quadrant dither still
   wins everywhere, per the original research. No output path emits
   glyph-matched ANSI yet.

6. **The nearest-color machinery shipped broken for years (fixed).**
   `buildKDTree` dropped colors via a depth cap (the ansi16 tree had 15
   of 16 colors — pure black missing), and `nearestNeighbor` searched
   with the wrong axes, wrapping uint8 arithmetic, and unit-mismatched
   pruning; every embedded table mapped pure black to `#555555`. Found
   through the glyph matcher's anchor probe. Tables are now built by
   exact linear scan, the tree search is validated against a
   linear-scan oracle (`kdtree_test.go`), and all `.palette` files were
   regenerated. See `docs/glyph-research/README.md` for the full
   anatomy.

## Critical Implementation Notes

**Convention — entries here and in "Hard-Won Lessons" are historical
learnings, not a live bug list.** Every entry describes a defect that has been
*fixed* unless it carries an explicit `**Status**: Open` marker. Entries are
often written as the problem statement at the time of discovery — do not infer
current state from prose or tense. Before treating anything here as a live
bug, re-read the cited code; if you fix or confirm an entry, update its status
so the next reader doesn't re-derive it. A stale "this is broken" note
manufactures phantom bugs as easily as a missing note hides real ones.

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

### Unicode Block Pattern Quadrants (Historical Bug, Fixed)

Error diffusion measures each pixel's residual against the color it
actually renders as (foreground or background), determined by the rune's
quadrant pattern via `getQuadrantsForRune` (a map built once from `Blocks`,
amortized over the 4 pixels of each block — not a hot-path concern).

**Historical note**: this code previously used a bitwise trick on the rune
*codepoint* (`bestRune & (1 << (3 - i))`) on the belief that the block
characters' codepoints encode their quadrant patterns. They do not — the
trick disagreed with the quadrant table for 25 of 64 pixels, so diffusion
propagated residuals measured against the wrong colors. Fixing it improved
the blurred-LAB error metric on every gradient and most photo tests (see
`diffusion_quality_test.go`).

What IS true: the `Blocks` array **index** encodes the quadrant pattern
(bit 0 = top-left, bit 1 = top-right, bit 2 = bottom-left, bit 3 =
bottom-right), verified by `TestBlocksIndexEncodesQuadrants`. If a bitwise
form is ever wanted again, it must use the array index, not the rune.

### Measuring Diffusion Quality

`diffusion_quality_test.go` is the quality harness. It renders block
output back to pixels and scores it against the pre-dither reference:

- **Blurred LAB ΔE (primary)**: Gaussian-blur both images (σ=1, 2, 4),
  compare mean ΔE in LAB. Diffusion's job is local average tone; the blur
  measures exactly that. Raw MSE *rises* under dithering (it rewards
  banding) and is reported only as a secondary signal.
- **Color transitions**: more transitions on a smooth ramp = smoother
  dithering (port of the original harness metric).
- Arms: `no-diffusion` (per-block quantization) vs `diffusion`.
- `TestDiffusionQualityPhotos` runs against any PNGs in `images/` (the
  committed reference corpus — see `images/README.md`); set
  `DIFFUSION_PNGS=<dir>` to dump comparison renders.

The cross-converter harness (`measureConverterArms`,
`TestConverterArms`) generalizes this to any `BlockConverter`: each
arm's input is prepared at its native source resolution for the same
cell grid, outputs are rendered at a common 8 px/cell (quadrant
geometry or font glyphs), and blur sigma is expressed in cell widths so
scores are comparable across converters. This is how glyph matchers get
scored against the quadrant dither.

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