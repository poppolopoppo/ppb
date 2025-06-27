package base

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestUnsafeBytesFromStringAndUnsafeStringFromBytes(t *testing.T) {
	s := "hello"
	b := UnsafeBytesFromString(s)
	if string(b) != s {
		t.Errorf("UnsafeBytesFromString failed: got %q, want %q", string(b), s)
	}
	s2 := UnsafeStringFromBytes(b)
	if s2 != s {
		t.Errorf("UnsafeStringFromBytes failed: got %q, want %q", s2, s)
	}
}

func TestUnsafeStringFromBuffer(t *testing.T) {
	buf := bytes.NewBufferString("buffer")
	s := UnsafeStringFromBuffer(buf)
	if s != "buffer" {
		t.Errorf("UnsafeStringFromBuffer failed: got %q", s)
	}
}

func TestStringerString(t *testing.T) {
	ss := StringerString{"abc"}
	if ss.String() != "abc" {
		t.Errorf("StringerString.String() failed: got %q", ss.String())
	}
}

func TestMakeStringer(t *testing.T) {
	s := MakeStringer(func() string { return "lambda" })
	if s.String() != "lambda" {
		t.Errorf("MakeStringer failed: got %q", s.String())
	}
}

type testStringer struct{ v string }

func (t testStringer) String() string { return t.v }

func TestJoinAndJoinString(t *testing.T) {
	a := testStringer{"a"}
	b := testStringer{"b"}
	c := testStringer{"c"}
	joined := Join(",", a, b, c)
	if joined.String() != "a,b,c" {
		t.Errorf("Join failed: got %q", joined.String())
	}
	joinedStr := JoinString("-", a, b, c)
	if joinedStr != "a-b-c" {
		t.Errorf("JoinString failed: got %q", joinedStr)
	}
}

func TestSanitizeIdentifier(t *testing.T) {
	in := "a!b@c#d$e"
	want := "a_b_c_d_e"
	got := SanitizeIdentifier(in)
	if got != want {
		t.Errorf("SanitizeIdentifier failed: got %q, want %q", got, want)
	}
}

func TestSplitWords(t *testing.T) {
	in := "foo bar\tbaz\nqux"
	want := []string{"foo", "bar", "baz", "qux"}
	got := SplitWords(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SplitWords failed: got %v, want %v", got, want)
	}
}

func TestMakeString(t *testing.T) {
	if MakeString("abc") != "abc" {
		t.Errorf("MakeString string failed")
	}
	if MakeString([]byte("def")) != "def" {
		t.Errorf("MakeString []byte failed")
	}
	s := StringerString{"ghi"}
	if MakeString(s) != "ghi" {
		t.Errorf("MakeString Stringer failed")
	}
	if !strings.HasPrefix(MakeString(123), "123") {
		t.Errorf("MakeString default failed")
	}
}

func TestFourCC(t *testing.T) {
	f := StringToFourCC("ABCD")
	if f.String() != "ABCD" {
		t.Errorf("FourCC.String() failed: got %q", f.String())
	}
	b := f.Bytes()
	if string(b[:]) != "ABCD" {
		t.Errorf("FourCC.Bytes() failed: got %q", string(b[:]))
	}
	if !f.Valid() {
		t.Errorf("FourCC.Valid() failed")
	}
}

func TestStringSetBasic(t *testing.T) {
	set := NewStringSet("a", "b", "c")
	if set.Len() != 3 {
		t.Errorf("StringSet.Len() failed")
	}
	if !set.Contains("a", "b") {
		t.Errorf("StringSet.Contains() failed")
	}
	if set.Any("x", "b") != true {
		t.Errorf("StringSet.Any() failed")
	}
	if set.IsUniq() != true {
		t.Errorf("StringSet.IsUniq() failed")
	}
	if idx, ok := set.IndexOf("b"); !ok || idx != 1 {
		t.Errorf("StringSet.IndexOf() failed")
	}
}

func TestStringSetAppendPrepend(t *testing.T) {
	set := NewStringSet("a")
	set.Append("b", "c")
	if !set.Contains("b", "c") {
		t.Errorf("StringSet.Append() failed")
	}
	set.Prepend("d")
	if set[0] != "d" {
		t.Errorf("StringSet.Prepend() failed")
	}
}

func TestStringSetAppendUniqPrependUniq(t *testing.T) {
	set := NewStringSet("a")
	set.AppendUniq("a", "b")
	if !set.Contains("a", "b") || set.Len() != 2 {
		t.Errorf("StringSet.AppendUniq() failed")
	}
	set.PrependUniq("b", "c")
	if !set.Contains("c") || set[0] != "c" {
		t.Errorf("StringSet.PrependUniq() failed")
	}
}

func TestStringSetRemoveDeleteClearAssign(t *testing.T) {
	set := NewStringSet("a", "b", "c", "b")
	set.RemoveAll("b")
	if set.Contains("b") {
		t.Errorf("StringSet.RemoveAll() failed")
	}
	set.Append("d")
	set.Remove("d")
	if set.Contains("d") {
		t.Errorf("StringSet.Remove() failed")
	}
	set.Append("e")
	set.Delete(0)
	if set[0] == "a" {
		t.Errorf("StringSet.Delete() failed")
	}
	set.Clear()
	if set.Len() != 0 {
		t.Errorf("StringSet.Clear() failed")
	}
	set.Assign([]string{"x", "y"})
	if !set.Contains("x", "y") {
		t.Errorf("StringSet.Assign() failed")
	}
}

func TestStringSetEqualsSortJoinString(t *testing.T) {
	set1 := NewStringSet("a", "b", "c")
	set2 := NewStringSet("a", "b", "c")
	if !set1.Equals(set2) {
		t.Errorf("StringSet.Equals() failed")
	}
	set2[2] = "d"
	if set1.Equals(set2) {
		t.Errorf("StringSet.Equals() should be false")
	}
	set1.Sort()
	set2.Sort()
	set1Str := set1.String()
	if !strings.Contains(set1Str, "a") {
		t.Errorf("StringSet.String() failed")
	}
	joined := set1.Join("-")
	if !strings.Contains(joined, "-") {
		t.Errorf("StringSet.Join() failed")
	}
}

func TestStringSetSet(t *testing.T) {
	var set StringSet
	set.Set("a, b, c")
	if !set.Contains("a", "b", "c") {
		t.Errorf("StringSet.Set() failed")
	}
}

func TestMakeStringerSet(t *testing.T) {
	a := testStringer{"x"}
	b := testStringer{"y"}
	set := MakeStringerSet(a, b)
	if !set.Contains("x", "y") {
		t.Errorf("MakeStringerSet failed")
	}
}
