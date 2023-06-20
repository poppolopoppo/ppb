package utils

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"
)

var archiveFlagsForTest = [][]ArchiveFlag{
	{},
	{AR_DETERMINISM},
}

type archiveFactoryFunc func(io.ReadWriter, ...ArchiveFlag) (name string, rd *ArchiveBinaryReader, wr *ArchiveBinaryWriter)

func archiveFactoryTestEquals[T any](t *testing.T, ar archiveFactoryFunc, serialize func(Archive, *T), equals func(T, T) bool, values ...T) {
	for _, flags := range archiveFlagsForTest {
		for _, value := range values {
			tmp := bytes.Buffer{}
			name, rd, wr := ar(&tmp, flags...)
			// t.Logf("test %s: %v", name, value)
			serialize(wr, &value)
			if err := wr.Close(); err != nil {
				t.Error(err)
			}
			var copy T
			serialize(rd, &copy)
			if err := rd.Close(); err != nil {
				t.Error(err)
			}
			if !equals(copy, value) {
				t.Errorf("invalid %s serialization: %v != %v", name, value, copy)
			}
		}
	}
}

func binaryArchiveFactory(rw io.ReadWriter, flags ...ArchiveFlag) (string, *ArchiveBinaryReader, *ArchiveBinaryWriter) {
	rd, wr := NewArchiveBinaryReader(rw, flags...), NewArchiveBinaryWriter(rw, flags...)
	return fmt.Sprintf(`raw binary [%v]`, MakeEnumSet(flags...)), &rd, &wr
}
func compressedArchiveFactory(options ...CompressionOptionFunc) archiveFactoryFunc {
	co := NewCompressionOptions(options...)
	return func(rw io.ReadWriter, flags ...ArchiveFlag) (string, *ArchiveBinaryReader, *ArchiveBinaryWriter) {
		rd, wr :=
			NewArchiveBinaryReader(NewCompressedReader(rw, options...), flags...),
			NewArchiveBinaryWriter(NewCompressedWriter(rw, options...), flags...)
		return fmt.Sprintf(`compressed %v:%v [%v]`, co.Format, co.Level, MakeEnumSet(flags...)), &rd, &wr
	}
}

const archiveTestCompressionLevels = true

func archiveTestEquals[T any](t *testing.T, serialize func(Archive, *T), equals func(T, T) bool, values ...T) {
	archiveFactoryTestEquals(t, binaryArchiveFactory, serialize, equals, values...)
	archiveFactoryTestEquals(t, compressedArchiveFactory(CompressionOptionFormat(COMPRESSION_FORMAT_LZ4)), serialize, equals, values...)
	archiveFactoryTestEquals(t, compressedArchiveFactory(CompressionOptionFormat(COMPRESSION_FORMAT_ZSTD)), serialize, equals, values...)
	if archiveTestCompressionLevels {
		archiveFactoryTestEquals(t, compressedArchiveFactory(CompressionOptionFormat(COMPRESSION_FORMAT_LZ4), CompressionOptionLevel(COMPRESSION_LEVEL_BALANCED)), serialize, equals, values...)
		archiveFactoryTestEquals(t, compressedArchiveFactory(CompressionOptionFormat(COMPRESSION_FORMAT_LZ4), CompressionOptionLevel(COMPRESSION_LEVEL_BEST)), serialize, equals, values...)
		archiveFactoryTestEquals(t, compressedArchiveFactory(CompressionOptionFormat(COMPRESSION_FORMAT_ZSTD), CompressionOptionLevel(COMPRESSION_LEVEL_BALANCED)), serialize, equals, values...)
		archiveFactoryTestEquals(t, compressedArchiveFactory(CompressionOptionFormat(COMPRESSION_FORMAT_ZSTD), CompressionOptionLevel(COMPRESSION_LEVEL_BEST)), serialize, equals, values...)
	}

}
func archiveTestComparable[T comparable](t *testing.T, serialize func(Archive, *T), values ...T) {
	archiveTestEquals(t, serialize, func(t1, t2 T) bool { return t1 == t2 }, values...)
}
func archiveTestEquatable[T Equatable[T]](t *testing.T, serialize func(Archive, *T), values ...T) {
	archiveTestEquals(t, serialize, func(t1, t2 T) bool { return t1.Equals(t2) }, values...)
}
func archiveTestSerializable[T Equatable[T], S interface {
	*T
	Serializable
}](t *testing.T, values ...T) {
	archiveTestEquals(t, func(a Archive, t *T) { S(t).Serialize(a) }, func(t1, t2 T) bool { return t1.Equals(t2) }, values...)
}

func TestArchiveBool(t *testing.T) {
	archiveTestComparable(t, func(ar Archive, i *bool) { ar.Bool(i) }, false, true)
}
func TestArchiveInt32(t *testing.T) {
	archiveTestComparable(t, func(ar Archive, i *int32) { ar.Int32(i) }, 0, 1, 32, -3, -128, 256, -256, -29983, 2897376, 1293812988, -1239848667)
}
func TestArchiveInt64(t *testing.T) {
	archiveTestComparable(t, func(ar Archive, i *int64) { ar.Int64(i) }, 0, 1, 32, -3, -128, 256, -256, -29983, 2897376, 1293812988, -1239848667, 908098918291828, -90891239848667)
}
func TestArchiveUInt32(t *testing.T) {
	archiveTestComparable(t, func(ar Archive, i *uint32) { ar.UInt32(i) }, 0, 1, 32, 3, 128, 256, 29983, 2897376, 1293812988, 473287239)
}
func TestArchiveUInt64(t *testing.T) {
	archiveTestComparable(t, func(ar Archive, i *uint64) { ar.UInt64(i) }, 0, 1, 32, 3, 128, 256, 29983, 2897376, 1293812988, 473287239, 908098918291828, 90891239848667)
}
func TestArchiveFloat32(t *testing.T) {
	archiveTestComparable(t, func(ar Archive, i *float32) { ar.Float32(i) }, 0, 1, -1, 1.12093129, 120983.123, -43298.4324)
}
func TestArchiveFloat64(t *testing.T) {
	archiveTestComparable(t, func(ar Archive, i *float32) { ar.Float32(i) }, 0, 1, -1, 1.12093129, 120983.123, -43298.4324, -543298123.432124, 12983845.520398)
}
func TestArchiveString(t *testing.T) {
	archiveTestComparable(t, func(ar Archive, i *string) { ar.String(i) }, "", "word", "text\nwith\tspaces\r\n")
}
func TestarchiveTime(t *testing.T) {
	archiveTestComparable(t, func(ar Archive, i *time.Time) { ar.Time(i) },
		time.UnixMilli(time.Date(2023, 12, 22, 23, 33, 12, 123, time.UTC).UnixMilli()),
		time.UnixMilli(time.UnixMicro(0).UnixMilli()),
		time.UnixMilli(time.Now().UnixMilli()))
}

func TestArchiveStringSet(t *testing.T) {
	archiveTestEquatable(t, func(ar Archive, i *StringSet) { ar.Serializable(i) },
		NewStringSet(),
		NewStringSet("word"),
		NewStringSet("", "word", "text\nwith\tspaces\r\n"),
		NewStringSet("word", "word", "word"),
		NewStringSet("word", "", "word"))
}
func TestArchiveFileSet(t *testing.T) {
	archiveTestSerializable(t,
		NewFileSet(),
		NewFileSet(MakeFilename("C:\\Test.txt")),
		NewFileSet(MakeFilename("C:\\Test.txt"), MakeFilename("D:\\Code\\..\\Test.txt")),
		NewFileSet(MakeFilename("C:\\Test.txt"), MakeFilename("C:\\Test.txt"), MakeFilename("C:\\Windows\\..\\Test.txt"), MakeFilename("D:\\Code\\PPE\\Test2.txt")))
}

type testSerializableMap struct {
	Items map[BuildAlias]Fingerprint
}

func (x testSerializableMap) Equals(other testSerializableMap) bool {
	if len(x.Items) != len(other.Items) {
		return false
	}
	for k, v1 := range x.Items {
		if v2, ok := other.Items[k]; !ok || v2 != v1 {
			return false
		}
	}
	return true
}
func (x *testSerializableMap) Serialize(ar Archive) {
	SerializeMap(ar, &x.Items)
}
func newTestSerializableMap(items map[BuildAlias]Fingerprint) testSerializableMap {
	return testSerializableMap{Items: items}
}

func TestArchiveMap(t *testing.T) {
	testMap := make(map[BuildAlias]Fingerprint, 3)
	testMap[MakeBuildAlias("Test", "First")] = StringFingerprint("Test/First")
	testMap[MakeBuildAlias("Second", "test")] = StringFingerprint("Second/test")
	testMap[MakeBuildAlias("finaL", "test", "end")] = StringFingerprint("finaL/test/end")

	archiveTestSerializable(t,
		newTestSerializableMap(map[BuildAlias]Fingerprint{}),
		newTestSerializableMap(make(map[BuildAlias]Fingerprint, 0)),
		newTestSerializableMap(testMap))
}
