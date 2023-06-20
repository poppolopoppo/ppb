package utils

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/djherbis/times"
)

var LogUFS = NewLogCategory("UFS")

/***************************************
 * Path to string
 ***************************************/

const OSPathSeparator = os.PathSeparator

func BuildSanitizedPath(sb *strings.Builder, pathname string, sep rune) error {
	hasSeparator := false
	for _, ch := range pathname {
		if os.IsPathSeparator((uint8)(ch)) {
			if !hasSeparator {
				hasSeparator = true
				if _, err := sb.WriteRune(sep); err != nil {
					return err
				}
			}
		} else if _, err := sb.WriteRune(ch); err != nil {
			return err
		} else {
			hasSeparator = false
		}
	}

	return nil
}
func SanitizePath(pathname string, sep rune) string {
	sb := strings.Builder{}
	sb.Grow(len(pathname))
	BuildSanitizedPath(&sb, pathname, sep)
	return sb.String()
}
func JoinPath(in string, args ...string) string {
	Assert(func() bool { return len(in) > 0 })
	sb := strings.Builder{}
	capacity := len(in)
	for _, it := range args {
		Assert(func() bool { return len(it) > 0 })
		capacity += len(it) + 1
	}
	sb.Grow(capacity)
	sb.WriteString(in)
	for _, it := range args {
		sb.WriteRune(OSPathSeparator)
		sb.WriteString(it)
	}
	return sb.String()
}
func SplitPath(in string) (results []string) {
	for {
		if i, ok := firstIndexOfPathSeparator(in); ok {
			results = append(results, in[:i])
			in = in[i+1:]
		} else {
			results = append(results, in)
			return
		}
	}
}

func firstIndexOfPathSeparator(in string) (int, bool) {
	for i, ch := range in {
		if os.IsPathSeparator((uint8)(ch)) {
			return i, true
		}
	}
	return len(in), false
}
func lastIndexOfPathSeparator(in string) (int, bool) {
	n := (len(in) - 1)
	for i := range in {
		i = n - i
		if os.IsPathSeparator(in[i]) {
			return i, true
		}
	}
	return len(in), false
}

// #TODO: enable back local path, but at the moment it's quite annoying to use with msvc
// #TODO: implement source indexing to resolve this issue
const localPathEnabled = false

func ForceLocalDirectory(d Directory) (relative string) {
	return d.Relative(UFS.Root)
}
func ForceLocalFilename(f Filename) (relative string) {
	return f.Relative(UFS.Root)
}

func MakeLocalDirectory(d Directory) (relative string) {
	if localPathEnabled {
		return ForceLocalDirectory(d)
	}
	return d.String()
}
func MakeLocalFilename(f Filename) (relative string) {
	if localPathEnabled {
		return ForceLocalFilename(f)
	}
	return f.String()
}

/***************************************
 * Directory
 ***************************************/

type Directory struct {
	Path string
}

func MakeDirectory(str string) Directory {
	return Directory{Path: CleanPath(str)}
}
func (d Directory) Len() int    { return len(d.Path) }
func (d Directory) Valid() bool { return len(d.Path) > 0 }
func (d Directory) Basename() string {
	if i, ok := lastIndexOfPathSeparator(d.String()); ok {
		return d.Path[i+1:]
	} else {
		return d.Path
	}
}
func (d Directory) Parent() Directory {
	if i, ok := lastIndexOfPathSeparator(d.Path); ok {
		return Directory{Path: d.Path[:i]}
	} else {
		UnexpectedValuePanic(d, d)
		return Directory{}
	}
}
func (d Directory) Split() (Directory, string) {
	if i, ok := lastIndexOfPathSeparator(d.String()); ok {
		return Directory{Path: d.Path[:i]}, d.String()[i+1:]
	} else {
		UnexpectedValuePanic(d, d)
		return Directory{}, ""
	}
}
func (d Directory) Folder(name ...string) Directory {
	return Directory{Path: JoinPath(d.String(), name...)}
}
func (d Directory) File(name ...string) Filename {
	return Filename{
		Dirname:  d.Folder(name[:len(name)-1]...),
		Basename: name[len(name)-1]}
}
func (d Directory) IsIn(o Directory) bool {
	return o.IsParentOf(d)
}
func (d Directory) IsParentOf(o Directory) bool {
	n := len(d.Path)
	if n > len(o.Path) {
		return false
	}
	for i := range d.Path[:n] {
		if d.Path[i] != o.Path[i] {
			return false
		}
	}
	return true
}
func (d Directory) AbsoluteFolder(rel ...string) (result Directory) {
	return Directory{Path: filepath.Clean(JoinPath(d.String(), rel...))}
}
func (d Directory) AbsoluteFile(rel ...string) (result Filename) {
	result.Dirname = d.AbsoluteFolder(rel...) // rel is not guaranteed to hold only one '/'
	result.Dirname, result.Basename = result.Dirname.Split()
	return
}
func (d Directory) Relative(to Directory) string {
	if path, err := filepath.Rel(to.String(), d.String()); err == nil {
		return path
	} else {
		return d.String()
	}
}
func (d Directory) Normalize() (result Directory) {
	return MakeDirectory(d.Path)
}
func (d Directory) Equals(o Directory) bool {
	return d == o
}
func (d Directory) Compare(o Directory) int {
	return strings.Compare(d.Path, o.Path)
}
func (d Directory) String() string {
	return d.Path
}

/***************************************
 * Filename
 ***************************************/

type Filename struct {
	Dirname  Directory
	Basename string
}

func MakeFilename(str string) Filename {
	str = CleanPath(str)
	dirname, basename := filepath.Split(str)
	if len(dirname) > 0 {
		// trim ending path separator
		dirname = dirname[:len(dirname)-1]
	}
	return Filename{
		Basename: basename,
		Dirname:  Directory{Path: dirname},
	}
}

func (f Filename) Len() int {
	if n := f.Dirname.Len(); n > 0 {
		return len(f.Basename) + n + 1
	} else {
		return len(f.Basename)
	}
}
func (f Filename) Valid() bool { return len(f.Basename) > 0 }
func (f Filename) Ext() string {
	return path.Ext(f.Basename)
}
func (f Filename) TrimExt() string {
	return strings.TrimSuffix(f.Basename, f.Ext())
}
func (f Filename) IsIn(o Directory) bool {
	return o.IsParentOf(f.Dirname)
}
func (f Filename) ReplaceExt(ext string) Filename {
	return Filename{
		Basename: f.TrimExt() + ext,
		Dirname:  f.Dirname,
	}
}
func (f Filename) Relative(to Directory) string {
	if path, err := filepath.Rel(to.String(), f.Dirname.String()); err == nil {
		return filepath.Join(path, f.Basename)
	} else {
		return f.String()
	}
}
func (f Filename) Normalize() (result Filename) {
	return MakeFilename(f.String())
}
func (f Filename) Equals(o Filename) bool {
	return (f.Basename == o.Basename && f.Dirname.Equals(o.Dirname))
}
func (f Filename) Compare(o Filename) int {
	if c := f.Dirname.Compare(o.Dirname); c != 0 {
		return c
	} else {
		return strings.Compare(f.Basename, o.Basename)
	}
}
func (f Filename) String() string {
	if len(f.Dirname.Path) > 0 {
		return JoinPath(f.Dirname.Path, f.Basename)
	} else {
		return f.Basename
	}
}

/***************************************
 * fmt.Value interface
 ***************************************/

func (d *Directory) Set(str string) error {
	if str != "" {
		if !filepath.IsAbs(str) {
			str = filepath.Join(UFS.Root.String(), str)
		}
		*d = MakeDirectory(str)
	} else {
		*d = Directory{}
	}
	return nil
}

func (f *Filename) Set(str string) error {
	if str != "" {
		if !filepath.IsAbs(str) {
			str = filepath.Join(UFS.Root.String(), str)
		}
		*f = MakeFilename(str)
	} else {
		*f = Filename{}
	}
	return nil
}

/***************************************
 * Entity info cache
 ***************************************/

type FileInfo struct {
	AbsolutePath string
	os.FileInfo
}
type DirectoryInfo struct {
	AbsolutePath string
	Files        []Filename
	Directories  []Directory
	os.FileInfo

	once sync.Once
}

func GetAccessTime(stat os.FileInfo) time.Time {
	return times.Get(stat).AccessTime()
}
func GetCreationTime(stat os.FileInfo) time.Time {
	return times.Get(stat).BirthTime()
}
func GetModificationTime(stat os.FileInfo) time.Time {
	return times.Get(stat).ModTime()
}

type UFSCacheBin struct {
	barrier        sync.RWMutex
	FileCache      map[string]*FileInfo
	DirectoryCache map[string]*DirectoryInfo
}

type UFSCache struct {
	bins [256]UFSCacheBin
}

func newUFSCache() *UFSCache {
	result := &UFSCache{}
	for i := range result.bins {
		result.bins[i] = UFSCacheBin{
			barrier:        sync.RWMutex{},
			FileCache:      map[string]*FileInfo{},
			DirectoryCache: map[string]*DirectoryInfo{},
		}
	}
	return result
}
func (cache *UFSCache) getBin(x Serializable) *UFSCacheBin {
	h := SerializeFingerpint(x, Fingerprint{})
	return &cache.bins[h[0]]
}

var ufsCache = newUFSCache()

type WrongFileInfoError struct {
	Message string
}

func (x WrongFileInfoError) Error() string { return x.Message }

func invalidate_file_info(f Filename) {
	cacheBin := ufsCache.getBin(&f)
	cacheBin.barrier.Lock()
	defer cacheBin.barrier.Unlock()
	delete(cacheBin.FileCache, f.String())
}
func MakeFileInfo(f Filename, optionalStat *os.FileInfo) (*FileInfo, error) {
	path := f.String() //.Normalize().String()

	cacheBin := ufsCache.getBin(&f)
	cacheBin.barrier.RLock()
	if cached, ok := cacheBin.FileCache[path]; ok {
		cacheBin.barrier.RUnlock()
		return cached, nil
	}
	cacheBin.barrier.RUnlock()

	cacheBin.barrier.Lock()
	defer cacheBin.barrier.Unlock()

	if cached, ok := cacheBin.FileCache[path]; !ok {
		cached = nil
		var err error
		var stat os.FileInfo
		if optionalStat != nil {
			cached = &FileInfo{
				AbsolutePath: path,
				FileInfo:     *optionalStat,
			}
		} else if stat, err = os.Stat(f.String()); !os.IsNotExist(err) {
			if !IsNil(stat) && !stat.IsDir() && stat.Mode().Type().IsRegular() {
				cached = &FileInfo{
					AbsolutePath: path,
					FileInfo:     stat,
				}
			} else {
				if err != nil {
					err = WrongFileInfoError{err.Error()}
				} else {
					err = WrongFileInfoError{"path does not point to a file"}
				}
			}
		}
		cacheBin.FileCache[path] = cached
		return cached, err
	} else {
		return cached, nil
	}
}

func invalidate_directory_info(d Directory) {
	cacheBin := ufsCache.getBin(&d)
	cacheBin.barrier.Lock()
	defer cacheBin.barrier.Unlock()
	delete(cacheBin.DirectoryCache, d.String())
}
func MakeDirectoryInfo(d Directory, optionalStat *os.FileInfo) (*DirectoryInfo, error) {
	path := d.String() //.Normalize().String()

	cacheBin := ufsCache.getBin(&d)
	cacheBin.barrier.RLock()
	if cached, ok := cacheBin.DirectoryCache[path]; ok && cached != nil {
		cacheBin.barrier.RUnlock()
		return cached, nil
	}
	cacheBin.barrier.RUnlock()

	cacheBin.barrier.Lock()
	defer cacheBin.barrier.Unlock()

	if cached, ok := cacheBin.DirectoryCache[path]; !ok || cached == nil {
		var stat os.FileInfo
		var cached *DirectoryInfo
		var err error
		if optionalStat != nil {
			cached = &DirectoryInfo{
				AbsolutePath: path,
				FileInfo:     *optionalStat,
				Files:        nil,
				Directories:  nil,
			}
		} else if stat, err = os.Stat(d.String()); !os.IsNotExist(err) {
			if stat.IsDir() {
				cached = &DirectoryInfo{
					AbsolutePath: path,
					FileInfo:     stat,
					Files:        nil,
					Directories:  nil,
				}
			} else {
				err = WrongFileInfoError{"path does not point to a directory"}
			}
		}
		cacheBin.DirectoryCache[path] = cached
		return cached, err
	} else {
		return cached, nil
	}
}

func enumerate_directory(d Directory) (*DirectoryInfo, error) {
	if info, err := d.Info(); err == nil && info != nil {
		info.once.Do(func() {
			var entries []os.DirEntry
			entries, err = os.ReadDir(info.AbsolutePath)

			if err == nil {
				files := []Filename{}
				directories := []Directory{}

				for _, it := range entries {
					var stat os.FileInfo
					stat, err = it.Info()
					if err != nil {
						continue
					} else if stat.IsDir() {
						child := d.Folder(it.Name())
						MakeDirectoryInfo(child, &stat)
						directories = append(directories, child)
					} else if it.Type().IsRegular() {
						child := d.File(it.Name())
						MakeFileInfo(child, &stat)
						files = append(files, child)
					}
				}

				info.Files = files
				info.Directories = directories
			}
		})
		return info, err
	} else {
		return nil, err
	}
}

/***************************************
 * IO
 ***************************************/

func (f Filename) Info() (*FileInfo, error) {
	return MakeFileInfo(f, nil)
}
func (f Filename) Invalidate() {
	invalidate_file_info(f)
}
func (f Filename) Exists() bool {
	info, err := f.Info()
	return (err == nil && info != nil)
}

func (d Directory) Info() (*DirectoryInfo, error) {
	return MakeDirectoryInfo(d, nil)
}
func (d Directory) Invalidate() {
	invalidate_directory_info(d)
}
func (d Directory) Exists() bool {
	info, err := d.Info()
	return (err == nil && info != nil)
}
func (d Directory) Files() []Filename {
	if info, err := enumerate_directory(d); err == nil {
		return info.Files
	} else {
		LogError(LogUFS, "Directory.Files(): %v", err)
		return []Filename{}
	}
}
func (d Directory) Directories() []Directory {
	if info, err := enumerate_directory(d); err == nil {
		return info.Directories
	} else {
		LogError(LogUFS, "Directory.Directories(): %v", err)
		return []Directory{}
	}
}
func (d Directory) MatchDirectories(each func(Directory) error, r *regexp.Regexp) error {
	if r == nil {
		return nil
	}
	LogVeryVerbose(LogUFS, "match directories in '%v' for /%v/...", d, r)
	if info, err := enumerate_directory(d); err == nil {
		for _, s := range info.Directories {
			if r.MatchString(s.Basename()) {
				if err := each(s); err != nil {
					return err
				}
			}
		}
		return nil
	} else {
		return err
	}
}
func (d Directory) MatchFiles(each func(Filename) error, r *regexp.Regexp) error {
	if r == nil {
		return nil
	}
	LogVeryVerbose(LogUFS, "match files in '%v' for /%v/...", d, r)
	if info, err := enumerate_directory(d); err == nil {
		for _, f := range info.Files {
			if r.MatchString(f.Basename) {
				if err := each(f); err != nil {
					return err
				}
			}
		}
		return nil
	} else {
		return err
	}
}
func (d Directory) MatchFilesRec(each func(Filename) error, r *regexp.Regexp) error {
	if r == nil {
		return nil
	}
	LogVeryVerbose(LogUFS, "match files rec in '%v' for /%v/...", d, r)
	if info, err := enumerate_directory(d); err == nil {
		for _, f := range info.Files {
			if r.MatchString(f.Basename) {
				if err := each(f); err != nil {
					return err
				}
			}
		}
		for _, s := range info.Directories {
			if err := s.MatchFilesRec(each, r); err != nil {
				return err
			}
		}
		return nil
	} else {
		return err
	}
}
func (d Directory) FindFileRec(r *regexp.Regexp) (Filename, error) {
	LogVeryVerbose(LogUFS, "find file rec in '%v' for /%v/...", d, r)
	if info, err := enumerate_directory(d); err == nil {
		for _, f := range info.Files {
			if r.MatchString(f.Basename) {
				return f, nil
			}
		}
		for _, s := range info.Directories {
			if f, err := s.FindFileRec(r); err == nil {
				return f, nil
			}
		}
		return Filename{}, fmt.Errorf("file not found '%v' in '%v'", r, d)
	} else {
		return Filename{}, err
	}
}

/***************************************
 * DirSet
 ***************************************/

type DirSet []Directory

func NewDirSet(x ...Directory) (result DirSet) {
	result = make(DirSet, len(x))
	copy(result, x)
	return
}

func (list DirSet) Len() int           { return len(list) }
func (list DirSet) Less(i, j int) bool { return list[i].Compare(list[j]) < 0 }
func (list DirSet) Slice() []Directory { return list }
func (list DirSet) Swap(i, j int)      { list[i], list[j] = list[j], list[i] }

func (list DirSet) IsUniq() bool {
	return IsUniq(list...)
}

func (list *DirSet) Sort() {
	sort.Slice(list.Slice(), func(i, j int) bool {
		return (*list)[i].Compare((*list)[j]) < 0
	})
}
func (list *DirSet) Contains(it ...Directory) bool {
	for _, x := range it {
		if _, ok := IndexIf(x.Equals, (*list)...); !ok {
			return false
		}
	}
	return true
}
func (list *DirSet) Append(it ...Directory) {
	*list = AppendEquatable_CheckUniq(*list, it...)
}
func (list *DirSet) AppendUniq(it ...Directory) {
	for _, x := range it {
		if !list.Contains(x) {
			*list = append(*list, x)
		}
	}
}
func (list *DirSet) Prepend(it ...Directory) {
	*list = PrependEquatable_CheckUniq(it, *list...)
}
func (list *DirSet) Remove(it ...Directory) {
	for _, x := range it {
		*list = RemoveUnless(x.Equals, *list...)
	}
}
func (list *DirSet) Clear() {
	*list = []Directory{}
}
func (list DirSet) Concat(it ...Directory) (result DirSet) {
	result = make(DirSet, len(list), len(list)+len(it))
	copy(result[:len(list)], list)
	result.Append(it...)
	return result
}
func (list DirSet) ConcatUniq(it ...Directory) (result DirSet) {
	result = NewDirSet(list...)
	for _, x := range it {
		result.AppendUniq(x)
	}
	return result
}
func (list *DirSet) Serialize(ar Archive) {
	SerializeSlice(ar, (*[]Directory)(list))
}
func (list DirSet) Equals(other DirSet) bool {
	if len(list) != len(other) {
		return false
	}
	for i, it := range list {
		if !other[i].Equals(it) {
			return false
		}
	}
	return true
}
func (list DirSet) StringSet() StringSet {
	return MakeStringerSet(list...)
}
func (list DirSet) Join(delim string) string {
	return JoinString(delim, list...)
}
func (list DirSet) Local(path Directory) StringSet {
	return (StringSet)(Map(MakeLocalDirectory, list...))
}
func (list DirSet) Normalize() DirSet {
	return ((DirSet)(Map(func(it Directory) Directory {
		return it.Normalize()
	}, list...)))
}

/***************************************
 * FileSet
 ***************************************/

type FileSet []Filename

func NewFileSet(x ...Filename) (result FileSet) {
	result = make(FileSet, len(x))
	copy(result, x)
	return
}

func (list FileSet) Len() int           { return len(list) }
func (list FileSet) At(i int) Filename  { return list[i] }
func (list FileSet) Less(i, j int) bool { return list[i].Compare(list[j]) < 0 }
func (list FileSet) Slice() []Filename  { return list }
func (list FileSet) Swap(i, j int)      { list[i], list[j] = list[j], list[i] }

func (list FileSet) IsUniq() bool {
	return IsUniq(list...)
}

func (list *FileSet) Sort() {
	sort.Slice(list.Slice(), func(i, j int) bool {
		return (*list)[i].Compare((*list)[j]) < 0
	})
}
func (list *FileSet) Contains(it ...Filename) bool {
	for _, x := range it {
		if _, ok := IndexIf(x.Equals, (*list)...); !ok {
			return false
		}
	}
	return true
}
func (list *FileSet) Append(it ...Filename) {
	*list = AppendEquatable_CheckUniq(*list, it...)
}
func (list *FileSet) AppendUniq(it ...Filename) {
	for _, x := range it {
		if !list.Contains(x) {
			*list = append(*list, x)
		}
	}
}
func (list *FileSet) Prepend(it ...Filename) {
	*list = PrependEquatable_CheckUniq(it, *list...)
}
func (list *FileSet) Remove(it ...Filename) {
	for _, x := range it {
		*list = RemoveUnless(x.Equals, *list...)
	}
}
func (list *FileSet) Clear() {
	*list = []Filename{}
}
func (list FileSet) Concat(it ...Filename) (result FileSet) {
	result = make(FileSet, len(list), len(list)+len(it))
	copy(result[:len(list)], list)
	result.Append(it...)
	return result
}
func (list FileSet) ConcatUniq(it ...Filename) (result FileSet) {
	result = NewFileSet(list...)
	for _, x := range it {
		result.AppendUniq(x)
	}
	return result
}
func (list FileSet) TotalSize() (result int64) {
	for _, x := range list {
		if info, err := x.Info(); info != nil {
			result += info.Size()
		} else {
			LogError(LogUFS, "%v: %v", x, err)
		}
	}
	return result
}
func (list *FileSet) Serialize(ar Archive) {
	SerializeSlice(ar, (*[]Filename)(list))
}
func (list FileSet) Equals(other FileSet) bool {
	if len(list) != len(other) {
		return false
	}
	for i, it := range list {
		if !other[i].Equals(it) {
			return false
		}
	}
	return true
}
func (list FileSet) StringSet() StringSet {
	return MakeStringerSet(list...)
}
func (list FileSet) Join(delim string) string {
	return JoinString(delim, list...)
}
func (list FileSet) Local(path Directory) StringSet {
	return (StringSet)(Map(MakeLocalFilename, list...))
}
func (list FileSet) Normalize() FileSet {
	return (FileSet)(Map(func(it Filename) Filename {
		return it.Normalize()
	}, list...))
}

/***************************************
 * JSON: marshal as string instead of array
 ***************************************/

func (x Filename) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *Filename) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}

func (x Directory) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *Directory) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}

/***************************************
 * Frontend
 ***************************************/

var UFS UFSFrontEnd = make_ufs_frontend()

type UFSFrontEnd struct {
	Executable Filename
	Caller     Filename

	Root     Directory
	Internal Directory
	Extras   Directory
	Source   Directory
	Output   Directory

	Binaries     Directory
	Cache        Directory
	Generated    Directory
	Intermediate Directory
	Projects     Directory
	Saved        Directory
	Scripts      Directory
	Transient    Directory
}

func (ufs *UFSFrontEnd) File(str string) Filename {
	return MakeFilename(str)
}
func (ufs *UFSFrontEnd) Dir(str string) Directory {
	return MakeDirectory(str)
}
func (ufs *UFSFrontEnd) Touch(dst Filename) error {
	return ufs.SetMTime(dst, time.Now().Local())
}
func (ufs *UFSFrontEnd) SetMTime(dst Filename, mtime time.Time) error {
	LogDebug(LogUFS, "chtimes %v", dst)
	path := dst.String()
	if err := os.Chtimes(path, mtime, mtime); err == nil {
		invalidate_file_info(dst)
		return nil
	} else {
		return err
	}
}
func (ufs *UFSFrontEnd) Remove(dst Filename) error {
	if err := os.Remove(dst.String()); err != nil {
		LogError(LogUFS, "%v", err)
		return err
	}
	return nil
}
func (ufs *UFSFrontEnd) Mkdir(dst Directory) {
	if err := ufs.MkdirEx(dst); err != nil {
		LogPanicErr(LogUFS, err)
	}
}
func (ufs *UFSFrontEnd) MkdirEx(dst Directory) error {
	path := dst.String()
	if st, err := os.Stat(path); st != nil && (err == nil || os.IsExist(err)) {
		if !st.IsDir() {
			LogDebug(LogUFS, "mkdir %v", dst)
			return fmt.Errorf("ufs: %q already exist, but is not a directory", dst)
		}
	} else {
		LogDebug(LogUFS, "mkdir %v", dst)
		invalidate_directory_info(dst)
		if err := os.MkdirAll(path, os.ModePerm); err != nil {
			return fmt.Errorf("ufs: mkdir %q got error %v", dst, err)
		}
	}
	return nil
}
func (ufs *UFSFrontEnd) CreateWriter(dst Filename) (*os.File, error) {
	invalidate_file_info(dst)
	ufs.Mkdir(dst.Dirname)
	LogDebug(LogUFS, "create '%v'", dst)
	return os.Create(dst.String())
}
func (ufs *UFSFrontEnd) CreateFile(dst Filename, write func(*os.File) error) error {
	outp, err := ufs.CreateWriter(dst)
	if err == nil {
		defer func() {
			closeErr := outp.Close()
			if err == nil {
				err = closeErr
			}
		}()
		if err = write(outp); err == nil {
			return err
		}
	}
	LogWarning(LogUFS, "CreateFile: caught %v while trying to create %v", err, dst)
	return err
}
func (ufs *UFSFrontEnd) Create(dst Filename, write func(io.Writer) error) error {
	return ufs.CreateFile(dst, func(f *os.File) error {
		return write(f)
	})
}
func (ufs *UFSFrontEnd) CreateBuffered(dst Filename, write func(io.Writer) error) error {
	return ufs.Create(dst, func(w io.Writer) error {
		var buffered bufio.Writer
		buffered.Reset(w)
		if err := write(&buffered); err != nil {
			return err
		}
		return buffered.Flush()
	})
}

const forceUnsafeCreate = true // os.Rename() is expansive, at least on Windows

func (ufs *UFSFrontEnd) SafeCreate(dst Filename, write func(io.Writer) error) error {
	if forceUnsafeCreate {
		return ufs.CreateBuffered(dst, write)
	} else {
		ufs.Mkdir(dst.Dirname)

		tmpFilename := dst.ReplaceExt(dst.Ext() + ".tmp")
		defer os.Remove(tmpFilename.String())

		err := UFS.CreateBuffered(tmpFilename, func(w io.Writer) error {
			return write(w)
		})

		if err == nil {
			if err = os.Rename(tmpFilename.String(), dst.String()); err != nil {
				LogWarning(LogUFS, "SafeCreate: %v", err)
			}
		}
		return err
	}
}

type TemporaryFile struct {
	Path Filename
}

func (x TemporaryFile) Close() error   { return UFS.Remove(x.Path) }
func (x TemporaryFile) String() string { return x.Path.String() }

func (ufs *UFSFrontEnd) CreateTemp(prefix string, write func(io.Writer) error) (TemporaryFile, error) {
	randBytes := [16]byte{}
	rand.Read(randBytes[:])
	tmp := UFS.Transient.Folder(prefix).File(hex.EncodeToString(randBytes[:]))
	return TemporaryFile{tmp}, ufs.CreateBuffered(tmp, write)
}

func (ufs *UFSFrontEnd) MTime(src Filename) time.Time {
	if info, err := src.Info(); err == nil {
		return info.ModTime()
	} else {
		LogPanicErr(LogUFS, err)
		return time.Time{}
	}
}
func (ufs *UFSFrontEnd) OpenFile(src Filename, read func(*os.File) error) error {
	input, err := os.Open(src.String())
	LogDebug(LogUFS, "open '%v'", src)

	if err == nil {
		defer func() {
			closeErr := input.Close()
			if err == nil {
				err = closeErr
			}
		}()
		if err = read(input); err == nil {
			return err
		}
	}

	LogWarning(LogUFS, "OpenFile: %v", err)
	return err
}
func (ufs *UFSFrontEnd) Open(src Filename, read func(io.Reader) error) error {
	return ufs.OpenFile(src, func(f *os.File) error {
		return read(f)
	})
}
func (ufs *UFSFrontEnd) OpenBuffered(src Filename, read func(io.Reader) error) error {
	return ufs.Open(src, func(r io.Reader) error {
		var buffered bufio.Reader
		buffered.Reset(r)
		return read(&buffered)
	})
}
func (ufs *UFSFrontEnd) ReadAll(src Filename) ([]byte, error) {
	var raw []byte
	err := UFS.OpenFile(src, func(f *os.File) error {
		var err error
		raw, err = io.ReadAll(f)
		return err
	})
	return raw, err
}
func (ufs *UFSFrontEnd) Read(src Filename, read func([]byte) error) error {
	return UFS.Open(src, func(r io.Reader) error {
		useBuffer := func(buffer []byte) error {
			n, err := r.Read(buffer)
			if err == io.EOF {
				err = nil
			}
			if err == nil {
				return read(buffer[:n])
			} else {
				return err
			}
		}

		// check if the file is small enough to fit in a transient buffer
		if info, err := src.Info(); info.Size() < LARGE_PAGE_CAPACITY {
			LogPanicIfFailed(LogUFS, err)

			transient := TransientLargePage.Allocate()
			defer TransientLargePage.Release(transient)
			return useBuffer(transient)

		} else {
			// for large files we revert to a dedicated allocation
			largeBuffer := make([]byte, info.Size())
			return useBuffer(largeBuffer) // don't want to keep large allocations alive
		}
	})
}
func (ufs *UFSFrontEnd) ReadLines(src Filename, line func(string) error) error {
	return ufs.Open(src, func(rd io.Reader) error {
		LogDebug(LogUFS, "read lines '%v'", src)

		const capacity = LARGE_PAGE_CAPACITY / 2
		buf := TransientLargePage.Allocate()
		defer TransientLargePage.Release(buf)

		scanner := bufio.NewScanner(rd)
		scanner.Buffer(buf, capacity)

		for scanner.Scan() {
			if err := scanner.Err(); err == nil {
				if err = line(scanner.Text()); err != nil {
					return err
				}
			} else {
				return err
			}
		}
		return nil
	})
}
func (ufs *UFSFrontEnd) Scan(src Filename, re *regexp.Regexp, match func([]string) error) error {
	return ufs.Open(src, func(rd io.Reader) error {
		LogDebug(LogUFS, "scan '%v' with regexp %v", src, re)

		const capacity = LARGE_PAGE_CAPACITY / 2
		buf := TransientLargePage.Allocate()
		defer TransientLargePage.Release(buf)

		scanner := bufio.NewScanner(rd)
		scanner.Buffer(buf, capacity)
		scanner.Split(SplitRegex(re, capacity))

		for scanner.Scan() {
			if err := scanner.Err(); err == nil {
				txt := scanner.Text()
				m := re.FindStringSubmatch(txt)
				if err := match(m[1:]); err != nil {
					return err
				}
			} else {
				return err
			}
		}
		return nil
	})
}
func (ufs *UFSFrontEnd) Rename(src, dst Filename) error {
	ufs.Mkdir(dst.Dirname)
	invalidate_file_info(src)
	invalidate_file_info(dst)
	LogDebug(LogUFS, "rename file '%v' to '%v'", src, dst)
	return os.Rename(src.String(), dst.String())
}
func (ufs *UFSFrontEnd) Copy(src, dst Filename) error {
	ufs.Mkdir(dst.Dirname)
	invalidate_file_info(dst)
	LogDebug(LogUFS, "copy file '%v' to '%v'", src, dst)
	return ufs.Open(src, func(r io.Reader) error {
		info, err := src.Info()
		if err != nil {
			return err
		}
		return ufs.Create(dst, func(w io.Writer) error {
			return CopyWithProgress(dst.Basename, info.Size(), w, r)
		})
	})
}

func (ufs *UFSFrontEnd) MountOutputDir(output Directory) error {
	LogVerbose(LogUFS, "mount output directory %q", output)
	ufs.Output = output
	ufs.Binaries = ufs.Output.Folder("Binaries")
	ufs.Cache = ufs.Output.Folder("Cache")
	ufs.Generated = ufs.Output.Folder("Generated")
	ufs.Intermediate = ufs.Output.Folder("Intermediate")
	ufs.Projects = ufs.Output.Folder("Projects")
	ufs.Scripts = ufs.Internal.Parent().Folder("scripts")
	ufs.Transient = ufs.Output.Folder("Transient")
	ufs.Saved = ufs.Output.Folder("Saved")
	return nil
}
func (ufs *UFSFrontEnd) MountRootDirectory(root Directory) error {
	LogVerbose(LogUFS, "mount root directory %q", root)
	if err := os.Chdir(root.String()); err != nil {
		return err
	}

	ufs.Root = root
	ufs.Extras = ufs.Root.Folder("Extras")
	ufs.Source = ufs.Root.Folder("Source")

	return ufs.MountOutputDir(ufs.Root.Folder("Output"))
}

func (ufs *UFSFrontEnd) GetWorkingDir() (Directory, error) {
	if wd, err := os.Getwd(); err == nil {
		return MakeDirectory(wd), nil
	} else {
		return Directory{}, err
	}
}
func (ufs *UFSFrontEnd) GetCallerFile(skip int) (Filename, error) {
	_, filename, _, ok := runtime.Caller(skip)
	if !ok {
		return Filename{}, errors.New("unable to get the current filename")
	}
	return MakeFilename(filename), nil
}
func (ufs *UFSFrontEnd) GetCallerFolder(skip int) (Directory, error) {
	if filename, err := ufs.GetCallerFile(skip); err == nil {
		return filename.Dirname, nil
	} else {
		return Directory{}, err
	}
}

func make_ufs_frontend() (ufs UFSFrontEnd) {
	executable, err := os.Executable()
	LogPanicIfFailed(LogUFS, err)

	ufs.Executable = MakeFilename(executable)
	if !ufs.Executable.Exists() {
		ufs.Executable = ufs.Executable.ReplaceExt(".exe")
	}
	if !ufs.Executable.Exists() {
		LogPanic(LogUFS, "executable path %q does not point to a valid file", ufs.Executable)
	}

	LogVeryVerbose(LogUFS, "mount executable file %q", ufs.Executable)

	ufs.Internal, err = ufs.GetCallerFolder(0)
	LogPanicIfFailed(LogUFS, err)

	ufs.Internal = ufs.Internal.Parent().Folder("internal")
	LogVeryVerbose(LogUFS, "mount internal directory %q", ufs.Internal)

	ufs.Root, err = ufs.GetWorkingDir()
	LogPanicIfFailed(LogUFS, err)

	err = ufs.MountRootDirectory(ufs.Root)
	LogPanicIfFailed(LogUFS, err)

	return ufs
}

func MakeGlobRegexp(glob ...string) *regexp.Regexp {
	if len(glob) == 0 {
		return nil
	}
	expr := "(?i)("
	for i, x := range glob {
		x = regexp.QuoteMeta(x)
		x = strings.ReplaceAll(x, "\\?", ".")
		x = strings.ReplaceAll(x, "\\*", ".*?")
		x = strings.ReplaceAll(x, "/", "[\\\\/]")
		x = "(" + x + ")"
		if i == 0 {
			expr += x
		} else {
			expr += "|" + x
		}
	}
	return regexp.MustCompile(expr + ")")
}

/***************************************
 * UFS Bindings for Build Graph
 ***************************************/

func (x *Filename) Alias() BuildAlias {
	return BuildAlias(x.String())
}
func (x Filename) Digest() (BuildStamp, error) {
	x.Invalidate()
	if info, err := x.Info(); err == nil {
		return MakeTimedBuildFingerprint(info.ModTime(), &x), nil
	} else {
		return BuildStamp{}, err
	}
}
func (x Filename) Build(bc BuildContext) error {
	x.Invalidate()
	if info, err := x.Info(); err == nil {
		bc.Annotate(SizeInBytes(info.Size()).String())
		bc.Timestamp(info.ModTime())
		return nil
	} else {
		return err
	}
}
func (x *Filename) Serialize(ar Archive) {
	ar.Serializable(&x.Dirname)
	ar.String(&x.Basename)
}

func (x *Directory) Alias() BuildAlias {
	return BuildAlias(x.String())
}
func (x Directory) Build(bc BuildContext) error {
	x.Invalidate()
	if info, err := x.Info(); err == nil {
		bc.Timestamp(GetCreationTime(info))
		return nil
	} else {
		return err
	}
}
func (x *Directory) Serialize(ar Archive) {
	ar.String(&x.Path)
}

func BuildFile(source Filename, staticDeps ...BuildAlias) BuildFactoryTyped[*Filename] {
	return MakeBuildFactory(func(bi BuildInitializer) (Filename, error) {
		return source.Normalize(), bi.DependsOn(staticDeps...)
	})
}
func BuildDirectory(source Directory, staticDeps ...BuildAlias) BuildFactoryTyped[*Directory] {
	return MakeBuildFactory(func(bi BuildInitializer) (Directory, error) {
		return source.Normalize(), bi.DependsOn(staticDeps...)
	})
}
