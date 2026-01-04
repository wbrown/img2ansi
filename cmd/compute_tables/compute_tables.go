package main

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"github.com/wbrown/img2ansi"
	"log"
	"os"
	"path/filepath"
)

func main() {
	// Accept path to the file as an argument
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run compute_tables.go <path>")
		os.Exit(1)
	}
	path := os.Args[1]
	data := make(img2ansi.ColorMethodCompactTables)

	// Create all three color distance methods
	methods := []img2ansi.ColorDistanceMethod{
		img2ansi.RGBMethod{},
		img2ansi.LABMethod{},
		img2ansi.RedmeanMethod{},
	}

	for _, method := range methods {
		fmt.Printf("Computing tables for %s\n", method.Name())
		fg, bg, paletteErr := img2ansi.LoadPaletteAsCompactTables(path, method)
		if paletteErr != nil {
			fmt.Printf("Error loading palette: %v\n", paletteErr)
			os.Exit(1)
		}
		data[method.Name()] = img2ansi.CompactTablePair{Fg: fg, Bg: bg}
	}

	// Remove any extensions from path
	p := path[:len(path)-len(filepath.Ext(path))] + ".palette"
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	enc := gob.NewEncoder(gzw)
	if err := enc.Encode(data); err != nil {
		log.Fatalf("Failed to encode computed tables for %s: %v", p, err)
	}
	if err := gzw.Close(); err != nil {
		log.Fatalf("Failed to close gzip writer for %s: %v", p, err)
	}

	if err := os.WriteFile(p, buf.Bytes(), 0644); err != nil {
		log.Fatalf("Failed to write compressed file %s: %v", p, err)
	}
}
