package img2ansi

import (
	"fmt"
	"math"
	"math/bits"
	"sort"

	"github.com/wbrown/img2ansi/imageutil"
)

// GlyphMatcher is a BlockConverter that matches each 8x8 source cell to
// a font glyph plus a (foreground, background) palette pair, replacing
// the retired multi-factor similarity scorer with an exact search.
//
// The search uses the ideal-mask formulation. For a candidate pair
// (fg, bg), let δᵢ = d(pᵢ, fg) − d(pᵢ, bg) for each of the 64 cell
// pixels. Any glyph's total error is
//
//	Σᵢ d(pᵢ, bg) + Σ_{i ∈ mask} δᵢ
//	= base(bg) + idealSum + Σ_{i ∈ mask XOR ideal} |δᵢ|
//
// where ideal = {i : δᵢ < 0} is the mask a perfect glyph would have.
// The best glyph for a pair is therefore the one nearest the ideal mask
// under |δ|-weighted Hamming distance — evaluated per glyph by XORing
// two uint64s and iterating only the mismatched bits, with early exit
// once a candidate exceeds the best total so far. This makes truly
// exhaustive glyph search cheap instead of approximated.
//
// Candidate colors are the cell's anchor palette colors: the nearest
// palette color of each pixel, ranked by frequency (the dominant-color
// heuristic validated by the original research), plus the nearest color
// to the cell mean — which keeps the flat block inside the search space
// and matters on smooth gradients under fine palettes. Candidate glyphs
// are the font's genuine glyphs only — synthesized stand-ins never
// expand what the target medium is claimed to support.
type GlyphMatcher struct {
	renderer *Renderer
	font     *FontBitmaps
	runes    []rune
	masks    []uint64
	flatRune rune

	// MaxAnchors bounds the per-cell candidate colors (top-K palette
	// colors by pixel frequency), and with it the pair search width.
	// Clamped to 8.
	MaxAnchors int

	// Diffusion enables Floyd-Steinberg error diffusion across cells:
	// after a cell's (glyph, fg, bg) is decided, each source pixel's
	// residual against the color it renders as is diffused to
	// neighboring source pixels with the same weights the quadrant
	// dither uses, letting later cells compensate. Residual portions
	// landing inside the already-decided cell are dead writes, exactly
	// as at the quadrant dither's block boundaries. Enabling this makes
	// Convert mutate its input image.
	Diffusion bool
}

var _ BlockConverter = (*GlyphMatcher)(nil)

// Alphabet presets for RestrictAlphabet. Each is a rune range that the
// matcher intersects with the font's genuine glyphs; combine them with
// append. The full genuine set is the default.
//
// AlphabetBlocks has no typographic spacing columns — every glyph's ink
// can reach all 8 columns and rows — so it avoids the 1px cell gutters
// that letterforms produce when used as area texture on photographs
// (letterform glyphs reserve column 7 for inter-character spacing).
// Restricting to blocks + box drawing measured tone-neutral against
// the full set on the blurred-ΔE harness; letterforms are an aesthetic
// choice, not a fidelity source.
var (
	// AlphabetBlocks: block elements and shades (U+2580–259F) plus space.
	AlphabetBlocks = alphabetRange(0x2580, 0x259F, ' ')
	// AlphabetBoxDrawing: box drawing (U+2500–257F).
	AlphabetBoxDrawing = alphabetRange(0x2500, 0x257F)
	// AlphabetASCII: printable ASCII — classic text-art output.
	AlphabetASCII = alphabetRange(0x20, 0x7E)
)

func alphabetRange(lo, hi rune, extra ...rune) []rune {
	runes := make([]rune, 0, int(hi-lo)+1+len(extra))
	for r := lo; r <= hi; r++ {
		runes = append(runes, r)
	}
	return append(runes, extra...)
}

// NewGlyphMatcher builds a matcher over the font's genuine glyph set
// using the renderer's palette and color distance method. Use
// RestrictAlphabet to narrow the candidate glyphs.
func NewGlyphMatcher(r *Renderer, font *FontBitmaps) *GlyphMatcher {
	m := &GlyphMatcher{
		renderer:   r,
		font:       font,
		MaxAnchors: 6,
	}
	// Cannot fail: the font loaders never produce an empty glyph set.
	_ = m.RestrictAlphabet(nil)
	return m
}

// RestrictAlphabet limits the candidate glyphs to the given runes,
// intersected with the font's genuine glyphs — the search space never
// exceeds what the font provides, and synthesized glyphs never count
// (the same rule as WithBlocksFromFont). Passing nil restores the
// font's full genuine set. If the intersection is empty an error is
// returned and the current alphabet is kept.
func (m *GlyphMatcher) RestrictAlphabet(alphabet []rune) error {
	candidates := alphabet
	if candidates == nil {
		candidates = m.font.Runes()
	}
	// Sorted copy for deterministic tie-breaking between equal-cost
	// glyphs (and so the caller's slice is left untouched).
	sorted := append([]rune{}, candidates...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var runes []rune
	var masks []uint64
	seen := make(map[rune]bool, len(sorted))
	for _, ru := range sorted {
		if seen[ru] {
			continue
		}
		seen[ru] = true
		if g, ok := m.font.GenuineGlyph(ru); ok {
			runes = append(runes, ru)
			masks = append(masks, uint64(g))
		}
	}
	if len(runes) == 0 {
		return fmt.Errorf("alphabet shares no genuine glyphs with font %s", m.font.Name())
	}

	m.runes, m.masks = runes, masks

	// Rune for flat cells (fg == bg renders identically under any
	// glyph; prefer the full block when the alphabet has one).
	m.flatRune = m.runes[0]
	for gi, mask := range m.masks {
		if mask == ^uint64(0) {
			m.flatRune = m.runes[gi]
			break
		}
	}
	return nil
}

// SourcePixelsPerCell implements BlockConverter: each character cell
// covers an 8x8 block of source pixels.
func (m *GlyphMatcher) SourcePixelsPerCell() int {
	return GlyphWidth
}

// Convert implements BlockConverter. The edge map is currently unused.
// With Diffusion enabled, img is mutated as cells are processed in
// raster order.
func (m *GlyphMatcher) Convert(img *imageutil.RGBAImage, edges *imageutil.GrayImage) [][]BlockRune {
	k := GlyphWidth
	cellsH, cellsW := img.Height()/k, img.Width()/k
	out := make([][]BlockRune, cellsH)
	for cy := range out {
		out[cy] = make([]BlockRune, cellsW)
		for cx := range out[cy] {
			cell, mask := m.matchCell(img, cx*k, cy*k)
			out[cy][cx] = cell
			if m.Diffusion {
				m.diffuseCellResidual(img, cx*k, cy*k, cell, mask)
			}
		}
	}
	return out
}

// diffuseCellResidual measures each source pixel's residual against the
// color it renders as under the chosen (glyph, fg, bg) and diffuses it
// with the quadrant dither's Floyd-Steinberg weights via
// distributeError. Only the portions that cross into not-yet-decided
// cells have any effect; in-cell targets are dead writes, matching the
// 2x2 dither's behavior at its own block boundaries.
func (m *GlyphMatcher) diffuseCellResidual(
	img *imageutil.RGBAImage,
	x0, y0 int,
	cell BlockRune,
	mask GlyphBitmap,
) {
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			p := img.GetRGB(x0+x, y0+y)
			target := cell.BG
			if mask.Bit(x, y) {
				target = cell.FG
			}
			residual := RGB{p.R, p.G, p.B}.subtractToError(target)
			distributeError(img, y0+y, x0+x, residual, false)
		}
	}
}

// matchCell finds the (glyph, fg, bg) with minimum total color error
// for the 8x8 cell at (x0, y0), returning the chosen cell and the
// winning glyph's mask (which diffusion needs to know each pixel's
// rendered color).
func (m *GlyphMatcher) matchCell(img *imageutil.RGBAImage, x0, y0 int) (BlockRune, GlyphBitmap) {
	const n = GlyphWidth * GlyphHeight

	var pixels [n]RGB
	var rSum, gSum, bSum int
	counts := make(map[RGB]int, 8)
	for i := 0; i < n; i++ {
		p := img.GetRGB(x0+i%GlyphWidth, y0+i/GlyphWidth)
		c := RGB{p.R, p.G, p.B}
		pixels[i] = c
		rSum += int(c.R)
		gSum += int(c.G)
		bSum += int(c.B)
		counts[m.renderer.nearestFgColor(c)]++
	}

	// The nearest color to the cell MEAN is anchored alongside the
	// per-pixel dominants: it guarantees the flat block is in the
	// search space (so exhaustive matching can never lose to the
	// mean-color baseline), and with fine palettes it often differs
	// from every per-pixel anchor on smooth gradients.
	meanAnchor := m.renderer.nearestFgColor(RGB{
		uint8(rSum / n), uint8(gSum / n), uint8(bSum / n)})

	anchors := topAnchors(counts, m.maxAnchors()-1)
	hasMean := false
	for _, a := range anchors {
		if a == meanAnchor {
			hasMean = true
			break
		}
	}
	if !hasMean {
		anchors = append(anchors, meanAnchor)
	}
	if len(anchors) == 1 {
		// Flat cell: with fg == bg every glyph renders identically, and
		// the mask is irrelevant for the same reason.
		return BlockRune{Rune: m.flatRune, FG: anchors[0], BG: anchors[0]},
			^GlyphBitmap(0)
	}

	// Distance matrix: pixel x anchor, computed once per cell and
	// shared by all (fg, bg) pairs.
	var dist [n][8]float64
	for i := 0; i < n; i++ {
		for a := range anchors {
			dist[i][a] = m.renderer.ColorMethod.Distance(pixels[i], anchors[a])
		}
	}

	best := math.Inf(1)
	var bestRune rune
	var bestMask GlyphBitmap
	var bestFG, bestBG RGB

	var absDelta [n]float64
	for f := range anchors {
		for b := range anchors {
			if f == b {
				continue
			}

			var base, idealSum float64
			var ideal uint64
			for i := 0; i < n; i++ {
				delta := dist[i][f] - dist[i][b]
				base += dist[i][b]
				if delta < 0 {
					ideal |= 1 << i
					idealSum += delta
					absDelta[i] = -delta
				} else {
					absDelta[i] = delta
				}
			}

			// Even a glyph matching the ideal mask exactly costs this
			// much; skip the pair if that cannot beat the best so far.
			pairFloor := base + idealSum
			if pairFloor >= best {
				continue
			}

			for gi, mask := range m.masks {
				xor := mask ^ ideal
				cost := pairFloor
				for xor != 0 && cost < best {
					cost += absDelta[bits.TrailingZeros64(xor)]
					xor &= xor - 1
				}
				if xor == 0 && cost < best {
					best = cost
					bestRune = m.runes[gi]
					bestMask = GlyphBitmap(mask)
					bestFG = anchors[f]
					bestBG = anchors[b]
				}
			}
		}
	}

	return BlockRune{Rune: bestRune, FG: bestFG, BG: bestBG}, bestMask
}

func (m *GlyphMatcher) maxAnchors() int {
	if m.MaxAnchors < 2 {
		return 2
	}
	if m.MaxAnchors > 8 {
		return 8
	}
	return m.MaxAnchors
}

// topAnchors returns up to maxK anchor colors ordered by descending
// pixel frequency, with RGB ordering as a deterministic tie-break.
func topAnchors(counts map[RGB]int, maxK int) []RGB {
	type anchor struct {
		color RGB
		count int
	}
	all := make([]anchor, 0, len(counts))
	for c, n := range counts {
		all = append(all, anchor{c, n})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].count != all[j].count {
			return all[i].count > all[j].count
		}
		a, b := all[i].color, all[j].color
		if a.R != b.R {
			return a.R < b.R
		}
		if a.G != b.G {
			return a.G < b.G
		}
		return a.B < b.B
	})
	if len(all) > maxK {
		all = all[:maxK]
	}
	colors := make([]RGB, len(all))
	for i, a := range all {
		colors[i] = a.color
	}
	return colors
}
