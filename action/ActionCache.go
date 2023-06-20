package action

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"archive/zip"

	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var LogActionCache = base.NewLogCategory("ActionCache")

/***************************************
 * ActionCache
 ***************************************/

const ACTIONCACHE_BULK_EXTNAME = ".bulk"
const ACTIONCACHE_ENTRY_EXTNAME = ".cache"

type ActionCache interface {
	GetCachePath() Directory
	GetCacheStats() *ActionCacheStats
	GetEntryExtname() string
	GetBulkExtname() string
	CacheRead(a *ActionRules, artifacts FileSet, options ...BuildOptionFunc) (ActionCacheKey, error)
	CacheWrite(action BuildAlias, key ActionCacheKey, artifacts FileSet, inputs FileSet, options ...BuildOptionFunc) error
	AsyncCacheWrite(node BuildNode, key ActionCacheKey, artifacts FileSet, options ...BuildOptionFunc) error
}

var actionCacheStats *ActionCacheStats

type actionCache struct {
	path  Directory
	seed  base.Fingerprint
	stats ActionCacheStats
}

var GetActionCache = base.Memoize(func() ActionCache {
	result := BuildActionCache(GetActionFlags().CachePath).Build(CommandEnv.BuildGraph())
	if result.Failure() == nil {
		// store global access to cache stats
		actionCacheStats = &result.Success().stats
		// print cache stats upon exit if specified on command-line
		if GetCommandFlags().Summary.Get() {
			CommandEnv.OnExit(func(*CommandEnvT) error {
				result.Success().stats.Print()
				return nil
			})
		}
	}
	return result.Success()
})

func BuildActionCache(path Directory) BuildFactoryTyped[*actionCache] {
	return MakeBuildFactory(func(bi BuildInitializer) (actionCache, error) {
		return actionCache{
			path: path,
		}, nil
	})
}

func (x *actionCache) Alias() BuildAlias {
	return MakeBuildAlias("Cache", "Actions", x.path.String())
}
func (x *actionCache) Serialize(ar base.Archive) {
	ar.Serializable(&x.path)
	ar.Serializable(&x.seed)
}
func (x *actionCache) Build(bc BuildContext) error {
	return internal_io.CreateDirectory(bc, x.path)
}

func (x *actionCache) GetEntryExtname() string {
	return ACTIONCACHE_ENTRY_EXTNAME
}
func (x *actionCache) GetBulkExtname() string {
	return ACTIONCACHE_BULK_EXTNAME
}

func (x *actionCache) GetCacheStats() *ActionCacheStats {
	return &x.stats
}
func (x *actionCache) GetCachePath() Directory {
	return x.path
}

func (x *actionCache) CacheRead(a *ActionRules, artifacts FileSet, options ...BuildOptionFunc) (key ActionCacheKey, err error) {
	readStat := StartBuildStats()
	defer x.stats.CacheRead.Append(&readStat)

	key = x.makeActionKey(a, options...)
	entry, err := x.fetchCacheEntry(key, false)
	if err == nil {
		err = entry.CacheRead(a, artifacts, options...)
	}

	if err == nil {
		base.LogTrace(LogActionCache, "cache hit for %q", a.Alias())
		base.LogDebug(LogActionCache, "%q has the following artifacts ->\n\t - %v", a.Alias(), base.MakeStringer(func() string {
			return base.JoinString("\n\t - ", artifacts.Slice()...)
		}))

		atomic.AddInt32(&x.stats.CacheHit, 1)
	} else {
		base.LogTrace(LogActionCache, "%v", err)
		atomic.AddInt32(&x.stats.CacheMiss, 1)
	}
	return
}
func (x *actionCache) CacheWrite(action BuildAlias, key ActionCacheKey, artifacts FileSet, inputs FileSet, options ...BuildOptionFunc) (err error) {
	scopedStat := StartBuildStats()
	defer x.stats.CacheWrite.Append(&scopedStat)

	if entry, err := x.fetchCacheEntry(key, true); err == nil {
		var dirty bool
		if dirty, err = entry.CacheWrite(x.path, inputs, artifacts, options...); err == nil {

			if dirty {
				if err = entry.writeCacheEntry(x.path); err == nil {
					atomic.AddInt32(&x.stats.CacheStore, 1)
				}
			}
		}
	}

	if err == nil {
		base.LogTrace(LogActionCache, "cache store for %q", action)
		base.LogDebug(LogActionCache, "%q has the following artifacts ->\n\t - %v", action, base.MakeStringer(func() string {
			return base.JoinString("\n\t - ", artifacts.Slice()...)
		}))
	} else {
		base.LogError(LogActionCache, "failed to cache in store %q with %v", action, err)
	}
	return
}
func (x *actionCache) AsyncCacheWrite(node BuildNode, cacheKey ActionCacheKey, artifacts FileSet, options ...BuildOptionFunc) error {
	action := node.Alias()
	base.GetGlobalThreadPool().Queue(func(base.ThreadContext) {
		inputs, err := CommandEnv.BuildGraph().GetDependencyInputFiles(action)
		if err == nil {
			inputs.Sort()
			err = x.CacheWrite(action, cacheKey, artifacts, inputs, options...)
		}
		base.LogPanicIfFailed(LogActionCache, err)
	})
	return nil
}
func (x *actionCache) fetchCacheEntry(cacheKey ActionCacheKey, writing bool) (ActionCacheEntry, error) {
	entry := ActionCacheEntry{Key: cacheKey}
	if err := entry.readCacheEntry(x.path); err != nil {
		if writing {
			err = nil
		} else {
			return entry, err
		}
	}
	return entry, nil
}
func (x *actionCache) makeActionKey(a *ActionRules, options ...BuildOptionFunc) ActionCacheKey {
	bg := CommandEnv.BuildGraph()

	fingerprint, err := base.SerializeAnyFingerprint(func(ar base.Archive) error {
		// serialize action inputs
		a.Serialize(ar)

		// serialize all input files content
		return internal_io.ForeachBuildFileDigest(bg, len(a.Inputs),
			func(i int) Filename {
				return a.Inputs[i]
			},
			func(i int, fd *internal_io.FileDigest) error {
				base.AssertIn(fd.Source, a.Inputs[i])
				base.Assert(fd.Digest.Valid)

				base.LogDebug(LogActionCache, "file digest for %q is %v", fd.Source, fd.Digest)
				ar.Serializable(fd)
				return nil
			},
			options...)
	}, x.seed)
	base.LogPanicIfFailed(LogActionCache, err)

	base.LogTrace(LogActionCache, "key: %v -> action: %q", ActionCacheKey(fingerprint), a.Alias())
	return ActionCacheKey(fingerprint)
}

/***************************************
 * ActionCacheKey
 ***************************************/

type ActionCacheKey base.Fingerprint

func (x *ActionCacheKey) Serialize(ar base.Archive) {
	ar.Serializable((*base.Fingerprint)(x))
}
func (x ActionCacheKey) String() string {
	return x.GetFingerprint().ShortString()
}
func (x ActionCacheKey) GetFingerprint() base.Fingerprint {
	return (base.Fingerprint)(x)
}

func (x ActionCacheKey) GetEntryPath(cachePath Directory) Filename {
	return makeCachePath(cachePath, x.GetFingerprint(), ACTIONCACHE_ENTRY_EXTNAME)
}

func makeCachePath(cachePath Directory, h base.Fingerprint, extname string) Filename {
	str := h.String()
	return cachePath.Folder(str[0:2]).Folder(str[2:4]).File(str).ReplaceExt(extname)
}

/***************************************
 * ActionCacheBulk
 ***************************************/

type ActionCacheBulk struct {
	Path   Filename
	Inputs []internal_io.FileDigest
}

func NewActionCacheBulk(cachePath Directory, key ActionCacheKey, inputs FileSet, options ...BuildOptionFunc) (bulk ActionCacheBulk, err error) {
	bulk.Inputs = make([]internal_io.FileDigest, len(inputs))

	var fingerprint base.Fingerprint
	fingerprint, err = base.SerializeAnyFingerprint(func(ar base.Archive) error {
		return internal_io.ForeachBuildFileDigest(CommandEnv.BuildGraph(), len(inputs),
			func(i int) Filename {
				return inputs[i]
			},
			func(i int, fd *internal_io.FileDigest) error {
				base.AssertIn(fd.Source, inputs[i])
				base.Assert(fd.Digest.Valid)

				bulk.Inputs[i] = *fd
				ar.Serializable(&bulk.Inputs[i])
				return nil
			},
			options...)
	}, key.GetFingerprint() /* use action key as bulk key seed */)

	if err == nil {
		bulk.Path = makeCachePath(cachePath, fingerprint, ACTIONCACHE_BULK_EXTNAME)
	}
	return
}
func (x *ActionCacheBulk) Equals(y ActionCacheBulk) bool {
	return x.Path.Equals(y.Path)
}
func (x *ActionCacheBulk) CacheHit(options ...BuildOptionFunc) bool {

	err := internal_io.ForeachBuildFileDigest(CommandEnv.BuildGraph(), len(x.Inputs),
		func(i int) Filename { return x.Inputs[i].Source },
		func(i int, fd *internal_io.FileDigest) error {
			base.AssertIn(fd.Source, x.Inputs[i].Source)
			base.Assert(fd.Digest.Valid)

			if fd.Digest == x.Inputs[i].Digest {
				return nil
			}
			return fmt.Errorf("cache-miss due to %q, recorded %v but computed %v",
				x.Inputs[i].Source, x.Inputs[i].Digest, fd.Digest)
		},
		options...)

	if err != nil {
		base.LogWarning(LogActionCache, "cache-miss error: %v", err)
		return false
	}
	return true
}
func (x *ActionCacheBulk) Deflate(root Directory, artifacts ...Filename) error {
	deflateStat := StartBuildStats()
	defer actionCacheStats.CacheDeflate.Append(&deflateStat)

	return UFS.CreateBuffered(x.Path, func(w io.Writer) error {
		compression := GetCacheCompression()

		zw := zip.NewWriter(w)
		zw.RegisterCompressor(compression.Method, compression.Compressor)

		for _, file := range artifacts {
			info, err := file.Info()
			if err != nil {
				return err
			}

			name := file.Relative(root)
			header := &zip.FileHeader{
				Name:     name,
				Method:   compression.Method,
				Modified: info.ModTime().UTC(), // keep modified time stable when restoring cache artifacts
			}

			w, err := zw.CreateHeader(header)
			if err != nil {
				return err
			}

			if err := UFS.Open(file, func(r io.Reader) error {
				return base.CopyWithProgress(file.String(), info.Size(), w, r)
			}); err != nil {
				return err
			}
		}

		return zw.Close()
	})
}
func (x *ActionCacheBulk) Inflate(dst Directory) (FileSet, error) {
	inflateStat := StartBuildStats()
	defer actionCacheStats.CacheInflate.Append(&inflateStat)

	var artifacts FileSet
	return artifacts, UFS.OpenFile(x.Path, func(r *os.File) error {
		info, err := r.Stat()
		if err != nil {
			return err
		}

		zr, err := zip.NewReader(r, info.Size())
		if err != nil {
			return err
		}

		if err := ForeachCacheCompression(func(cc CacheCompression) error {
			zr.RegisterDecompressor(cc.Method, cc.Decompressor)
			return nil
		}); err != nil {
			return err
		}

		for _, file := range zr.File {
			rc, err := file.Open()
			if err != nil {
				return err
			}

			dst := dst.AbsoluteFile(file.Name)
			err = UFS.CreateFile(dst, func(w *os.File) error {
				// this is much faster than separate UFS.MTime()/os.Chtimes(), but OS dependant...
				if err := base.SetMTime(w, file.Modified); err != nil {
					return err
				}
				return base.CopyWithProgress(dst.String(), int64(file.UncompressedSize64), w, rc)
			})
			rc.Close()

			if err != nil {
				return err
			}

			artifacts.Append(dst)
		}

		return nil
	})
}
func (x *ActionCacheBulk) Serialize(ar base.Archive) {
	ar.Serializable(&x.Path)
	base.SerializeSlice(ar, &x.Inputs)
}

/***************************************
 * ActionCacheEntry
 ***************************************/

type ActionCacheEntry struct {
	Key   ActionCacheKey
	Bulks []ActionCacheBulk
}

func (x *ActionCacheEntry) Serialize(ar base.Archive) {
	ar.Serializable((*base.Fingerprint)(&x.Key))
	base.SerializeSlice(ar, &x.Bulks)
}
func (x *ActionCacheEntry) CacheRead(a *ActionRules, artifacts FileSet, options ...BuildOptionFunc) error {
	for _, bulk := range x.Bulks {
		if bulk.CacheHit(options...) {
			retrieved, err := bulk.Inflate(UFS.Root)

			if err == nil && !retrieved.Equals(artifacts) {
				err = fmt.Errorf("action-cache: artifacts file set do not match for action %q", a.Alias())
			}

			return err
		}
	}
	return fmt.Errorf("action-cache: cache miss for action %q, recompiling", a.Alias())
}
func (x *ActionCacheEntry) CacheWrite(cachePath Directory, inputs FileSet, artifacts FileSet, options ...BuildOptionFunc) (bool, error) {
	bulk, err := NewActionCacheBulk(cachePath, x.Key, inputs)
	if err != nil {
		return false, err
	}

	dirty := true
	for i, b := range x.Bulks {
		// check if the same bulk is already present
		if b.Equals(bulk) {
			dirty = len(b.Inputs) != len(bulk.Inputs)
			if !dirty {
				// check if bulk with the same has also the same inputs
				for j, it := range b.Inputs {
					jt := bulk.Inputs[j]
					if !it.Source.Equals(jt.Source) || it.Digest != jt.Digest {
						dirty = true
						break
					}
				}
			}
			if dirty {
				x.Bulks[i] = bulk
			}
			break
		}
	}

	if dirty {
		err = bulk.Deflate(UFS.Root, artifacts...)

		x.Bulks = append(x.Bulks, bulk)
	}
	return dirty, err
}

func (x *ActionCacheEntry) Load(src Filename) error {
	benchmark := base.LogBenchmark(LogActionCache, "read cache entry with key %q", x.Key)
	defer benchmark.Close()

	return UFS.Open(src, func(r io.Reader) error {
		_, err := base.CompressedArchiveFileRead(r, func(ar base.Archive) {
			ar.Serializable(x)
		})
		return err
	})
}
func (x *ActionCacheEntry) readCacheEntry(cachePath Directory) error {
	path := x.Key.GetEntryPath(cachePath)

	if !path.Exists() {
		return fmt.Errorf("no cache entry with key %q", x.Key)
	}

	return x.Load(path)
}
func (x *ActionCacheEntry) writeCacheEntry(cachePath Directory) error {
	path := x.Key.GetEntryPath(cachePath)

	benchmark := base.LogBenchmark(LogActionCache, "store cache entry with key %q", x.Key)
	defer benchmark.Close()

	return UFS.Create(path, func(w io.Writer) error {
		return base.CompressedArchiveFileWrite(w, func(ar base.Archive) {
			ar.Serializable(x)
		})
	})
}

/***************************************
 * ActionCacheStats
 ***************************************/

type ActionCacheStats struct {
	CacheRead    BuildStats
	CacheInflate BuildStats

	CacheWrite   BuildStats
	CacheDeflate BuildStats

	CacheHit   int32
	CacheMiss  int32
	CacheStore int32

	CacheReadCompressed   uint64
	CacheReadUncompressed uint64

	CacheWriteCompressed   uint64
	CacheWriteUncompressed uint64
}

func (x *ActionCacheStats) StatRead(compressed, uncompressed int) {
	if compressed > 0 {
		atomic.AddUint64(&x.CacheReadCompressed, uint64(compressed))
	}
	if uncompressed > 0 {
		atomic.AddUint64(&x.CacheReadUncompressed, uint64(uncompressed))
	}
}
func (x *ActionCacheStats) StatWrite(compressed, uncompressed int) {
	if compressed > 0 {
		atomic.AddUint64(&x.CacheWriteCompressed, uint64(compressed))
	}
	if uncompressed > 0 {
		atomic.AddUint64(&x.CacheWriteUncompressed, uint64(uncompressed))
	}
}
func (x *ActionCacheStats) Print() {
	base.LogForwardf("\nAction cache was hit %d times and missed %d times, stored %d new cache entries (hit rate: %.3f%%)",
		x.CacheHit, x.CacheMiss, x.CacheStore,
		100*float32(x.CacheHit)/float32(x.CacheHit+x.CacheMiss))

	base.LogForwardf("   READ <==  %8.3f seconds - %5d cache entries",
		x.CacheRead.Duration.Exclusive.Seconds(), x.CacheRead.Count)
	base.LogForwardf("INFLATE  ->  %8.3f seconds - %5d cache bulks    - %8.3f MiB/Sec  - %8.3f MiB  ->> %9.3f MiB  (x%4.2f)",
		x.CacheInflate.Duration.Exclusive.Seconds(), x.CacheInflate.Count,
		base.MebibytesPerSec(uint64(x.CacheReadUncompressed), x.CacheInflate.Duration.Exclusive),
		base.Mebibytes(x.CacheReadCompressed),
		base.Mebibytes(x.CacheReadUncompressed),
		float64(x.CacheReadUncompressed)/(float64(x.CacheReadCompressed)+0.00001))

	base.LogForwardf("  WRITE ==>  %8.3f seconds - %5d cache entries",
		x.CacheWrite.Duration.Exclusive.Seconds(), x.CacheWrite.Count)
	base.LogForwardf("DEFLATE <-   %8.3f seconds - %5d cache bulks    - %8.3f MiB/Sec  - %8.3f MiB <<-  %9.3f MiB  (x%4.2f)",
		x.CacheDeflate.Duration.Exclusive.Seconds(), x.CacheDeflate.Count,
		base.MebibytesPerSec(uint64(x.CacheWriteUncompressed), x.CacheDeflate.Duration.Exclusive),
		base.Mebibytes(x.CacheWriteCompressed),
		base.Mebibytes(x.CacheWriteUncompressed),
		float64(x.CacheWriteUncompressed)/(float64(x.CacheWriteCompressed)+0.00001))
}

/***************************************
 * CacheCompression
 ***************************************/

type CacheCompression struct {
	Method       uint16
	Compressor   zip.Compressor
	Decompressor zip.Decompressor
}

func newCacheCompressor(compressor func(writer io.Writer, lvl base.CompressionLevel) base.CompressedWriter, lvl base.CompressionLevel) zip.Compressor {
	if GetCommandFlags().Summary.Get() {
		return func(w io.Writer) (io.WriteCloser, error) {
			return internal_io.NewObservableWriter(compressor(internal_io.NewObservableWriter(w,
				func(w io.Writer, buf []byte) (n int, err error) {
					n, err = w.Write(buf)
					actionCacheStats.StatWrite(n, 0)
					return
				}), lvl),
				func(w io.Writer, buf []byte) (n int, err error) {
					n, err = w.Write(buf)
					actionCacheStats.StatWrite(0, n)
					return
				}), nil
		}
	} else {
		return func(w io.Writer) (io.WriteCloser, error) {
			return compressor(w, lvl), nil
		}
	}

}
func newCacheDecompressor(decompressor func(reader io.Reader) base.CompressedReader) zip.Decompressor {
	if GetCommandFlags().Summary.Get() {
		return func(r io.Reader) io.ReadCloser {
			return internal_io.NewObservableReader(decompressor(internal_io.NewObservableReader(r,
				func(r io.Reader, buf []byte) (n int, err error) {
					n, err = r.Read(buf)
					actionCacheStats.StatRead(n, 0)
					return
				})),
				func(r io.Reader, buf []byte) (n int, err error) {
					n, err = r.Read(buf)
					actionCacheStats.StatRead(0, n)
					return
				})
		}
	} else {
		return func(r io.Reader) io.ReadCloser {
			return decompressor(r)
		}
	}
}

func NewCacheCompressionLZ4(level base.CompressionLevel) CacheCompression {
	return CacheCompression{
		Method:       0xFFFF,
		Compressor:   newCacheCompressor(base.NewLz4Writer, level),
		Decompressor: newCacheDecompressor(base.NewLz4Reader),
	}
}

// ZipMethodWinZip is the method for Zstandard compressed data inside Zip files for WinZip.
// See https://www.winzip.com/win/en/comp_info.html
const ZipMethodWinZip = 93

func NewCacheCompressionZStd(level base.CompressionLevel) CacheCompression {
	return CacheCompression{
		Method:       ZipMethodWinZip,
		Compressor:   newCacheCompressor(base.NewZStdWriter, level),
		Decompressor: newCacheDecompressor(base.NewZStdReader),
	}
}

func ForeachCacheCompression(each func(CacheCompression) error) error {
	for _, it := range []CacheCompression{
		NewCacheCompressionLZ4(base.COMPRESSION_LEVEL_INHERIT),
		NewCacheCompressionZStd(base.COMPRESSION_LEVEL_INHERIT),
	} {
		if err := each(it); err != nil {
			return err
		}
	}
	return nil
}

var GetCacheCompression = base.Memoize[*CacheCompression](func() *CacheCompression {
	flags := GetActionFlags()
	var compression CacheCompression
	switch flags.CacheCompression {
	case base.COMPRESSION_FORMAT_LZ4:
		compression = NewCacheCompressionLZ4(flags.CacheCompressionLevel)
	case base.COMPRESSION_FORMAT_ZSTD:
		compression = NewCacheCompressionZStd(flags.CacheCompressionLevel)
	default:
		base.UnexpectedValuePanic(flags.CacheCompression, flags.CacheCompression)
	}
	return &compression
})

/***************************************
 * CacheModeType
 ***************************************/

type CacheModeType int32

const (
	CACHE_INHERIT CacheModeType = iota
	CACHE_NONE
	CACHE_READ
	CACHE_READWRITE
)

func CacheModeTypes() []CacheModeType {
	return []CacheModeType{
		CACHE_INHERIT,
		CACHE_NONE,
		CACHE_READ,
		CACHE_READWRITE,
	}
}
func (x CacheModeType) Description() string {
	switch x {
	case CACHE_INHERIT:
		return "inherit from default configuration"
	case CACHE_NONE:
		return "disable cache"
	case CACHE_READ:
		return "enable fetching from cache"
	case CACHE_READWRITE:
		return "enable both fetching from and writing to cache"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x CacheModeType) String() string {
	switch x {
	case CACHE_INHERIT:
		return "INHERIT"
	case CACHE_NONE:
		return "NONE"
	case CACHE_READ:
		return "READ"
	case CACHE_READWRITE:
		return "READWRITE"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x CacheModeType) IsInheritable() bool {
	return x == CACHE_INHERIT
}
func (x *CacheModeType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case CACHE_INHERIT.String():
		*x = CACHE_INHERIT
	case CACHE_NONE.String():
		*x = CACHE_NONE
	case CACHE_READ.String():
		*x = CACHE_READ
	case CACHE_READWRITE.String():
		*x = CACHE_READWRITE
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *CacheModeType) Serialize(ar base.Archive) {
	ar.Int32((*int32)(x))
}
func (x CacheModeType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *CacheModeType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *CacheModeType) AutoComplete(in base.AutoComplete) {
	for _, it := range CacheModeTypes() {
		in.Add(it.String(), it.Description())
	}
}

func (x CacheModeType) HasRead() bool {
	switch x {
	case CACHE_READ, CACHE_READWRITE:
		return true
	case CACHE_INHERIT, CACHE_NONE:
	default:
		base.UnexpectedValuePanic(x, x)
	}
	return false
}
func (x CacheModeType) HasWrite() bool {
	switch x {
	case CACHE_READWRITE:
		return true
	case CACHE_INHERIT, CACHE_NONE, CACHE_READ:
	default:
		base.UnexpectedValuePanic(x, x)
	}
	return false
}
func (x CacheModeType) IsDisabled() bool {
	switch x {
	case CACHE_READWRITE, CACHE_READ:
		return false
	case CACHE_INHERIT, CACHE_NONE:
	default:
		base.UnexpectedValuePanic(x, x)
	}
	return true
}
