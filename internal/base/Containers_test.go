package base

import (
	"slices"
	"sort"
	"testing"
)

func TestIndexOfInts(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	if i, ok := IndexOf(1, items...); !ok || i != 0 {
		t.Errorf("invalid indexof: %v != %v || %v != %v", ok, true, i, 0)
	}
	if i, ok := IndexOf(5, items...); !ok || i != len(items)-1 {
		t.Errorf("invalid indexof: %v != %v || %v != %v", ok, true, i, len(items)-1)
	}
	if _, ok := IndexOf(6, items...); ok {
		t.Errorf("invalid indexof: %v != %v", ok, false)
	}
}

func TestIndexIfInts(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	if i, ok := IndexIf(func(i int) bool {
		return (i & 1) == 0
	}, items...); !ok || i != 1 {
		t.Errorf("invalid indexof: %v != %v || %v != %v", ok, true, i, 1)
	}
	if i, ok := IndexIf(func(i int) bool {
		return i > 3
	}, items...); !ok || i != 3 {
		t.Errorf("invalid indexof: %v != %v || %v != %v", ok, true, i, 3)
	}
	if _, ok := IndexIf(func(i int) bool {
		return i < 0
	}, items...); ok {
		t.Errorf("invalid indexof: %v != %v", ok, false)
	}
}

func TestInsertAtInts(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	if new := InsertAt(items, 0, 0); !slices.Equal(new, []int{0, 1, 2, 3, 4, 5}) {
		t.FailNow()
	}
	if new := InsertAt(items, 4, 0); !slices.Equal(new, []int{1, 2, 3, 4, 0, 5}) {
		t.FailNow()
	}
	if new := InsertAt(items, 5, 0); !slices.Equal(new, []int{1, 2, 3, 4, 5, 0}) {
		t.FailNow()
	}
}

func TestContainsInts(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	if !Contains(items, 1) {
		t.FailNow()
	}
	if !Contains(items, 5) {
		t.FailNow()
	}
	if !Contains(items, 3) {
		t.FailNow()
	}
	if Contains(items, 0) {
		t.FailNow()
	}
}

func TestAppendBoundedSortInts(t *testing.T) {
	less := func(i, j int) bool { return i < j }
	if new := AppendBoundedSort([]int{1, 2, 3, 4, 5}, 6, 0, less); !slices.Equal(new, []int{0, 1, 2, 3, 4, 5}) {
		t.FailNow()
	}
	if new := AppendBoundedSort([]int{1, 2, 3, 4, 5}, 5, 0, less); !slices.Equal(new, []int{0, 1, 2, 3, 4}) {
		t.FailNow()
	}
	if new := AppendBoundedSort([]int{1, 2, 3, 4, 5}, 5, 6, less); !slices.Equal(new, []int{1, 2, 3, 4, 5}) {
		t.FailNow()
	}
	if new := AppendBoundedSort([]int{1, 2, 3, 4, 5}, 5, 3, less); !slices.Equal(new, []int{1, 2, 3, 3, 4}) {
		t.FailNow()
	}
}

func TestAppendUniqInts(t *testing.T) {
	if new := AppendUniq([]int{1, 2, 3, 4, 5}, 6, 0); !slices.Equal(new, []int{1, 2, 3, 4, 5, 6, 0}) {
		t.FailNow()
	}
	if new := AppendUniq([]int{1, 2, 3, 4, 5}, 2, 3); !slices.Equal(new, []int{1, 2, 3, 4, 5}) {
		t.FailNow()
	}
	if new := AppendUniq([]int{1, 2, 3, 4, 5}, 5, 6); !slices.Equal(new, []int{1, 2, 3, 4, 5, 6}) {
		t.FailNow()
	}
	if new := AppendUniq([]int{1, 2, 3, 4, 5}, 0, 1, 2, 3); !slices.Equal(new, []int{1, 2, 3, 4, 5, 0}) {
		t.FailNow()
	}
}

func TestRemoveInts(t *testing.T) {
	if new := Remove([]int{1, 2, 3, 4, 5}, 6, 0); !slices.Equal(new, []int{1, 2, 3, 4, 5}) {
		t.FailNow()
	}
	if new := Remove([]int{1, 2, 3, 4, 5}, 2, 3); !slices.Equal(new, []int{1, 5, 4}) {
		t.FailNow()
	}
	if new := Remove([]int{1, 2, 3, 4, 5}, 5, 6); !slices.Equal(new, []int{1, 2, 3, 4}) {
		t.FailNow()
	}
	if new := Remove([]int{1, 2, 3, 4, 5}, 0, 1, 2, 3); !slices.Equal(new, []int{5, 4}) {
		t.FailNow()
	}
}

func TestRemoveUnlessInts(t *testing.T) {
	if new := RemoveUnless(func(i int) bool {
		return (i & 1) == 0
	}, 1, 2, 3, 4, 5); !slices.Equal(new, []int{2, 4}) {
		t.FailNow()
	}
	if new := RemoveUnless(func(i int) bool {
		return i > 3
	}, 1, 2, 3, 4, 5); !slices.Equal(new, []int{4, 5}) {
		t.FailNow()
	}
	if new := RemoveUnless(func(i int) bool {
		return i < 0
	}, 1, 2, 3, 4, 5); !slices.Equal(new, []int{}) {
		t.FailNow()
	}
	if new := RemoveUnless(func(i int) bool {
		return i > 0
	}, 1, 2, 3, 4, 5); !slices.Equal(new, []int{1, 2, 3, 4, 5}) {
		t.FailNow()
	}
}

func TestKeys(t *testing.T) {
	m := map[int]int{1: 0, 2: 1, 3: 2}
	k := Keys(m)
	sort.Slice(k, func(i, j int) bool {
		return k[i] < k[j]
	})
	if !slices.Equal(k, []int{1, 2, 3}) {
		t.FailNow()
	}
}

func TestInterset(t *testing.T) {
	if new := Intersect([]int{1, 2, 3, 4, 5}, []int{2, 3}); !slices.Equal(new, []int{2, 3}) {
		t.FailNow()
	}
	if new := Intersect([]int{1, 2, 3, 4, 5}, []int{1, 5}); !slices.Equal(new, []int{1, 5}) {
		t.FailNow()
	}
	if new := Intersect([]int{1, 2, 3, 4, 5}, []int{4, 2}); !slices.Equal(new, []int{4, 2}) {
		t.FailNow()
	}
	if new := Intersect([]int{1, 2, 3, 4, 5}, []int{0, 6, 8, 9, 10, 4, 7, 2, 11, 22, 33}); !slices.Equal(new, []int{2, 4}) {
		t.FailNow()
	}
	if new := Intersect([]int{1, 2, 3, 4, 5}, []int{}); !slices.Equal(new, []int{}) {
		t.FailNow()
	}
}

func TestIsUniq(t *testing.T) {
	if !IsUniq(1, 2, 3, 4, 5) {
		t.FailNow()
	}
	if IsUniq(1, 2, 3, 4, 5, 5) {
		t.FailNow()
	}
	if !IsUniq(0, 1, 2, 3, 4, 5) {
		t.FailNow()
	}
	if IsUniq(0, 3, 1, 2, 3, 4, 5) {
		t.FailNow()
	}
	if IsUniq(0, 1, 2, 3, 4, 5, 0) {
		t.FailNow()
	}
}
