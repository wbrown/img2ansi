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
palette, blurred ΔE σ=2, `TestBlockAlphabetQuality`):

| image | 16-block | 6-block | restriction cost |
|---|---|---|---|
| gray-gradient | 2.54 | 3.42 | +35% |
| fleshtone | 13.39 | 13.58 | +1% |
| color-ramp | 9.66 | 10.00 | +4% |
| mandrill | 11.43 | 11.53 | +1% |

Error diffusion absorbs most of the loss on textured content; smooth
grayscale ramps are where the quadrant patterns genuinely earn their
keep. Practically: BBS mode's pattern restriction is nearly free for
photos.

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
widths*, so numbers are comparable across source resolutions. The
`Renderer` itself is the reference converter; `TestConverterArms` pins
the floor with an 8×8 mean-color full-block baseline (blurred ΔE,
σ = 1 cell, ansi16):

| image | quadrant dither (2×2) | mean-color blocks (8×8) |
|---|---|---|
| gray-gradient | 2.52 | 10.54 |
| fleshtone | 13.40 | 29.18 |
| color-ramp | 9.65 | 26.43 |
| mandrill | 11.56 | 19.64 |

A glyph matcher enters the tournament by implementing `BlockConverter`
and being added as a `fontArm` — nothing else. To justify itself it has
to beat the mean-color floor decisively and approach (or beat, in its
target regimes) the quadrant dither.

## Where the matching should go next

1. **Replace similarity scoring with the ideal-mask formulation.** For a
   fixed (fg, bg) pair, let `δᵢ = d(pᵢ, fg) − d(pᵢ, bg)` per pixel. Any
   glyph's error is a constant plus the sum of `δᵢ` over its set bits,
   so the *ideal* mask is `{i : δᵢ < 0}` and the best glyph is the one
   closest to it under |δ|-weighted Hamming distance. `GlyphBitmap` is
   already a `uint64`: XOR + popcount makes true exhaustive matching
   real-time instead of 5 minutes, and deletes the heuristic zoo. 'O'
   matches circles because nothing approximates anymore. Wrap it in a
   `BlockConverter` and the harness above scores it immediately.
2. **Hybrid cells.** Glyph matching where structure is high and color
   variance low (line art, edges); 2×2 quadrant dithering elsewhere.
   This was the most promising direction in the old lab notes and is
   now measurable.
3. **More fonts via ROM dumps.** `LoadROMFont` accepts the classic
   2048-byte CP437 format directly — bit-perfect, no rasterization, and
   covers the PETSCII/ATASCII/DOS font family this research targets.
