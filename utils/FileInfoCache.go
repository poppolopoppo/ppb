package utils

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync/atomic"

	"github.com/poppolopoppo/ppb/internal/base"
)

const EnableFileInfoCache = true

/***************************************
 * FileInfoCache
 ***************************************/

type FileInfoCache interface {
	InvalidateDirectory(d Directory)
	InvalidateFile(f Filename)
	SetFileInfo(f Filename, stat os.FileInfo, err error)
	GetFileInfo(f Filename) (os.FileInfo, error)
	SetDirectoryInfo(d Directory, stat os.FileInfo, err error)
	GetDirectoryInfo(d Directory) (os.FileInfo, error)
	EnumerateDirectory(d Directory) (FileSet, DirSet, error)
	Reset()
	PrintStats(w io.Writer) error
}

var FileInfos FileInfoCache = func() FileInfoCache {
	if EnableFileInfoCache {
		return &onceFileInfoCache{}
	} else {
		return dummyFileInfoCache{}
	}
}()

/***************************************
 * FileInfoCache dummy (no caching)
 ***************************************/

type dummyFileInfoCache struct{}

func (x dummyFileInfoCache) InvalidateDirectory(d Directory)                           {}
func (x dummyFileInfoCache) InvalidateFile(f Filename)                                 {}
func (x dummyFileInfoCache) Reset()                                                    {}
func (x dummyFileInfoCache) PrintStats(w io.Writer) error                              { return nil }
func (x dummyFileInfoCache) SetFileInfo(f Filename, stat os.FileInfo, err error)       {}
func (x dummyFileInfoCache) SetDirectoryInfo(d Directory, stat os.FileInfo, err error) {}

func (x dummyFileInfoCache) GetFileInfo(f Filename) (os.FileInfo, error) {
	return os.Stat(f.String())
}
func (x dummyFileInfoCache) GetDirectoryInfo(d Directory) (os.FileInfo, error) {
	return os.Stat(d.String())
}
func (x dummyFileInfoCache) EnumerateDirectory(d Directory) (files FileSet, directories DirSet, err error) {
	var entries []os.DirEntry
	entries, err = os.ReadDir(d.String())
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			dir := d.Folder(e.Name())
			directories.Append(dir)
		} else if e.Type().IsRegular() {
			file := d.File(e.Name())
			files.Append(file)
		}
	}
	return
}

/***************************************
 * FileInfoCache once
 ***************************************/

type onceFileInfoCacheDir struct {
	Files FileSet
	Dirs  DirSet
}

var errNotADirectory = errors.New("not a directory")
var invalidEnumerateDirForFiles = func() base.Optional[onceFileInfoCacheDir] {
	return base.UnexpectedOption[onceFileInfoCacheDir](errNotADirectory)
}

type onceFileInfoCacheEntry struct {
	GetStat            func() base.Optional[os.FileInfo]
	EnumerateDirectory func() base.Optional[onceFileInfoCacheDir]
}

func newGetState(f Filename) func() base.Optional[os.FileInfo] {
	return base.Memoize(func() base.Optional[os.FileInfo] {
		if len(f.Basename) == 0 {
			onceFileCacheStats.GetDirStat.OnExecute()
		} else {
			onceFileCacheStats.GetFileStat.OnExecute()
		}

		if info, err := os.Stat(f.String()); err == nil {
			return base.NewOption(info)
		} else {
			return base.UnexpectedOption[os.FileInfo](err)
		}
	})
}
func newSetState(stat os.FileInfo, err error) func() base.Optional[os.FileInfo] {
	if err == nil {
		return func() base.Optional[os.FileInfo] {
			return base.NewOption(stat)
		}
	} else {
		return func() base.Optional[os.FileInfo] {
			return base.UnexpectedOption[os.FileInfo](err)
		}
	}
}
func newEnumerateDir(d Directory) func() base.Optional[onceFileInfoCacheDir] {
	return base.Memoize(func() base.Optional[onceFileInfoCacheDir] {
		onceFileCacheStats.EnumerateDir.OnExecute()

		entries, err := os.ReadDir(d.String())
		if err != nil {
			return base.UnexpectedOption[onceFileInfoCacheDir](err)
		}

		var result onceFileInfoCacheDir
		for _, e := range entries {
			if e.IsDir() {
				dir := d.Folder(e.Name())
				result.Dirs.Append(dir)
			} else if e.Type().IsRegular() {
				file := d.File(e.Name())
				result.Files.Append(file)
			}
		}
		return base.NewOption(result)
	})
}

func newOnceFileInfoCacheEntry(f Filename) onceFileInfoCacheEntry {
	return onceFileInfoCacheEntry{
		GetStat:            newGetState(f),
		EnumerateDirectory: invalidEnumerateDirForFiles,
	}
}
func newOnceDirInfoCacheEntry(d Directory) onceFileInfoCacheEntry {
	return onceFileInfoCacheEntry{
		GetStat:            newGetState(Filename{Dirname: d}),
		EnumerateDirectory: newEnumerateDir(d),
	}
}

type onceFileInfoCache struct {
	entries base.SharedMapT[Filename, onceFileInfoCacheEntry]
}

func (x *onceFileInfoCache) InvalidateDirectory(d Directory) {
	onceFileCacheStats.GetDirStat.OnInvalidate()

	x.entries.Add(Filename{Dirname: d}, newOnceDirInfoCacheEntry(d))
}
func (x *onceFileInfoCache) InvalidateFile(f Filename) {
	onceFileCacheStats.GetFileStat.OnInvalidate()

	x.entries.Add(f, newOnceFileInfoCacheEntry(f))
}
func (x *onceFileInfoCache) Reset() {
	x.entries.Clear()
}

func (x *onceFileInfoCache) SetFileInfo(f Filename, stat os.FileInfo, err error) {
	onceFileCacheStats.GetFileStat.OnSet()

	x.entries.Add(f, onceFileInfoCacheEntry{
		GetStat:            newSetState(stat, err),
		EnumerateDirectory: invalidEnumerateDirForFiles,
	})
}
func (x *onceFileInfoCache) SetDirectoryInfo(d Directory, stat os.FileInfo, err error) {
	onceFileCacheStats.GetFileStat.OnSet()

	x.entries.Add(Filename{Dirname: d}, onceFileInfoCacheEntry{
		GetStat:            newSetState(stat, err),
		EnumerateDirectory: newEnumerateDir(d),
	})
}

func (x *onceFileInfoCache) GetFileInfo(f Filename) (os.FileInfo, error) {
	onceFileCacheStats.GetFileStat.OnGet()

	entry, _ := x.entries.FindOrAdd(f, newOnceFileInfoCacheEntry(f))
	return entry.GetStat().Get()

}
func (x *onceFileInfoCache) GetDirectoryInfo(d Directory) (os.FileInfo, error) {
	onceFileCacheStats.GetDirStat.OnGet()

	entry, _ := x.entries.FindOrAdd(Filename{Dirname: d}, newOnceDirInfoCacheEntry(d))
	return entry.GetStat().Get()
}
func (x *onceFileInfoCache) EnumerateDirectory(d Directory) (FileSet, DirSet, error) {
	onceFileCacheStats.EnumerateDir.OnGet()

	entry, _ := x.entries.FindOrAdd(Filename{Dirname: d}, newOnceDirInfoCacheEntry(d))
	list, err := entry.EnumerateDirectory().Get()
	return list.Files, list.Dirs, err
}

/***************************************
 * FileInfoCache stats (only IF_PROFILING)
 ***************************************/

type onceFileCacheFunctionStats struct {
	Sets        atomic.Int32
	Gets        atomic.Int32
	Executes    atomic.Int32
	Invalidates atomic.Int32
}

func (x *onceFileCacheFunctionStats) OnSet() {
	if PROFILING_ENABLED {
		x.Sets.Add(1)
	}
}
func (x *onceFileCacheFunctionStats) OnGet() {
	if PROFILING_ENABLED {
		x.Gets.Add(1)
	}
}
func (x *onceFileCacheFunctionStats) OnExecute() {
	if PROFILING_ENABLED {
		x.Executes.Add(1)
	}
}
func (x *onceFileCacheFunctionStats) OnInvalidate() {
	if PROFILING_ENABLED {
		x.Invalidates.Add(1)
	}
}

var onceFileCacheStats struct {
	GetFileStat  onceFileCacheFunctionStats
	GetDirStat   onceFileCacheFunctionStats
	EnumerateDir onceFileCacheFunctionStats
}

func (x *onceFileCacheFunctionStats) PrintStats(name string, w io.Writer) (err error) {
	if PROFILING_ENABLED {
		_, err = fmt.Printf(
			"FileInfoCache= Get:%04d | Set:%04d | Exe:%04d | Clr:%04d | %s -> cache hit = %.2f%%\n",
			x.Gets.Load(),
			x.Sets.Load(),
			x.Executes.Load(),
			x.Invalidates.Load(),
			name,
			(float64(x.Sets.Load())*100.0)/float64(x.Gets.Load()))
	}
	return
}

func (x *onceFileInfoCache) PrintStats(w io.Writer) (err error) {
	if PROFILING_ENABLED {
		if err = onceFileCacheStats.GetDirStat.PrintStats("GetDirStat", w); err == nil {
			if err = onceFileCacheStats.GetFileStat.PrintStats("GetFileStat", w); err == nil {
				err = onceFileCacheStats.EnumerateDir.PrintStats("EnumerateDir", w)
			}
		}
	}
	return
}
