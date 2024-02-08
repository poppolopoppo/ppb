package base

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unsafe"
)

var LogBase = NewLogCategory("Base")

var StartedAt = Memoize[time.Time](func() time.Time {
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
