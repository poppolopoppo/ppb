package base

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"golang.org/x/exp/constraints"
)

var LogSerialize = NewLogCategory("Serialize")

const AR_USE_COMPACT_INDICES = false // saves about 8% on just -configured db, but slower to consecutive ar.Byte() calls instead of fixed ar.Int()

/***************************************
 * Archive
 ***************************************/

type ArchiveFlag int32

const (
	AR_LOADING ArchiveFlag = iota
	AR_DETERMINISM
	AR_TOLERANT
	AR_GUARD
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
	case AR_GUARD.String():
		*x = AR_GUARD
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
	case AR_GUARD:
		str = "GUARD"
	default:
		UnexpectedValuePanic(x, x)
	}
	return
}

type ArchiveFlags struct {
	EnumSet[ArchiveFlag, *ArchiveFlag]
}

func (fl ArchiveFlags) IsLoading() bool {
	return fl.Has(AR_LOADING)
}
func (fl ArchiveFlags) IsDeterministic() bool {
	return fl.Has(AR_DETERMINISM)
}
func (fl ArchiveFlags) IsTolerant() bool {
	return fl.Has(AR_TOLERANT)
}
func (fl ArchiveFlags) IsGuarded() bool {
	return fl.Has(AR_GUARD)
}

const (
	BOOL_SIZE    int32 = 1
	BYTE_SIZE    int32 = 1
	INT32_SIZE   int32 = 4
	UINT32_SIZE  int32 = 4
	INT64_SIZE   int32 = 8
	UINT64_SIZE  int32 = 8
	FLOAT32_SIZE int32 = 4
	FLOAT64_SIZE int32 = 8
)

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
 * Serializable Factory
 ***************************************/

type SerializableFactory interface {
	RegisterName(typeptr uintptr, name string, concreteType reflect.Type)
	ResolveConreteType(guid serializableGuid) reflect.Type
	ResolveTypename(typeptr uintptr) serializableGuid
}

type serializableGuid [16]byte

func (x serializableGuid) String() string {
	return hex.EncodeToString(x[:])
}

type serializableType struct {
	Name string
	Type reflect.Type
	Guid serializableGuid
}

type serializableFactory struct {
	typeptrToType SharedMapT[uintptr, serializableType]
	guidToType    SharedMapT[serializableGuid, serializableType]
}

var globalSerializableFactory serializableFactory

func GetGlobalSerializableFactory() SerializableFactory {
	return &globalSerializableFactory
}

func (x *serializableFactory) RegisterName(typeptr uintptr, name string, concreteType reflect.Type) {
	Assert(func() bool { return len(name) > 0 })

	typ := serializableType{
		Name: name,
		Type: concreteType}
	fingerprint := StringFingerprint(name)
	copy(typ.Guid[:], fingerprint[:len(typ.Guid)])

	LogDebug(LogSerialize, "register type %v as %q : [%v]", concreteType, name, typ.Guid)

	if prev, ok := x.typeptrToType.FindOrAdd(typeptr, typ); ok && prev != typ {
		LogPanic(LogSerialize, "overwriting factory type %q from <%v> to <%v>", name, prev.Type, concreteType)
	}
	if prev, ok := x.guidToType.FindOrAdd(typ.Guid, typ); ok && prev != typ {
		LogPanic(LogSerialize, "duplicate factory type <%v> from %q to %q", concreteType, prev.Type, concreteType)
	}
}
func (x *serializableFactory) ResolveConreteType(guid serializableGuid) reflect.Type {
	it, ok := x.guidToType.Get(guid)
	if !ok {
		LogPanic(LogSerialize, "could not resolve concrete type from %q", guid)
	}
	return it.Type
}
func (x *serializableFactory) ResolveTypename(typeptr uintptr) serializableGuid {
	it, ok := x.typeptrToType.Get(typeptr)
	if !ok {
		LogPanic(LogSerialize, "could not resolve type name from %X", typeptr)
	}
	return it.Guid
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

func RegisterSerializable[T Serializable](value T) {
	typ, ok := GetTypeptr(value)
	if !ok {
		LogPanic(LogSerialize, "don't register a nil pointer to a struct %T", value)
	}
	rt := reflect.TypeOf(value)
	globalSerializableFactory.RegisterName(typ, reflectTypename(rt), rt)
}

func reflectSerializable[T Serializable](factory SerializableFactory, value T) (serializableGuid, bool) {
	if typ, ok := GetTypeptr(value); ok {
		return factory.ResolveTypename(typ), true
	} else {
		return serializableGuid{}, false
	}
}
func resolveSerializable(factory SerializableFactory, guid serializableGuid) reflect.Type {
	return factory.ResolveConreteType(guid)
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

	if ar.Flags().IsLoading() {
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
	if ar.Flags().IsDeterministic() {
		// sort keys to serialize as a slice with deterministic order, since maps are randomized
		var tmp []SerializablePair[K, V, SK, SV]
		if ar.Flags().IsLoading() {
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

		if ar.Flags().IsLoading() {
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
	if ar.Flags().IsLoading() {
		var guid, null serializableGuid
		if ar.Raw(guid[:]); guid != null {
			concreteType := resolveSerializable(ar.Factory(), guid)
			if concreteType.Kind() == reflect.Pointer {
				concreteType = concreteType.Elem()
			}

			value := reflect.New(concreteType)
			*external = value.Interface().(T)
		} else {
			return
		}
	} else {
		guid, ok := reflectSerializable(ar.Factory(), *external)
		ar.Raw(guid[:])
		if !ok {
			return
		}
	}

	ar.Serializable(*external)
}

func SerializeCompactSigned[Signed constraints.Signed](ar Archive, index *Signed) {
	var b byte
	if ar.Flags().IsLoading() {
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
	if ar.Flags().IsLoading() {
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
	bytes   []byte
	tags    []FourCC
	flags   ArchiveFlags
	factory SerializableFactory
	onError func(error)
	err     error
}

func newBasicArchive(flags ...ArchiveFlag) basicArchive {
	ar := basicArchive{
		factory: GetGlobalSerializableFactory(),
		bytes:   TransientSmallPage.Allocate(),
		err:     nil,
		flags: ArchiveFlags{
			MakeEnumSet(flags...),
		},
	}
	return ar
}

func (x basicArchive) Bytes() []byte                { return x.bytes }
func (x basicArchive) Factory() SerializableFactory { return x.factory }
func (x basicArchive) Flags() ArchiveFlags          { return x.flags }
func (x basicArchive) Error() error                 { return x.err }

func (x *basicArchive) Close() error {
	TransientSmallPage.Release(x.bytes)
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
	} else if x.flags.IsTolerant() {
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

/***************************************
 * ArchiveBinaryWriter
 ***************************************/

type ArchiveBinaryReader struct {
	reader        io.Reader
	indexToString []string
	basicArchive
}

func ArchiveBinaryRead(reader io.Reader, scope func(ar Archive)) (err error) {
	return Recover(func() error {
		ar := NewArchiveBinaryReader(reader)
		defer ar.Close()
		scope(NewArchiveGuard(&ar))
		return ar.Error()
	})
}

func NewArchiveBinaryReader(reader io.Reader, flags ...ArchiveFlag) ArchiveBinaryReader {
	return ArchiveBinaryReader{
		reader:        reader,
		indexToString: []string{},
		basicArchive:  newBasicArchive(append(flags, AR_LOADING)...),
	}
}

func (ar *ArchiveBinaryReader) Close() (err error) {
	if cls, ok := ar.reader.(io.Closer); ok {
		err = cls.Close()
	}
	if er := ar.basicArchive.Close(); er != nil && err == nil {
		err = er
	}
	return
}
func (ar *ArchiveBinaryReader) Reset(reader io.Reader) error {
	if rst, ok := ar.reader.(ReadReseter); ok {
		rst.Reset(reader)
	} else {
		ar.reader = reader
	}
	ar.indexToString = ar.indexToString[:0]
	return ar.basicArchive.Reset()
}

func (ar *ArchiveBinaryReader) Raw(value []byte) {
	for off := 0; off != len(value); {
		if n, err := ar.reader.Read(value[off:]); err == nil {
			off += n
		} else {
			ar.OnError(err)
			break
		}
	}
}
func (ar *ArchiveBinaryReader) Byte(value *byte) {
	raw := ar.bytes[:1]
	if _, err := ar.reader.Read(raw); err == nil {
		*value = raw[0]
	} else {
		ar.onError(err)
	}
}
func (ar *ArchiveBinaryReader) Bool(value *bool) {
	var b byte
	ar.Byte(&b)
	*value = (b != 0)
}
func (ar *ArchiveBinaryReader) Int32(value *int32) {
	if AR_USE_COMPACT_INDICES {
		SerializeCompactSigned(ar, value)
	} else {
		raw := ar.bytes[:INT32_SIZE]
		ar.Raw(raw)
		*value = int32(binary.LittleEndian.Uint32(raw))
	}
}
func (ar *ArchiveBinaryReader) Int64(value *int64) {
	if AR_USE_COMPACT_INDICES {
		SerializeCompactSigned(ar, value)
	} else {
		raw := ar.bytes[:INT64_SIZE]
		ar.Raw(raw)
		*value = int64(binary.LittleEndian.Uint64(raw))
	}
}
func (ar *ArchiveBinaryReader) UInt32(value *uint32) {
	if AR_USE_COMPACT_INDICES {
		SerializeCompactUnsigned(ar, value)
	} else {
		raw := ar.bytes[:UINT32_SIZE]
		ar.Raw(raw)
		*value = binary.LittleEndian.Uint32(raw)
	}
}
func (ar *ArchiveBinaryReader) UInt64(value *uint64) {
	if AR_USE_COMPACT_INDICES {
		SerializeCompactUnsigned(ar, value)
	} else {
		raw := ar.bytes[:UINT64_SIZE]
		ar.Raw(raw)
		*value = binary.LittleEndian.Uint64(raw)
	}
}
func (ar *ArchiveBinaryReader) Float32(value *float32) {
	raw := ar.bytes[:FLOAT32_SIZE]
	ar.Raw(raw)
	*value = math.Float32frombits(binary.LittleEndian.Uint32(raw))
}
func (ar *ArchiveBinaryReader) Float64(value *float64) {
	raw := ar.bytes[:FLOAT64_SIZE]
	ar.Raw(raw)
	*value = math.Float64frombits(binary.LittleEndian.Uint64(raw))
}
func (ar *ArchiveBinaryReader) String(value *string) {
	var size int32
	ar.Int32(&size)

	if size < 0 { // check if the length if negative
		*value = ar.indexToString[-size-1] // cache hit on string already read
		return
	}

	AssertErr(func() error {
		if size < 2048 {
			return nil
		}
		return fmt.Errorf("serializable: sanity check failed on string length (%d > 2048)", size)
	})
	ar.Raw(ar.bytes[:size])
	*value = string(ar.bytes[:size])

	// record the string for future occurrences
	ar.indexToString = append(ar.indexToString, *value)
}
func (ar *ArchiveBinaryReader) Time(value *time.Time) {
	raw := ar.bytes[:INT64_SIZE]
	ar.Raw(raw)
	*value = time.UnixMilli(int64(binary.LittleEndian.Uint64(raw)))
}
func (ar *ArchiveBinaryReader) Serializable(value Serializable) {
	value.Serialize(ar)
}

/***************************************
 * ArchiveBinaryWriter
 ***************************************/

type ArchiveBinaryWriter struct {
	writer          io.Writer
	str             io.StringWriter
	hasStringWriter bool
	stringToIndex   map[string]int32
	basicArchive
}

func ArchiveBinaryWrite(writer io.Writer, scope func(ar Archive)) (err error) {
	return Recover(func() error {
		ar := NewArchiveBinaryWriter(writer)
		defer ar.Close()
		scope(NewArchiveGuard(&ar))
		return ar.Error()
	})
}

func NewArchiveBinaryWriter(writer io.Writer, flags ...ArchiveFlag) ArchiveBinaryWriter {
	str, hasStringWriter := writer.(io.StringWriter)
	return ArchiveBinaryWriter{
		writer:          writer,
		str:             str,
		hasStringWriter: hasStringWriter,
		stringToIndex:   make(map[string]int32),
		basicArchive:    newBasicArchive(flags...),
	}
}

func (ar *ArchiveBinaryWriter) Close() (err error) {
	if cls, ok := ar.writer.(io.Closer); ok {
		err = cls.Close()
	}
	if er := ar.basicArchive.Close(); er != nil && err == nil {
		err = er
	}
	return
}
func (ar *ArchiveBinaryWriter) Reset(writer io.Writer) error {
	FlushWriterIFP(writer)

	if rst, ok := ar.writer.(WriteReseter); ok {
		rst.Reset(writer)
	} else {
		ar.writer = writer
		ar.str, ar.hasStringWriter = writer.(io.StringWriter)
	}

	ar.stringToIndex = make(map[string]int32)
	return ar.basicArchive.Reset()
}

func (ar *ArchiveBinaryWriter) Raw(value []byte) {
	for off := 0; off != len(value); {
		if n, err := ar.writer.Write(value[off:]); err == nil {
			off += n
		} else {
			ar.OnError(err)
			break
		}
	}
}
func (ar *ArchiveBinaryWriter) Byte(value *byte) {
	raw := ar.bytes[:BYTE_SIZE]
	raw[0] = *value
	if _, err := ar.writer.Write(raw); err != nil {
		ar.onError(err)
	}
}
func (ar *ArchiveBinaryWriter) Bool(value *bool) {
	raw := ar.bytes[:BOOL_SIZE]
	raw[0] = 0
	if *value {
		raw[0] = 0xFF
	}
	ar.Raw(raw)
}
func (ar *ArchiveBinaryWriter) Int32(value *int32) {
	if AR_USE_COMPACT_INDICES {
		SerializeCompactSigned(ar, value)
	} else {
		raw := ar.bytes[:INT32_SIZE]
		binary.LittleEndian.PutUint32(raw, uint32(*value))
		ar.Raw(raw)
	}
}
func (ar *ArchiveBinaryWriter) Int64(value *int64) {
	if AR_USE_COMPACT_INDICES {
		SerializeCompactSigned(ar, value)
	} else {
		raw := ar.bytes[:INT64_SIZE]
		binary.LittleEndian.PutUint64(raw, uint64(*value))
		ar.Raw(raw)
	}
}
func (ar *ArchiveBinaryWriter) UInt32(value *uint32) {
	if AR_USE_COMPACT_INDICES {
		SerializeCompactUnsigned(ar, value)
	} else {
		raw := ar.bytes[:UINT32_SIZE]
		binary.LittleEndian.PutUint32(raw, *value)
		ar.Raw(raw)
	}
}
func (ar *ArchiveBinaryWriter) UInt64(value *uint64) {
	if AR_USE_COMPACT_INDICES {
		SerializeCompactUnsigned(ar, value)
	} else {
		raw := ar.bytes[:UINT64_SIZE]
		binary.LittleEndian.PutUint64(raw, *value)
		ar.Raw(raw)
	}
}
func (ar *ArchiveBinaryWriter) Float32(value *float32) {
	raw := ar.bytes[:FLOAT32_SIZE]
	binary.LittleEndian.PutUint32(raw, math.Float32bits(*value))
	ar.Raw(raw)
}
func (ar *ArchiveBinaryWriter) Float64(value *float64) {
	raw := ar.bytes[:FLOAT64_SIZE]
	binary.LittleEndian.PutUint64(raw, math.Float64bits(*value))
	ar.Raw(raw)
}
func (ar *ArchiveBinaryWriter) String(value *string) {
	if index, alreadySerialized := ar.stringToIndex[*value]; alreadySerialized {
		Assert(func() bool { return index < 0 })
		ar.Int32(&index) // serialize the index, which is negative, instead of the string
		return
	} else { // record the index in local cache for future occurences
		ar.stringToIndex[*value] = int32(-len(ar.stringToIndex) - 1)
	}

	// serialize string length, followed by content bytes

	size := int32(len(*value)) // return the number of bytes in string, not number of runes
	AssertErr(func() error {
		if size < 2048 {
			return nil
		}
		return fmt.Errorf("serializable: sanity check failed on string length (%d > 2048)", size)
	})
	ar.Int32(&size)

	if ar.hasStringWriter { // avoid temporary string copy
		if n, err := ar.str.WriteString(*value); err != nil {
			ar.OnError(err)
		} else if int32(n) != size {
			ar.OnErrorf("serializable: not enough bytes written -> %d != %d", size, n)
		}

	} else {
		ar.Raw(UnsafeBytesFromString(*value))
	}
}
func (ar *ArchiveBinaryWriter) Time(value *time.Time) {
	raw := ar.bytes[:INT64_SIZE]
	binary.LittleEndian.PutUint64(raw, uint64(value.UnixMilli()))
	ar.Raw(raw)
}
func (ar *ArchiveBinaryWriter) Serializable(value Serializable) {
	value.Serialize(ar)
}

/***************************************
 * ArchiveFile
 ***************************************/

type ArchiveFile struct {
	Magic   FourCC
	Version FourCC
	Tags    []FourCC
}

var ArchiveTags = []FourCC{}

func MakeArchiveTag(tag FourCC) FourCC {
	AssertNotIn(tag, ArchiveTags...)
	ArchiveTags = append(ArchiveTags, tag)
	return tag
}

func NewArchiveFile() ArchiveFile {
	return ArchiveFile{
		Magic:   MakeFourCC('A', 'R', 'B', 'F'),
		Version: MakeFourCC('1', '0', '0', '0'),
		Tags:    ArchiveTags,
	}
}
func (x *ArchiveFile) Serialize(ar Archive) {
	ar.Serializable(&x.Magic)
	ar.Serializable(&x.Version)
	SerializeSlice(ar, &x.Tags)

	// forward serialized tags to the archive
	ar.SetTags(x.Tags...)
}

func ArchiveFileRead(reader io.Reader, scope func(ar Archive)) (file ArchiveFile, err error) {
	return file, ArchiveBinaryRead(reader, func(ar Archive) {
		ar.Serializable(&file)
		if err := ar.Error(); err == nil {
			defaultFile := NewArchiveFile()
			if file.Magic != defaultFile.Magic {
				ar.OnErrorf("archive: invalid file magic (%q != %q)", file.Magic, defaultFile.Magic)
			}
			if file.Version > defaultFile.Version {
				ar.OnErrorf("archive: newer file version (%q > %q)", file.Version, defaultFile.Version)
			}
			if err = ar.Error(); err == nil {
				scope(ar)
			}
		}
	})
}
func ArchiveFileWrite(writer io.Writer, scope func(ar Archive)) (err error) {
	return ArchiveBinaryWrite(writer, func(ar Archive) {
		file := NewArchiveFile()
		ar.Serializable(&file)
		if err := ar.Error(); err == nil {
			scope(ar)
		}
	})
}

/***************************************
 * CompressedArchiveFile
 ***************************************/

func CompressedArchiveFileRead(reader io.Reader, scope func(ar Archive), compression ...CompressionOptionFunc) (file ArchiveFile, err error) {
	return ArchiveFileRead(NewCompressedReader(reader, compression...), scope)
}
func CompressedArchiveFileWrite(writer io.Writer, scope func(ar Archive), compression ...CompressionOptionFunc) (err error) {
	return ArchiveFileWrite(NewCompressedWriter(writer, compression...), scope)
}

/***************************************
 * ArchiveDiff
 ***************************************/

type ArchiveDiff struct {
	buffer  *bytes.Buffer
	compare ArchiveBinaryReader
	stack   []Serializable
	level   int
	len     int
	verbose bool
	basicArchive
}

func SerializableDiff(a, b Serializable) error {
	ar := NewArchiveDiff()
	defer ar.Close()
	return ar.Diff(a, b)
}

func NewArchiveDiff() ArchiveDiff {
	return ArchiveDiff{
		basicArchive: newBasicArchive(),
		buffer:       TransientBuffer.Allocate(),
		verbose:      IsLogLevelActive(LOG_TRACE),
	}
}

func (x *ArchiveDiff) Len() int {
	return x.len
}

func (x *ArchiveDiff) Diff(a, b Serializable) error {
	x.buffer.Reset()
	x.err = nil
	x.level = 0
	x.stack = []Serializable{}

	return Recover(func() error {
		// write a in memory
		if ram := NewArchiveBinaryWriter(x.buffer, AR_DETERMINISM); true {
			ram.Serializable(a)
			if err := ram.Close(); err != nil {
				return err
			}
		}

		// record serialized size
		x.len = x.buffer.Len()

		// read b from memory, but do not actually update b
		x.compare = NewArchiveBinaryReader(x.buffer, AR_DETERMINISM)
		x.compare.flags.Remove(AR_LOADING) // don't want to overwrite b, so we tweak AR_LOADING flag
		defer x.compare.Close()

		x.Serializable(b)
		return x.Error()
	})
}

func (x *ArchiveDiff) Close() error {
	TransientBuffer.Release(x.buffer)
	x.buffer = nil
	return nil
}

func (x *ArchiveDiff) Flags() ArchiveFlags {
	return x.compare.flags
}
func (x *ArchiveDiff) Error() error {
	if err := x.basicArchive.Error(); err != nil {
		return x.basicArchive.err
	}
	if err := x.compare.Error(); err != nil {
		return err
	}
	return nil
}

func (ar ArchiveDiff) serializeLog(format string, args ...interface{}) {
	if ar.verbose && IsLogLevelActive(LOG_DEBUG) {
		indent := strings.Repeat("  ", ar.level)
		LogDebug(LogSerialize, "%s%s", indent, fmt.Sprintf(format, args...))
	}
}
func (x *ArchiveDiff) onDiff(old, new any) {
	details := strings.Builder{}
	indent := "  "
	for i, it := range x.stack {
		fmt.Fprintf(&details, "%s[%d] %T: %q\n", indent, i, it, it)
		indent += "  "
	}
	x.OnErrorf("serializable: difference found -> %q != %q\n%s", old, new, details.String())
}

func checkArchiveDiffForScalar[T comparable](ar *ArchiveDiff, serialize func(*T), value *T) {
	ar.compare.flags.Add(AR_LOADING)          // need to restore AR_LOADING while reading scalar values,
	defer ar.compare.flags.Remove(AR_LOADING) // for this scope only

	cmp := *value
	serialize(&cmp)
	ar.serializeLog("%T: '%v' == '%v'", *value, *value, cmp)

	if cmp != *value {
		ar.onDiff(*value, cmp)
	}
}

func (x *ArchiveDiff) Raw(value []byte) {
	cmp := x.compare.bytes[:len(value)]
	x.compare.Raw(cmp)
	x.serializeLog("%T: '%v' == '%v'", value, value, cmp)
	if !bytes.Equal(cmp, value) {
		x.onDiff(value, cmp)
	}
}
func (x *ArchiveDiff) Byte(value *byte) {
	checkArchiveDiffForScalar(x, x.compare.Byte, value)
}
func (x *ArchiveDiff) Bool(value *bool) {
	checkArchiveDiffForScalar(x, x.compare.Bool, value)
}
func (x *ArchiveDiff) Int32(value *int32) {
	checkArchiveDiffForScalar(x, x.compare.Int32, value)
}
func (x *ArchiveDiff) Int64(value *int64) {
	checkArchiveDiffForScalar(x, x.compare.Int64, value)
}
func (x *ArchiveDiff) UInt32(value *uint32) {
	checkArchiveDiffForScalar(x, x.compare.UInt32, value)
}
func (x *ArchiveDiff) UInt64(value *uint64) {
	checkArchiveDiffForScalar(x, x.compare.UInt64, value)
}
func (x *ArchiveDiff) Float32(value *float32) {
	checkArchiveDiffForScalar(x, x.compare.Float32, value)
}
func (x *ArchiveDiff) Float64(value *float64) {
	checkArchiveDiffForScalar(x, x.compare.Float64, value)
}
func (x *ArchiveDiff) String(value *string) {
	checkArchiveDiffForScalar(x, x.compare.String, value)
}
func (x *ArchiveDiff) Time(value *time.Time) {
	checkArchiveDiffForScalar(x, x.compare.Time, value)
}
func (x *ArchiveDiff) Serializable(value Serializable) {
	x.serializeLog(" --> %T", value)

	x.level++
	x.stack = append(x.stack, value)

	// don't use compare here, or recursive descent won't use diffs functions above
	value.Serialize(x)

	AssertIn(x.stack[len(x.stack)-1], value)
	x.stack = x.stack[:len(x.stack)-1]
	x.level--
}

/***************************************
 * ArchiveGuard
 ***************************************/

const (
	ARCHIVEGUARD_ENABLED        = false
	ARCHIVEGUARD_CANARY  uint32 = 0xDEADBEEF
)

var (
	ARCHIVEGUARD_TAG_RAW          FourCC = MakeFourCC('R', 'A', 'W', 'B')
	ARCHIVEGUARD_TAG_BYTE         FourCC = MakeFourCC('B', 'Y', 'T', 'E')
	ARCHIVEGUARD_TAG_BOOL         FourCC = MakeFourCC('B', 'O', 'O', 'L')
	ARCHIVEGUARD_TAG_INT32        FourCC = MakeFourCC('S', 'I', '3', '2')
	ARCHIVEGUARD_TAG_INT64        FourCC = MakeFourCC('S', 'I', '6', '4')
	ARCHIVEGUARD_TAG_UINT32       FourCC = MakeFourCC('U', 'I', '3', '2')
	ARCHIVEGUARD_TAG_UINT64       FourCC = MakeFourCC('U', 'I', '6', '4')
	ARCHIVEGUARD_TAG_FLOAT32      FourCC = MakeFourCC('F', 'T', '3', '2')
	ARCHIVEGUARD_TAG_FLOAT64      FourCC = MakeFourCC('F', 'T', '6', '4')
	ARCHIVEGUARD_TAG_STRING       FourCC = MakeFourCC('S', 'T', 'R', 'G')
	ARCHIVEGUARD_TAG_TIME         FourCC = MakeFourCC('T', 'I', 'M', 'E')
	ARCHIVEGUARD_TAG_SERIALIZABLE FourCC = MakeFourCC('S', 'R', 'L', 'Z')
)

type ArchiveGuard struct {
	inner Archive
	level int
}

func NewArchiveGuard(ar Archive) Archive {
	shouldUseGuard := false
	if ar.Flags().IsLoading() {
		shouldUseGuard = ar.Flags().IsGuarded()
	} else {
		shouldUseGuard = ARCHIVEGUARD_ENABLED
	}
	if shouldUseGuard {
		return ArchiveGuard{inner: ar}
	} else {
		return ar
	}
}

func (ar ArchiveGuard) serializeLog(format string, args ...interface{}) {
	if IsLogLevelActive(LOG_DEBUG) {
		indent := strings.Repeat("  ", ar.level)
		LogDebug(LogSerialize, "%s%s", indent, fmt.Sprintf(format, args...))
	}
}
func (ar ArchiveGuard) checkTag(tag FourCC) {
	canary := ARCHIVEGUARD_CANARY
	if ar.inner.UInt32(&canary); canary != ARCHIVEGUARD_CANARY {
		LogPanic(LogSerialize, "invalid canary guard : 0x%08X vs 0x%08X", canary, ARCHIVEGUARD_CANARY)
	}

	check := tag
	if check.Serialize(ar.inner); tag != check {
		LogPanic(LogSerialize, "invalid tag guard : %s (0x%08X) vs %s (0x%08X)", check, check, tag, tag)
	}
}
func (ar *ArchiveGuard) typeGuard(value any, tag FourCC, format string, args ...interface{}) func() {
	ar.serializeLog(format, args...)
	ar.checkTag(tag)

	if tag == ARCHIVEGUARD_TAG_SERIALIZABLE {
		ar.level++
	}

	return func() {
		LogPanicIfFailed(LogSerialize, ar.inner.Error())
		ar.checkTag(tag)

		vo := reflect.ValueOf(value)
		if vo.Kind() == reflect.Pointer {
			vo = vo.Elem()
		}
		ar.serializeLog("\t-> %v: %v", vo.Type(), vo)

		if tag == ARCHIVEGUARD_TAG_SERIALIZABLE {
			ar.level--
		}
	}
}

func (ar ArchiveGuard) Factory() SerializableFactory     { return ar.inner.Factory() }
func (ar ArchiveGuard) Error() error                     { return ar.inner.Error() }
func (ar ArchiveGuard) OnError(err error)                { ar.inner.OnError(err) }
func (ar ArchiveGuard) OnErrorf(msg string, args ...any) { ar.inner.OnErrorf(msg, args...) }
func (ar ArchiveGuard) HasTags(tags ...FourCC) bool      { return ar.inner.HasTags(tags...) }
func (ar ArchiveGuard) SetTags(tags ...FourCC)           { ar.inner.SetTags(tags...) }

func (ar ArchiveGuard) Flags() ArchiveFlags {
	flags := ar.inner.Flags()
	flags.Add(AR_GUARD)
	return flags
}

func (ar ArchiveGuard) Raw(value []byte) {
	Assert(func() bool { return len(value) > 0 })
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_RAW, "ar.Raw(%d)", len(value))()
	ar.inner.Raw(value)
}
func (ar ArchiveGuard) Byte(value *byte) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_BYTE, "ar.Byte()")()
	ar.inner.Byte(value)
}
func (ar ArchiveGuard) Bool(value *bool) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_BOOL, "ar.Bool()")()
	ar.inner.Bool(value)
}
func (ar ArchiveGuard) Int32(value *int32) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_INT32, "ar.Int32()")()
	ar.inner.Int32(value)
}
func (ar ArchiveGuard) Int64(value *int64) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_INT64, "ar.Int64()")()
	ar.inner.Int64(value)
}
func (ar ArchiveGuard) UInt32(value *uint32) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_UINT32, "ar.UInt32()")()
	ar.inner.UInt32(value)
}
func (ar ArchiveGuard) UInt64(value *uint64) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_UINT64, "ar.UInt64()")()
	ar.inner.UInt64(value)
}
func (ar ArchiveGuard) Float32(value *float32) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_FLOAT32, "ar.Float32()")()
	ar.inner.Float32(value)
}
func (ar ArchiveGuard) Float64(value *float64) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_FLOAT64, "ar.Float64()")()
	ar.inner.Float64(value)
}
func (ar ArchiveGuard) String(value *string) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_STRING, "ar.String()")()
	ar.inner.String(value)
}
func (ar ArchiveGuard) Time(value *time.Time) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_TIME, "ar.Time()")()
	ar.inner.Time(value)
}
func (ar ArchiveGuard) Serializable(value Serializable) {
	Assert(func() bool { return !IsNil(value) })
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_SERIALIZABLE, "ar.Serializable(%T)", value)()
	value.Serialize(ar) // don't use inner here, or recursive descent won't use guards
}
