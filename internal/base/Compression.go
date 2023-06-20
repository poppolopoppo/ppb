package base

import (
	"io"
	"strings"

	"github.com/DataDog/zstd"
	"github.com/pierrec/lz4/v4"
)

var LogCompression = NewLogCategory("Compression")

type CompressedReader interface {
	io.ReadCloser
}
type CompressedWriter interface {
	Flush() error
	io.WriteCloser
}

type CompressionOptions struct {
	Format     CompressionFormat
	Level      CompressionLevel
	Dictionary []byte
}

type CompressionOptionFunc func(*CompressionOptions)

func CompressionOptionFormat(fmt CompressionFormat) CompressionOptionFunc {
	return func(co *CompressionOptions) {
		Overwrite(&co.Format, fmt)
	}
}
func CompressionOptionLevel(lvl CompressionLevel) CompressionOptionFunc {
	return func(co *CompressionOptions) {
		Overwrite(&co.Level, lvl)
	}
}
func CompressionOptionDictionary(dict []byte) CompressionOptionFunc {
	return func(co *CompressionOptions) {
		co.Dictionary = dict
	}
}

/*
func CompressionOptionDictionaryFile(f Filename) CompressionOptionFunc {
	return func(co *CompressionOptions) {
		defer base.LogBenchmark(LogCompression, "load zstd dictionary located at %q", f).Close()
		dict, err := UFS.ReadAll(UFS.Internal.Folder("zstd").File("ppb-message-dict.zstd"))
		base.LogPanicIfFailed(LogCompression, err)
		co.Dictionary = dict
	}
}
*/

func NewCompressionOptions(options ...CompressionOptionFunc) (result CompressionOptions) {
	// Lz4 is almost as fast as uncompressed, but with fewer IO: when using Fast speed it is almost always a free win
	result.Format = COMPRESSION_FORMAT_LZ4
	result.Level = COMPRESSION_LEVEL_FAST

	for _, opt := range options {
		opt(&result)
	}
	return
}
func (x *CompressionOptions) Options(co *CompressionOptions) {
	*co = *x
}

func NewCompressedReader(reader io.Reader, options ...CompressionOptionFunc) CompressedReader {
	co := NewCompressionOptions(options...)
	switch co.Format {

	case COMPRESSION_FORMAT_LZ4:
		return NewLz4Reader(reader)

	case COMPRESSION_FORMAT_ZSTD:
		if co.Dictionary == nil {
			return NewZStdReader(reader)
		} else {
			return NewZStdReaderDict(reader, co.Dictionary)
		}

	default:
		UnexpectedValuePanic(co.Format, co.Format)
		return nil
	}
}

func NewCompressedWriter(writer io.Writer, options ...CompressionOptionFunc) CompressedWriter {
	co := NewCompressionOptions(options...)
	switch co.Format {

	case COMPRESSION_FORMAT_LZ4:
		return NewLz4Writer(writer, co.Level)

	case COMPRESSION_FORMAT_ZSTD:
		if co.Dictionary == nil {
			return NewZStdWriter(writer, co.Level)
		} else {
			return NewZStdWriterDict(writer, co.Level, co.Dictionary)
		}

	default:
		UnexpectedValuePanic(co.Format, co.Format)
		return nil
	}
}

/***************************************
 * LZ4 Compression Pool
 ***************************************/

func NewLz4Reader(reader io.Reader) CompressedReader {
	result := transientLz4Reader{TransientLz4Reader.Allocate()}
	result.Reset(reader)
	return result
}
func NewLz4Writer(writer io.Writer, lvl CompressionLevel) CompressedWriter {
	result := transientLz4Writer{TransientLz4Writer.Allocate()}
	switch lvl {
	case COMPRESSION_LEVEL_FAST:
		result.Apply(lz4.CompressionLevelOption(lz4.Fast))
	case COMPRESSION_LEVEL_BALANCED:
		result.Apply(lz4.CompressionLevelOption(lz4.Level3))
	case COMPRESSION_LEVEL_BEST:
		result.Apply(lz4.CompressionLevelOption(lz4.Level7))
	}
	result.Reset(writer)
	return result
}

type transientLz4Reader struct {
	*lz4.Reader
}

// https://indico.fnal.gov/event/16264/contributions/36466/attachments/22610/28037/Zstd__LZ4.pdf
// var lz4CompressionLevelOptionDefault = lz4.CompressionLevelOption(lz4.Level4) // Level4 is already very slow (1.21 Gb in 359s)
var lz4CompressionLevelOptionDefault = lz4.CompressionLevelOption(lz4.Fast) // Fast is... fast ^^ (1.40 Gb in 52s)

func applyLz4Options(lz interface {
	Apply(...lz4.Option) error
}, options ...lz4.Option) {
	options = append(options, lz4.ConcurrencyOption(1))
	err := lz.Apply(options...)
	LogPanicIfFailed(LogCompression, err)
}

func (x transientLz4Reader) Close() error {
	TransientLz4Reader.Release(x.Reader)
	return nil
}

var TransientLz4Reader = NewRecycler[*lz4.Reader](
	func() *lz4.Reader {
		r := lz4.NewReader(nil)
		applyLz4Options(r)
		return r
	},
	func(r *lz4.Reader) {
		r.Reset(nil)
		applyLz4Options(r)
	})

type transientLz4Writer struct {
	*lz4.Writer
}

func (x transientLz4Writer) Close() (err error) {
	defer TransientLz4Writer.Release(x.Writer)
	return x.Writer.Close()
}

var TransientLz4Writer = NewRecycler[*lz4.Writer](
	func() *lz4.Writer {
		w := lz4.NewWriter(nil)
		applyLz4Options(w,
			lz4CompressionLevelOptionDefault,
			lz4.BlockSizeOption(LARGE_PAGE_CAPACITY),
			lz4.ChecksumOption(false))
		return w
	},
	func(w *lz4.Writer) {
		w.Close()
		w.Reset(nil)
		applyLz4Options(w,
			lz4CompressionLevelOptionDefault,
			lz4.BlockSizeOption(LARGE_PAGE_CAPACITY),
			lz4.ChecksumOption(false))
	})

/***************************************
 * ZSTD Compression Pool
 ***************************************/

var LogZStd = NewLogCategory(`zstd`)

var zstdCompressionLevelDefault = zstd.DefaultCompression

func getZStdCompressionLevel(lvl CompressionLevel) (result int) {
	result = zstdCompressionLevelDefault
	switch lvl {
	case COMPRESSION_LEVEL_FAST:
		result = zstd.BestSpeed
	case COMPRESSION_LEVEL_BALANCED:
		result = zstd.DefaultCompression
	case COMPRESSION_LEVEL_BEST:
		result = zstd.BestCompression
	}
	return
}

func NewZStdReader(reader io.Reader) CompressedReader {
	return zstd.NewReader(reader)
}
func NewZStdWriter(writer io.Writer, lvl CompressionLevel) CompressedWriter {
	result := zstd.NewWriterLevel(writer, getZStdCompressionLevel(lvl))
	result.SetNbWorkers(1)
	return result
}

func NewZStdReaderDict(reader io.Reader, dictionary []byte) CompressedReader {
	return zstd.NewReaderDict(reader, dictionary)
}
func NewZStdWriterDict(writer io.Writer, lvl CompressionLevel, dictionary []byte) CompressedWriter {
	result := zstd.NewWriterLevelDict(writer, getZStdCompressionLevel(lvl), dictionary)
	result.SetNbWorkers(1)
	return result
}

/***************************************
 * CompressionLevelType
 ***************************************/

type CompressionLevel int32

const (
	COMPRESSION_LEVEL_INHERIT CompressionLevel = iota
	COMPRESSION_LEVEL_FAST
	COMPRESSION_LEVEL_BALANCED
	COMPRESSION_LEVEL_BEST
)

func CompressionLevels() []CompressionLevel {
	return []CompressionLevel{
		COMPRESSION_LEVEL_INHERIT,
		COMPRESSION_LEVEL_FAST,
		COMPRESSION_LEVEL_BALANCED,
		COMPRESSION_LEVEL_BEST,
	}
}
func (x CompressionLevel) Description() string {
	switch x {
	case COMPRESSION_LEVEL_INHERIT:
		return "inherit default value from configuration"
	case COMPRESSION_LEVEL_FAST:
		return "faster compression times with lower compression ratio"
	case COMPRESSION_LEVEL_BALANCED:
		return "balance between times ratio and compression ratio"
	case COMPRESSION_LEVEL_BEST:
		return "best compression ratio possible, but much slower compression times"
	default:
		UnexpectedValue(x)
		return ""
	}
}
func (x CompressionLevel) String() string {
	switch x {
	case COMPRESSION_LEVEL_INHERIT:
		return "INHERIT"
	case COMPRESSION_LEVEL_FAST:
		return "FAST"
	case COMPRESSION_LEVEL_BALANCED:
		return "BALANCED"
	case COMPRESSION_LEVEL_BEST:
		return "BEST"
	default:
		UnexpectedValue(x)
		return ""
	}
}
func (x CompressionLevel) IsInheritable() bool {
	return x == COMPRESSION_LEVEL_INHERIT
}
func (x *CompressionLevel) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case COMPRESSION_LEVEL_INHERIT.String():
		*x = COMPRESSION_LEVEL_INHERIT
	case COMPRESSION_LEVEL_FAST.String():
		*x = COMPRESSION_LEVEL_FAST
	case COMPRESSION_LEVEL_BALANCED.String():
		*x = COMPRESSION_LEVEL_BALANCED
	case COMPRESSION_LEVEL_BEST.String():
		*x = COMPRESSION_LEVEL_BEST
	default:
		err = MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *CompressionLevel) Serialize(ar Archive) {
	ar.Int32((*int32)(x))
}
func (x CompressionLevel) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *CompressionLevel) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}
func (x *CompressionLevel) AutoComplete(in AutoComplete) {
	for _, it := range CompressionLevels() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * CompressionFormat
 ***************************************/

type CompressionFormat int32

const (
	COMPRESSION_FORMAT_INHERIT CompressionFormat = iota
	COMPRESSION_FORMAT_LZ4
	COMPRESSION_FORMAT_ZSTD
)

func CompressionFormats() []CompressionFormat {
	return []CompressionFormat{
		COMPRESSION_FORMAT_INHERIT,
		COMPRESSION_FORMAT_LZ4,
		COMPRESSION_FORMAT_ZSTD,
	}
}
func (x CompressionFormat) Description() string {
	switch x {
	case COMPRESSION_FORMAT_INHERIT:
		return "inherit default value from configuration"
	case COMPRESSION_FORMAT_LZ4:
		return "use extremely fast LZ4 compression from https://github.com/lz4/lz4"
	case COMPRESSION_FORMAT_ZSTD:
		return "use facebook ZStandard compression from https://github.com/facebook/zstd"
	default:
		UnexpectedValue(x)
		return ""
	}
}
func (x CompressionFormat) String() string {
	switch x {
	case COMPRESSION_FORMAT_INHERIT:
		return "INHERIT"
	case COMPRESSION_FORMAT_LZ4:
		return "LZ4"
	case COMPRESSION_FORMAT_ZSTD:
		return "ZSTD"
	default:
		UnexpectedValue(x)
		return ""
	}
}
func (x CompressionFormat) IsInheritable() bool {
	return x == COMPRESSION_FORMAT_INHERIT
}
func (x *CompressionFormat) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case COMPRESSION_FORMAT_INHERIT.String():
		*x = COMPRESSION_FORMAT_INHERIT
	case COMPRESSION_FORMAT_LZ4.String():
		*x = COMPRESSION_FORMAT_LZ4
	case COMPRESSION_FORMAT_ZSTD.String():
		*x = COMPRESSION_FORMAT_ZSTD
	default:
		err = MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *CompressionFormat) Serialize(ar Archive) {
	ar.Int32((*int32)(x))
}
func (x CompressionFormat) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *CompressionFormat) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}
func (x *CompressionFormat) AutoComplete(in AutoComplete) {
	for _, it := range CompressionFormats() {
		in.Add(it.String(), it.Description())
	}
}
