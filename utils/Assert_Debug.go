//go:build ppb_debug
// +build ppb_debug

package utils

import (
	"fmt"
	"reflect"
	"sync/atomic"
)

const DEBUG_ENABLED = true

var LogAssert = NewLogCategory("Assert")

var DebugTag = MakeArchiveTag(MakeFourCC('D', 'E', 'B', 'G'))

var enableDiagnostics bool = true

func EnableDiagnostics() bool {
	return enableDiagnostics
}
func SetEnableDiagnostics(enabled bool) {
	enableDiagnostics = enabled
}

func NewLogger() Logger {
	return newImmediateLogger(newInteractiveLogger(newBasicLogger()))
}

func AssertMessage(pred func() bool, msg string, args ...interface{}) {
	if !pred() {
		CommandPanicF(msg, args...)
	}
}

func Assert(pred func() bool) {
	AssertMessage(pred, "failed assertion")
}

func AssertSameType[T any](a T, b T) {
	ta := reflect.TypeOf(a)
	tb := reflect.TypeOf(b)
	if ta != tb {
		CommandPanicF("expected type <%v> but got <%v>", ta, tb)
	}
}

func AssertIn[T comparable](elt T, values ...T) {
	for _, x := range values {
		if x == elt {
			return
		}
	}
	CommandPanicF("element <%v> is not in the slice", elt)
}
func AssertNotIn[T comparable](elt T, values ...T) {
	for _, x := range values {
		if x == elt {
			CommandPanicF("element <%v> is already in the slice", elt)
		}
	}
}

func AssertInStrings[T fmt.Stringer](elt T, values ...T) {
	for _, x := range values {
		if x.String() == elt.String() {
			return
		}
	}
	CommandPanicF("element <%v> is not in the slice", elt)
}
func AssertNotInStrings[T fmt.Stringer](elt T, values ...T) {
	for _, x := range values {
		if x.String() == elt.String() {
			CommandPanicF("element <%v> is already in the slice", elt)
		}
	}
}

func NotImplemented(m string, a ...interface{}) {
	LogWarning(LogAssert, "not implemented: "+m, a...)
}
func UnreachableCode() {
	CommandPanicF("unreachable code")
}
func UnexpectedValue(x interface{}) {
	CommandPanicF("unexpected value: <%T> %#v", x, x)
}
func UnexpectedType(expected reflect.Type, given interface{}) {
	if reflect.TypeOf(given) != expected {
		CommandPanicF("expected <%#v>, given %#v <%T>", expected, given, given)
	}
}

func AppendComparable_CheckUniq[T comparable](src []T, elts ...T) (result []T) {
	result = src
	for _, x := range elts {
		if !Contains(src, x) {
			result = append(result, x)
		} else {
			CommandPanicF("element already in set: %v (%v)", x, src)
		}
	}
	return result
}
func PrependComparable_CheckUniq[T comparable](src []T, elts ...T) (result []T) {
	result = src
	for _, x := range elts {
		if !Contains(src, x) {
			result = append([]T{x}, result...)
		} else {
			CommandPanicF("element already in set: %v (%v)", x, src)
		}
	}
	return result
}

func AppendSlice_CheckUniq[T any](src []T, elts []T, equals func(T, T) bool) (result []T) {
	result = src
	for _, x := range elts {
		for _, y := range src {
			if equals(x, y) {
				CommandPanicF("element already in set: %v (%v)", x, src)
			}
		}
		result = append(result, x)
	}
	return result
}
func PrependSlice_CheckUniq[T any](src []T, elts []T, equals func(T, T) bool) (result []T) {
	result = src
	for _, x := range elts {
		for _, y := range src {
			if equals(x, y) {
				CommandPanicF("element already in set: %v (%v)", x, src)
			}
		}
		result = append([]T{x}, result...)
	}
	return result
}

func AppendEquatable_CheckUniq[T Equatable[T]](src []T, elts ...T) (result []T) {
	result = src
	for _, x := range elts {
		for _, y := range src {
			if x.Equals(y) {
				CommandPanicF("element already in set: %v (%v)", x, src)
			}
		}
		result = append(result, x)
	}
	return result
}
func PrependEquatable_CheckUniq[T Equatable[T]](src []T, elts ...T) (result []T) {
	result = src
	for _, x := range elts {
		for _, y := range src {
			if x.Equals(y) {
				CommandPanicF("element already in set: %v (%v)", x, src)
			}
		}
		result = append([]T{x}, result...)
	}
	return result
}

type AtomicFuture[T any] struct {
	atomic.Pointer[sync_future[T]]
}

func (x *AtomicFuture[T]) Reset() {
	x.Pointer.Store(nil)
}
func (x *AtomicFuture[T]) Store(future Future[T]) {
	x.Pointer.Store(future.(*sync_future[T]))
}

func MakeFuture[T any](f func() (T, error), debug ...fmt.Stringer) Future[T] {
	return make_sync_future(f, debug...)
}

func ParallelJoin[T any](each func(int, T) error, futures ...Future[T]) error {
	return ParallelJoin_Sync(each, futures...)
}
func ParallelMap[IN any, OUT any](each func(IN) (OUT, error), in ...IN) ([]OUT, error) {
	return ParallelMap_Sync(each, in...)
}
func ParallelRange[IN any](each func(IN) error, in ...IN) error {
	return ParallelRange_Sync(each, in...)
}
