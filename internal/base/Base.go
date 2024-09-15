package base

import (
	"fmt"
	"io"
	"net"
	"reflect"
	"sort"
	"time"
	"unsafe"
)

var LogBase = NewLogCategory("Base")

var StartedAt = Memoize(func() time.Time {
	return time.Now()
})

func Recover(scope func() error) (result error) {
	if !DEBUG_ENABLED {
		defer func() {
			if err := recover(); err != nil {
				var ok bool
				if result, ok = err.(error); !ok {
					result = fmt.Errorf("%v", err)
				}
			}
		}()
	}
	result = scope()
	return
}

/***************************************
 * Elimination of reflection
 * https://github.com/goccy/go-json#elimination-of-reflection
 ***************************************/

type emptyInterface struct {
	typ unsafe.Pointer
	ptr unsafe.Pointer
}

func getEmptyInterface(v interface{}) *emptyInterface {
	return (*emptyInterface)(unsafe.Pointer(&v))
}

func GetTypeptr(v interface{}) (uintptr, bool) {
	iface := getEmptyInterface(v)
	if iface.ptr != nil {
		return uintptr(iface.typ), true
	} else {
		return 0, false
	}
}

func UnsafeTypeptr(v interface{}) uintptr {
	if typeId, ok := GetTypeptr(v); ok {
		return typeId
	}
	panic(fmt.Errorf("can't extract type id from nil interface (%T)", v))
}

func GetTypename(v interface{}) string {
	rt := reflect.TypeOf(v)
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	return rt.Name()
}

func GetTypenameT[T any]() string {
	var defaultValue T
	return GetTypename(defaultValue)
}

/***************************************
 * https://mangatmodi.medium.com/go-check-nil-interface-the-right-way-d142776edef1
 ***************************************/

func IsNil(v interface{}) bool {
	if v == nil {
		return true
	}
	// val := reflect.ValueOf(v)
	// switch val.Kind() {
	// case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.UnsafePointer, reflect.Interface, reflect.Slice:
	// 	return val.IsNil()
	// }
	_, ok := GetTypeptr(v)
	return !ok
}

func AnyError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

/***************************************
 * Higher order programming
 ***************************************/

func DuplicateObjectForDebug[T any](src T) T {
	v := reflect.ValueOf(src).Elem()
	copy := reflect.New(v.Type())
	copy.Elem().Set(v)
	return copy.Interface().(T)
}

type WriteReseter interface {
	Reset(io.Writer) error
	io.WriteCloser
}

type ReadReseter interface {
	Reset(io.Reader) error
	io.ReadCloser
}

type Closable interface {
	Close() error
}

type Flushable interface {
	Flush() error
}

func FlushWriterIFP(w io.Writer) (err error) {
	if flush, ok := w.(Flushable); ok {
		err = flush.Flush()
	}
	return
}

type Equatable[T any] interface {
	Equals(other T) bool
}

type Comparable[T any] interface {
	Compare(other T) int
}

type OrderedComparable[T any] interface {
	Comparable[T]
	comparable
}

func Map[T, R any](transform func(T) R, src ...T) (dst []R) {
	dst = make([]R, len(src))
	for i, x := range src {
		dst[i] = transform(x)
	}
	return dst
}

func Range[T any](transform func(int) T, n int) (dst []T) {
	dst = make([]T, n)
	for i := 0; i < n; i++ {
		dst[i] = transform(i)
	}
	return dst
}

func Sum(each func(int) int, n int) (total int) {
	for i := 0; i < n; i++ {
		total += each(i)
	}
	return
}

func Blend[T any](ifFalse, ifTrue T, selector bool) T {
	if selector {
		return ifTrue
	} else {
		return ifFalse
	}
}

/***************************************
 * Get first network interface
 ***************************************/

var LogNetwork = NewLogCategory("Network")

func GetFirstNetInterface() (net.Interface, net.IPNet, error) {
	defer LogBenchmark(LogNetwork, "GetFirstNetInterface").Close()

	interfaces, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, net.IPNet{}, err
	}

	sort.Slice(interfaces, func(i, j int) bool {
		return interfaces[i].Index < interfaces[j].Index
	})

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagRunning == 0 || iface.Flags&net.FlagLoopback != 0 {
			LogDebug(LogNetwork, "ignore network interface %q: %v", iface.Name, iface.Flags)
			continue
		}

		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			LogDebug(LogNetwork, "invalid network interface %q: %v", iface.Name, addrErr)
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				LogDebug(LogNetwork, "invalid network address %q: %v (%T)", iface.Name, addr, addr)
				continue
			}

			if ipNet.IP.To4() != nil { //|| ipNet.IP.To16() != nil { // #TODO: test IPv6
				benchmark := LogBenchmark(LogNetwork, "LookupAddr[%d] %v -> %v (%v)", iface.Index, iface.Name, ipNet.IP, iface.Flags)
				ifaceHostnames, ifaceErr := net.LookupAddr(ipNet.IP.String())
				benchmark.Close()

				if ifaceErr == nil {
					LogDebug(LogNetwork, "network inteface %q hostnames for %v: %v", iface.Name, ipNet, ifaceHostnames)
				} else {
					LogDebug(LogNetwork, "invalid lookup address for %q: %v", iface.Name, addrErr)
					continue
				}

				return iface, *ipNet, nil
			}
		}
	}

	return net.Interface{}, net.IPNet{}, fmt.Errorf("failed to find main network interface")
}
