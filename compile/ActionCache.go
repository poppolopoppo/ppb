package compile

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"archive/zip"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/internal/io"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var LogActionCache = NewLogCategory("ActionCache")

/***************************************
 * ActionCache
 ***************************************/

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
	seed  Fingerprint
	stats ActionCacheStats
}

var GetActionCache = Memoize(func() ActionCache {
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
func (x *actionCache) Serialize(ar Archive) {
	ar.Serializable(&x.path)
	ar.Serializable(&x.seed)
}
func (x *actionCache) Build(bc BuildContext) error {
	return CreateDirectory(bc, x.path)
}

func (x *actionCache) GetEntryExtname() string {
	return getBulkCompress().EntryExtname
}
func (x *actionCache) GetBulkExtname() string {
	return getBulkCompress().BulkExtname
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
		LogTrace(LogActionCache, "cache hit for %q", a.Alias())
		LogDebug(LogActionCache, "%q has the following artifacts ->\n\t - %v", a.Alias(), MakeStringer(func() string {
			return JoinString("\n\t - ", artifacts...)
		}))

		atomic.AddInt32(&x.stats.CacheHit, 1)
	} else {
		LogTrace(LogActionCache, "%v", err)
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
		LogTrace(LogActionCache, "cache store for %q", action)
		LogDebug(LogActionCache, "%q has the following artifacts ->\n\t - %v", action, MakeStringer(func() string {
			return JoinString("\n\t - ", artifacts...)
		}))
	} else {
		LogError(LogActionCache, "failed to cache in store %q with %v", action, err)
	}
	return
}
func (x *actionCache) AsyncCacheWrite(node BuildNode, cacheKey ActionCacheKey, artifacts FileSet, options ...BuildOptionFunc) error {
	action := node.Alias()
	GetGlobalThreadPool().Queue(func(ThreadContext) {
		inputs, err := CommandEnv.BuildGraph().GetDependencyInputFiles(action)
		if err == nil {
			inputs.Sort()
			err = x.CacheWrite(action, cacheKey, artifacts, inputs, options...)
		}
		LogPanicIfFailed(LogActionCache, err)
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

	fingerprint, err := SerializeAnyFingerprint(func(ar Archive) error {
		// serialize action inputs
		a.Serialize(ar)

		// serialize all input files content
		future := BuildFileDigests(bg, a.Inputs, options...)

		if digests, err := future.Join().Get(); err == nil {
			for i, fd := range digests {
				AssertIn(fd.Source, a.Inputs[i])
				Assert(fd.Digest.Valid)

				LogDebug(LogActionCache, "file digest for %q is %v", fd.Source, fd.Digest)
				ar.Serializable(fd)
			}
		} else {
			return err
		}

		return nil
	}, x.seed)
	LogPanicIfFailed(LogActionCache, err)

	LogTrace(LogActionCache, "key: %v -> action: %q", ActionCacheKey(fingerprint), a.Alias())
	return ActionCacheKey(fingerprint)
}

/***************************************
 * ActionCacheKey
 ***************************************/

type ActionCacheKey Fingerprint

func (x *ActionCacheKey) Serialize(ar Archive) {
	ar.Serializable((*Fingerprint)(x))
}
func (x ActionCacheKey) String() string {
	return x.GetFingerprint().ShortString()
}
func (x ActionCacheKey) GetFingerprint() Fingerprint {
	return (Fingerprint)(x)
}

func (x ActionCacheKey) GetEntryPath(cachePath Directory) Filename {
	return makeCachePath(cachePath, x.GetFingerprint(), getBulkCompress().EntryExtname)
}

func makeCachePath(cachePath Directory, h Fingerprint, extname string) Filename {
	str := h.String()
	return cachePath.Folder(str[0:2]).Folder(str[2:4]).File(str).ReplaceExt(extname)
}

/***************************************
 * ActionCacheBulk
 ***************************************/

type bulkCompress struct {
	BulkExtname  string
	EntryExtname string
	Method       uint16
	Compressor   zip.Compressor
	Decompressor zip.Decompressor
}

var bulkCompressLz4 bulkCompress = bulkCompress{
	BulkExtname:  ".bulk.lz4",
	EntryExtname: ".cache.lz4",
	Method:       0xFFFF,
	Compressor: func(w io.Writer) (io.WriteCloser, error) {
		return NewLz4Writer(w, COMPRESSION_LEVEL_INHERIT), nil
	},
	Decompressor: func(r io.Reader) io.ReadCloser {
		return NewLz4Reader(r)
	},
}

// ZipMethodWinZip is the method for Zstandard compressed data inside Zip files for WinZip.
// See https://www.winzip.com/win/en/comp_info.html
const ZipMethodWinZip = 93

var bulkCompressZStd bulkCompress = bulkCompress{
	BulkExtname:  ".bulk.zstd",
	EntryExtname: ".cache.zstd",
	Method:       ZipMethodWinZip,
	Compressor: func(w io.Writer) (io.WriteCloser, error) {
		return NewZStdWriter(w, COMPRESSION_LEVEL_INHERIT), nil
	},
	Decompressor: func(r io.Reader) io.ReadCloser {
		return NewZStdReader(r)
	},
}

func getBulkCompress() *bulkCompress {
	return &bulkCompressLz4
}

type ActionCacheBulk struct {
	Path   Filename
	Inputs []FileDigest
}

func NewActionCacheBulk(cachePath Directory, key ActionCacheKey, inputs FileSet, options ...BuildOptionFunc) (bulk ActionCacheBulk, err error) {
	bulk.Inputs = make([]FileDigest, len(inputs))

	var fingerprint Fingerprint
	fingerprint, err = SerializeAnyFingerprint(func(ar Archive) error {
		future := BuildFileDigests(CommandEnv.BuildGraph(), inputs, options...)

		if digests, err := future.Join().Get(); err == nil {
			for i, fd := range digests {
				AssertIn(fd.Source, inputs[i])
				Assert(fd.Digest.Valid)

				bulk.Inputs[i] = *fd
				ar.Serializable(&bulk.Inputs[i])
			}
		} else {
			return err
		}

		return nil
	}, key.GetFingerprint() /* use action key as bulk key seed */)

	if err == nil {
		bulk.Path = makeCachePath(cachePath, fingerprint, getBulkCompress().BulkExtname)
	}
	return
}
func (x *ActionCacheBulk) Equals(y ActionCacheBulk) bool {
	return x.Path.Equals(y.Path)
}
func (x *ActionCacheBulk) CacheHit(options ...BuildOptionFunc) bool {
	future := BuildFileDigests(CommandEnv.BuildGraph(), Map(func(fd FileDigest) Filename { return fd.Source }, x.Inputs...), options...)

	if digests, err := future.Join().Get(); err == nil {
		for i, fd := range digests {
			AssertIn(fd.Source, x.Inputs[i].Source)
			Assert(fd.Digest.Valid)

			if fd.Digest != x.Inputs[i].Digest {
				LogVeryVerbose(LogActionCache, "cache-miss due to %q, recorded %v but computed %v",
					x.Inputs[i].Source, x.Inputs[i].Digest, fd.Digest)
				return false
			}
		}
	} else {
		LogPanicIfFailed(LogActionCache, err)
		return false
	}

	return true
}
func (x *ActionCacheBulk) Deflate(root Directory, artifacts ...Filename) error {
	deflateStat := StartBuildStats()
	defer actionCacheStats.CacheInflate.Append(&deflateStat)

	return UFS.CreateBuffered(x.Path, func(w io.Writer) error {
		bulk := getBulkCompress()
		zw := zip.NewWriter(w)
		zw.RegisterCompressor(bulk.Method, bulk.Compressor)

		for _, file := range artifacts {
			info, err := file.Info()
			if err != nil {
				return err
			}

			name := file.Relative(root)
			header := &zip.FileHeader{
				Name:     name,
				Method:   bulk.Method,
				Modified: info.ModTime().UTC(), // keep modified time stable when restoring cache artifacts
			}

			w, err := zw.CreateHeader(header)
			if err != nil {
				return err
			}

			if err := UFS.Open(file, func(r io.Reader) error {
				return CopyWithProgress(file.String(), info.Size(), w, r)
			}); err != nil {
				return err
			}

			actionCacheStats.StatWrite(header.CompressedSize64, header.UncompressedSize64)
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
		zr.RegisterDecompressor(bulkCompressLz4.Method, bulkCompressLz4.Decompressor)
		zr.RegisterDecompressor(bulkCompressZStd.Method, bulkCompressZStd.Decompressor)

		for _, file := range zr.File {
			rc, err := file.Open()
			if err != nil {
				return err
			}

			actionCacheStats.StatRead(file.CompressedSize64, file.UncompressedSize64)

			dst := dst.AbsoluteFile(file.Name)
			err = UFS.CreateFile(dst, func(w *os.File) error {
				// this is much faster than separate UFS.MTime()/os.Chtimes(), but OS dependant...
				if err := SetMTime(w, file.Modified); err != nil {
					return err
				}
				return CopyWithProgress(dst.String(), int64(file.UncompressedSize64), w, rc)
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
func (x *ActionCacheBulk) Serialize(ar Archive) {
	ar.Serializable(&x.Path)
	SerializeSlice(ar, &x.Inputs)
}

/***************************************
 * ActionCacheEntry
 ***************************************/

type ActionCacheEntry struct {
	Key   ActionCacheKey
	Bulks []ActionCacheBulk
}

func (x *ActionCacheEntry) Serialize(ar Archive) {
	ar.Serializable((*Fingerprint)(&x.Key))
	SerializeSlice(ar, &x.Bulks)
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
	benchmark := LogBenchmark(LogActionCache, "read cache entry with key %q", x.Key)
	defer benchmark.Close()

	return UFS.Open(src, func(r io.Reader) error {
		_, err := CompressedArchiveFileRead(r, func(ar Archive) {
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

	benchmark := LogBenchmark(LogActionCache, "store cache entry with key %q", x.Key)
	defer benchmark.Close()

	return UFS.Create(path, func(w io.Writer) error {
		return CompressedArchiveFileWrite(w, func(ar Archive) {
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

func (x *ActionCacheStats) StatRead(compressed, uncompressed uint64) {
	atomic.AddUint64(&x.CacheReadCompressed, compressed)
	atomic.AddUint64(&x.CacheReadUncompressed, uncompressed)
}
func (x *ActionCacheStats) StatWrite(compressed, uncompressed uint64) {
	atomic.AddUint64(&x.CacheWriteCompressed, compressed)
	atomic.AddUint64(&x.CacheWriteUncompressed, uncompressed)
}
func (x *ActionCacheStats) Print() {
	LogForwardf("\nAction cache was hit %d times and missed %d times, stored %d new cache entries (hit rate: %.3f%%)",
		x.CacheHit, x.CacheMiss, x.CacheStore,
		100*float32(x.CacheHit)/float32(x.CacheHit+x.CacheMiss))

	LogForwardf("   READ <==  %8.3f seconds - %5d cache entries",
		x.CacheRead.Duration.Exclusive.Seconds(), x.CacheRead.Count)
	LogForwardf("INFLATE  ->  %8.3f seconds - %5d cache bulks    - %8.3f MiB/Sec  - %8.3f MiB  ->> %9.3f MiB  (x%4.2f)",
		x.CacheInflate.Duration.Exclusive.Seconds(), x.CacheInflate.Count,
		MebibytesPerSec(uint64(x.CacheReadUncompressed), x.CacheInflate.Duration.Exclusive),
		Mebibytes(x.CacheReadCompressed),
		Mebibytes(x.CacheReadUncompressed),
		float64(x.CacheReadUncompressed)/(float64(x.CacheReadCompressed)+0.00001))

	LogForwardf("  WRITE ==>  %8.3f seconds - %5d cache entries",
		x.CacheWrite.Duration.Exclusive.Seconds(), x.CacheWrite.Count)
	LogForwardf("DEFLATE <-   %8.3f seconds - %5d cache bulks    - %8.3f MiB/Sec  - %8.3f MiB <<-  %9.3f MiB  (x%4.2f)",
		x.CacheDeflate.Duration.Exclusive.Seconds(), x.CacheDeflate.Count,
		MebibytesPerSec(uint64(x.CacheWriteUncompressed), x.CacheDeflate.Duration.Exclusive),
		Mebibytes(x.CacheWriteCompressed),
		Mebibytes(x.CacheWriteUncompressed),
		float64(x.CacheWriteUncompressed)/(float64(x.CacheWriteCompressed)+0.00001))
}

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
		UnexpectedValue(x)
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
		err = MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *CacheModeType) Serialize(ar Archive) {
	ar.Int32((*int32)(x))
}
func (x CacheModeType) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *CacheModeType) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}
func (x *CacheModeType) AutoComplete(in AutoComplete) {
	for _, it := range CacheModeTypes() {
		in.Add(it.String())
	}
}

func (x CacheModeType) HasRead() bool {
	switch x {
	case CACHE_READ, CACHE_READWRITE:
		return true
	case CACHE_INHERIT, CACHE_NONE:
	default:
		UnexpectedValuePanic(x, x)
	}
	return false
}
func (x CacheModeType) HasWrite() bool {
	switch x {
	case CACHE_READWRITE:
		return true
	case CACHE_INHERIT, CACHE_NONE, CACHE_READ:
	default:
		UnexpectedValuePanic(x, x)
	}
	return false
}
