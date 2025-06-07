package base

import (
	"flag"
	"fmt"
	"math/bits"
	"strings"
)

/***************************************
 * Enum Flags
 ***************************************/

type EnumFlag interface {
	Ord() int32
	Mask() int32
	fmt.Stringer
	AutoCompletable
}

type EnumUnderlyingType interface {
	~int8 | ~int16 | ~int32 |
		~uint8 | ~uint16 | ~uint32
}

type EnumValue interface {
	comparable
	EnumUnderlyingType
	EnumFlag
}

func EnumBitMask[T EnumValue](values ...T) (mask int32) {
	for _, it := range values {
		mask |= int32(1) << int32(it)
	}
	return
}

/***************************************
 * Enum Sets
 ***************************************/

type EnumGettable interface {
	Ord() int32
	IsInheritable() bool
	Empty() bool
	Len() int
	Test(value string) bool
	fmt.Stringer
	AutoCompletable
}

type EnumSettable interface {
	FromOrd(v int32)
	Clear()
	Select(value string, enabled bool) error
	flag.Value
	Serializable
	EnumGettable
}

type EnumSet[T EnumValue, E interface {
	*T
	flag.Value
}] int32

func NewEnumSet[T EnumValue, E interface {
	*T
	flag.Value
}](list ...T) EnumSet[T, E] {
	return EnumSet[T, E](EnumBitMask(list...))
}

func (x EnumSet[T, E]) Ord() int32          { return int32(x) }
func (x *EnumSet[T, E]) FromOrd(v int32)    { *x = EnumSet[T, E](v) }
func (x EnumSet[T, E]) IsInheritable() bool { return x.Empty() }
func (x EnumSet[T, E]) Empty() bool         { return int32(x) == 0 }
func (x EnumSet[T, E]) Len() int            { return bits.OnesCount32(uint32(x)) }
func (x *EnumSet[T, E]) Clear()             { *x = EnumSet[T, E](0) }

func (x *EnumSet[T, E]) Serialize(ar Archive) {
	ar.Int32((*int32)(x))
}

func (x EnumSet[T, E]) Has(it T) bool {
	mask := (int32(1) << int32(it))
	return (int32(x) & mask) == mask
}
func (x EnumSet[T, E]) Intersect(other EnumSet[T, E]) EnumSet[T, E] {
	return EnumSet[T, E](int32(x) & int32(other))
}
func (x EnumSet[T, E]) Any(list ...T) bool {
	return !x.Intersect(NewEnumSet[T, E](list...)).Empty()
}
func (x EnumSet[T, E]) All(list ...T) bool {
	other := NewEnumSet[T, E](list...)
	return x.Intersect(other) == other
}

func (x EnumSet[T, E]) GreaterThan(pivot T) EnumSet[T, E] {
	return EnumSet[T, E](int32(x) & ^((int32(1) << (int32(pivot) + 1)) - int32(1)))
}
func (x EnumSet[T, E]) GreaterThanEqual(pivot T) EnumSet[T, E] {
	return EnumSet[T, E](int32(x) & ^((int32(1) << int32(pivot)) - int32(1)))
}
func (x EnumSet[T, E]) LessThan(pivot T) EnumSet[T, E] {
	return EnumSet[T, E](int32(x) & ((int32(1) << int32(pivot)) - int32(1)))
}
func (x EnumSet[T, E]) LessThanEqual(pivot T) EnumSet[T, E] {
	return EnumSet[T, E](int32(x) & ((int32(1) << (int32(pivot) + 1)) - int32(1)))
}

func (x EnumSet[T, E]) Slice() (result []T) {
	result = make([]T, x.Len())
	x.Range(func(i int, it T) error {
		result[i] = it
		return nil
	})
	return
}
func (x EnumSet[T, E]) Range(each func(int, T) error) error {
	for i := 0; x != 0; i++ {
		bit := bits.Len32(uint32(x)) - 1
		x = EnumSet[T, E](int32(x) & ^(int32(1) << int32(bit)))
		if err := each(i, T(bit)); err != nil {
			return err
		}
	}
	return nil
}
func (x *EnumSet[T, E]) Append(other EnumSet[T, E]) *EnumSet[T, E] {
	*x = EnumSet[T, E](int32(*x) | int32(other))
	return x
}
func (x *EnumSet[T, E]) Add(list ...T) *EnumSet[T, E] {
	x.Append(NewEnumSet[T, E](list...))
	return x
}
func (x *EnumSet[T, E]) RemoveAll(other EnumSet[T, E]) *EnumSet[T, E] {
	*x = EnumSet[T, E](int32(*x) & ^int32(other))
	return x
}
func (x *EnumSet[T, E]) Remove(list ...T) *EnumSet[T, E] {
	x.RemoveAll(NewEnumSet[T, E](list...))
	return x
}
func (x EnumSet[T, E]) Concat(list ...T) EnumSet[T, E] {
	x.Add(list...)
	return x
}
func (x EnumSet[T, E]) Equals(other EnumSet[T, E]) bool {
	return int32(x) == int32(other)
}
func (x EnumSet[T, E]) Compare(other EnumSet[T, E]) int {
	if int32(x) < int32(other) {
		return -1
	} else if int32(x) == int32(other) {
		return 0
	} else {
		return 1
	}
}

func (x *EnumSet[T, E]) Set(in string) error {
	*x = 0
	for _, s := range strings.Split(in, `|`) {
		var it T
		if err := E(&it).Set(strings.TrimSpace(s)); err == nil {
			x.Add(it)
		} else {
			return err
		}
	}
	return nil
}
func (x EnumSet[T, E]) Test(value string) bool {
	var parsed T
	if err := E(&parsed).Set(value); err != nil {
		return false
	}
	return x.Has(parsed)
}
func (x *EnumSet[T, E]) Select(value string, enabled bool) error {
	var parsed T
	if err := E(&parsed).Set(value); err != nil {
		return err
	}
	if enabled {
		x.Add(parsed)
	} else {
		x.Remove(parsed)
	}
	return nil
}

func (x EnumSet[T, E]) String() string {
	sb := strings.Builder{}

	x.Range(func(i int, it T) error {
		if i > 0 {
			sb.WriteRune('|')
		}
		sb.WriteString(it.String())
		return nil
	})

	return sb.String()
}
func (x EnumSet[T, E]) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *EnumSet[T, E]) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}

func (x EnumSet[T, E]) AutoComplete(in AutoComplete) {
	var defaultValue T
	mask := defaultValue.Mask()
	for i := int32(1); i <= mask; i++ {
		var set EnumSet[T, E]
		set.FromOrd(i)
		in.Add(set.String(), "")
	}
}
