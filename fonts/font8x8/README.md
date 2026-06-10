# font8x8

Public domain 8x8 bitmap font data, vendored from
https://github.com/dhepper/font8x8 (Daniel Hepper), itself based on
Marcel Sondaar's work and IBM's public domain VGA fonts.

- License: **Public Domain** (stated in each header)
- Format: C headers, one byte per row per glyph, **LSB = leftmost
  pixel** (matching `GlyphBitmap`'s layout directly)
- Vendored files and coverage:
  - `font8x8_basic.h` — U+0000–007F (basic latin)
  - `font8x8_block.h` — U+2580–259F (block elements, complete: all 16
    quadrant blocks genuine, plus shades and eighth blocks)
  - `font8x8_box.h` — U+2500–257F (box drawing, complete, including the
    heavy and dashed variants CP437 fonts lack)

Upstream has further headers (ext_latin, greek, hiragana, misc, sga)
that can be vendored the same way if the matcher wants more vocabulary.

## Local divergence from upstream

`font8x8_box.h` lines 95–96: upstream labels these glyphs U+2547 and
U+254B, but the bitmaps (and upstream's own prose descriptions) are
U+2548 (╈, up light / down+horizontal heavy) and U+2547 (╇, up+
horizontal heavy / down light); the real U+254B appears later in the
file. The duplicate U+254B label shadowed one codepoint and left U+2548
missing entirely. Our copy fixes the two labels, marked inline with
`[label fixed: was U+25xx upstream]`. `compute_glyphs` warns on
duplicate labels so this class of bug is visible, and
`TestFont8x8Coverage` pins the corrected glyphs.

## Regenerating

```bash
cd cmd/compute_glyphs && go build .
./compute_glyphs -font8x8 ../../fonts/font8x8 -output ../../fontdata/font8x8.glyphs
```
