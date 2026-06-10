# Glyph Matching Research

Research toward font-agnostic image rendering: matching 8×8 pixel blocks
to arbitrary font glyphs instead of the 16 Unicode quadrant blocks.

This directory continues work from two research branches:

- **`font-analysis`** — the original `cmd/compute_fonts` lab: 39 files of
  matching experiments, color selectors, and the experiment log preserved
  here as [GLYPH_MATCHING_EXPERIMENTS.md](GLYPH_MATCHING_EXPERIMENTS.md).
- **`feature/font-png-rendering`** — the later integration pass that
  produced the `GlyphBitmap` infrastructure and `.glyphs` data format.

## What lives where (after the port to main)

| piece | location |
|---|---|
| `GlyphBitmap` (8×8 as uint64), `FontBitmaps`, `.glyphs` loading, ROM font loading, font-based block rendering | `glyph.go` |
| CP437 ↔ Unicode table (also the ROM glyph order) | `cp437.go` |
| Glyph generation, geometry calibration, comparison/inspection tooling | `cmd/compute_glyphs/` |
| Pre-rendered IBM BIOS glyphs (embedded) | `fontdata/pxplus_ibm_bios.glyphs` |
| The TTF source for regeneration | `fonts/PxPlus_IBM_BIOS.ttf` |

**Deliberately not ported**: the multi-factor similarity scorer from
`font-analysis` (`fonts.go` — shape/pattern/density/zone scoring with
hand-tuned weights). The experiment log's own conclusion stands: it
accumulated special cases while still producing unintuitive matches.
See "Where the matching should go next" below for the replacement
formulation before reaching for it again.

## Bit ordering: one canonical layout

Three incompatible bit orderings coexisted in earlier implementations
(`analyzeGlyph`, `getBit`, and `String` each used their own). The
canonical layout is now row-major with the LSB at the top-left — bit
`y*8+x` is pixel `(x, y)` — implemented only in `GlyphBitmap.Bit` /
`SetBit` and locked by `TestGlyphBitmapOrdering`. ROM dumps store rows
MSB-left; `LoadROMFont` performs that reversal in exactly one place.

## The rasterization story (read this before touching the rasterizer)

Rendering a TTF recreation of a bitmap font back into bitmaps is a
loaded footgun: metrics-derived baselines, hinting, anti-aliasing, and
thresholding can each shift or fatten strokes, and at 8×8 one pixel is
1.5% of the glyph. The original pipeline (8pt at 72dpi, full hinting,
25% alpha threshold, baseline `(8+ascent-descent)/2`) caused well-
documented grief during development — but, for this font, its shipped
output was correct.

We verified that empirically while porting. `compute_glyphs` now
rasterizes at 8× supersampling with hinting off and **calibrates**
geometry instead of trusting metrics, searching (ppem, baseline,
x-offset) for the configuration where:

1. `█` renders as a solid cell and `▀` as exactly the top half
   (hard requirements), and
2. total coverage *ambiguity* over probe glyphs (`█▀▌AMgs+=|░`) is
   minimal — on the design grid every cell is ~0% or ~100% covered;
   half-pixel phase errors show up as ~50% cells.

Requirement 1 alone is **not sufficient** — block edges sit on cell
boundaries, so a half-pixel vertical offset (baseline 52 instead of 56)
passes the block checks while smearing every single-pixel stroke into
two rows: 225 of 233 glyphs wrong, 1817 pixels changed, all while the
calibration looked "perfect." The ambiguity criterion catches this.

With correct calibration the result is **bit-identical to the original
embedded data** (0 glyphs differ, ambiguity 0.00) for every rune the
font actually maps — which validates those glyphs and gives us a
regression check:

```bash
cd cmd/compute_glyphs && go build .
./compute_glyphs -font ../../fonts/PxPlus_IBM_BIOS.ttf \
    -compare ../../fontdata/pxplus_ibm_bios.glyphs    # expect: 0 differ
./compute_glyphs -font ... -dump '|+sA█'              # eyeball glyphs
./compute_glyphs -rom somefont.bin -output out.glyphs # ROM conversion
```

A corollary of the bit-identical round trip: the "font quirks" noted in
CLAUDE.md are confirmed real font behavior, not rasterization bugs —
`|` genuinely has a gap at row 3 (the CP437 broken-bar tradition), `+`
genuinely sits left of center (column 7 is the spacing column in 8×8
glyph design).

### The .notdef poisoning (the second footgun)

Bit-identical agreement between two rasterizers says nothing about runes
the font does not map: `DrawString` happily renders the font's
missing-glyph box for those, and both the original generator and the
first port embedded it. In PxPlus IBM BIOS that box is an inverse-video
`?`, and **89 runes** of the research glyph set are unmapped — including
all 10 quadrant-only blocks (`▘▝▖▗▚▞▛▜▙▟`, which are not CP437) that the
dither pipeline emits constantly. Result: question marks all over every
font-rendered dither, with `GetGlyph` reporting the glyphs as present.

Fixes, in both directions:

- `compute_glyphs` skips runes with no glyph mapping (`Index == 0`) and
  logs them, so missing coverage is visible instead of poisoned.
- The library synthesizes the 16 quadrant block characters from their
  exact geometric definitions when a font lacks them
  (`synthesizeBlockGlyphs`, into the fallback map so genuine font glyphs
  always win). CP437 fonts provide only 6 of the 16, so this path is the
  norm, not the exception — it is what makes `RenderBlocks` usable on
  dither output with ROM-derived fonts at all.
- `TestBlockGlyphsCoverDitherOutput` pins every rune the dither can emit
  to its exact quadrant geometry.

## Fonts

Two glyph sets are embedded:

| font | license | coverage |
|---|---|---|
| `pxplus_ibm_bios` | CC BY-SA 4.0 (TTF recreation) | CP437: ASCII, 6 of 16 blocks (rest synthesized), single/double box drawing |
| `font8x8` | **Public domain** (dhepper/font8x8) | ASCII, complete U+2580–259F (all 16 quadrants genuine), complete U+2500–257F box drawing |

For permissively-licensed work and for matcher experiments wanting full
genuine block/box coverage, `font8x8` is the recommended default — it
loads from raw byte data (`fonts/font8x8/`, no rasterization at all)
and its row format is bit-compatible with `GlyphBitmap`. The IBM BIOS
font remains the authentic-CP437-aesthetic option. ROM dumps of other
8x8 fonts load directly via `LoadROMFont`.

## Alphabet derivation: search space = medium

The recurring rule behind this session's fixes (BBS bright colors,
diffusion targets, .notdef boxes): **the space the search optimizes
over must equal the space the target medium can display.** For pattern
alphabets this is now first-class:

```go
font, _ := img2ansi.LoadEmbeddedFont("pxplus_ibm_bios")
r := img2ansi.NewRenderer(
    img2ansi.WithBlocksFromFont(font),   // dither with Blocks ∩ font
    img2ansi.WithPalette("ansi16"),
)
```

- The derivation uses `GenuineGlyph`, never `GetGlyph`: synthesized
  glyphs exist so previews render; they must never expand what a target
  is claimed to support.
- `WithBBSMode`'s hand-written `BBSBlocks` list is exactly what this
  derivation produces for a CP437 font — pinned by
  `TestBBSBlocksAreCP437Intersection` and `TestWithBlocksFromFont`.
- The future glyph matcher inherits the rule structurally: its candidate
  set is the font's genuine glyph map.

Measured cost of the CP437 restriction (16 blocks → 6, identical ansi16
palette, blurred ΔE σ=2, `TestBlockAlphabetQuality`, corrected tables):

| image | 16-block | 6-block | restriction cost |
|---|---|---|---|
| gray-gradient | 2.54 | 3.34 | +32% |
| fleshtone | 13.39 | 13.58 | +1% |
| color-ramp | 9.66 | 10.01 | +4% |
| fox | 7.53 | 7.64 | +1% |
| mandrill | 11.43 | 11.53 | +1% |
| wheel | 9.30 | 8.12 | −13% |

Error diffusion absorbs most of the loss on textured content; smooth
grayscale ramps are where the quadrant patterns genuinely earn their
keep, and on the flat saturated wedges of the color wheel the
restricted alphabet actually wins — fewer pattern choices means less
spurious quadrant texture for diffusion to clean up. Practically: BBS
mode's pattern restriction is nearly free for photos.

## Findings carried over from the experiment log

From [GLYPH_MATCHING_EXPERIMENTS.md](GLYPH_MATCHING_EXPERIMENTS.md) and
the `font-analysis` lab:

- 2×2 quadrant blocks beat 8×8 glyph matching at 16 colors, decisively.
  Both spend one glyph + two colors per terminal cell, but 8×8 matching
  feeds the cell 16× more source pixels — the 2×2 path converts its
  lower spatial resolution into effective *color* resolution via
  dithering. 256 colors largely rescue 8×8; no matching algorithm does.
- True exhaustive search (589k combinations/block) validated that the
  simple DominantColorSelector heuristic is near-optimal at 256 colors:
  the constraint is the medium, not the algorithm.
- Removing space/full-block from the glyph set forces pattern usage and
  improves perceived structure.

## The cross-converter harness

The framework for running matcher experiments is in place. A converter
is anything implementing `BlockConverter` (`converter.go`):

```go
type BlockConverter interface {
    Convert(img *imageutil.RGBAImage, edges *imageutil.GrayImage) [][]BlockRune
    SourcePixelsPerCell() int // 2 for the quadrant dither, 8 for glyph matchers
}
```

`measureConverterArms` (`diffusion_quality_test.go`) runs any set of
converters over the same cell grid: each arm's input is prepared at its
native source resolution, its output rendered back to pixels at a
common 8 px/cell (quadrant geometry or font glyphs), and every arm is
scored against the same reference with blur sigma expressed in *cell
widths*, so numbers are comparable across source resolutions. With
`DIFFUSION_PNGS=<dir>` set, the harness writes a labeled side-by-side
comparison image per test image (labels rendered through font8x8).

## The glyph matcher

`GlyphMatcher` (`glyphmatch.go`) implements the ideal-mask formulation
that replaces the retired similarity scorer. For a candidate (fg, bg)
pair, `δᵢ = d(pᵢ, fg) − d(pᵢ, bg)` per pixel; a glyph's total error is
`base(bg) + idealSum + Σ_{mask XOR ideal} |δᵢ|`, so the best glyph is
the one nearest the ideal mask `{i : δᵢ < 0}` under |δ|-weighted
Hamming distance — one XOR plus a bit-iteration per glyph, with early
exit. Exhaustive search over the font's genuine glyphs is exact and
cheap (~22s for the full six-image harness including three photos).
Candidate colors are the cell's dominant palette anchors (the heuristic
the original research validated); `TestGlyphMatcherExactGlyph` pins the
promise the old scorer never kept: a cell that IS a glyph matches that
glyph.

Current standings (blurred ΔE, σ = 1 cell, ansi16, `TestConverterArms`,
measured with the corrected nearest-color tables):

| image | quadrant dither (2×2) | glyph matcher (8×8) | mean-color blocks (8×8) |
|---|---|---|---|
| gray-gradient | 2.52 | 6.58 | 6.58 |
| fleshtone | 13.40 | 29.34 | 29.18 |
| color-ramp | 9.65 | 23.69 | 23.61 |
| fox | 7.69 | 17.08 | 17.63 |
| mandrill | 11.56 | 15.84 | 17.22 |
| wheel | 9.29 | 24.49 | 24.38 |

This sharpens the original lab's headline finding: at 16 colors the 2×2
quadrant dither wins everywhere by roughly 2×, and exhaustive glyph
matching beats the flat mean-color block only modestly on photos
(mandrill −8%, fox −3%) while tying it on smooth ramps — one (fg, bg)
pair per 8×8 cell cannot follow a gradient no matter which glyph is
chosen. The medium, not the matching, is the constraint. (An earlier
draft of this table showed the matcher far ahead of the baseline; that
gap was an artifact of the baseline reading the then-broken
nearest-color tables.)

And at 256 colors (`TestConverterArms/ansi256`):

| image | quadrant dither (2×2) | glyph matcher (8×8) | mean-color blocks (8×8) |
|---|---|---|---|
| gray-gradient | 0.29 | 0.36 | 0.38 |
| fleshtone | 3.97 | 10.82 | 11.06 |
| color-ramp | 2.70 | 7.77 | 8.09 |
| fox | 3.81 | 9.18 | 9.77 |
| mandrill | 3.52 | 5.57 | 7.17 |
| wheel | 2.40 | 11.39 | 11.47 |

This **refines** the original lab's "256 colors largely close the gap"
finding rather than confirming it. Under the blurred-ΔE referee,
256 colors improve everything 3–4×, and the matcher's advantage over
flat blocks grows where there is texture (mandrill −22% vs −8% at 16
colors) — but the quadrant dither improves even faster, so the
*relative* gap to 2×2 widens (mandrill ratio 1.37→1.58, fox
2.22→2.41). Error diffusion exploits a finer palette better than
per-cell color pairs can. The old conclusion was a visual judgment
made without a quantitative referee: both arms improving 3× reads as
"the gap closed" to the eye. Two caveats keep the door open for
glyphs: the matcher has no error diffusion of its own, and a
tone-oriented metric structurally favors diffusion — a
structure-sensitive metric, or the hybrid below, may read differently.

## Where the matching should go next

1. **Hybrid cells.** Glyph matching where structure is high and color
   variance low (line art, edges); 2×2 quadrant dithering elsewhere.
   The `edges` argument of `Convert` is currently unused — it is the
   natural input for the mode decision. The 256-color standings make
   this the live question: the matcher's wins are localized to
   texture/structure, exactly where a hybrid would deploy it.
2. **Error diffusion for the matcher.** The dither's widening lead at
   256 colors is diffusion exploiting the finer palette. Diffusing each
   cell's residual (against its rendered glyph) into neighboring cells
   would give the matcher the same lever.
3. **A structure-sensitive referee.** Blurred ΔE measures tone; glyph
   matching's pitch is structure. An SSIM-like arm in the harness would
   test whether the matcher preserves edges better than the numbers
   above can show.
4. **More fonts via ROM dumps.** `LoadROMFont` accepts the classic
   2048-byte CP437 format directly — bit-perfect, no rasterization, and
   covers the PETSCII/ATASCII/DOS font family this research targets.

## The nearest-color defect (found via the matcher, fixed)

The matcher's anchor probe exposed that every embedded table mapped
pure black to `#555555`. Two compounding bugs, both pinned by tests in
`kdtree_test.go`:

- **`buildKDTree` dropped colors.** A `log2(n)+1` depth cap silently
  discarded the remainder slice whenever duplicate component values
  skewed the median — the shipped ansi16 tree contained 15 of 16
  colors, and pure black (first in every sort order) was the one that
  fell off. No search could return a color that was not in the tree.
  The cap is gone; median splitting terminates naturally and every
  color becomes a node (`TestBuildKDTreeContainsAllColors`).
- **`nearestNeighbor` was wrong three ways.** It walked with `depth%3`
  axes although the tree splits on per-node largest-range axes (the
  stored `SplitAxis` was ignored), computed plane distance with
  wrapping uint8 subtraction, and pruned *squared RGB* units against
  *linear* method distances. The traversal now honors `SplitAxis`, uses
  float arithmetic, and prunes with per-method lower bounds (exact for
  RGB; weight floors for Redmean; LAB and custom methods never prune —
  palettes are small enough that full traversal is cheap). Validated
  against a linear-scan oracle over all palettes and methods
  (`TestNearestNeighborMatchesLinearScan`).

Tables are now built by exact linear scan (the table is the product and
must be exact; the tree remains a runtime structure), and every
`.palette` file was regenerated. Consequences of the old defect: cache
keys quantized through wrong anchors, and >32-color candidate search
could never propose dropped colors — dark regions in ansi256 output
systematically avoided true black.
