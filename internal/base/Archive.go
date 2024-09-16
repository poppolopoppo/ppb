package base

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"time"
)

/***************************************
 * ArchiveBinaryWriter
 ***************************************/

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

const AR_USE_COMPACT_INDICES = false // saves about 8% on -configure output db, but slower due to consecutive ar.Byte() calls instead of fixed ar.Int()

type ArchiveBinaryReader struct {
	reader        io.Reader
	indexToString []string
	basicArchive
}

func ArchiveBinaryRead(reader io.Reader, scope func(ar Archive), flags ...ArchiveFlag) (err error) {
	return Recover(func() error {
		ar := NewArchiveBinaryReader(reader, flags...)
		defer ar.Close()
		scope(&ar)
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
	raw := (*ar.bytes)[:1]
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
		raw := (*ar.bytes)[:INT32_SIZE]
		ar.Raw(raw)
		*value = int32(binary.LittleEndian.Uint32(raw))
	}
}
func (ar *ArchiveBinaryReader) Int64(value *int64) {
	if AR_USE_COMPACT_INDICES {
		SerializeCompactSigned(ar, value)
	} else {
		raw := (*ar.bytes)[:INT64_SIZE]
		ar.Raw(raw)
		*value = int64(binary.LittleEndian.Uint64(raw))
	}
}
func (ar *ArchiveBinaryReader) UInt32(value *uint32) {
	if AR_USE_COMPACT_INDICES {
		SerializeCompactUnsigned(ar, value)
	} else {
		raw := (*ar.bytes)[:UINT32_SIZE]
		ar.Raw(raw)
		*value = binary.LittleEndian.Uint32(raw)
	}
}
func (ar *ArchiveBinaryReader) UInt64(value *uint64) {
	if AR_USE_COMPACT_INDICES {
		SerializeCompactUnsigned(ar, value)
	} else {
		raw := (*ar.bytes)[:UINT64_SIZE]
		ar.Raw(raw)
		*value = binary.LittleEndian.Uint64(raw)
	}
}
func (ar *ArchiveBinaryReader) Float32(value *float32) {
	raw := (*ar.bytes)[:FLOAT32_SIZE]
	ar.Raw(raw)
	*value = math.Float32frombits(binary.LittleEndian.Uint32(raw))
}
func (ar *ArchiveBinaryReader) Float64(value *float64) {
	raw := (*ar.bytes)[:FLOAT64_SIZE]
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
	ar.Raw((*ar.bytes)[:size])
	*value = string((*ar.bytes)[:size])

	// record the string for future occurrences
	ar.indexToString = append(ar.indexToString, *value)
}
func (ar *ArchiveBinaryReader) Time(value *time.Time) {
	raw := (*ar.bytes)[:INT64_SIZE]
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
	stringToIndex   map[string]int32
	hasStringWriter bool
	basicArchive
}

func ArchiveBinaryWrite(writer io.Writer, scope func(ar Archive)) (err error) {
	return Recover(func() error {
		ar := NewArchiveBinaryWriter(writer)
		defer ar.Close()
		scope(&ar)
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
	raw := (*ar.bytes)[:BYTE_SIZE]
	raw[0] = *value
	if _, err := ar.writer.Write(raw); err != nil {
		ar.onError(err)
	}
}
func (ar *ArchiveBinaryWriter) Bool(value *bool) {
	raw := (*ar.bytes)[:BOOL_SIZE]
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
		raw := (*ar.bytes)[:INT32_SIZE]
		binary.LittleEndian.PutUint32(raw, uint32(*value))
		ar.Raw(raw)
	}
}
func (ar *ArchiveBinaryWriter) Int64(value *int64) {
	if AR_USE_COMPACT_INDICES {
		SerializeCompactSigned(ar, value)
	} else {
		raw := (*ar.bytes)[:INT64_SIZE]
		binary.LittleEndian.PutUint64(raw, uint64(*value))
		ar.Raw(raw)
	}
}
func (ar *ArchiveBinaryWriter) UInt32(value *uint32) {
	if AR_USE_COMPACT_INDICES {
		SerializeCompactUnsigned(ar, value)
	} else {
		raw := (*ar.bytes)[:UINT32_SIZE]
		binary.LittleEndian.PutUint32(raw, *value)
		ar.Raw(raw)
	}
}
func (ar *ArchiveBinaryWriter) UInt64(value *uint64) {
	if AR_USE_COMPACT_INDICES {
		SerializeCompactUnsigned(ar, value)
	} else {
		raw := (*ar.bytes)[:UINT64_SIZE]
		binary.LittleEndian.PutUint64(raw, *value)
		ar.Raw(raw)
	}
}
func (ar *ArchiveBinaryWriter) Float32(value *float32) {
	raw := (*ar.bytes)[:FLOAT32_SIZE]
	binary.LittleEndian.PutUint32(raw, math.Float32bits(*value))
	ar.Raw(raw)
}
func (ar *ArchiveBinaryWriter) Float64(value *float64) {
	raw := (*ar.bytes)[:FLOAT64_SIZE]
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
	raw := (*ar.bytes)[:INT64_SIZE]
	binary.LittleEndian.PutUint64(raw, uint64(value.UnixMilli()))
	ar.Raw(raw)
}
func (ar *ArchiveBinaryWriter) Serializable(value Serializable) {
	value.Serialize(ar)
}
