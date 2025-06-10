# Fonts Directory

This directory contains TrueType fonts used for rendering ANSI art to PNG images.

## PxPlus_IBM_BIOS.ttf

The IBM PC BIOS 8x8 font, recreated as a TrueType font. This font includes:
- Full ASCII character set
- Unicode block drawing characters (▀▄▌▐█ etc.)
- Box drawing characters

This font provides an authentic terminal appearance when rendering ANSI art to images.

## Usage

```go
fonts, err := LoadFontBitmaps("fonts/PxPlus_IBM_BIOS.ttf", "")
```

## Legal Note

The IBM PC BIOS font bitmaps are widely considered to be in the public domain as bitmap fonts cannot be copyrighted in the US. This TrueType recreation is used for compatibility and rendering purposes.