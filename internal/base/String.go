package base

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unsafe"
)

/***************************************
 * Avoid allocation for string/[]byte conversions
 ***************************************/

func UnsafeBytesFromString(in string) []byte {
	return unsafe.Slice(unsafe.StringData(in), len(in))
}
func UnsafeStringFromBytes(raw []byte) string {
	// from func (strings.Builder) String() string
	return unsafe.String(unsafe.SliceData(raw), len(raw))
}
func UnsafeStringFromBuffer(buf *bytes.Buffer) string {
	// from func (strings.Builder) String() string
	return UnsafeStringFromBytes(buf.Bytes())
}

/***************************************
 * StringVariant implements fmt.Stringer
 ***************************************/

type StringerString struct {
	Value string
}

func (x StringerString) String() string {
	return x.Value
}

/***************************************
 * Create fmt.Stringer from a func
 ***************************************/

type lambdaStringer func() string

func (x lambdaStringer) String() string {
	return x()
}
func MakeStringer(fn func() string) fmt.Stringer {
	return lambdaStringer(fn)
}

/***************************************
 * Join fmt.Stringer lazily
 ***************************************/

type jointStringer[T fmt.Stringer] struct {
	it    []T
	delim string
}

func (join jointStringer[T]) String() string {
	var notFirst bool
	sb := strings.Builder{}
	for _, x := range join.it {
		if notFirst {
			sb.WriteString(join.delim)
		}
		sb.WriteString(x.String())
		notFirst = true
	}
	return sb.String()
}

func Join[T fmt.Stringer](delim string, it ...T) fmt.Stringer {
	return jointStringer[T]{delim: delim, it: it}
}
func JoinString[T fmt.Stringer](delim string, it ...T) string {
	return Join(delim, it...).String()
}

/***************************************
 * String helpers
 ***************************************/

var re_nonAlphaNumeric = regexp.MustCompile(`[^\w\d]+`)

func SanitizeIdentifier(in string) string {
	return re_nonAlphaNumeric.ReplaceAllString(in, "_")
}

var re_whiteSpace = regexp.MustCompile(`\s+`)

func SplitWords(in string) []string {
	return re_whiteSpace.Split(in, -1)
}

func SplitRegex(re *regexp.Regexp, capacity int) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if loc := re.FindIndex(data); loc != nil {
			return loc[1] + 1, data[loc[0]:loc[1]], nil
		}
		if atEOF {
			return 0, nil, io.EOF
		}
		if len(data) >= capacity {
			return len(data) - capacity, nil, nil
		}
		return 0, nil, nil
	}
}

func MakeString(x any) string {
	switch it := x.(type) {
	case string:
		return it
	case fmt.Stringer:
		return it.String()
	case []byte:
		return UnsafeStringFromBytes(it)
	default:
		return fmt.Sprint(x)
	}
}

/***************************************
 * FourCC
 ***************************************/

type FourCC uint32

func StringToFourCC(in string) FourCC {
	runes := ([]rune)(in)[:4]
	return MakeFourCC(runes[0], runes[1], runes[2], runes[3])
}
func BytesToFourCC(a, b, c, d byte) FourCC {
	return FourCC(uint32(a) | (uint32(b) << 8) | (uint32(c) << 16) | (uint32(d) << 24))
}
func MakeFourCC(a, b, c, d rune) FourCC {
	return BytesToFourCC(byte(a), byte(b), byte(c), byte(d))
}
func (x FourCC) Valid() bool { return x != 0 }
func (x FourCC) Bytes() (result [4]byte) {
	result[0] = byte((uint32(x) >> 0) & 0xFF)
	result[1] = byte((uint32(x) >> 8) & 0xFF)
	result[2] = byte((uint32(x) >> 16) & 0xFF)
	result[3] = byte((uint32(x) >> 24) & 0xFF)
	return
}
func (x FourCC) String() string {
	raw := x.Bytes()
	return UnsafeStringFromBytes(raw[:])
}
func (x *FourCC) Serialize(ar Archive) {
	var raw [4]byte
	if ar.Flags().IsLoading() {
		ar.Raw(raw[:])
		*x = BytesToFourCC(raw[0], raw[1], raw[2], raw[3])
	} else {
		raw = x.Bytes()
		ar.Raw(raw[:])
	}
}

/***************************************
 * String set
 ***************************************/

type StringSet SetT[string]

type StringSetable interface {
	StringSet() StringSet
}

func (set StringSet) Len() int {
	return len(set)
}
func (set StringSet) TotalContentLen() (total int) {
	for _, it := range set {
		total += len(it)
	}
	return
}
func (set StringSet) At(i int) string {
	return set[i]
}
func (set StringSet) Swap(i, j int) {
	set[i], set[j] = set[j], set[i]
}
func (set StringSet) Range(each func(string) error) error {
	for _, x := range set {
		if err := each(x); err != nil {
			return err
		}
	}
	return nil
}
func (set StringSet) Slice() []string {
	return set
}

func (set StringSet) IsUniq() bool {
	return IsUniq(set.Slice()...)
}

func (set StringSet) IndexOf(it string) (int, bool) {
	for i, x := range set {
		if x == it {
			return i, true
		}
	}
	return len(set), false
}

func (set StringSet) Any(it ...string) bool {
	for _, x := range it {
		if _, ok := set.IndexOf(x); ok {
			return true
		}
	}
	return false
}
func (set StringSet) Contains(it ...string) bool {
	for _, x := range it {
		if _, ok := set.IndexOf(x); !ok {
			return false
		}
	}
	return true
}
func (set *StringSet) Concat(other StringSet) StringSet {
	result := make(StringSet, set.Len(), set.Len()+other.Len())
	copy(result.Slice()[:set.Len()], set.Slice())
	result.Append(other.Slice()...)
	return result
}
func (set *StringSet) ConcatUniq(other StringSet) StringSet {
	result := make(StringSet, set.Len(), set.Len()+other.Len())
	copy(result.Slice()[:set.Len()], set.Slice())
	result.AppendUniq(other.Slice()...)
	return result
}
func (set *StringSet) Append(it ...string) *StringSet {
	Assert(func() bool {
		for _, x := range it {
			if len(x) <= 0 {
				return false
			}
		}
		return true
	})
	*set = AppendComparable_CheckUniq(*set, it...)
	return set
}
func (set *StringSet) Prepend(it ...string) *StringSet {
	Assert(func() bool {
		for _, x := range it {
			if len(x) <= 0 {
				return false
			}
		}
		return true
	})
	*set = PrependComparable_CheckUniq(*set, it...)
	return set
}
func (set *StringSet) AppendUniq(it ...string) *StringSet {
	for _, x := range it {
		if !set.Contains(x) {
			*set = append(*set, x)
		}
	}
	return set
}
func (set *StringSet) PrependUniq(it ...string) *StringSet {
	for _, x := range it {
		if !set.Contains(x) {
			*set = append([]string{x}, *set...)
		}
	}
	return set
}
func (set *StringSet) RemoveAll(x string) (found bool) {
	for i := 0; i < len(*set); {
		if (*set)[i] == x {
			set.Delete(i)
			found = true
		} else {
			i++
		}
	}
	return
}
func (set *StringSet) Remove(it ...string) (numRemoved int) {
	for _, x := range it {
		AssertNotIn(x, "")
		if i, ok := set.IndexOf(x); ok {
			set.Delete(i)
			numRemoved++
		}
	}
	return
}
func (set *StringSet) Delete(i int) *StringSet {
	*set = append((*set)[:i], (*set)[i+1:]...)
	return set
}
func (set *StringSet) Clear() *StringSet {
	*set = []string{}
	return set
}
func (set *StringSet) Assign(arr []string) *StringSet {
	*set = arr
	return set
}
func (set StringSet) Equals(other StringSet) bool {
	if len(set) != len(other) {
		return false
	}
	for i, x := range set {
		if other[i] != x {
			return false
		}
	}
	return true
}
func (set StringSet) Sort() {
	sort.Strings(set)
}
func (set StringSet) StringSet() StringSet {
	return set
}
func (set *StringSet) Serialize(ar Archive) {
	SerializeMany(ar, ar.String, (*[]string)(set))
}

func NewStringSet(x ...string) (result StringSet) {
	result = make(StringSet, len(x))
	copy(result, x)
	return
}

func MakeStringerSet[T fmt.Stringer](it ...T) (result StringSet) {
	if !enableDiagnostics {
		result = make(StringSet, len(it))
		for i, x := range it {
			result[i] = x.String()
		}
	} else {
		for _, x := range it {
			result.Append(x.String())
		}
	}
	return result
}

func (set StringSet) Join(sep string) string {
	return strings.Join(set.Slice(), sep)
}
func (set StringSet) String() string {
	return set.Join(",")
}
func (set *StringSet) Set(in string) error {
	set.Clear()
	for _, x := range strings.Split(in, ",") {
		set.Append(strings.TrimSpace(x))
	}
	return nil
}
