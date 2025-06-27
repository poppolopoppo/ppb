package base

import (
	"math"
	"math/rand"
	"sort"
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

func ReverseSlice[T any](in ...T) []T {
	for i, j := 0, len(in)-1; i < j; i, j = i+1, j-1 {
		in[i], in[j] = in[j], in[i]
	}
	return in
}

func CopyMap[K comparable, V any](in map[K]V) map[K]V {
	out := make(map[K]V, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
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

func AppendBoundedSort[T any](sorted []T, n int, newEntry T, less func(T, T) bool) []T {
	for i, it := range sorted {
		if less(newEntry, it) {
			if len(sorted) >= n {
				return InsertAt(sorted[:n-1], i, newEntry)
			} else {
				return InsertAt(sorted, i, newEntry)
			}
		}
	}

	if len(sorted) < n {
		return append(sorted, newEntry)
	}
	return sorted
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

func Delete[T any](src []T, index int) []T {
	return append(src[:index], src[index+1:]...)
}

func Delete_DontPreserveOrder[T any](src []T, index int) (result []T) {
	if swap := len(src) - 1; swap != index {
		src[index] = src[swap]
	}
	return src[:len(src)-1]
}

func Remove[T comparable](src []T, elts ...T) (result []T) {
	result = src
	numDeleteds := 0
	for i, it := range result {
		if _, ok := IndexOf(it, elts...); ok {
			result[i] = result[len(result)-1-numDeleteds]
			numDeleteds++
			if numDeleteds == len(elts) {
				break
			}
		}
	}
	return result[:len(result)-numDeleteds]
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
func (set SetT[T]) Range(each func(T) error) error {
	for _, x := range set {
		if err := each(x); err != nil {
			return err
		}
	}
	return nil
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
	*set = Delete(*set, i)
	return set
}
func (set *SetT[T]) Delete_DontPreserveOrder(i int) *SetT[T] {
	*set = Delete_DontPreserveOrder(*set, i)
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
 * Shared map
 ***************************************/

type SharedMapT[K any, V any] struct {
	intern sync.Map
}

func NewSharedMapT[K any, V any]() *SharedMapT[K, V] {
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
func (shared *SharedMapT[K, V]) Range(each func(K, V) error) (lastErr error) {
	shared.intern.Range(func(k, v interface{}) bool {
		if err := each(k.(K), v.(V)); err == nil {
			return true
		} else {
			lastErr = err
			return false
		}
	})

	return lastErr
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
func (shared *SharedMapT[K, V]) LoadAndDelete(key K) (V, bool) {
	if value, loaded := shared.intern.LoadAndDelete(key); loaded {
		return value.(V), true
	} else {
		var defaultValue V
		return defaultValue, false
	}
}
func (shared *SharedMapT[K, V]) Make(fn func() (K, V)) V {
	k, v := fn()
	shared.Add(k, v)
	return v
}

/***************************************
 * Shared string map
 ***************************************/

// state-less FNV1a hasher
func Fnv1a(s string, basis uint64) (h uint64) {
	const prime64 = 1099511628211
	h = basis
	/*
		This is an unrolled version of this algorithm:

		for _, c := range s {
			h = (h ^ uint64(c)) * prime64
		}

		It seems to be ~1.5x faster than the simple loop in BenchmarkHash64:

		- BenchmarkHash64/hash_function-4   30000000   56.1 ns/op   642.15 MB/s   0 B/op   0 allocs/op
		- BenchmarkHash64/hash_function-4   50000000   38.6 ns/op   932.35 MB/s   0 B/op   0 allocs/op

	*/
	for len(s) >= 8 {
		h = (h ^ uint64(s[0])) * prime64
		h = (h ^ uint64(s[1])) * prime64
		h = (h ^ uint64(s[2])) * prime64
		h = (h ^ uint64(s[3])) * prime64
		h = (h ^ uint64(s[4])) * prime64
		h = (h ^ uint64(s[5])) * prime64
		h = (h ^ uint64(s[6])) * prime64
		h = (h ^ uint64(s[7])) * prime64
		s = s[8:]
	}

	if len(s) >= 4 {
		h = (h ^ uint64(s[0])) * prime64
		h = (h ^ uint64(s[1])) * prime64
		h = (h ^ uint64(s[2])) * prime64
		h = (h ^ uint64(s[3])) * prime64
		s = s[4:]
	}

	if len(s) >= 2 {
		h = (h ^ uint64(s[0])) * prime64
		h = (h ^ uint64(s[1])) * prime64
		s = s[2:]
	}

	if len(s) > 0 {
		h = (h ^ uint64(s[0])) * prime64
	}
	return
}

// prime number generator

// isPrime checks whether a number is prime.
func isPrime(n int) bool {
	if n <= 1 {
		return false
	}
	if n <= 3 {
		return true
	}
	if n%2 == 0 || n%3 == 0 {
		return false
	}
	sqrtN := int(math.Sqrt(float64(n)))
	for i := 5; i <= sqrtN; i += 6 {
		if n%i == 0 || n%(i+2) == 0 {
			return false
		}
	}
	return true
}

// NextPrime returns the smallest prime number greater than n.
func nextPrime(n int) int {
	if n < 2 {
		return 2
	}
	// Start from n+1 and go up until we find a prime.
	for candidate := n + 1; ; candidate++ {
		if isPrime(candidate) {
			return candidate
		}
	}
}

// lower contentions using mutiple shards

type Hashable interface {
	GetHashValue(basis uint64) uint64
}

type ShardedMapT[K interface {
	comparable
	Hashable
}, V any] struct {
	basis  uint64
	shards []*SharedMapT[K, V]
}

func NewShardedMap[K interface {
	comparable
	Hashable
}, V any](numShards int) *ShardedMapT[K, V] {
	numShards = nextPrime(numShards)
	shards := make([]*SharedMapT[K, V], numShards)
	for i := range shards {
		shards[i] = NewSharedMapT[K, V]()
	}
	return &ShardedMapT[K, V]{basis: rand.Uint64() + 14695981039346656037, shards: shards}
}

func (x *ShardedMapT[K, V]) getShard(key K) *SharedMapT[K, V] {
	return x.shards[key.GetHashValue(x.basis)%uint64(len(x.shards))]
}
func (x *ShardedMapT[K, V]) Len() (count int) {
	for _, shard := range x.shards {
		count += shard.Len()
	}
	return
}
func (x *ShardedMapT[K, V]) Clear() {
	for _, shard := range x.shards {
		shard.Clear()
	}
}
func (x *ShardedMapT[K, V]) Keys() (result []K) {
	for _, shard := range x.shards {
		result = append(result, shard.Keys()...)
	}
	return
}
func (x *ShardedMapT[K, V]) Values() (result []V) {
	for _, shard := range x.shards {
		result = append(result, shard.Values()...)
	}
	return
}
func (x *ShardedMapT[K, V]) Range(each func(K, V) error) error {
	for _, shard := range x.shards {
		if err := shard.Range(each); err != nil {
			return err
		}
	}
	return nil
}
func (x *ShardedMapT[K, V]) Add(key K, value V) V {
	return x.getShard(key).Add(key, value)
}
func (x *ShardedMapT[K, V]) FindOrAdd(key K, value V) (V, bool) {
	return x.getShard(key).FindOrAdd(key, value)
}
func (x *ShardedMapT[K, V]) Get(key K) (result V, ok bool) {
	return x.getShard(key).Get(key)
}
func (x *ShardedMapT[K, V]) Delete(key K) {
	x.getShard(key).Delete(key)
}
