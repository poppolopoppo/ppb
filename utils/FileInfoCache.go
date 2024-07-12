package utils

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/poppolopoppo/ppb/internal/base"
)

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

type fileInfoEntry struct {
	stat os.FileInfo
	err  error

	directory struct {
		sync.Once
		files       FileSet
		directories DirSet
	}
}

type fileInfoCache struct {
	entries  base.SharedMapT[Filename, *fileInfoEntry]
	recycler base.Recycler[*fileInfoEntry]

	// stats struct {
	// 	InfoHit  atomic.Int64
	// 	InfoMiss atomic.Int64

	// 	EnumerateHit  atomic.Int64
	// 	EnumerateMiss atomic.Int64
	// }
}

var FileInfos FileInfoCache = &fileInfoCache{
	recycler: base.NewRecycler[*fileInfoEntry](
		func() *fileInfoEntry {
			return &fileInfoEntry{}
		},
		func(fie *fileInfoEntry) {
			*fie = fileInfoEntry{}
		},
	),
}

func (x *fileInfoCache) InvalidateDirectory(d Directory) {
	if entry, loaded := x.entries.LoadAndDelete(Filename{Dirname: d}); loaded {
		x.recycler.Release(entry)
	}
}
func (x *fileInfoCache) InvalidateFile(f Filename) {
	if entry, loaded := x.entries.LoadAndDelete(f); loaded {
		x.recycler.Release(entry)
	}
}

func (x *fileInfoCache) SetFileInfo(f Filename, stat os.FileInfo, err error) {
	x.findOrAddEntry(f, stat, err)
}
func (x *fileInfoCache) GetFileInfo(f Filename) (stat os.FileInfo, err error) {
	if it := x.findOrCreateEntry(f); it != nil {
		stat = it.stat
		err = it.err

		if err == nil && (stat.IsDir() || !stat.Mode().IsRegular()) {
			err = fmt.Errorf("expected file at %q, found: %v", f, stat.Mode())
		}
	}
	return
}

func (x *fileInfoCache) SetDirectoryInfo(d Directory, stat os.FileInfo, err error) {
	x.findOrAddEntry(Filename{Dirname: d}, stat, err)
}
func (x *fileInfoCache) GetDirectoryInfo(d Directory) (stat os.FileInfo, err error) {
	if it := x.findOrCreateEntry(Filename{Dirname: d}); it != nil {
		stat = it.stat
		err = it.err

		if err == nil && !stat.IsDir() {
			err = fmt.Errorf("expected directory at %q, found: %v", d, stat.Mode())
		}
	}
	return
}

func (x *fileInfoCache) EnumerateDirectory(d Directory) (FileSet, DirSet, error) {
	if it := x.findOrCreateEntry(Filename{Dirname: d}); it != nil {
		if it.err != nil {
			return FileSet{}, DirSet{}, it.err
		}

		it.directory.Once.Do(func() {
			// x.stats.EnumerateMiss.Add(1)
			// x.stats.EnumerateHit.Add(-1)

			it.directory.files.Clear()
			it.directory.directories.Clear()

			var entries []os.DirEntry
			entries, it.err = os.ReadDir(d.String())

			if it.err == nil {
				for _, e := range entries {
					stat, err := e.Info()
					if e.IsDir() {
						dir := d.Folder(e.Name())
						it.directory.directories.Append(dir)
						x.SetDirectoryInfo(dir, stat, err)
					} else if e.Type().IsRegular() {
						file := d.File(e.Name())
						it.directory.files.Append(file)
						x.SetFileInfo(file, stat, err)
					}
				}
			}
		})

		// x.stats.EnumerateHit.Add(1)
		return it.directory.files, it.directory.directories, it.err
	}
	base.UnreachableCode()
	return FileSet{}, DirSet{}, nil
}
func (x *fileInfoCache) Reset() {
	x.entries.Clear()
}

func (x *fileInfoCache) findOrAddEntry(f Filename, stat os.FileInfo, err error) *fileInfoEntry {
	entry := x.recycler.Allocate()
	entry.stat = stat
	entry.err = err

	if newEntry, loaded := x.entries.FindOrAdd(f, entry); loaded {
		base.AssertNotIn(entry, newEntry)
		x.recycler.Release(entry)
		entry = newEntry
	}

	return entry
}
func (x *fileInfoCache) findOrCreateEntry(f Filename) *fileInfoEntry {
	entry, ok := x.entries.Get(f)
	if !ok {
		// x.stats.InfoMiss.Add(1)

		var path string
		if len(f.Basename) > 0 {
			path = f.String()
		} else {
			path = f.Dirname.String()
		}

		stat, err := os.Stat(path)
		entry = x.findOrAddEntry(f, stat, err)
	}
	// else {

	// 	// x.stats.InfoHit.Add(1)
	// }
	return entry
}

func (x *fileInfoCache) PrintStats(w io.Writer) error {
	// fmt.Fprintf(w, "File infos cache hit: %4d / miss: %4d\n",
	// 	x.stats.InfoHit.Load(),
	// 	x.stats.InfoMiss.Load())
	// fmt.Fprintf(w, "Enumerate cache hit : %4d / miss: %4d\n",
	// 	x.stats.EnumerateHit.Load(),
	// 	x.stats.EnumerateMiss.Load())
	return nil
}
