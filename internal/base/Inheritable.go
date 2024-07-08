package base

import (
	"flag"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

/***************************************
 * Inheritable interface
 ***************************************/

type InheritableBase interface {
	IsInheritable() bool
}

type Inheritable[T any] interface {
	Inherit(*T)
	Overwrite(*T)
}

type TInheritableScalar[T InheritableBase] struct {
	Value T
}

func (x *TInheritableScalar[T]) Inherit(other T) {
	if x.Value.IsInheritable() {
		x.Value = other
	}
}
func (x *TInheritableScalar[T]) Overwrite(other T) {
	if !other.IsInheritable() {
		x.Value = other
	}
}

func MakeInheritable[T InheritableBase](value T) TInheritableScalar[T] {
	return TInheritableScalar[T]{value}
}

func Inherit[T InheritableBase](result *T, values ...T) {
	wrapper := MakeInheritable(*result)
	for _, it := range values {
		wrapper.Inherit(it)
	}
	*result = wrapper.Value
}
func Overwrite[T InheritableBase](result *T, values ...T) {
	wrapper := MakeInheritable(*result)
	for _, it := range values {
		wrapper.Overwrite(it)
	}
	*result = wrapper.Value
}

func InheritableCommandLine(name, input string, variable flag.Value) (bool, error) {
	if len(input) > len(name)+1 && input[0] == '-' {
		if input[1:1+len(name)] == name {
			if input[1+len(name)] == '=' {
				return true, variable.Set(input[len(name)+2:])
			}
		}
	}
	return false, nil
}

/***************************************
 * InheritableString
 ***************************************/

type InheritableString string

const (
	INHERIT_STRING = "INHERIT"
)

func (x InheritableString) Empty() bool { return x == "" }
func (x InheritableString) Get() string { return (string)(x) }
func (x *InheritableString) Assign(in string) {
	*(*string)(x) = in
}
func (x InheritableString) String() string { return (string)(x) }
func (x InheritableString) IsInheritable() bool {
	return x == INHERIT_STRING || x == ""
}
func (x InheritableString) Equals(y InheritableString) bool {
	return x == y
}
func (x InheritableString) Compare(y InheritableString) int {
	return strings.Compare(x.Get(), y.Get())
}
func (x InheritableString) GetHashValue(basis uint64) uint64 {
	return Fnv1a(x.Get(), basis)
}
func (x *InheritableString) Serialize(ar Archive) {
	ar.String((*string)(x))
}
func (x *InheritableString) Set(in string) error {
	*x = InheritableString(in)
	return nil
}

func (x *InheritableString) Inherit(y InheritableString) {
	if x.IsInheritable() {
		*x = y
	}
}
func (x *InheritableString) Overwrite(y InheritableString) {
	if !y.IsInheritable() {
		*x = y
	}
}

func (x InheritableString) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *InheritableString) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}

/***************************************
 * InheritableInt
 ***************************************/

type InheritableInt int32

const (
	INHERIT_VALUE InheritableInt = 0
)

func (x InheritableInt) Get() int { return int(x) }
func (x *InheritableInt) Assign(in int) {
	*(*int32)(x) = int32(in)
}
func (x InheritableInt) Equals(o InheritableInt) bool {
	return x == o
}
func (x *InheritableInt) Serialize(ar Archive) {
	ar.Int32((*int32)(x))
}
func (x InheritableInt) IsInheritable() bool {
	return x == INHERIT_VALUE
}

func (x InheritableInt) String() string {
	if x.IsInheritable() {
		return INHERIT_STRING
	}
	return strconv.Itoa(x.Get())
}
func (x *InheritableInt) Set(in string) error {
	switch strings.ToUpper(in) {
	case INHERIT_STRING:
		*x = INHERIT_VALUE
	default:
		if i64, err := strconv.ParseInt(in, 10, 32); err == nil {
			*x = InheritableInt(int32(i64))
		} else {
			return err
		}
	}
	return nil
}

func (x InheritableInt) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *InheritableInt) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}

func (x *InheritableInt) CommandLine(name, input string) (bool, error) {
	if ok, err := InheritableCommandLine(name, input, x); ok || err != nil {
		return ok, err
	}
	if len(name) == 1 && len(input) > 2 && input[0] == '-' && input[1] == name[0] {
		return true, x.Set(input[2:])
	}
	return false, nil
}

/***************************************
 * InheritableBigInt
 ***************************************/

type InheritableBigInt int64

func (x InheritableBigInt) Get() int64 { return int64(x) }
func (x *InheritableBigInt) Assign(in int64) {
	*(*int64)(x) = in
}
func (x InheritableBigInt) Equals(o InheritableBigInt) bool {
	return x == o
}
func (x *InheritableBigInt) Serialize(ar Archive) {
	ar.Int64((*int64)(x))
}
func (x InheritableBigInt) IsInheritable() bool {
	return x.Get() == int64(INHERIT_VALUE)
}

func (x InheritableBigInt) String() string {
	if x.IsInheritable() {
		return INHERIT_STRING
	}
	return strconv.FormatInt(x.Get(), 10)
}
func (x *InheritableBigInt) Set(in string) error {
	switch strings.ToUpper(in) {
	case INHERIT_STRING:
		x.Assign(int64(INHERIT_VALUE))
		return nil
	default:
		if v, err := strconv.ParseInt(in, 10, 64); err == nil {
			x.Assign(v)
			return nil
		} else {
			return err
		}
	}
}

func (x InheritableBigInt) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *InheritableBigInt) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}

func (x *InheritableBigInt) CommandLine(name, input string) (bool, error) {
	if ok, err := InheritableCommandLine(name, input, x); ok || err != nil {
		return ok, err
	}
	if len(name) == 1 && len(input) > 2 && input[0] == '-' && input[1] == name[0] {
		return true, x.Set(input[2:])
	}
	return false, nil
}

/***************************************
 * SizeInBytes
 ***************************************/

type SizeInBytes int64

const (
	KiB SizeInBytes = 1024
	MiB             = KiB * 1024
	GiB             = MiB * 1024
	TiB             = GiB * 1024
	PiB             = TiB * 1024
)

func Kibibytes(sz int64) float64 { return float64(sz) / float64(KiB) }
func Mebibytes(sz int64) float64 { return float64(sz) / float64(MiB) }
func Gibibytes(sz int64) float64 { return float64(sz) / float64(GiB) }
func Tebibytes(sz int64) float64 { return float64(sz) / float64(TiB) }
func Pebibytes(sz int64) float64 { return float64(sz) / float64(PiB) }

func MebibytesPerSec(sz int64, d time.Duration) float64 {
	return Mebibytes(sz) / float64(d.Seconds()+0.00001)
}

func (x *SizeInBytes) Add(sz int64) { *(*int64)(x) += sz }
func (x SizeInBytes) String() string {
	switch {
	case x < KiB:
		return fmt.Sprintf("%d b", x.Get())
	case x < MiB:
		return fmt.Sprintf("%.2f Kib", Kibibytes(x.Get()))
	case x < GiB:
		return fmt.Sprintf("%.2f Mib", Mebibytes(x.Get()))
	case x < TiB:
		return fmt.Sprintf("%.2f Gib", Gibibytes(x.Get()))
	case x < PiB:
		return fmt.Sprintf("%.2f Tib", Tebibytes(x.Get()))
	default:
		return fmt.Sprintf("%.2f Pib", Pebibytes(x.Get()))
	}
}

func (x SizeInBytes) Get() int64 { return int64(x) }
func (x *SizeInBytes) Assign(in int64) {
	*(*int64)(x) = in
}
func (x SizeInBytes) Equals(o SizeInBytes) bool {
	return x == o
}
func (x *SizeInBytes) Serialize(ar Archive) {
	ar.Int64((*int64)(x))
}
func (x SizeInBytes) IsInheritable() bool {
	return x.Get() == int64(INHERIT_VALUE)
}

var sizeInBytesUnits = map[string]int64{
	"B":   1,
	"KB":  1000,
	"MB":  1000 * 1000,
	"GB":  1000 * 1000 * 1000,
	"TB":  1000 * 1000 * 1000 * 1000,
	"KIB": 1024,
	"MIB": 1024 * 1024,
	"GIB": 1024 * 1024 * 1024,
	"TIB": 1024 * 1024 * 1024 * 1024,
}

var re_sizeInBytes = regexp.MustCompile(`^\s*(\d+)\s*([A-Z]+)?\s*$`)

func (x *SizeInBytes) Set(in string) error {
	upper := strings.ToUpper(in)
	switch upper {
	case INHERIT_STRING:
		x.Assign(int64(INHERIT_VALUE))
		return nil
	default:
		matches := re_sizeInBytes.FindStringSubmatch(upper)
		if len(matches) != 3 {
			return fmt.Errorf("invalid input format for size: %v", upper)
		}

		sizeStr, unit := matches[1], matches[2]
		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid numeric part for size: %v", err)
		}

		// assume bytes if no unit provided
		var unitMultiplier int64 = 1
		if len(unit) > 0 {
			var ok bool
			unitMultiplier, ok = sizeInBytesUnits[unit]
			if !ok {
				return fmt.Errorf("invalid unit for size: %v", size)
			}
		}

		x.Assign(size * unitMultiplier)
		return nil
	}
}

func (x SizeInBytes) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *SizeInBytes) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}

func (x *SizeInBytes) CommandLine(name, input string) (bool, error) {
	if ok, err := InheritableCommandLine(name, input, x); ok || err != nil {
		return ok, err
	}
	if len(name) == 1 && len(input) > 2 && input[0] == '-' && input[1] == name[0] {
		return true, x.Set(input[2:])
	}
	return false, nil
}

/***************************************
 * Timespan
 ***************************************/

type Timespan int64

const (
	Microsecond Timespan = 1
	Millisecond          = Microsecond * 1000
	Second               = Millisecond * 1000
	Minute               = Second * 60
	Hour                 = Minute * 60
	Day                  = Hour * 24
	Week                 = Day * 7
)

func Milliseconds(t int64) float64 { return float64(t) / float64(Millisecond) }
func Seconds(t int64) float64      { return float64(t) / float64(Second) }
func Minutes(t int64) float64      { return float64(t) / float64(Minute) }
func Hours(t int64) float64        { return float64(t) / float64(Hour) }
func Days(t int64) float64         { return float64(t) / float64(Day) }
func Weeks(t int64) float64        { return float64(t) / float64(Week) }

func (x *Timespan) Add(sz int64) { *(*int64)(x) += sz }
func (x Timespan) String() string {
	switch {
	case x < Millisecond:
		return fmt.Sprintf("%d µs", x.Get())
	case x < Second:
		return fmt.Sprintf("%.2f ms", Milliseconds(x.Get()))
	case x < Minute:
		return fmt.Sprintf("%.2f seconds", Seconds(x.Get()))
	case x < Hour:
		return fmt.Sprintf("%.2f minutes", Minutes(x.Get()))
	case x < Day:
		return fmt.Sprintf("%.2f hours", Hours(x.Get()))
	case x < Week:
		return fmt.Sprintf("%.2f days", Days(x.Get()))
	default:
		return fmt.Sprintf("%.2f weeks", Weeks(x.Get()))
	}
}

func (x Timespan) Get() int64                    { return int64(x) }
func (x Timespan) Duration() time.Duration       { return time.Microsecond * time.Duration(x.Get()) }
func (x *Timespan) SetDuration(in time.Duration) { x.Assign(in.Microseconds()) }
func (x *Timespan) Assign(in int64) {
	*(*int64)(x) = in
}
func (x Timespan) Equals(o Timespan) bool {
	return x == o
}
func (x *Timespan) Serialize(ar Archive) {
	ar.Int64((*int64)(x))
}
func (x Timespan) IsInheritable() bool {
	return x.Get() == int64(INHERIT_VALUE)
}

var timespanUnits = map[string]int64{
	"US":           int64(Microsecond),
	"ΜS":           int64(Microsecond),
	"MICROSECONDS": int64(Microsecond),
	"MS":           int64(Millisecond),
	"MILLISECONDS": int64(Millisecond),
	"S":            int64(Second),
	"SEC":          int64(Second),
	"SECONDS":      int64(Second),
	"M":            int64(Minute),
	"MIN":          int64(Minute),
	"MINUTES":      int64(Minute),
	"H":            int64(Hour),
	"HOURS":        int64(Hour),
	"D":            int64(Day),
	"DAYS":         int64(Day),
	"W":            int64(Week),
	"WEEEKS":       int64(Week),
}

func (x *Timespan) Set(in string) error {
	upper := strings.ToUpper(in)
	switch upper {
	case INHERIT_STRING:
		x.Assign(int64(INHERIT_VALUE))
		return nil
	default:
		matches := re_sizeInBytes.FindStringSubmatch(upper)
		if len(matches) != 3 {
			return fmt.Errorf("invalid input format for timespan: %v", upper)
		}

		sizeStr, unit := matches[1], matches[2]
		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid numeric part for timespan: %v", err)
		}

		// assume seconds if no unit provided
		var unitMultiplier int64 = int64(Second)
		if len(unit) > 0 {
			var ok bool
			unitMultiplier, ok = timespanUnits[unit]
			if !ok {
				return fmt.Errorf("invalid unit for timespan: %v", size)
			}
		}

		x.Assign(size * unitMultiplier)
		return nil
	}
}

func (x Timespan) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *Timespan) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}

func (x *Timespan) CommandLine(name, input string) (bool, error) {
	if ok, err := InheritableCommandLine(name, input, x); ok || err != nil {
		return ok, err
	}
	if len(name) == 1 && len(input) > 2 && input[0] == '-' && input[1] == name[0] {
		return true, x.Set(input[2:])
	}
	return false, nil
}

/***************************************
 * InheritableBool
 ***************************************/

type InheritableBool InheritableInt

const INHERITABLE_INHERIT InheritableBool = 0
const INHERITABLE_FALSE InheritableBool = 1
const INHERITABLE_TRUE InheritableBool = 2

func MakeBoolVar(enabled bool) (result InheritableBool) {
	if enabled {
		result.Enable()
	} else {
		result.Disable()
	}
	return
}

func (x *InheritableBool) AsInt() *InheritableInt {
	return (*InheritableInt)(x)
}

func (x InheritableBool) Get() bool       { return x == INHERITABLE_TRUE }
func (x InheritableBool) IsEnabled() bool { return x == INHERITABLE_TRUE }
func (x *InheritableBool) Assign(in bool) {
	if in {
		x.Enable()
	} else {
		x.Disable()
	}
}
func (x InheritableBool) Equals(o InheritableBool) bool {
	return x == o
}
func (x *InheritableBool) Serialize(ar Archive) {
	x.AsInt().Serialize(ar)
}
func (x InheritableBool) IsInheritable() bool {
	return x == INHERITABLE_INHERIT
}

func (x *InheritableBool) Enable() {
	*x = INHERITABLE_TRUE
}
func (x *InheritableBool) Disable() {
	*x = INHERITABLE_FALSE
}
func (x *InheritableBool) Toggle() {
	if x.Get() {
		x.Disable()
	} else {
		x.Enable()
	}
}

func (x InheritableBool) String() string {
	if x.Get() {
		return "TRUE"
	} else if !x.IsInheritable() {
		return "FALSE"
	} else {
		return "INHERIT"
	}
}
func (x *InheritableBool) Set(in string) error {
	switch strings.ToUpper(in) {
	case "TRUE":
		x.Enable()
		return nil
	case "FALSE":
		x.Disable()
		return nil
	default:
		return x.AsInt().Set(in)
	}
}

func (x *InheritableBool) AutoComplete(in AutoComplete) {
	in.Add(INHERITABLE_TRUE.String(), "enabled")
	in.Add(INHERITABLE_FALSE.String(), "disabled")
	// in.Add(INHERITABLE_INHERIT.String(), "inherit default value from configuration")
}
func (x *InheritableBool) AutoCompleteFlag(in AutoComplete, prefix, description string) {
	in.Add(prefix, description)
	in.Add("-no"+prefix, "disable "+description)
}
func (x *InheritableBool) CommandLine(name, input string) (bool, error) {
	if ok, err := InheritableCommandLine(name, input, x); ok || err != nil {
		return ok, err
	}
	if len(input) >= len(name)+1 && input[0] == '-' {
		if input[1:] == name {
			*x = INHERITABLE_TRUE
			return true, nil
		}
		if len(input) == 4+len(name) && input[:4] == "-no-" && input[4:] == name {
			*x = INHERITABLE_FALSE
			return true, nil
		}
	}
	return false, nil
}

func (x InheritableBool) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *InheritableBool) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}

/***************************************
 * InheritableSlice
 ***************************************/

type InheritableSlicable[T any] interface {
	Equatable[T]
	InheritableBase
	fmt.Stringer
}

type InheritableSlice[T InheritableSlicable[T], P interface {
	*T
	Serializable
	flag.Value
}] []T

func (x InheritableSlice[T, P]) Get() []T { return ([]T)(x) }

func (x InheritableSlice[T, P]) IsInheritable() bool {
	if len(x) == 0 {
		return true
	}
	for _, v := range x {
		if !v.IsInheritable() {
			return false
		}
	}
	return true
}
func (x InheritableSlice[T, P]) Equals(y InheritableSlice[T, P]) bool {
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if !x[i].Equals(y[i]) {
			return false
		}
	}
	return true
}
func (x *InheritableSlice[T, P]) Serialize(ar Archive) {
	SerializeSlice[T, P](ar, (*[]T)(x))
}
func (x InheritableSlice[T, P]) String() string {
	return JoinString(",", x.Get()...)
}
func (x *InheritableSlice[T, P]) Set(in string) error {
	args := strings.Split(in, ",")
	*x = make([]T, len(args))
	for i, a := range args {
		if err := P(&(*x)[i]).Set(strings.TrimSpace(a)); err != nil {
			return err
		}
	}
	return nil
}

func (x *InheritableSlice[T, P]) Inherit(y InheritableSlice[T, P]) {
	if x.IsInheritable() {
		*x = y
	}
}
func (x *InheritableSlice[T, P]) Overwrite(y InheritableSlice[T, P]) {
	if !y.IsInheritable() {
		*x = y
	}
}

func (x InheritableSlice[T, P]) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *InheritableSlice[T, P]) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}

func (x *InheritableSlice[T, P]) AutoComplete(in AutoComplete) {
	var defaultValue T
	var anon interface{} = P(&defaultValue)
	if autocomplete, ok := anon.(AutoCompletable); ok {
		autocomplete.AutoComplete(in)
	}
}
