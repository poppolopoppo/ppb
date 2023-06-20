package utils

import (
	"flag"
	"fmt"
	"math/bits"
	"sort"
	"strings"
	"sync"
)

var LogContainers = NewLogCategory("Containers")

/***************************************
 * Container helpers
 ***************************************/

func CopySlice[T any](in ...T) []T {
	out := make([]T, len(in))
	copy(out, in)
	return out
}

func CopyMap[K comparable, V any](in map[K]V) map[K]V {
	out := make(map[K]V, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func Where[T any](pred func(T) bool, values ...T) (result T) {
	if i, ok := IndexIf(pred, values...); ok {
		result = values[i]
	}
	return result
}

func IndexOf[T comparable](match T, values ...T) (int, bool) {
	for i, x := range values {
		if x == match {
			return i, true
		}
	}
	return -1, false
}

func IndexIf[T any](pred func(T) bool, values ...T) (int, bool) {
	for i, x := range values {
		if pred(x) {
			return i, true
		}
	}
	return -1, false
}

func InsertAt[T any](arr []T, index int, value T) []T {
	if len(arr) == index {
		return append(arr, value)
	}
	arr = append(arr[:index+1], arr[index:]...)
	arr[index] = value
	return arr
}

func Contains[T comparable](arr []T, values ...T) bool {
	for _, x := range values {
		if _, ok := IndexOf(x, arr...); !ok {
			return false
		}
	}
	return true
}

func AppendUniq[T comparable](src []T, elts ...T) (result []T) {
	result = src
	for _, x := range elts {
		if _, ok := IndexOf(x, result...); !ok {
			result = append(result, x)
		}
	}
	return result
}

func Remove[T comparable](src []T, elts ...T) (result []T) {
	result = src
	for _, x := range elts {
		if i, ok := IndexOf(x, result...); !ok {
			result = append(result[:i], result[i+1:]...)
		}
	}
	return result
}

func RemoveUnless[T any](pred func(T) bool, src ...T) (result []T) {
	off := 0
	result = make([]T, len(src))
	for i, x := range src {
		if pred(x) {
			result[off] = src[i]
			off++
		}
	}
	return result[:off]
}

func Keys[K comparable, V any](elts ...map[K]V) []K {
	n := 0
	for _, it := range elts {
		n += len(it)
	}
	off := 0
	result := make([]K, n)
	for _, it := range elts {
		for key := range it {
			result[off] = key
			off++
		}
	}
	return result
}

func Intersect[T comparable](a, b []T) (result []T) {
	if len(a) > len(b) {
		a, b = b, a
	}
	for _, it := range a {
		if Contains(b, it) {
			result = append(result, it)
		}
	}
	return
}

func IsUniq[T comparable](arr ...T) bool {
	for i := 0; i < len(arr); i++ {
		for j := i + 1; j < len(arr); j++ {
			if arr[i] == arr[j] {
				return false
			}
		}
	}
	return true
}

/***************************************
 * Set (slice with unique items)
 ***************************************/

type SetT[T comparable] []T

func NewSet[T comparable](x ...T) (result SetT[T]) {
	result = make(SetT[T], len(x))
	copy(result, x)
	return
}

func (set SetT[T]) Empty() bool {
	return len(set) == 0
}
func (set SetT[T]) Len() int {
	return len(set)
}
func (set SetT[T]) At(i int) T {
	return set[i]
}
func (set SetT[T]) Swap(i, j int) {
	set[i], set[j] = set[j], set[i]
}
func (set SetT[T]) Range(each func(T)) {
	for _, x := range set {
		each(x)
	}
}
func (set *SetT[T]) Ref() *[]T {
	return (*[]T)(set)
}
func (set SetT[T]) Slice() []T {
	return set
}

func (set SetT[T]) IsUniq() bool {
	return IsUniq(set.Slice()...)
}

func (set SetT[T]) IndexOf(it T) (int, bool) {
	for i, x := range set {
		if x == it {
			return i, true
		}
	}
	return len(set), false
}

func (set SetT[T]) Contains(it ...T) bool {
	for _, x := range it {
		if _, ok := set.IndexOf(x); !ok {
			return false
		}
	}
	return true
}
func (set *SetT[T]) Append(it ...T) *SetT[T] {
	*set = AppendComparable_CheckUniq(*set, it...)
	return set
}
func (set *SetT[T]) Prepend(it ...T) *SetT[T] {
	*set = PrependComparable_CheckUniq(it, (*set)...)
	return set
}
func (set *SetT[T]) AppendUniq(it ...T) (modified bool) {
	for _, x := range it {
		if !set.Contains(x) {
			*set = append(*set, x)
			modified = true
		}
	}
	return
}
func (set *SetT[T]) PrependUniq(it ...T) (modified bool) {
	for _, x := range it {
		if !set.Contains(x) {
			*set = append([]T{x}, *set...)
			modified = true
		}
	}
	return
}
func (set *SetT[T]) Remove(x T) *SetT[T] {
	if i, ok := set.IndexOf(x); ok {
		set.Delete(i)
	} else {
		LogPanic(LogGlobal, "could not find item in set")
	}
	return set
}
func (set *SetT[T]) RemoveAll(it ...T) *SetT[T] {
	for _, x := range it {
		if i, ok := set.IndexOf(x); ok {
			set.Delete(i)
		}
	}
	return set
}
func (set *SetT[T]) RemoveUnless(pred func(T) bool) (result SetT[T]) {
	return SetT[T](RemoveUnless(pred, set.Slice()...))
}
func (set *SetT[T]) Delete(i int) *SetT[T] {
	*set = append((*set)[:i], (*set)[i+1:]...)
	return set
}
func (set *SetT[T]) Clear() *SetT[T] {
	*set = []T{}
	return set
}
func (set *SetT[T]) Assign(arr []T) *SetT[T] {
	*set = arr
	return set
}
func (set SetT[T]) Sort(less func(a, b T) bool) {
	sort.Slice(set, func(i, j int) bool {
		return less(set[i], set[j])
	})
}

/***************************************
 * Enum Flags
 ***************************************/

type EnumFlag interface {
	Ord() int32
	comparable
	fmt.Stringer
}

type EnumSet[T EnumFlag, E interface {
	*T
	FromOrd(int32)
	EnumFlag
	flag.Value
}] int32

func (x EnumSet[T, E]) Ord() int32  { return int32(x) }
func (x EnumSet[T, E]) Empty() bool { return x == 0 }
func (x EnumSet[T, E]) Has(it T) bool {
	return (x.Ord() & (1 << it.Ord())) != 0
}
func (x EnumSet[T, E]) Len() (total int) {
	return bits.OnesCount32(uint32(x.Ord()))
}
func (x EnumSet[T, E]) Elements() (result []T) {
	result = make([]T, x.Len())
	x.Range(func(i int, it T) error {
		result[i] = it
		return nil
	})
	return
}
func (x EnumSet[T, E]) Range(each func(int, T) error) error {
	var it T
	i := 0
	for mask := uint32(x.Ord()); mask != 0; i++ {
		bit := bits.Len32(mask) - 1

		E(&it).FromOrd(int32(bit))
		Assert(func() bool { return x.Has(it) })
		mask &= ^(uint32(1) << bit)

		if err := each(i, it); err != nil {
			return err
		}
	}
	return nil
}
func (x EnumSet[T, E]) Intersect(other EnumSet[T, E]) EnumSet[T, E] {
	return EnumSet[T, E](x.Ord() & other.Ord())
}
func (x *EnumSet[T, E]) Clear() {
	*x = EnumSet[T, E](0)
}
func (x *EnumSet[T, E]) Append(other EnumSet[T, E]) {
	*x = EnumSet[T, E](x.Ord() | other.Ord())
}
func (x *EnumSet[T, E]) Add(elements ...T) {
	for _, it := range elements {
		*x = EnumSet[T, E](x.Ord() | (1 << E(&it).Ord()))
	}
}
func (x *EnumSet[T, E]) Remove(elements ...T) {
	for _, it := range elements {
		*x = EnumSet[T, E](x.Ord() & ^(1 << E(&it).Ord()))
	}
}
func (x EnumSet[T, E]) Compare(other EnumSet[T, E]) int {
	if x.Ord() < other.Ord() {
		return -1
	} else if x.Ord() == other.Ord() {
		return 0
	} else {
		return 1
	}
}
func (x EnumSet[T, E]) String() string {
	sb := strings.Builder{}

	x.Range(func(i int, t T) error {
		if i > 0 {
			sb.WriteRune('|')
		}
		sb.WriteString(t.String())
		return nil
	})

	return sb.String()
}
func (x *EnumSet[T, E]) Set(in string) error {
	*x = EnumSet[T, E](0)
	for _, s := range strings.Split(in, "|") {
		var it T
		if err := E(&it).Set(strings.TrimSpace(s)); err == nil {
			x.Add(it)
		} else {
			return err
		}
	}
	return nil
}
func (x *EnumSet[T, E]) Serialize(ar Archive) {
	ar.Int32((*int32)(x))
}
func (x EnumSet[T, E]) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *EnumSet[T, E]) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}

func MakeEnumSet[T EnumFlag, E interface {
	*T
	FromOrd(int32)
	EnumFlag
	flag.Value
}](elements ...T) (result EnumSet[T, E]) {
	result.Add(elements...)
	return result
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
func (set StringSet) Range(each func(string)) {
	for _, x := range set {
		each(x)
	}
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
func (set *StringSet) Remove(it ...string) *StringSet {
	for _, x := range it {
		Assert(func() bool { return len(x) > 0 })
		if i, ok := set.IndexOf(x); ok {
			set.Delete(i)
		}
	}
	return set
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
		set.Append(x)
	}
	return nil
}

func (set StringSet) ToDirSet(root Directory) (result DirSet) {
	return Map(func(x string) Directory {
		return root.AbsoluteFolder(x).Normalize()
	}, set.Slice()...)
}
func (set StringSet) ToFileSet(root Directory) (result FileSet) {
	return Map(func(x string) Filename {
		return root.AbsoluteFile(x).Normalize()
	}, set.Slice()...)
}

/***************************************
 * Shared map
 ***************************************/

type SharedMapT[K comparable, V any] struct {
	intern sync.Map
}

func NewSharedMapT[K comparable, V any]() *SharedMapT[K, V] {
	return &SharedMapT[K, V]{sync.Map{}}
}
func (shared *SharedMapT[K, V]) Len() (count int) {
	shared.intern.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
func (shared *SharedMapT[K, V]) Clear() {
	shared.intern = sync.Map{}
}
func (shared *SharedMapT[K, V]) Keys() (result []K) {
	result = make([]K, 0, shared.Len())
	shared.intern.Range(func(k, _ interface{}) bool {
		result = append(result, k.(K))
		return true
	})
	return
}
func (shared *SharedMapT[K, V]) Values() (result []V) {
	result = make([]V, 0, shared.Len())
	shared.intern.Range(func(_, v interface{}) bool {
		result = append(result, v.(V))
		return true
	})
	return
}
func (shared *SharedMapT[K, V]) Range(each func(K, V)) {
	shared.intern.Range(func(k, v interface{}) bool {
		each(k.(K), v.(V))
		return true
	})
}
func (shared *SharedMapT[K, V]) Add(key K, value V) V {
	shared.intern.Store(key, value)
	return value
}
func (shared *SharedMapT[K, V]) FindOrAdd(key K, value V) (V, bool) {
	actual, loaded := shared.intern.LoadOrStore(key, value)
	return actual.(V), loaded
}
func (shared *SharedMapT[K, V]) Get(key K) (result V, ok bool) {
	if value, ok := shared.intern.Load(key); ok {
		return value.(V), true
	} else {
		return result, false
	}
}
func (shared *SharedMapT[K, V]) Delete(key K) {
	shared.intern.Delete(key)
}
func (shared *SharedMapT[K, V]) Pin() map[K]V {
	result := make(map[K]V, shared.Len())
	shared.Range(func(k K, v V) {
		result[k] = v
	})
	return result
}
func (shared *SharedMapT[K, V]) Make(fn func() (K, V)) V {
	k, v := fn()
	shared.Add(k, v)
	return v
}

/***************************************
 * Shared string map
 ***************************************/

func Djb2(str string) (hash uint32) {
	hash = 5381
	for i := 0; i < len(str); i++ {
		hash = ((hash << 5) + hash) + uint32(str[i])
	}
	return
}

// lower contentions using mutiple shards

type SharedStringMapT[V any] struct {
	shards []*SharedMapT[string, V]
}

func NewSharedStringMap[V any](numShards int) *SharedStringMapT[V] {
	shards := make([]*SharedMapT[string, V], numShards)
	for i := range shards {
		shards[i] = NewSharedMapT[string, V]()
	}
	return &SharedStringMapT[V]{shards: shards}
}

func (x *SharedStringMapT[V]) getShard(key string) *SharedMapT[string, V] {
	return x.shards[Djb2(key)%uint32(len(x.shards))]
}
func (x *SharedStringMapT[V]) Len() (count int) {
	for _, shard := range x.shards {
		count += shard.Len()
	}
	return
}
func (x *SharedStringMapT[V]) Clear() {
	for _, shard := range x.shards {
		shard.Clear()
	}
}
func (x *SharedStringMapT[V]) Keys() (result []string) {
	for _, shard := range x.shards {
		result = append(result, shard.Keys()...)
	}
	return
}
func (x *SharedStringMapT[V]) Values() (result []V) {
	for _, shard := range x.shards {
		result = append(result, shard.Values()...)
	}
	return
}
func (x *SharedStringMapT[V]) Range(each func(string, V)) {
	for _, shard := range x.shards {
		shard.Range(each)
	}
}
func (x *SharedStringMapT[V]) Add(key string, value V) V {
	return x.getShard(key).Add(key, value)
}
func (x *SharedStringMapT[V]) FindOrAdd(key string, value V) (V, bool) {
	return x.getShard(key).FindOrAdd(key, value)
}
func (x *SharedStringMapT[V]) Get(key string) (result V, ok bool) {
	return x.getShard(key).Get(key)
}
func (x *SharedStringMapT[V]) Delete(key string) {
	x.getShard(key).Delete(key)
}
