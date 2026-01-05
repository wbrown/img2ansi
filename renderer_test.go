package img2ansi

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/wbrown/img2ansi/imageutil"
)

func TestRendererPaletteCaching(t *testing.T) {
	t.Parallel()

	// Create renderer with palette
	r := NewRenderer(
		WithColorMethod(LABMethod{}),
		WithPalette("colordata/ansi16.json"),
	)

	// Verify palette is loaded
	if !r.paletteLoaded {
		t.Fatal("Palette should be loaded")
	}
	if r.palettePath != "colordata/ansi16.json" {
		t.Errorf("Expected palettePath='colordata/ansi16.json', got '%s'", r.palettePath)
	}

	// Verify internal state is populated
	if r.fgAnsi == nil || r.fgAnsi.Len() == 0 {
		t.Error("fgAnsi should be populated")
	}
	if r.bgAnsi == nil || r.bgAnsi.Len() == 0 {
		t.Error("bgAnsi should be populated")
	}
	if r.fgClosestColor == nil || len(*r.fgClosestColor) == 0 {
		t.Error("fgClosestColor should be populated")
	}
	if r.bgClosestColor == nil || len(*r.bgClosestColor) == 0 {
		t.Error("bgClosestColor should be populated")
	}
	if r.fgTree == nil {
		t.Error("fgTree should be populated")
	}
	if r.bgTree == nil {
		t.Error("bgTree should be populated")
	}
	if len(r.fgColors) == 0 {
		t.Error("fgColors should be populated")
	}
	if len(r.bgColors) == 0 {
		t.Error("bgColors should be populated")
	}

	// Load same palette again - should be cached (no-op)
	initialCache := r.lookupTable
	err := r.LoadPalette("colordata/ansi16.json")
	if err != nil {
		t.Errorf("Reloading same palette failed: %v", err)
	}

	// Cache should be preserved when reloading same palette
	if !reflect.DeepEqual(r.lookupTable, initialCache) {
		t.Error("Cache should be preserved when reloading same palette")
	}
}

func TestRendererCachePreservation(t *testing.T) {
	t.Parallel()

	r := NewRenderer(
		WithColorMethod(RedmeanMethod{}),
		WithPalette("colordata/ansi256.json"),
	)

	// Run some operations to populate cache
	testBlock := [4]RGB{
		{128, 64, 32},
		{140, 70, 35},
		{150, 75, 38},
		{160, 80, 40},
	}
	r.FindBestBlockRepresentation(testBlock, false)

	initialHits, initialMisses, _ := r.CacheStats()

	// Reload same palette
	err := r.LoadPalette("colordata/ansi256.json")
	if err != nil {
		t.Fatalf("Failed to reload palette: %v", err)
	}

	// Stats should be preserved
	hits, misses, _ := r.CacheStats()
	if hits != initialHits || misses != initialMisses {
		t.Errorf("Cache stats changed after reload: was (%d, %d), now (%d, %d)",
			initialHits, initialMisses, hits, misses)
	}

	// Load different palette - cache should be cleared
	err = r.LoadPalette("colordata/ansi16.json")
	if err != nil {
		t.Fatalf("Failed to load different palette: %v", err)
	}

	hits, misses, _ = r.CacheStats()
	if hits != 0 || misses != 0 {
		t.Errorf("Cache should be cleared when loading different palette, got hits=%d misses=%d", hits, misses)
	}
}

func TestRendererConcurrent(t *testing.T) {
	t.Parallel()

	// Create multiple renderers concurrently
	var wg sync.WaitGroup
	renderers := make([]*Renderer, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			renderers[idx] = NewRenderer(
				WithColorMethod(LABMethod{}),
				WithPalette("colordata/ansi16.json"),
			)
		}(i)
	}
	wg.Wait()

	// All renderers should be properly initialized
	for i, r := range renderers {
		if !r.paletteLoaded {
			t.Errorf("Renderer %d: palette not loaded", i)
		}
		if r.fgAnsi == nil || r.fgAnsi.Len() == 0 {
			t.Errorf("Renderer %d: fgAnsi not populated", i)
		}
	}

	// Use all renderers concurrently
	testBlock := [4]RGB{
		{100, 50, 25},
		{110, 55, 28},
		{120, 60, 30},
		{130, 65, 33},
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _, _ = renderers[idx].FindBestBlockRepresentation(testBlock, false)
		}(i)
	}
	wg.Wait()

	// All should have cache entries
	for i, r := range renderers {
		hits, misses, _ := r.CacheStats()
		total := hits + misses
		if total == 0 {
			t.Errorf("Renderer %d: no cache activity", i)
		}
	}
}

func TestRendererOptions(t *testing.T) {
	t.Parallel()

	// Test all options
	r := NewRenderer(
		WithTargetWidth(120),
		WithScaleFactor(1.5),
		WithMaxChars(500000),
		WithKdSearch(25),
		WithCacheThreshold(60.0),
		WithColorMethod(RGBMethod{}),
		WithPalette("colordata/jetbrains32.json"),
	)

	if r.TargetWidth != 120 {
		t.Errorf("Expected TargetWidth=120, got %d", r.TargetWidth)
	}
	if r.ScaleFactor != 1.5 {
		t.Errorf("Expected ScaleFactor=1.5, got %f", r.ScaleFactor)
	}
	if r.MaxChars != 500000 {
		t.Errorf("Expected MaxChars=500000, got %d", r.MaxChars)
	}
	if r.KdSearch != 25 {
		t.Errorf("Expected KdSearch=25, got %d", r.KdSearch)
	}
	if r.CacheThreshold != 60.0 {
		t.Errorf("Expected CacheThreshold=60.0, got %f", r.CacheThreshold)
	}
	if _, ok := r.ColorMethod.(RGBMethod); !ok {
		t.Errorf("Expected RGBMethod, got %T", r.ColorMethod)
	}
	if r.palettePath != "colordata/jetbrains32.json" {
		t.Errorf("Expected jetbrains32 palette, got %s", r.palettePath)
	}
}

func TestRendererStatsTracking(t *testing.T) {
	t.Parallel()

	r := NewRenderer(
		WithColorMethod(RedmeanMethod{}),
		WithPalette("colordata/ansi16.json"),
	)

	// Initial stats should be zero
	hits, misses, hitRate := r.CacheStats()
	if hits != 0 || misses != 0 || hitRate != 0 {
		t.Errorf("Initial stats should be zero, got hits=%d misses=%d rate=%f", hits, misses, hitRate)
	}

	// Run some operations
	testBlocks := [][4]RGB{
		{{128, 64, 32}, {140, 70, 35}, {150, 75, 38}, {160, 80, 40}},
		{{128, 64, 32}, {140, 70, 35}, {150, 75, 38}, {160, 80, 40}}, // Same block
		{{200, 100, 50}, {210, 105, 53}, {220, 110, 55}, {230, 115, 58}},
	}

	for _, block := range testBlocks {
		r.FindBestBlockRepresentation(block, false)
	}

	// Should have some cache activity
	hits, misses, hitRate = r.CacheStats()
	total := hits + misses
	if total == 0 {
		t.Error("Should have cache activity after operations")
	}

	if hitRate < 0 || hitRate > 1 {
		t.Errorf("Hit rate should be between 0 and 1, got %f", hitRate)
	}

	// Reset stats
	r.ResetStats()
	hits, misses, hitRate = r.CacheStats()
	if hits != 0 || misses != 0 || hitRate != 0 {
		t.Errorf("Stats should be zero after reset, got hits=%d misses=%d rate=%f", hits, misses, hitRate)
	}

	// BestBlockTime should also be reset
	if r.GetBestBlockTime() != 0 {
		t.Errorf("BestBlockTime should be zero after reset, got %v", r.GetBestBlockTime())
	}
}

func TestRendererPaletteState(t *testing.T) {
	t.Parallel()

	// Test that Renderer's palette state matches what we'd get from direct loading
	r := NewRenderer(
		WithColorMethod(LABMethod{}),
		WithPalette("colordata/ansi256.json"),
	)

	// Load same palette directly and convert to OrderedMap (deduplicates)
	fgData, _, err := ReadAnsiDataFromJSON("colordata/ansi256.json")
	if err != nil {
		t.Fatalf("Failed to load palette directly: %v", err)
	}

	// Convert to OrderedMap to get same deduplication as Renderer
	expectedFgMap := fgData.ToOrderedMap()
	expectedFgData := expectedFgMap.ToAnsiData()

	// Compare Renderer's AnsiData with deduplicated data - ORDER MUST MATCH
	rendererFgData := r.fgAnsi.ToAnsiData()
	if len(rendererFgData) != len(expectedFgData) {
		t.Fatalf("Renderer fgData length mismatch: got %d, expected %d", len(rendererFgData), len(expectedFgData))
	}

	// Check that order is exactly preserved
	for i := 0; i < len(expectedFgData); i++ {
		if rendererFgData[i].Key != expectedFgData[i].Key || rendererFgData[i].Value != expectedFgData[i].Value {
			t.Errorf("fgData entry %d ORDER MISMATCH: renderer has (%d, %s), expected (%d, %s)",
				i, rendererFgData[i].Key, rendererFgData[i].Value, expectedFgData[i].Key, expectedFgData[i].Value)
			if i > 10 {
				t.Fatal("Too many order mismatches, stopping")
			}
		}
	}

	// Verify ColorArr length matches raw palette size (not deduplicated)
	// fgColors is used for KD-tree depth calculations and comes from raw AnsiData
	if len(r.fgColors) != len(fgData) {
		t.Errorf("fgColors length mismatch: got %d, expected %d (raw palette size)", len(r.fgColors), len(fgData))
	}

	// Verify reverse map has all ANSI codes from raw palette (for parsing support)
	if len(r.fgAnsiRev) != len(fgData) {
		t.Errorf("fgAnsiRev length mismatch: got %d, expected %d (all ANSI codes)", len(r.fgAnsiRev), len(fgData))
	}

	// Verify ClosestColorArr is fully populated (256^3 entries)
	expectedSize := 256 * 256 * 256
	if len(*r.fgClosestColor) != expectedSize {
		t.Errorf("fgClosestColor size mismatch: got %d, expected %d", len(*r.fgClosestColor), expectedSize)
	}
}

// CustomTestMethod is a custom ColorDistanceMethod for testing
type CustomTestMethod struct{}

func (m CustomTestMethod) Distance(c1, c2 RGB) float64 {
	// Simple Manhattan distance for testing
	return float64(abs(int(c1.R)-int(c2.R)) +
		abs(int(c1.G)-int(c2.G)) +
		abs(int(c1.B)-int(c2.B)))
}

func (m CustomTestMethod) Name() string {
	return "CustomTest"
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func TestRendererCustomColorMethod(t *testing.T) {
	t.Parallel()

	// Create renderer with custom color method
	// This should use the fast path (no 16.7M table computation)
	r := NewRenderer(
		WithColorMethod(CustomTestMethod{}),
		WithPalette("ansi16"),
	)

	// Verify palette is loaded
	if !r.paletteLoaded {
		t.Fatal("Palette should be loaded")
	}

	// Verify we're using fast mode - ClosestColorArr should be nil
	if r.fgClosestColor != nil {
		t.Error("fgClosestColor should be nil for custom ColorDistanceMethod (fast mode)")
	}
	if r.bgClosestColor != nil {
		t.Error("bgClosestColor should be nil for custom ColorDistanceMethod (fast mode)")
	}

	// But KD-tree should be populated for runtime lookups
	if r.fgTree == nil {
		t.Error("fgTree should be populated for KD-tree lookups")
	}
	if r.bgTree == nil {
		t.Error("bgTree should be populated for KD-tree lookups")
	}

	// Verify the custom method is being used
	if _, ok := r.ColorMethod.(CustomTestMethod); !ok {
		t.Errorf("Expected CustomTestMethod, got %T", r.ColorMethod)
	}

	// Test that FindBestBlockRepresentation works with runtime KD-tree lookups
	testBlock := [4]RGB{
		{128, 64, 32},
		{140, 70, 35},
		{150, 75, 38},
		{160, 80, 40},
	}
	rune, fg, bg := r.FindBestBlockRepresentation(testBlock, false)

	// Verify we got a valid result
	if rune == 0 {
		t.Error("Expected non-zero rune from FindBestBlockRepresentation")
	}
	if fg == (RGB{}) && bg == (RGB{}) {
		t.Error("Expected non-zero colors from FindBestBlockRepresentation")
	}

	// Verify cache is working
	hits, misses, _ := r.CacheStats()
	if hits+misses == 0 {
		t.Error("Expected cache activity after FindBestBlockRepresentation")
	}
}

func TestRendererCacheErrorDistribution(t *testing.T) {
	// Load the mandrill test image
	img, err := imageutil.LoadImage("testdata/mandrill.tiff")
	if err != nil {
		t.Fatalf("Failed to load mandrill.tiff: %v", err)
	}

	r := NewRenderer(
		WithColorMethod(RGBMethod{}),
		WithPalette("ansi256"),
		WithKdSearch(50),
		WithCacheThreshold(40.0),
	)

	width, height := 80, 40
	resized, edges := imageutil.PrepareForANSI(img, width, height)
	_ = r.BrownDitherForBlocks(resized, edges)

	// Analyze cache entries
	var totalEntries, totalMatches int
	var errorSum float64
	errorBuckets := make(map[int]int) // bucket by error/10

	for _, entry := range r.lookupTable {
		totalEntries++
		for _, match := range entry.Matches {
			totalMatches++
			errorSum += match.Error
			bucket := int(match.Error / 10)
			errorBuckets[bucket]++
		}
	}

	t.Logf("Cache entries: %d, Total matches: %d", totalEntries, totalMatches)
	t.Logf("Average error: %.2f", errorSum/float64(totalMatches))
	t.Logf("Error distribution (bucket size=10):")
	for i := 0; i <= 20; i++ {
		if count := errorBuckets[i]; count > 0 {
			t.Logf("  %3d-%3d: %d blocks", i*10, (i+1)*10-1, count)
		}
	}

	hits, misses, rate := r.CacheStats()
	t.Logf("Within-image cache: hits=%d, misses=%d, rate=%.2f%%", hits, misses, rate*100)

	// Count how many keys have multiple matches (potential for approximate hits)
	keysWithMultiple := 0
	potentialApproxHits := 0
	for _, entry := range r.lookupTable {
		if len(entry.Matches) > 1 {
			keysWithMultiple++
			potentialApproxHits += len(entry.Matches) - 1
		}
	}
	t.Logf("Keys with multiple blocks: %d, potential approx hits: %d", keysWithMultiple, potentialApproxHits)
}

func TestRendererCachingWithMandrill(t *testing.T) {
	// Load the mandrill test image
	img, err := imageutil.LoadImage("testdata/mandrill.tiff")
	if err != nil {
		t.Fatalf("Failed to load mandrill.tiff: %v", err)
	}

	// Use default ansify settings: width=80, kdsearch=50, threshold=40, RGB method
	r := NewRenderer(
		WithColorMethod(RGBMethod{}),
		WithPalette("ansi256"),
		WithKdSearch(50),
		WithCacheThreshold(40.0),
	)

	width, height := 80, 40

	// First render - cache is cold
	resized1, edges1 := imageutil.PrepareForANSI(img, width, height)
	start1 := time.Now()
	blocks1 := r.BrownDitherForBlocks(resized1, edges1)
	duration1 := time.Since(start1)
	ansi1 := r.RenderToAnsi(blocks1)

	hits1, misses1, rate1 := r.CacheStats()
	t.Logf("First render:  %v, hits=%d, misses=%d, rate=%.2f%%",
		duration1, hits1, misses1, rate1*100)

	// Second render - cache should be warm (don't reset stats to see cumulative)
	resized2, edges2 := imageutil.PrepareForANSI(img, width, height)
	start2 := time.Now()
	blocks2 := r.BrownDitherForBlocks(resized2, edges2)
	duration2 := time.Since(start2)
	ansi2 := r.RenderToAnsi(blocks2)

	hits2, misses2, rate2 := r.CacheStats()
	// Calculate second render stats (cumulative - first)
	secondHits := hits2 - hits1
	secondMisses := misses2 - misses1
	t.Logf("Second render: %v, hits=%d, misses=%d, cumulative_rate=%.2f%%",
		duration2, secondHits, secondMisses, rate2*100)

	// Second render should have more hits than misses
	if secondHits == 0 {
		t.Errorf("Expected cache hits on second render, got hits=%d misses=%d", secondHits, secondMisses)
	}

	// Second render should be faster due to cache hits
	if duration2 >= duration1 {
		t.Errorf("Expected second render to be faster: first=%v, second=%v", duration1, duration2)
	}

	// Verify output is identical (deterministic)
	if ansi1 != ansi2 {
		t.Error("Expected identical output from both renders")
	}

	t.Logf("Speedup: %.2fx", float64(duration1)/float64(duration2))
}

// Benchmarks for profiling

// BenchmarkRenderMandrillAnsi256 benchmarks full render pipeline with 256-color palette.
// Uses precomputed tables (KdSearch=0, the default).
func BenchmarkRenderMandrillAnsi256(b *testing.B) {
	img, err := imageutil.LoadImage("testdata/mandrill.tiff")
	if err != nil {
		b.Fatalf("Failed to load mandrill.tiff: %v", err)
	}

	r := NewRenderer(WithPalette("ansi256"))
	width, height := 80, 40

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resized, edges := imageutil.PrepareForANSI(img, width, height)
		blocks := r.BrownDitherForBlocks(resized, edges)
		_ = r.CompressANSI(r.RenderToAnsi(blocks))
	}
}

// BenchmarkRenderMandrillAnsi16 benchmarks full render pipeline with 16-color palette.
func BenchmarkRenderMandrillAnsi16(b *testing.B) {
	img, err := imageutil.LoadImage("testdata/mandrill.tiff")
	if err != nil {
		b.Fatalf("Failed to load mandrill.tiff: %v", err)
	}

	r := NewRenderer(WithPalette("ansi16"))
	width, height := 80, 40

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resized, edges := imageutil.PrepareForANSI(img, width, height)
		blocks := r.BrownDitherForBlocks(resized, edges)
		_ = r.CompressANSI(r.RenderToAnsi(blocks))
	}
}

// BenchmarkRenderMandrillColdCache benchmarks with fresh cache each iteration.
func BenchmarkRenderMandrillColdCache(b *testing.B) {
	img, err := imageutil.LoadImage("testdata/mandrill.tiff")
	if err != nil {
		b.Fatalf("Failed to load mandrill.tiff: %v", err)
	}

	width, height := 80, 40

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// New renderer each time = cold cache
		r := NewRenderer(WithPalette("ansi256"))
		resized, edges := imageutil.PrepareForANSI(img, width, height)
		blocks := r.BrownDitherForBlocks(resized, edges)
		_ = r.CompressANSI(r.RenderToAnsi(blocks))
	}
}

// BenchmarkRenderMandrillWarmCache benchmarks with pre-warmed cache.
func BenchmarkRenderMandrillWarmCache(b *testing.B) {
	img, err := imageutil.LoadImage("testdata/mandrill.tiff")
	if err != nil {
		b.Fatalf("Failed to load mandrill.tiff: %v", err)
	}

	r := NewRenderer(WithPalette("ansi256"))
	width, height := 80, 40

	// Warm the cache
	resized, edges := imageutil.PrepareForANSI(img, width, height)
	blocks := r.BrownDitherForBlocks(resized, edges)
	_ = r.CompressANSI(r.RenderToAnsi(blocks))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resized, edges := imageutil.PrepareForANSI(img, width, height)
		blocks := r.BrownDitherForBlocks(resized, edges)
		_ = r.CompressANSI(r.RenderToAnsi(blocks))
	}
}

// BenchmarkRenderMandrillLarge benchmarks at larger output size (160x80).
func BenchmarkRenderMandrillLarge(b *testing.B) {
	img, err := imageutil.LoadImage("testdata/mandrill.tiff")
	if err != nil {
		b.Fatalf("Failed to load mandrill.tiff: %v", err)
	}

	r := NewRenderer(WithPalette("ansi256"))
	width, height := 160, 80

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resized, edges := imageutil.PrepareForANSI(img, width, height)
		blocks := r.BrownDitherForBlocks(resized, edges)
		_ = r.CompressANSI(r.RenderToAnsi(blocks))
	}
}

// BenchmarkBrownDitherOnly benchmarks just the dithering step (no image prep).
func BenchmarkBrownDitherOnly(b *testing.B) {
	img, err := imageutil.LoadImage("testdata/mandrill.tiff")
	if err != nil {
		b.Fatalf("Failed to load mandrill.tiff: %v", err)
	}

	r := NewRenderer(WithPalette("ansi256"))
	width, height := 80, 40

	// Prepare image once
	resized, edges := imageutil.PrepareForANSI(img, width, height)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset cache to measure dithering cost consistently
		r.lookupTable = make(ApproximateCache)
		r.lookupHits = 0
		r.lookupMisses = 0
		_ = r.BrownDitherForBlocks(resized, edges)
	}
}

// BenchmarkPrepareForANSI benchmarks just the image preparation step.
func BenchmarkPrepareForANSI(b *testing.B) {
	img, err := imageutil.LoadImage("testdata/mandrill.tiff")
	if err != nil {
		b.Fatalf("Failed to load mandrill.tiff: %v", err)
	}

	width, height := 80, 40

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = imageutil.PrepareForANSI(img, width, height)
	}
}

// BenchmarkRenderKdSearch benchmarks with runtime KD-tree search.
// This is slower than precomputed tables but supports custom ColorDistanceMethods.
func BenchmarkRenderKdSearch(b *testing.B) {
	img, err := imageutil.LoadImage("testdata/mandrill.tiff")
	if err != nil {
		b.Fatalf("Failed to load mandrill.tiff: %v", err)
	}

	r := NewRenderer(
		WithPalette("ansi256"),
		WithKdSearch(50),
	)
	width, height := 80, 40

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resized, edges := imageutil.PrepareForANSI(img, width, height)
		blocks := r.BrownDitherForBlocks(resized, edges)
		_ = r.CompressANSI(r.RenderToAnsi(blocks))
	}
}
