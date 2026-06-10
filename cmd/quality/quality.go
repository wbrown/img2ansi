// Command quality runs the converter-quality harness outside the test
// binary's time budget. It sweeps converter arms across images,
// palettes, and color-distance methods, optionally scores every arm
// through the CRT display model, and writes labeled comparison
// composites — all against the same scoring code the test suite uses
// (img2ansi.MeasureConverterArms), so its numbers and the tests'
// numbers are the same numbers.
//
// The cells that motivate it are the ones a test cannot hold: a
// LAB-matched ansi256 dither run takes ~12 minutes alone. Examples:
//
//	# the slow color-method cell
//	quality -images ../../images/mandrill.png -palettes ansi256 \
//	        -methods lab -arms dither
//
//	# alphabet ladder with composite renders
//	quality -images ../../images/mandrill.png \
//	        -alphabets full,blocks+box,ascii -pngs /tmp/out -tag alphabets
//
//	# byte-matched vs display-matched, scored raw and through the CRT
//	quality -images '../../images/*.png' \
//	        -arms matcher-diff,display-matcher -crt
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wbrown/img2ansi"
	"github.com/wbrown/img2ansi/imageutil"
)

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "quality: "+format+"\n", args...)
	os.Exit(1)
}

// expandImagePaths splits a comma-separated list and expands each entry
// as a glob, preserving order.
func expandImagePaths(spec string) ([]string, error) {
	var paths []string
	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		matches, err := filepath.Glob(entry)
		if err != nil {
			return nil, fmt.Errorf("bad pattern %q: %w", entry, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no files match %q", entry)
		}
		paths = append(paths, matches...)
	}
	return paths, nil
}

func colorMethod(name string) (img2ansi.ColorDistanceMethod, error) {
	switch name {
	case "rgb":
		return img2ansi.RGBMethod{}, nil
	case "redmean":
		return img2ansi.RedmeanMethod{}, nil
	case "lab":
		return img2ansi.LABMethod{}, nil
	}
	return nil, fmt.Errorf("unknown color method %q (rgb, redmean, lab)", name)
}

// buildArms constructs the requested converter arms over one renderer.
func buildArms(specs []string, r *img2ansi.Renderer, font *img2ansi.FontBitmaps, beam float64) ([]img2ansi.ConverterArm, error) {
	var arms []img2ansi.ConverterArm
	for _, spec := range specs {
		switch spec {
		case "dither":
			arms = append(arms, img2ansi.FontArm("quadrant-dither", r, font))
		case "matcher":
			arms = append(arms,
				img2ansi.FontArm("glyph-matcher", img2ansi.NewGlyphMatcher(r, font), font))
		case "matcher-diff":
			m := img2ansi.NewGlyphMatcher(r, font)
			m.Diffusion = true
			arms = append(arms, img2ansi.FontArm("glyph-matcher-diff", m, font))
		case "display-matcher":
			m := img2ansi.NewGlyphMatcher(r, font)
			m.Diffusion = true
			if err := m.SetBeamSigma(beam); err != nil {
				return nil, err
			}
			arms = append(arms, img2ansi.FontArm("display-matcher", m, font))
		default:
			return nil, fmt.Errorf(
				"unknown arm %q (dither, matcher, matcher-diff, display-matcher)", spec)
		}
	}
	return arms, nil
}

// alphabetArms constructs one diffused-matcher arm per requested
// alphabet restriction.
func alphabetArms(specs []string, r *img2ansi.Renderer, font *img2ansi.FontBitmaps) ([]img2ansi.ConverterArm, error) {
	var arms []img2ansi.ConverterArm
	for _, spec := range specs {
		var runes []rune
		switch spec {
		case "full":
			runes = nil
		case "blocks+box":
			runes = append(append([]rune{}, img2ansi.AlphabetBlocks...),
				img2ansi.AlphabetBoxDrawing...)
		case "ascii":
			runes = img2ansi.AlphabetASCII
		default:
			return nil, fmt.Errorf(
				"unknown alphabet %q (full, blocks+box, ascii)", spec)
		}
		m := img2ansi.NewGlyphMatcher(r, font)
		m.Diffusion = true
		if err := m.RestrictAlphabet(runes); err != nil {
			return nil, err
		}
		arms = append(arms, img2ansi.FontArm(spec, m, font))
	}
	return arms, nil
}

func main() {
	images := flag.String("images", "",
		"comma-separated image paths or globs (required)")
	palettes := flag.String("palettes", "ansi16,ansi256",
		"comma-separated palettes to sweep")
	methods := flag.String("methods", "redmean",
		"comma-separated color-distance methods: rgb, redmean, lab")
	armSpec := flag.String("arms", "dither,matcher-diff,display-matcher",
		"comma-separated arms: dither, matcher, matcher-diff, display-matcher")
	alphabets := flag.String("alphabets", "",
		"alphabet ladder for the diffused matcher (full, blocks+box, ascii); overrides -arms")
	beam := flag.Float64("beam", 0.5,
		"beam-spot sigma in font pixels (display-matcher objective and -crt scoring)")
	crt := flag.Bool("crt", false,
		"also score every arm through the CRT display model")
	pngs := flag.String("pngs", "",
		"directory for the labeled comparison composite per image/palette/method")
	tag := flag.String("tag", "compare",
		"composite filename suffix: <image>-<palette>[-<method>]_<tag>.png")
	fontName := flag.String("font", "font8x8",
		"embedded font for glyph arms and composite labels")
	flag.Parse()

	if *images == "" {
		flag.Usage()
		os.Exit(2)
	}

	font, err := img2ansi.LoadEmbeddedFont(*fontName)
	if err != nil {
		fatalf("loading font: %v", err)
	}
	paths, err := expandImagePaths(*images)
	if err != nil {
		fatalf("%v", err)
	}

	logf := func(format string, args ...any) {
		fmt.Printf(format+"\n", args...)
	}

	for _, path := range paths {
		img, err := imageutil.LoadImage(path)
		if err != nil {
			fatalf("loading %s: %v", path, err)
		}
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		cols, rows := img2ansi.FitGrid(float64(img.Width()) / float64(img.Height()))

		for _, pal := range strings.Split(*palettes, ",") {
			for _, methodName := range strings.Split(*methods, ",") {
				method, err := colorMethod(methodName)
				if err != nil {
					fatalf("%v", err)
				}
				r := img2ansi.NewRenderer(
					img2ansi.WithPalette(pal),
					img2ansi.WithColorMethod(method))

				name := base + "-" + pal
				if methodName != "redmean" {
					name += "-" + methodName
				}

				var arms []img2ansi.ConverterArm
				if *alphabets != "" {
					arms, err = alphabetArms(strings.Split(*alphabets, ","), r, font)
				} else {
					arms, err = buildArms(strings.Split(*armSpec, ","), r, font, *beam)
				}
				if err != nil {
					fatalf("%v", err)
				}

				scores, reference, err := img2ansi.MeasureConverterArms(
					name, img2ansi.PhotoSource(img), cols, rows, arms, logf, "")
				if err != nil {
					fatalf("measuring %s: %v", name, err)
				}

				labels := []string{"reference"}
				panels := []*imageutil.RGBAImage{reference}
				for _, s := range scores {
					labels = append(labels,
						fmt.Sprintf("%s dE=%.2f", s.Name, s.OneCell))
					panels = append(panels, s.Rendered)
					if *crt {
						crtView := img2ansi.CRTDisplay(s.Rendered, *beam)
						crtScore := img2ansi.BlurredLabError(
							crtView, reference, img2ansi.GlyphWidth)
						logf("%-14s %-18s ΔE crt(σ=%.1f)=%6.2f  (%+.0f%% vs raw)",
							name, s.Name, *beam, crtScore, (crtScore/s.OneCell-1)*100)
						labels = append(labels,
							fmt.Sprintf("%s crt dE=%.2f", s.Name, crtScore))
						panels = append(panels, crtView)
					}
				}

				if *pngs != "" {
					out := filepath.Join(*pngs,
						fmt.Sprintf("%s_%s.png", name, *tag))
					composite := img2ansi.ComposeComparison(font, labels, panels)
					if err := imageutil.SavePNG(composite, out); err != nil {
						fatalf("saving %s: %v", out, err)
					}
					logf("wrote %s", out)
				}
			}
		}
	}
}
