package base

import (
	"errors"
	"flag"
	"reflect"
	"strings"
	"testing"
)

// Dummy implementations for interfaces used in EnumSet
type DummyEnum int32

const (
	DummyA DummyEnum = iota
	DummyB
	DummyC
	DummyD
)

func (d DummyEnum) Ord() int32                { return int32(d) }
func (d DummyEnum) Mask() int32               { return 0xF }
func (d DummyEnum) String() string            { return [...]string{"A", "B", "C", "D"}[d] }
func (d DummyEnum) AutoComplete(AutoComplete) {}

func (d *DummyEnum) Set(s string) error {
	switch strings.ToUpper(s) {
	case "A":
		*d = DummyEnum(DummyA)
	case "B":
		*d = DummyEnum(DummyB)
	case "C":
		*d = DummyEnum(DummyC)
	case "D":
		*d = DummyEnum(DummyD)
	default:
		return errors.New("invalid")
	}
	return nil
}

// Satisfy flag.Value
func (d *DummyEnum) Get() interface{} { return *d }

func TestEnumBitMask(t *testing.T) {
	mask := EnumBitMask(DummyA, DummyC)
	expected := (int32(1) << DummyA) | (int32(1) << DummyC)
	if mask != expected {
		t.Errorf("EnumBitMask got %d, want %d", mask, expected)
	}
}

func TestEnumSetBasic(t *testing.T) {
	set := NewEnumSet(DummyA, DummyC)
	if !set.Has(DummyA) || !set.Has(DummyC) {
		t.Error("EnumSet.Has failed")
	}
	if set.Has(DummyB) {
		t.Error("EnumSet.Has false positive")
	}
	if set.Len() != 2 {
		t.Errorf("EnumSet.Len got %d, want 2", set.Len())
	}
}

func TestEnumSetAddRemove(t *testing.T) {
	set := NewEnumSet(DummyA)
	set.Add(DummyB)
	if !set.Has(DummyB) {
		t.Error("EnumSet.Add failed")
	}
	set.Remove(DummyA)
	if set.Has(DummyA) {
		t.Error("EnumSet.Remove failed")
	}
}

func TestEnumSetEqualsCompare(t *testing.T) {
	a := NewEnumSet(DummyA, DummyB)
	b := NewEnumSet(DummyA, DummyB)
	c := NewEnumSet(DummyC)
	if !a.Equals(b) {
		t.Error("EnumSet.Equals failed")
	}
	if a.Compare(c) != -1 {
		t.Error("EnumSet.Compare failed")
	}
}

func TestEnumSetIntersectAnyAll(t *testing.T) {
	a := NewEnumSet(DummyA, DummyB)
	b := NewEnumSet(DummyB, DummyC)
	if !a.Intersect(b).Has(DummyB) {
		t.Error("EnumSet.Intersect failed")
	}
	if !a.Any(DummyB, DummyC) {
		t.Error("EnumSet.Any failed")
	}
	if !a.All(DummyA, DummyB) {
		t.Error("EnumSet.All failed")
	}
}

func TestEnumSetSliceRange(t *testing.T) {
	set := NewEnumSet(DummyA, DummyC)
	slice := set.Slice()
	if len(slice) != 2 {
		t.Error("EnumSet.Slice length wrong")
	}
	var foundA, foundC bool
	set.Range(func(i int, it DummyEnum) error {
		if it == DummyA {
			foundA = true
		}
		if it == DummyC {
			foundC = true
		}
		return nil
	})
	if !foundA || !foundC {
		t.Error("EnumSet.Range failed")
	}
}

func TestEnumSetStringMarshal(t *testing.T) {
	type E = *DummyEnum
	set := NewEnumSet(DummyA, DummyC)
	s := set.String()
	if !strings.Contains(s, "A") || !strings.Contains(s, "C") {
		t.Error("EnumSet.String failed")
	}
	data, err := set.MarshalText()
	if err != nil || !strings.Contains(string(data), "A") {
		t.Error("EnumSet.MarshalText failed")
	}
	var set2 EnumSet[DummyEnum, E]
	if err := set2.UnmarshalText(data); err != nil {
		t.Error("EnumSet.UnmarshalText failed")
	}
	if !set2.Equals(set) {
		t.Error("EnumSet.UnmarshalText did not restore set")
	}
}

func TestEnumSetSetSelectTest(t *testing.T) {
	type E = *DummyEnum
	var set EnumSet[DummyEnum, E]
	if err := set.Set("A|C"); err != nil {
		t.Error("EnumSet.Set failed")
	}
	if !set.Test("A") || !set.Test("C") {
		t.Error("EnumSet.Test failed")
	}
	if err := set.Select("B", true); err != nil {
		t.Error("EnumSet.Select failed to add")
	}
	if !set.Has(DummyB) {
		t.Error("EnumSet.Select did not add")
	}
	if err := set.Select("A", false); err != nil {
		t.Error("EnumSet.Select failed to remove")
	}
	if set.Has(DummyA) {
		t.Error("EnumSet.Select did not remove")
	}
}

func TestEnumSetGreaterLess(t *testing.T) {
	set := NewEnumSet(DummyA, DummyB, DummyC, DummyD)
	gt := set.GreaterThan(DummyB)
	if gt.Has(DummyA) || gt.Has(DummyB) {
		t.Error("EnumSet.GreaterThan failed")
	}
	lt := set.LessThan(DummyC)
	if !lt.Has(DummyA) || !lt.Has(DummyB) || lt.Has(DummyC) {
		t.Error("EnumSet.LessThan failed")
	}
	gte := set.GreaterThanEqual(DummyB)
	if gte.Has(DummyA) || !gte.Has(DummyB) {
		t.Error("EnumSet.GreaterThanEqual failed")
	}
	lte := set.LessThanEqual(DummyB)
	if !lte.Has(DummyA) || !lte.Has(DummyB) || lte.Has(DummyC) {
		t.Error("EnumSet.LessThanEqual failed")
	}
}

func TestEnumSetClearAppendRemoveAllConcat(t *testing.T) {
	set := NewEnumSet(DummyA, DummyB)
	set.Clear()
	if !set.Empty() {
		t.Error("EnumSet.Clear failed")
	}
	set.Add(DummyA)
	set2 := NewEnumSet(DummyB)
	set.Append(set2)
	if !set.Has(DummyB) {
		t.Error("EnumSet.Append failed")
	}
	set.RemoveAll(set2)
	if set.Has(DummyB) {
		t.Error("EnumSet.RemoveAll failed")
	}
	set = set.Concat(DummyC)
	if !set.Has(DummyC) {
		t.Error("EnumSet.Concat failed")
	}
}

func TestEnumSetFromOrd(t *testing.T) {
	type E = *DummyEnum
	var set EnumSet[DummyEnum, E]
	set.FromOrd(3)
	if int32(set) != 3 {
		t.Error("EnumSet.FromOrd failed")
	}
}

func TestEnumSetFlagValueInterface(t *testing.T) {
	type E = *DummyEnum
	var _ flag.Value = new(EnumSet[DummyEnum, E])
}

func TestEnumSetRangeBreak(t *testing.T) {
	set := NewEnumSet(DummyA, DummyB, DummyC)
	count := 0
	set.Range(func(i int, it DummyEnum) error {
		count++
		if count == 2 {
			return errors.New("stop")
		}
		return nil
	})
	if count != 2 {
		t.Error("EnumSet.Range did not break early")
	}
}

func TestEnumSetMarshalUnmarshalText(t *testing.T) {
	type E = *DummyEnum
	set := NewEnumSet(DummyA, DummyB)
	data, err := set.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	var set2 EnumSet[DummyEnum, E]
	if err := set2.UnmarshalText(data); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(set, set2) {
		t.Error("Marshal/UnmarshalText mismatch")
	}
}
