package img2ansi

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"sort"
	"strconv"
	"strings"
)

//go:embed colordata/ansi16.json
//go:embed colordata/ansi16.palette
//go:embed colordata/ansi256.json
//go:embed colordata/ansi256.palette
//go:embed colordata/jetbrains32.json
//go:embed colordata/jetbrains32.palette
var f embed.FS

// ByAnsiCode implements sort.Interface for AnsiData based on the numeric
// value of the ANSI code
type ByAnsiCode AnsiData

func (a ByAnsiCode) Len() int      { return len(a) }
func (a ByAnsiCode) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByAnsiCode) Less(i, j int) bool {
	// Extract numeric part from ANSI code strings
	numI, _ := strconv.Atoi(a[i].Value)
	numJ, _ := strconv.Atoi(a[j].Value)
	return numI < numJ
}

// ReadAnsiDataFromJSON reads ANSI color data from a JSON file and returns
// the foreground and background ANSI color data as AnsiData slices. The
// function takes a filename as a string and returns the foreground and
// background AnsiData slices, or an error if the file cannot be read or
// the data cannot be unmarshalled.
func ReadAnsiDataFromJSON(filename string) (AnsiData, AnsiData, error) {
	var data []byte
	// First, try the VFS.
	template := "colordata/%s.json"
	data, vfsErr := f.ReadFile(fmt.Sprintf(template, filename))
	if vfsErr != nil {
		// If the VFS fails, try the filesystem.
		var fsErr error
		data, fsErr = ioutil.ReadFile(filename)
		if fsErr != nil {
			return nil, nil, fmt.Errorf("error reading file: %v", fsErr)
		}
	}

	// Unmarshal JSON data
	var colorMap map[string]string
	if jsonErr := json.Unmarshal(data, &colorMap); jsonErr != nil {
		return nil, nil, fmt.Errorf("error unmarshalling JSON: %v",
			jsonErr)
	}

	// Initialize foreground and background AnsiData slices
	var fgAnsiData, bgAnsiData AnsiData

	// Process each entry in the color map
	for code, hexColor := range colorMap {
		// Convert hex color to uint32
		hexColor = strings.TrimPrefix(hexColor, "#")
		colorUint, cErr := strconv.ParseUint(hexColor, 16, 32)
		if cErr != nil {
			return nil, nil, fmt.Errorf("error parsing color %s: %v",
				hexColor, cErr)
		}

		// Create AnsiEntry
		entry := AnsiEntry{
			Key:   uint32(colorUint),
			Value: code,
		}

		// Add to appropriate slice based on foreground/background
		if colorIsForeground(code) {
			fgAnsiData = append(fgAnsiData, entry)
		} else if colorIsBackground(code) {
			bgAnsiData = append(bgAnsiData, entry)
		} else {
			return nil, nil, fmt.Errorf("unknown color code type: %s",
				code)
		}
	}

	// Sort the AnsiData slices
	sort.Sort(ByAnsiCode(fgAnsiData))
	sort.Sort(ByAnsiCode(bgAnsiData))

	return fgAnsiData, bgAnsiData, nil
}

type ComputedTables struct {
	AnsiData        AnsiData
	ColorArr        *[]RGB
	ClosestColorArr *[]RGB
	ColorTable      *map[RGB]uint32
	KdTree          *ColorNode
}

// ComputeTables computes the color tables and KD-tree for a given color map.
// The function takes an OrderedMap of color codes and RGB colors as input,
// and returns a ComputedTables struct containing the color array, closest
// color array, color table, and KD-tree.
func ComputeTables(colorData AnsiData) ComputedTables {
	closestColorArr := make([]RGB, 256*256*256)
	colorTable := make(map[RGB]uint32)
	colorArr := make([]RGB, len(colorData))
	for idx, entry := range colorData {
		colorArr[idx] = rgbFromUint32(entry.Key)
		colorTable[rgbFromUint32(entry.Key)] = uint32(idx)
	}
	maxDepth := int(math.Log2(float64(len(colorArr))) + 1)
	kdTree := buildKDTree(colorArr, 0, maxDepth)

	for r := 0; r < 256; r++ {
		for g := 0; g < 256; g++ {
			for b := 0; b < 256; b++ {
				rgb := RGB{uint8(r), uint8(g), uint8(b)}
				closest, _ := kdTree.nearestNeighbor(
					rgb, kdTree.Color, math.MaxFloat64, 0)
				closestColorArr[r<<16|g<<8|b] = closest
			}
		}
	}
	return ComputedTables{
		ColorArr:        &colorArr,
		ClosestColorArr: &closestColorArr,
		ColorTable:      &colorTable,
		KdTree:          kdTree,
	}
}

type ColorTableEntry struct {
	Color RGB
	Index uint32
}

type CompactTablePair struct {
	Fg CompactComputedTables
	Bg CompactComputedTables
}

type ColorMethodCompactTables map[ColorDistanceMethod]CompactTablePair

type CompactComputedTables struct {
	ColorArr        []RGB
	AnsiData        AnsiData
	ClosestColorIdx []byte // Use 1 byte per color instead of full RGB
	ColorTable      []ColorTableEntry
	KDTreeData      []byte // Serialized KD-tree data
}

func CompactComputeTables(colorData AnsiData) CompactComputedTables {
	tables := ComputeTables(colorData)
	colorTable := make([]ColorTableEntry, len(colorData))
	closestColorArr := make([]uint8, 256*256*256)

	for idx, entry := range colorData {
		rgb := rgbFromUint32(entry.Key)
		colorTable[idx] = ColorTableEntry{Color: rgb, Index: uint32(idx)}
	}

	for i, color := range *tables.ClosestColorArr {
		closestColorIdx := findColorIndex(colorTable, color)
		closestColorArr[i] = uint8(closestColorIdx)
	}

	// Serialize KD-tree
	kdTreeData := tables.KdTree.Serialize()

	return CompactComputedTables{
		ColorArr:        *tables.ColorArr,
		AnsiData:        colorData,
		ClosestColorIdx: closestColorArr,
		ColorTable:      colorTable,
		KDTreeData:      kdTreeData,
	}
}

func findColorIndex(colorTable []ColorTableEntry, color RGB) uint32 {
	for _, entry := range colorTable {
		if entry.Color == color {
			return entry.Index
		}
	}
	return 0 // or handle error
}

func LoadPaletteAsCompactTables(path string) (CompactComputedTables,
	CompactComputedTables, error) {
	fgData, bgData, err := ReadAnsiDataFromJSON(path)
	if err != nil {
		return CompactComputedTables{}, CompactComputedTables{}, err
	}
	fgAnsi = fgData.ToOrderedMap()
	fgComputedTable := CompactComputeTables(fgData)
	fgComputedTable.AnsiData = fgData

	var bgComputedTable CompactComputedTables
	if !PaletteSame(fgData, bgData) {
		bgAnsi = bgData.ToOrderedMap()
		bgComputedTable = CompactComputeTables(bgData)
	} else {
		bgComputedTable = CompactComputedTables{
			AnsiData: bgData,
		}
	}
	return fgComputedTable, bgComputedTable, nil
}

func PaletteSame(fgData AnsiData, bgData AnsiData) bool {
	// Check if every color in bgData is also in fgData
	fgBgSame := true

	// Check if the sizes are the same
	if len(fgData) != len(fgData) {
		fgBgSame = false
	} else {
		fgExist := make(map[uint32]bool)
		for _, entry := range fgData {
			fgExist[entry.Key] = true
		}
		for _, entry := range bgData {
			if _, ok := fgExist[entry.Key]; !ok {
				fgBgSame = false
				break
			}
		}
	}
	return fgBgSame
}

func (cct CompactComputedTables) Restore() ComputedTables {
	closestColorArr := make([]RGB, 256*256*256)

	for i, idx := range cct.ClosestColorIdx {
		closestColorArr[i] = cct.ColorTable[idx].Color
	}
	colorTable := make(map[RGB]uint32, len(cct.ColorTable))
	for _, entry := range cct.ColorTable {
		colorTable[entry.Color] = entry.Index
	}

	var kdTree *ColorNode
	if cct.KDTreeData != nil {
		kdTree, _ = DeserializeKDTree(cct.KDTreeData)
	}

	return ComputedTables{
		ColorArr:        &cct.ColorArr,
		AnsiData:        cct.AnsiData,
		ClosestColorArr: &closestColorArr,
		ColorTable:      &colorTable,
		KdTree:          kdTree,
	}
}

func LoadPalette(path string) (*ComputedTables, *ComputedTables, error) {
	if strings.HasSuffix(path, ".palette") {
		return LoadPaletteBinary(path)
	} else if strings.HasSuffix(path, ".json") {
		return LoadPaletteJSON(path)
	} else {
		return LoadPaletteBinary(path)
	}
}

func LoadPaletteBinary(path string) (fg, bg *ComputedTables, err error) {
	// First, try the VFS.
	template := "colordata/%s.palette"
	data, vfsErr := f.ReadFile(fmt.Sprintf(template, path))
	if vfsErr != nil {
		var fsErr error
		data, fsErr = ioutil.ReadFile(path)
		if fsErr != nil {
			return nil, nil, fmt.Errorf("failed to read file: %v", fsErr)
		}
	}
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gzr.Close()

	var cmct ColorMethodCompactTables
	dec := gob.NewDecoder(gzr)
	if err := dec.Decode(&cmct); err != nil {
		return nil, nil, fmt.Errorf("failed to decode palette data: %v", err)
	}
	cct := cmct[CurrentColorDistanceMethod]

	fgTables := cct.Fg.Restore()
	fgAnsi = fgTables.AnsiData.ToOrderedMap()
	bgTables := cct.Bg.Restore()
	bgAnsi = bgTables.AnsiData.ToOrderedMap()

	fgClosestColor = fgTables.ClosestColorArr
	fgColorTable = *fgTables.ColorTable
	fgTree = fgTables.KdTree
	fgColors = *fgTables.ColorArr

	bgAnsiData := bgTables.AnsiData
	if len(*bgTables.ColorTable) == 0 {
		bgTables = fgTables
		bgTables.AnsiData = bgAnsiData
		bgClosestColor = fgClosestColor
		bgColorTable = fgColorTable
		bgTree = fgTree
		bgColors = fgColors
	} else {
		bgClosestColor = bgTables.ClosestColorArr
		bgColorTable = *bgTables.ColorTable
		bgTree = bgTables.KdTree
		bgColors = *bgTables.ColorArr
	}

	fgAnsi = fgTables.AnsiData.ToOrderedMap()
	bgAnsi = bgTables.AnsiData.ToOrderedMap()
	lookupTable = make(map[Uint256]lookupEntry)
	ComputeDistinctColors()

	return &fgTables, &bgTables, nil
}

func ComputeDistinctColors() {
	// Count the number of distinct colors
	seen := make(map[RGB]bool)
	fgAnsi.Iterate(
		func(key, _ interface{}) {
			seen[rgbFromUint32(key.(uint32))] = true
		})
	bgAnsi.Iterate(
		func(key, _ interface{}) {
			seen[rgbFromUint32(key.(uint32))] = true
		})
	DistinctColors = len(seen)
}

func LoadPaletteJSON(path string) (*ComputedTables, *ComputedTables, error) {
	fgData, bgData, err := ReadAnsiDataFromJSON(path)
	if err != nil {
		return nil, nil, err
	}
	fgAnsi = fgData.ToOrderedMap()
	fgComputedTable := ComputeTables(fgData)
	fgComputedTable.AnsiData = fgData
	fgClosestColor = fgComputedTable.ClosestColorArr
	fgColorTable = *fgComputedTable.ColorTable
	fgTree = fgComputedTable.KdTree
	fgColors = *fgComputedTable.ColorArr

	var bgComputedTable ComputedTables
	bgAnsi = bgData.ToOrderedMap()
	if !PaletteSame(fgData, bgData) {
		// If the foreground and background colors are different, load the
		// background colors separately
		bgComputedTable = ComputeTables(bgData)
		bgClosestColor = bgComputedTable.ClosestColorArr
		bgColorTable = *bgComputedTable.ColorTable
		bgTree = bgComputedTable.KdTree
		bgColors = *bgComputedTable.ColorArr
	} else {
		// If the foreground and background colors are the same, use the
		// same tables for both
		bgClosestColor = fgClosestColor
		bgColorTable = fgColorTable
		bgTree = fgTree
		bgColors = fgColors
		bgComputedTable = fgComputedTable
	}
	bgComputedTable.AnsiData = bgData

	lookupTable = make(map[Uint256]lookupEntry)
	ComputeDistinctColors()

	return &fgComputedTable, &bgComputedTable, nil
}

// LookupClosestColor finds the closest palette color for an RGB value.
// Returns the closest palette RGB and its ANSI 256 color number.
// LoadPalette must be called first.
func LookupClosestColor(r, g, b uint8) (closestRGB RGB, ansiCode int, ok bool) {
	if fgClosestColor == nil {
		return RGB{}, 0, false
	}
	idx := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	closest := (*fgClosestColor)[idx]
	codeStr, exists := fgAnsi.Get(closest.toUint32())
	if !exists {
		return closest, 0, false
	}
	// Parse "38;5;N" or "48;5;N" to extract N
	code := ParseANSICodeString(codeStr.(string))
	return closest, code, true
}

// ParseANSICodeString extracts the color number from an ANSI code string.
// E.g., "38;5;17" -> 17, "48;5;17" -> 17
func ParseANSICodeString(code string) int {
	var n int
	if _, err := fmt.Sscanf(code, "38;5;%d", &n); err == nil {
		return n
	}
	if _, err := fmt.Sscanf(code, "48;5;%d", &n); err == nil {
		return n
	}
	return 0
}

// GetPaletteColors returns all colors in the loaded foreground palette.
// LoadPalette must be called first.
func GetPaletteColors() []RGB {
	if fgColors == nil {
		return nil
	}
	return fgColors
}

// GetPaletteSize returns the number of colors in the loaded palette.
func GetPaletteSize() int {
	if fgColors == nil {
		return 0
	}
	return len(fgColors)
}

// GetANSICode returns the ANSI 256 color number for a palette RGB color.
// Returns 0 if the color is not in the palette.
func GetANSICode(rgb RGB) int {
	codeStr, exists := fgAnsi.Get(rgb.toUint32())
	if !exists {
		return 0
	}
	return ParseANSICodeString(codeStr.(string))
}
