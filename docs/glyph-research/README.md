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
| Measurement harness: display chain, blurred-LAB scoring, `MeasureConverterArms`, CRT model | `harness.go` |
| Standalone harness runner (sweeps, ladders, composites, over-budget cells) | `cmd/quality/` |

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
palette, blurred ΔE σ=2, `TestBlockAlphabetQuality`, corrected tables;
this table predates the display-geometry chain and is scored on flat
2×2 square-pixel renders — the within-table comparison is what matters
and is unaffected):

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

`MeasureConverterArms` (`harness.go`) runs any set of converters over
the same cell grid: each arm's input is prepared at its native source
resolution, its output rendered through the display chain, and every
arm is scored against the same reference with blur sigma expressed in
*cell widths*, so numbers are comparable across source resolutions.

**Display geometry.** The harness targets the canonical 80×25 text
screen (`FitGrid` fits a photo's aspect inside it). On a 4:3 CRT the
80-column modes displayed cells at ~1:2.4 — CGA 640×200 (8×8 glyphs at
1:2.4 pixel aspect) and VGA 720×400 (9×16 cells) both land on exactly
1:2.4. The modeled chain: every cell renders as its 8×8 font glyph —
including the quadrant dither's runes, which are font glyphs like any
others, never idealized rectangles — then rows are scan-doubled to
8×16 (what 400-line text modes did), then the CRT's 4:3 geometry adds
the remaining ×1.2 vertical stretch. Rendered output and reference
share the source image's true aspect; nothing is squashed. All
standings below are measured under this chain.

The same machinery is reachable two ways:

- **Tests** (`diffusion_quality_test.go`): `TestConverterArms` is the
  standings table, `TestAlphabetLadderUnderCRT` the alphabet/display
  ladder. With `DIFFUSION_PNGS=<dir>` set, `TestConverterArms` writes
  labeled side-by-side comparison images (labels rendered through
  font8x8).
- **`cmd/quality`**: the standalone runner for everything that exceeds
  the test binary's time budget — LAB-matched dither cells (~12
  minutes each), method sweeps, alphabet ladders, CRT-scored arms, and
  the comparison composites committed under
  [comparisons/](comparisons/). It drives the same exported harness
  functions, so its numbers and the tests' numbers are the same
  numbers. Out-of-tree copies of the scoring pipeline are how
  measurement bugs are born; this tool exists so they never are.

## The glyph matcher

`GlyphMatcher` (`glyphmatch.go`) implements the ideal-mask formulation
that replaces the retired similarity scorer. For a candidate (fg, bg)
pair, `δᵢ = d(pᵢ, fg) − d(pᵢ, bg)` per pixel; a glyph's total error is
`base(bg) + idealSum + Σ_{mask XOR ideal} |δᵢ|`, so the best glyph is
the one nearest the ideal mask `{i : δᵢ < 0}` under |δ|-weighted
Hamming distance — one XOR plus a bit-iteration per glyph, with early
exit. Exhaustive search over the font's genuine glyphs is exact and
cheap (the full six-image, five-arm, two-palette `TestConverterArms`
run takes ~80 s).
Candidate colors are the cell's dominant palette anchors (the heuristic
the original research validated); `TestGlyphMatcherExactGlyph` pins the
promise the old scorer never kept: a cell that IS a glyph matches that
glyph.

### Alphabet restriction

`RestrictAlphabet(runes)` limits the candidate glyphs to the given
runes ∩ the font's genuine glyphs (nil restores the full set; an empty
intersection is rejected). Preset ranges compose with `append`:
`AlphabetBlocks` (block elements + shades + space), `AlphabetBoxDrawing`,
`AlphabetASCII`.

This exists because of a finding that masqueraded as an off-by-one:
font8x8's letterforms reserve column 7 (and mostly row 7) as the
typographic spacing column, so when the exact search deploys 'a'/'z'/'Q'
as dither texture over smooth photo regions, each such cell carries a
grid-aligned 1px background gutter — visually a systematic shift,
though the registration is pixel-true (`TestGlyphMatcherMultiCellRoundTrip`
pins it). Measured on mandrill-ansi256 with diffusion: full alphabet
ΔE 4.56, blocks+box 4.58, ASCII-only 4.94 (at ansi16: 12.79, 12.73,
13.88 — blocks+box actually edges out the full alphabet there) —
alphabet choice is an aesthetics knob, nearly free on tone; letterforms
are not a fidelity source. The hybrid converter (roadmap #1) removes
smooth regions from glyph duty entirely. Side-by-side:
[comparisons/mandrill-ansi256_alphabets.png](comparisons/mandrill-ansi256_alphabets.png).

### The CRT display model

The gutter artifact stands out because our renderer is the first
display these fonts were never designed for: axis-aligned rectangle
pixels at infinite contrast. A CRT's beam spot low-passes — spacing
columns read as "slightly dimmer," not hard black slots — and the 8×8
designers drew against that. `CRTDisplay` (`harness.go`) models it: a
normalized Gaussian beam-spot PSF convolved in **linear light** (glow
adds in luminance, not sRGB code values), applied asymmetrically to the
rendered side only.

Measured (`TestAlphabetLadderUnderCRT`), the result is a finding about
the *metric*, not the font: under the display model every alphabet
shifts up uniformly (+2% at both palettes — a global linear-light
brightening relative to the reference, alphabet-independent; the
scan-doubled display chain absorbed most of what was an 8% shift at
ansi256 under the old square-cell scoring). Blurred ΔE never charged
the gutter penalty in the first
place — that is *why* alphabet choice measured tone-neutral — so it
cannot reward the cure either. The artifact and its cure are structure
percepts, invisible to a tone metric that blurs at σ = 1 cell.
Visually the model confirms the hypothesis completely: bloom fills the
gutters and the cell grid recedes (see
[comparisons/mandrill-ansi256_crt.png](comparisons/mandrill-ansi256_crt.png)).
This is the strongest concrete case for the structure-sensitive
referee on the roadmap, and `CRTDisplay` doubles as a period-faithful
preview stage meanwhile (`cmd/quality -crt` writes the composites).

### Display-aware matching

The display model's real payoff is on the *matching* side:
`GlyphMatcher.SetBeamSigma` scores candidates as their CRT'd appearance
instead of their 1-bit masks. Each glyph's mask is convolved with the
beam PSF and quantized to coverage levels (cell-local, cross-cell bleed
not modeled); per (fg, bg) pair the matcher builds a ladder of
linear-light blends and charges each pixel the distance to its
coverage level's blend, with a per-pixel-minimum floor for pruning —
the display-model analogue of the ideal mask. Diffusion residuals are
likewise measured against the blended appearance.

Measured (mandrill / fox / wheel, diffusion on, blurred ΔE σ = 1 cell,
hard-pixel scoring — quadrant dither shown for context):

| image / palette | byte-matched | display-matched | 2×2 dither |
|---|---|---|---|
| mandrill ansi16 | 12.79 | **9.32** | 12.08 |
| mandrill ansi256 | 4.56 | **3.81** | 4.55 |
| fox ansi16 | 11.08 | **9.00** | 9.80 |
| fox ansi256 | 5.89 | **5.30** | 6.01 |
| wheel ansi16 | 13.56 | 10.73 | 9.64 |
| wheel ansi256 | 5.42 | 3.82 | 3.36 |

Display-aware matching improves the matcher 10–30% across the board —
on raw hard-pixel scoring, not just under the display model, because
the blend-ladder objective is coverage-aware: it selects (glyph, fg,
bg) whose area-weighted appearance matches the cell, where the 1-bit
objective demands per-pixel mask agreement and so favors harsh
ink-on-ground pairings. The glyph matcher now **beats the quadrant
dither on mandrill and fox at both palettes**, and closes the
wheel gap to 1.1×. Visual verification: the gain is real texture/color
selection, not metric-gaming — softer pairings, smoother fur, no
smearing. Each `_displaymatch` composite shows reference /
quadrant-dither / byte-matched / display-matched:
[mandrill-ansi256](comparisons/mandrill-ansi256_displaymatch.png),
[mandrill-ansi16](comparisons/mandrill-ansi16_displaymatch.png),
[fox-ansi16](comparisons/fox-ansi16_displaymatch.png) (the new
16-color win),
[wheel-ansi16](comparisons/wheel-ansi16_displaymatch.png) (the
dither's remaining win, kept for honesty).

### Which color metric — matching vs judging

Two metrics are in play and they were never the same: every renderer in
the harness uses the `Renderer` default, **Redmean**, for matching and
search distances (nothing ever set `WithColorMethod`), while the
scoring referee (`BlurredLabError`) judges in **LAB ΔE**. Every
standings number above is Redmean-matched unless labeled. Aligning the
objective with the referee is worth 9–17% to both converters (mandrill,
display-aware matcher with diffusion, blurred ΔE σ = 1 cell):

| arm / matching metric | RGB | Redmean | LAB |
|---|---|---|---|
| quadrant dither, ansi16 | — | 12.08 | 10.28 |
| display matcher, ansi16 | 9.10 | 9.32 | **7.97** |
| quadrant dither, ansi256 | — | 4.55 | 3.79 |
| display matcher, ansi256 | 3.99 | 3.81 | **3.47** |

Aligned LAB-vs-LAB, the display-aware matcher still beats the dither at
both palettes (7.97 vs 10.28; 3.47 vs 3.79) — the headline survives the
fairness correction. Naive RGB and Redmean are nearly tied for the
matcher on this image. The cost: `LABMethod.Distance` converts both
colors per call, so LAB matching at 256 colors runs roughly an order
of magnitude slower than Redmean — the ansi256 LAB dither cell takes
~12 minutes, which no test can hold; it runs through `cmd/quality`
(`-methods lab -arms dither`). A `toLab` memoization would make LAB
matching practical; it is a perf item, not a design limit.

A dither-vs-matcher comparison conflates two variables: the cell
REPRESENTATION (2×2 quadrants vs 8×8 glyphs) and ERROR DIFFUSION (the
dither has it, the matcher does not yet). The harness therefore carries
a diffusion-ablated control arm (`quadrant-no-diff`: the same per-block
search as the dither, diffusion off), so the two factors can be read
separately. Standings (blurred ΔE, σ = 1 cell, `TestConverterArms`,
corrected nearest-color tables, display-geometry chain):

ansi16:

| image | 2×2 dither | 2×2 no-diffusion | 8×8 matcher | 8×8 matcher+diffusion | 8×8 flat blocks |
|---|---|---|---|---|---|
| gray-gradient | 2.53 | 6.67 | 6.67 | **2.33** | 6.67 |
| fleshtone | 13.72 | 29.43 | 29.48 | 16.55 | 29.33 |
| color-ramp | 10.77 | 25.15 | 25.24 | 13.71 | 25.05 |
| fox | 9.80 | 14.47 | 15.73 | 11.08 | 17.96 |
| mandrill | 12.08 | 15.39 | 15.86 | 12.79 | 17.71 |
| wheel | 9.64 | 22.13 | 24.06 | 13.56 | 24.49 |

ansi256:

| image | 2×2 dither | 2×2 no-diffusion | 8×8 matcher | 8×8 matcher+diffusion | 8×8 flat blocks |
|---|---|---|---|---|---|
| gray-gradient | 0.51 | 0.46 | **0.45** | 0.63 | 0.55 |
| fleshtone | 4.08 | 11.27 | 11.27 | **4.10** | 11.18 |
| color-ramp | 3.24 | 8.76 | 8.85 | 4.76 | 8.82 |
| fox | 6.01 | 7.79 | 9.11 | **5.89** | 10.94 |
| mandrill | 4.55 | 6.35 | 6.09 | **4.56** | 7.92 |
| wheel | 3.36 | 9.75 | 11.09 | 5.42 | 11.26 |

Reading the factors apart:

- **Representation (2×2 no-diffusion vs 8×8 matcher, apples to
  apples):** near parity. At 16 colors they tie on every synthetic ramp
  and 2×2 leads by 3–9% on photos. At 256 colors the matcher ties or
  wins on four of six images (gray, fleshtone, color-ramp within 1%,
  mandrill) — consistent with the original lab's conclusion that 256
  colors close the representation gap; fox and wheel are the
  exceptions. (An earlier draft claimed the opposite by comparing the
  diffused dither against the undiffused matcher.)
- **Diffusion:** the dominant factor, worth 1.8–2.9× on ramps and
  flat-color content and 19–35% on photos, and — now measured on both
  sides — a *mechanism*, not a property of either representation.
  `GlyphMatcher.Diffusion` diffuses each cell's per-pixel residuals
  (against the rendered glyph colors, via the same `distributeError`
  weights as the quadrant dither) into undecided neighboring cells.
- **With diffusion on both sides, photos are nearly equivalent.** The
  diffused matcher lands within 1% of the full quadrant dither at 256
  colors on mandrill (4.56 vs 4.55) and fleshtone (4.10 vs 4.08),
  *beats* it outright on fox (5.89 vs 6.01) and on the 16-color gray
  gradient (2.33 vs 2.53 — finer spatial masks pay off when the
  palette is coarse). The real gaps that remain are saturated ramps
  and flats — color-ramp (4.76 vs 3.24) and wheel (5.42 vs 3.36) —
  where 2 px cells simply track hue boundaries better than 8 px cells.
- **One measured regression, kept deliberately visible:** on the
  ansi256 gray gradient, diffusion *worsens* the matcher (0.45 → 0.63)
  — and under the display chain the quadrant dither now shows the same
  effect against its own ablation (0.51 vs 0.46). The gradient is
  nearly exactly representable at 256 colors, so the residuals are
  sub-quantum and diffusing them just adds noise — the classic "don't
  dither what you can represent" effect. The harness assertion
  excludes this case explicitly.

## Where the matching should go next

1. **Hybrid cells.** Glyph matching where structure is high and color
   variance low (line art, edges); 2×2 quadrant dithering elsewhere.
   The `edges` argument of `Convert` is currently unused — it is the
   natural input for the mode decision. With diffusion on both modes,
   the hybrid's two halves are finally comparable in strength.
2. **A structure-sensitive referee.** Blurred ΔE measures tone; glyph
   matching's pitch is structure. An SSIM-like arm in the harness would
   test whether the matcher preserves edges better than the numbers
   above can show.
3. **Diffusion refinements.** Adaptive diffusion (suppress it where
   residuals are sub-quantum, per the ansi256-gradient regression),
   serpentine scanning for both converters, and edge-damped diffusion
   for the matcher like the quadrant dither has.
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
