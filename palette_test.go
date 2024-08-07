package img2ansi

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func CompareAnsiData(original, deserialized AnsiData) string {
	var differences strings.Builder

	if len(original) != len(deserialized) {
		differences.WriteString(fmt.Sprintf("Length mismatch: original %d, deserialized %d\n",
			len(original), len(deserialized)))
	}

	maxLen := len(original)
	if len(deserialized) > maxLen {
		maxLen = len(deserialized)
	}

	for i := 0; i < maxLen; i++ {
		if i >= len(original) {
			differences.WriteString(fmt.Sprintf("Extra entry in deserialized at index %d: %+v\n",
				i, deserialized[i]))
			continue
		}
		if i >= len(deserialized) {
			differences.WriteString(fmt.Sprintf("Missing entry in deserialized at index %d: %+v\n",
				i, original[i]))
			continue
		}

		if original[i].Key != deserialized[i].Key {
			differences.WriteString(fmt.Sprintf("Key mismatch at index %d: original %d, deserialized %d\n",
				i, original[i].Key, deserialized[i].Key))
		}
		if original[i].Value != deserialized[i].Value {
			differences.WriteString(fmt.Sprintf("Value mismatch at index %d: original %s, deserialized %s\n",
				i, original[i].Value, deserialized[i].Value))
		}
	}

	if differences.Len() == 0 {
		return "No differences found in AnsiData"
	}
	return differences.String()
}

func CompareClosestColorArr(original, deserialized *[]RGB) string {
	var differences strings.Builder

	if len(*original) != len(*deserialized) {
		differences.WriteString(fmt.Sprintf("Length mismatch: original %d, deserialized %d\n",
			len(*original), len(*deserialized)))
	}

	maxLen := len(*original)
	if len(*deserialized) > maxLen {
		maxLen = len(*deserialized)
	}

	mismatchCount := 0
	for i := 0; i < maxLen; i++ {
		if i >= len(*original) {
			differences.WriteString(fmt.Sprintf("Extra entry in deserialized at index %d: %+v\n",
				i, (*deserialized)[i]))
			continue
		}
		if i >= len(*deserialized) {
			differences.WriteString(fmt.Sprintf("Missing entry in deserialized at index %d: %+v\n",
				i, (*original)[i]))
			continue
		}

		if (*original)[i] != (*deserialized)[i] {
			mismatchCount++
			if mismatchCount <= 10 { // Limit the output to the first 10 mismatches
				differences.WriteString(fmt.Sprintf("Mismatch at index %d: original %+v, deserialized %+v\n",
					i, (*original)[i], (*deserialized)[i]))
			}
		}
	}

	if mismatchCount > 10 {
		differences.WriteString(fmt.Sprintf("... and %d more mismatches\n", mismatchCount-10))
	}

	if differences.Len() == 0 {
		return "No differences found in ClosestColorArr"
	}
	return differences.String()
}

func CompareComputedTables(original, deserialized *ComputedTables) error {
	// Compare AnsiData
	ansiDifferences := CompareAnsiData(original.AnsiData, deserialized.AnsiData)
	if ansiDifferences != "No differences found in AnsiData" {
		return fmt.Errorf("AnsiData mismatch:\n%s", ansiDifferences)
	}

	// Compare ColorArr
	if !reflect.DeepEqual(*original.ColorArr, *deserialized.ColorArr) {
		return fmt.Errorf("ColorArr mismatch")
	}

	// Compare ClosestColorArr
	closestColorDifferences := CompareClosestColorArr(original.ClosestColorArr, deserialized.ClosestColorArr)
	if closestColorDifferences != "No differences found in ClosestColorArr" {
		return fmt.Errorf("ClosestColorArr mismatch:\n%s", closestColorDifferences)
	}

	// Compare ColorTable
	if len(*original.ColorTable) != len(*deserialized.ColorTable) {
		return fmt.Errorf("ColorTable length mismatch: original %d, deserialized %d",
			len(*original.ColorTable), len(*deserialized.ColorTable))
	}
	for k, v := range *original.ColorTable {
		if dv, ok := (*deserialized.ColorTable)[k]; !ok || v != dv {
			return fmt.Errorf("ColorTable mismatch for key %v: original %d, deserialized %d", k, v, dv)
		}
	}

	// Compare KdTree
	if err := compareKdTrees(original.KdTree, deserialized.KdTree); err != nil {
		return fmt.Errorf("KdTree mismatch: %v", err)
	}

	return nil
}

func compareKdTrees(original, deserialized *ColorNode) error {
	if (original == nil) != (deserialized == nil) {
		return fmt.Errorf("One tree is nil, the other is not")
	}
	if original == nil {
		return nil
	}
	if original.Color != deserialized.Color {
		return fmt.Errorf("Color mismatch: original %v, deserialized %v", original.Color, deserialized.Color)
	}
	if original.SplitAxis != deserialized.SplitAxis {
		return fmt.Errorf("SplitAxis mismatch: original %d, deserialized %d", original.SplitAxis, deserialized.SplitAxis)
	}
	if err := compareKdTrees(original.Left, deserialized.Left); err != nil {
		return fmt.Errorf("Left subtree mismatch: %v", err)
	}
	if err := compareKdTrees(original.Right, deserialized.Right); err != nil {
		return fmt.Errorf("Right subtree mismatch: %v", err)
	}
	return nil
}

// Helper function to use in your tests
func CompareComputedTablesSerialization(original *ComputedTables) error {
	// Serialize
	compact := CompactComputeTables(original.AnsiData)

	// Deserialize
	deserialized := compact.Restore()

	// Compare
	return CompareComputedTables(original, &deserialized)
}

func TestSerialization(t *testing.T) {
	// Load your original palette
	original, _, err := LoadPalette("colordata/ansi256.json")
	if err != nil {
		t.Fatalf("Failed to load original palette: %v", err)
	}

	// Test serialization and deserialization
	err = CompareComputedTablesSerialization(original)
	if err != nil {
		t.Errorf("Serialization test failed: %v", err)
	}
}
