package base

import (
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	"golang.org/x/exp/constraints"
)

var LogSerialize = NewLogCategory("Serialize")

/***************************************
 * Archive Flags
 ***************************************/

type ArchiveFlag int32

const (
	AR_LOADING ArchiveFlag = iota
	AR_DETERMINISM
	AR_TOLERANT
)

func GetArchiveFlags() []ArchiveFlag {
	return []ArchiveFlag{
		AR_LOADING,
		AR_DETERMINISM,
		AR_TOLERANT,
	}
}

type ArchiveFlags = EnumSet[ArchiveFlag, *ArchiveFlag]

const (
	AR_FLAGS_NONE        = ArchiveFlags(0)
	AR_FLAGS_DETERMINISM = ArchiveFlags(1 << AR_DETERMINISM)
	AR_FLAGS_TOLERANT    = ArchiveFlags(1 << AR_TOLERANT)
)

func (x ArchiveFlag) Ord() int32        { return int32(x) }
func (x *ArchiveFlag) FromOrd(in int32) { *x = ArchiveFlag(in) }
func (x *ArchiveFlag) Set(in string) (err error) {
	switch in {
	case AR_LOADING.String():
		*x = AR_LOADING
	case AR_DETERMINISM.String():
		*x = AR_DETERMINISM
	case AR_TOLERANT.String():
		*x = AR_TOLERANT
	default:
		err = fmt.Errorf("unkown archive flags: %v", in)
	}
	return
}

func (x ArchiveFlag) String() (str string) {
	switch x {
	case AR_LOADING:
		str = "LOADING"
	case AR_DETERMINISM:
		str = "DETERMINISM"
	case AR_TOLERANT:
		str = "TOLERANT"
	default:
		UnexpectedValuePanic(x, x)
	}
	return
}

func (x ArchiveFlag) Description() (str string) {
	switch x {
	case AR_LOADING:
		str = "loading indicator, saving when disabled"
	case AR_DETERMINISM:
		str = "will sort certain values -like map[]- to keep archive deterministic"
	case AR_TOLERANT:
		str = "serialization errors are fatal by default, except if archive is tolerant"
	default:
		UnexpectedValue(x)
	}
	return
}
func (x ArchiveFlag) AutoComplete(in AutoComplete) {
	for _, it := range GetArchiveFlags() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * Archive
 ***************************************/

type Archive interface {
	Factory() SerializableFactory

	Error() error
	OnError(error)
	OnErrorf(string, ...any)

	Flags() ArchiveFlags

	HasTags(...FourCC) bool
	SetTags(...FourCC)

	Raw(value []byte)
	Byte(value *byte)
	Bool(value *bool)
	Int32(value *int32)
	Int64(value *int64)
	UInt32(value *uint32)
	UInt64(value *uint64)
	Float32(value *float32)
	Float64(value *float64)
	String(value *string)
	Time(value *time.Time)
	Serializable(value Serializable)
}

type Serializable interface {
	Serialize(ar Archive)
}

/***************************************
 * Serializable Guid
 ***************************************/

var ErrInvalidGuidLen = errors.New("Invalid GUID length")

type SerializableGuid [16]byte

func (x SerializableGuid) String() string {
	return hex.EncodeToString(x[:])
}
func (x *SerializableGuid) Serialize(ar Archive) {
	ar.Raw(x[:])
}
func (x *SerializableGuid) Set(in string) error {
	raw, err := hex.DecodeString(in)
	if err == nil && len(raw) != len(*x) {
		err = ErrInvalidGuidLen
	}
	if err == nil {
		copy(x[:], raw)
	}
	return err
}
func (x SerializableGuid) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *SerializableGuid) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}

/***************************************
 * Serializable Factory
 ***************************************/

type SerializableFactory interface {
	RegisterName(typeptr uintptr, name string, factory func() Serializable)
	CreateNew(guid SerializableGuid) Serializable
	ResolveTypename(typeptr uintptr) SerializableGuid
}

type serializableType struct {
	Name    string
	Guid    SerializableGuid
	Factory func() Serializable
}

type serializableFactory struct {
	typeptrToType map[uintptr]serializableType
	guidToType    map[SerializableGuid]serializableType
}

var globalSerializableFactory = serializableFactory{
	typeptrToType: make(map[uintptr]serializableType, 128),
	guidToType:    make(map[SerializableGuid]serializableType, 128),
}

func GetGlobalSerializableFactory() SerializableFactory {
	return &globalSerializableFactory
}

func (x *serializableFactory) RegisterName(typeptr uintptr, name string, factory func() Serializable) {
	Assert(func() bool { return len(name) > 0 })

	typ := serializableType{
		Name:    name,
		Factory: factory}
	fingerprint := StringFingerprint(name)
	copy(typ.Guid[:], fingerprint[:len(typ.Guid)])

	LogDebug(LogSerialize, "register type <%v> [%v] (typeptr: %X)", name, typ.Guid, typeptr)

	if prev, ok := x.typeptrToType[typeptr]; ok && prev.Guid != typ.Guid {
		LogPanic(LogSerialize, "overwriting factory %q from <%v> to <%v>", typeptr, prev.Name, name)
	}
	if prev, ok := x.guidToType[typ.Guid]; ok && prev.Guid != typ.Guid {
		LogPanic(LogSerialize, "duplicate factory %q from <%v> to <%v>", typ.Guid, prev.Name, name)
	}

	x.typeptrToType[typeptr] = typ
	x.guidToType[typ.Guid] = typ
}
func (x *serializableFactory) CreateNew(guid SerializableGuid) Serializable {
	if it, ok := x.guidToType[guid]; ok {
		return it.Factory()
	}
	LogPanic(LogSerialize, "could not resolve concrete type from %q", guid)
	return nil
}
func (x *serializableFactory) ResolveTypename(typeptr uintptr) SerializableGuid {
	if it, ok := x.typeptrToType[typeptr]; ok {
		return it.Guid
	}
	LogPanic(LogSerialize, "could not resolve type name from %X", typeptr)
	return SerializableGuid{}
}

func reflectTypename(input reflect.Type) string {
	// see gob.Register()

	// Default to printed representation for unnamed types
	rt := input
	name := rt.String()

	// But for named types (or pointers to them), qualify with import path
	// Dereference one pointer looking for a named type.
	star := ""
	if rt.Name() == "" {
		if pt := rt; pt.Kind() == reflect.Pointer {
			star = "*"
			rt = pt.Elem()
		}
	}
	if rt.Name() != "" {
		if rt.PkgPath() == "" {
			name = star + rt.Name()
		} else {
			name = star + rt.PkgPath() + "." + rt.Name()
		}
	}

	return name
}

// allocate 16 objects and distribute them when needed
// this pattern lessen stress on allocations (we know we will allocate far more than 16 objects)
const serializable_batchnew_stride = 16

type serializableBatchNew[T any] struct {
	sync.Mutex
	slice []T
}

func (x *serializableBatchNew[T]) Allocate() *T {
	x.Mutex.Lock()
	defer x.Mutex.Unlock()
	if len(x.slice) == 0 {
		x.slice = make([]T, serializable_batchnew_stride)
	}
	p := &x.slice[0]
	x.slice = x.slice[1:]
	return p
}

func reflectSerializable[T Serializable](factory SerializableFactory, value T) SerializableGuid {
	emptyPtr := getEmptyInterface(value)
	AssertNotIn(emptyPtr.typ, nil)

	return factory.ResolveTypename(uintptr(emptyPtr.typ))
}
func resolveSerializable(factory SerializableFactory, guid SerializableGuid) Serializable {
	return factory.CreateNew(guid)
}

func RegisterSerializable[T any, S interface {
	*T
	Serializable
}]() {
	var defautValue S
	emptyPtr := getEmptyInterface(defautValue)
	AssertNotIn(emptyPtr.typ, nil)

	rt := reflect.TypeOf(defautValue)
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}

	batchNew := new(serializableBatchNew[T])
	globalSerializableFactory.RegisterName(uintptr(emptyPtr.typ), reflectTypename(rt), func() Serializable {
		return S(batchNew.Allocate()) // S(new(T)) -> faster with batch new
	})
}

/***************************************
 * Archive Container Helpers
 ***************************************/

func SerializeMany[T any](ar Archive, serialize func(*T), slice *[]T) {
	size := uint32(len(*slice))
	ar.UInt32(&size)
	AssertErr(func() error {
		if size < 32000 {
			return nil
		}
		return fmt.Errorf("serializable: sanity check failed on slice length (%d > 32000)", size)
	})

	if ar.Flags().Has(AR_LOADING) {
		*slice = make([]T, size)
	}

	for i := range *slice {
		serialize(&(*slice)[i])
	}
}

func SerializeSlice[T any, S interface {
	*T
	Serializable
}](ar Archive, slice *[]T) {
	SerializeMany(ar, func(it *T) {
		ar.Serializable(S(it))
	}, slice)
}

type SerializablePair[
	K OrderedComparable[K], V any,
	SK interface {
		*K
		Serializable
	},
	SV interface {
		*V
		Serializable
	}] struct {
	Key   K
	Value V
}

func (x *SerializablePair[K, V, SK, SV]) Serialize(ar Archive) {
	ar.Serializable(SK(&x.Key))
	ar.Serializable(SV(&x.Value))
}

func SerializeMap[K OrderedComparable[K], V any,
	SK interface {
		*K
		Serializable
	},
	SV interface {
		*V
		Serializable
	}](ar Archive, assoc *map[K]V) {
	if ar.Flags().Has(AR_DETERMINISM) {
		// sort keys to serialize as a slice with deterministic order, since maps are randomized
		var tmp []SerializablePair[K, V, SK, SV]
		if ar.Flags().Has(AR_LOADING) {
			SerializeSlice(ar, &tmp)

			*assoc = make(map[K]V, len(tmp))
			for _, pair := range tmp {
				(*assoc)[pair.Key] = pair.Value
			}
		} else {
			tmp = make([]SerializablePair[K, V, SK, SV], 0, len(*assoc))
			for key, value := range *assoc {
				tmp = append(tmp, SerializablePair[K, V, SK, SV]{Key: key, Value: value})
			}

			sort.SliceStable(tmp, func(i, j int) bool {
				return tmp[i].Key.Compare(tmp[j].Key) < 0
			})

			SerializeSlice(ar, &tmp)
		}
	} else {
		// simply iterate through the map and serialize in random order whem determinism is not needed
		size := uint32(len(*assoc))
		ar.UInt32(&size)
		AssertErr(func() error {
			if size < 32000 {
				return nil
			}
			return fmt.Errorf("serializable: sanity check failed on map length (%d > 32000)", size)
		})

		if ar.Flags().Has(AR_LOADING) {
			*assoc = make(map[K]V, size)
			var key K
			var value V
			for i := uint32(0); i < size; i++ {
				ar.Serializable(SK(&key))
				ar.Serializable(SV(&value))
				(*assoc)[key] = value
			}
		} else {
			for key, value := range *assoc {
				ar.Serializable(SK(&key))
				ar.Serializable(SV(&value))
			}
		}
	}
}

func SerializeExternal[T Serializable](ar Archive, external *T) {
	if ar.Flags().Has(AR_LOADING) {
		var guid, null SerializableGuid
		if ar.Raw(guid[:]); guid != null {
			*external = resolveSerializable(ar.Factory(), guid).(T)
		} else {
			return
		}
	} else {
		if IsNil(*external) {
			var null SerializableGuid
			ar.Raw(null[:])
			return
		}
		guid := reflectSerializable(ar.Factory(), *external)
		ar.Raw(guid[:])
	}

	ar.Serializable(*external)
}

func SerializeOptional[T any, E interface {
	*T
	Serializable
}](ar Archive, optional *Optional[T]) {
	if ar.Flags().Has(AR_LOADING) {
		var valid bool
		ar.Bool(&valid)
		if valid {
			ar.Serializable(E(&optional.value))
		} else {
			optional.err = ErrEmptyOptional
		}
	} else {
		valid := optional.Valid()
		ar.Bool(&valid)
		if valid {
			ar.Serializable(E(&optional.value))
		}
	}
}

func SerializeCompactSigned[Signed constraints.Signed](ar Archive, index *Signed) {
	var b byte
	if ar.Flags().Has(AR_LOADING) {
		ar.Byte(&b)
		sign := b & 0x80 // sign bit
		r := Signed(b & 0x3f)
		if (b & 0x40) != 0 { // has 2nd byte ?
			for shift := 6; ; shift += 7 {
				ar.Byte(&b)
				r |= Signed(b&0x7f) << shift
				if (b & 0x80) == 0 {
					break // no more bytes
				}
			}
		}
		*index = Blend(r, -r, sign != 0)
	} else {
		v := *index
		b = 0
		if v < 0 {
			v = -v
			b |= 0x80 // record sign bit
		}
		b |= byte(v & 0x3f)
		if v <= 0x3f {
			ar.Byte(&b)
		} else {
			b |= 0x40 // has 2nd byte
			v >>= 6
			ar.Byte(&b)
			for v != 0 {
				b = byte(v & 0x7f)
				v >>= 7
				if v != 0 {
					b |= 0x80 // has more bytes
				}
				ar.Byte(&b)
			}
		}
	}
}
func SerializeCompactUnsigned[Unsigned constraints.Unsigned](ar Archive, index *Unsigned) {
	var b byte
	if ar.Flags().Has(AR_LOADING) {
		ar.Byte(&b)
		shift := 7
		r := Unsigned(b & 0x7f)
		for (b & 0x80) != 0 { // has 2nd byte ?
			ar.Byte(&b)
			r |= Unsigned(b&0x7f) << shift
			shift += 7
		}
		*index = r
	} else {
		v := *index
		for {
			b = byte(v & 0x7f)
			if v >>= 7; v == 0 {
				ar.Byte(&b)
				break
			} else {
				b |= 0x80
				ar.Byte(&b)
			}
		}
	}
}

/***************************************
 * BasicArchive
 ***************************************/

type basicArchive struct {
	bytes   *[]byte
	tags    []FourCC
	flags   ArchiveFlags
	factory SerializableFactory
	onError func(error)
	err     error
}

func newBasicArchive(flags ArchiveFlags) basicArchive {
	ar := basicArchive{
		factory: GetGlobalSerializableFactory(),
		bytes:   TransientPage4KiB.Allocate(),
		err:     nil,
		flags:   flags,
	}
	return ar
}

func (x basicArchive) Bytes() []byte                { return *x.bytes }
func (x basicArchive) Factory() SerializableFactory { return x.factory }
func (x basicArchive) Flags() ArchiveFlags          { return x.flags }
func (x basicArchive) Error() error                 { return x.err }

func (x *basicArchive) Close() error {
	TransientPage4KiB.Release(x.bytes)
	x.bytes = nil
	return x.err
}
func (x *basicArchive) HandleErrors(onError func(error)) {
	x.onError = onError
}
func (x *basicArchive) OnError(err error) {
	if err == nil {
		return
	}
	x.err = err
	if x.onError != nil {
		x.onError(err)
	} else if x.flags.Has(AR_TOLERANT) {
		LogError(LogSerialize, "%v", err)
	} else {
		LogPanic(LogSerialize, "%v", err)
	}
}
func (x *basicArchive) OnErrorf(msg string, args ...any) {
	x.OnError(fmt.Errorf(msg, args...))
}
func (x basicArchive) HasTags(tags ...FourCC) bool {
	for _, tag := range tags {
		if !Contains(x.tags, tag) {
			return false
		}
	}
	return true
}
func (x *basicArchive) SetTags(tags ...FourCC) {
	x.tags = tags
}
func (x *basicArchive) Reset() (err error) {
	err = x.err
	x.err = nil
	return
}
