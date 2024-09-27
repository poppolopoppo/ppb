package utils

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
)

var LogUFS = base.NewLogCategory("UFS")

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
	base.Assert(func() bool { return len(in) > 0 })
	sb := strings.Builder{}
	capacity := len(in)
	for _, it := range args {
		base.Assert(func() bool { return len(it) > 0 })
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

func MakeBasename(path string) string {
	if index, found := lastIndexOfPathSeparator(path); found {
		return path[index+1:]
	} else {
		return path
	}
}
func MakeParentFolder(path string) string {
	if index, found := lastIndexOfPathSeparator(path); found {
		return path[0:index]
	} else {
		return ""
	}
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
	return MakeLocalDirectoryIFP(d, localPathEnabled)
}
func MakeLocalDirectoryIFP(d Directory, allowed bool) (relative string) {
	if allowed && localPathEnabled {
		return ForceLocalDirectory(d)
	}
	return d.String()
}

func MakeLocalFilename(f Filename) (relative string) {
	return MakeLocalFilenameIFP(f, localPathEnabled)
}
func MakeLocalFilenameIFP(f Filename, allowed bool) (relative string) {
	if allowed && localPathEnabled {
		return ForceLocalFilename(f)
	}
	return f.String()
}

func SafeNormalize[T interface {
	Normalize() T
	fmt.Stringer
	comparable
}](in T) T {
	base.AssertErr(func() error {
		if normalized := in.Normalize(); in == normalized {
			return nil
		} else {
			return fmt.Errorf("entry %q was not normalized ! real path is %q", in, normalized)
		}
	})
	return in
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
		base.UnexpectedValuePanic(d, d)
		return Directory{}
	}
}
func (d Directory) Split() (Directory, string) {
	if i, ok := lastIndexOfPathSeparator(d.String()); ok {
		return Directory{Path: d.Path[:i]}, d.String()[i+1:]
	} else {
		base.UnexpectedValuePanic(d, d)
		return Directory{}, ""
	}
}
func (d Directory) Folder(name ...string) Directory {
	if len(name) > 0 {
		return Directory{Path: JoinPath(d.String(), name...)}
	} else {
		return d
	}
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
	if n <= len(o.Path) {
		return d.Path == o.Path[:n]
	}
	return false
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
	return d.Path == o.Path
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
func (f Filename) SafeRelative(to Directory) string {
	if f.Valid() {
		return f.Relative(to)
	}
	return ""
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
	if len(f.Basename) > 0 && len(f.Dirname.Path) > 0 {
		return JoinPath(f.Dirname.Path, f.Basename)
	} else if len(f.Dirname.Path) == 0 {
		return f.Basename
	} else {
		return f.Dirname.Path
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
 * IO
 ***************************************/

func (f Filename) Info() (os.FileInfo, error) {
	return FileInfos.GetFileInfo(f)
}
func (f Filename) Invalidate() {
	FileInfos.InvalidateFile(f)
}
func (f Filename) Exists() bool {
	info, err := f.Info()
	return (err == nil && info != nil)
}

func (d Directory) Info() (os.FileInfo, error) {
	return FileInfos.GetDirectoryInfo(d)
}
func (d Directory) Invalidate() {
	FileInfos.InvalidateDirectory(d)
}
func (d Directory) Exists() bool {
	info, err := d.Info()
	return (err == nil && info != nil)
}
func (d Directory) Files() (results FileSet, err error) {
	files, _, err := FileInfos.EnumerateDirectory(d)
	return files, err
}
func (d Directory) Directories() (results DirSet, err error) {
	_, dirs, err := FileInfos.EnumerateDirectory(d)
	return dirs, err
}
func (d Directory) MatchDirectories(each func(Directory) error, r *regexp.Regexp) error {
	if r == nil {
		return nil
	}
	base.LogVeryVerbose(LogUFS, "match directories in '%v' for /%v/...", d, r)

	if _, dirs, err := FileInfos.EnumerateDirectory(d); err == nil {
		for _, it := range dirs {
			if r.MatchString(it.Basename()) {
				if err = each(it); err != nil {
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
	base.LogVeryVerbose(LogUFS, "match files in '%v' for /%v/...", d, r)

	if files, _, err := FileInfos.EnumerateDirectory(d); err == nil {
		for _, it := range files {
			if r.MatchString(it.Basename) {
				if err = each(it); err != nil {
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
	base.LogVeryVerbose(LogUFS, "match files rec in '%v' for /%v/...", d, r)

	if files, dirs, err := FileInfos.EnumerateDirectory(d); err == nil {
		for _, it := range files {
			if r.MatchString(it.Basename) {
				if err = each(it); err != nil {
					return err
				}
			}
		}
		for _, it := range dirs {
			if err = it.MatchFilesRec(each, r); err != nil {
				return err
			}
		}
		return nil
	} else {
		return err
	}
}

type findFileNotFoundError struct {
	d Directory
	r *regexp.Regexp
}

func (x findFileNotFoundError) Error() string {
	return fmt.Sprintf("file not found '%v' in '%v'", x.r, x.d)
}

func (d Directory) FindFileRec(r *regexp.Regexp) (Filename, error) {
	base.LogVeryVerbose(LogUFS, "find file rec in '%v' for /%v/...", d, r)

	if files, dirs, err := FileInfos.EnumerateDirectory(d); err == nil {
		for _, it := range files {
			if r.MatchString(it.Basename) {
				return it, nil
			}
		}
		for _, it := range dirs {
			if f, err := it.FindFileRec(r); err == nil {
				return f, nil
			}
		}
		return Filename{}, findFileNotFoundError{d: d, r: r}
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
	return base.IsUniq(list.Slice()...)
}
func (list DirSet) IsSorted() bool {
	return sort.IsSorted(list)
}
func (list DirSet) Sort() {
	sort.Sort(list)
}
func (list DirSet) Contains(it ...Directory) bool {
	for _, x := range it {
		if _, ok := base.IndexIf(x.Equals, list...); !ok {
			return false
		}
	}
	return true
}
func (list *DirSet) Append(it ...Directory) {
	*list = base.AppendEquatable_CheckUniq(*list, it...)
}
func (list *DirSet) AppendUniq(it ...Directory) {
	for _, x := range it {
		if !list.Contains(x) {
			*list = append(*list, x)
		}
	}
}
func (list *DirSet) Prepend(it ...Directory) {
	*list = base.PrependEquatable_CheckUniq(it, *list...)
}
func (list *DirSet) Remove(it ...Directory) {
	*list = base.Remove(*list, it...)
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
func (list *DirSet) Serialize(ar base.Archive) {
	base.SerializeSlice(ar, (*[]Directory)(list))
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
func (list DirSet) StringSet() base.StringSet {
	return base.MakeStringerSet(list.Slice()...)
}
func (list DirSet) Join(delim string) string {

	return base.JoinString(delim, list.Slice()...)
}
func (list DirSet) Local(path Directory) base.StringSet {
	return (base.StringSet)(base.Map(MakeLocalDirectory, list...))
}
func (list DirSet) Normalize() DirSet {
	return ((DirSet)(base.Map(func(it Directory) Directory {
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
func (list FileSet) Swap(i, j int)      { list[i], list[j] = list[j], list[i] }

func (list FileSet) IsUniq() bool {
	return base.IsUniq(list...)
}
func (list FileSet) IsSorted() bool {
	return sort.IsSorted(list)
}
func (list FileSet) Sort() {
	sort.Sort(list)
}
func (list FileSet) Contains(it ...Filename) bool {
	for _, x := range it {
		if _, ok := base.IndexIf(x.Equals, list...); !ok {
			return false
		}
	}
	return true
}
func (list *FileSet) Append(it ...Filename) {
	*list = base.AppendEquatable_CheckUniq(*list, it...)
}
func (list *FileSet) AppendUniq(it ...Filename) {
	for _, x := range it {
		if !list.Contains(x) {
			*list = append(*list, x)
		}
	}
}
func (list *FileSet) Prepend(it ...Filename) {
	*list = base.PrependEquatable_CheckUniq(it, *list...)
}
func (list *FileSet) Delete(i int) *FileSet {
	*list = base.Delete_DontPreserveOrder(*list, i)
	return list
}
func (list *FileSet) Remove(it ...Filename) {
	*list = base.Remove(*list, it...)
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
func (list FileSet) TotalSize() (result int64, err error) {
	var info os.FileInfo
	for _, x := range list {
		if info, err = x.Info(); err == nil {
			result += info.Size()
		} else {
			break
		}
	}
	return
}
func (list *FileSet) Serialize(ar base.Archive) {
	base.SerializeSlice(ar, (*[]Filename)(list))
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
func (list FileSet) StringSet() base.StringSet {
	return base.MakeStringerSet(list...)
}
func (list FileSet) Join(delim string) string {
	return base.JoinString(delim, list...)
}
func (list FileSet) Local(path Directory) base.StringSet {
	return (base.StringSet)(base.Map(MakeLocalFilename, list...))
}
func (list FileSet) Normalize() FileSet {
	return (FileSet)(base.Map(func(it Filename) Filename {
		return it.Normalize()
	}, list...))
}

/***************************************
 * JSON: marshal as string instead of array
 ***************************************/

func (x Filename) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *Filename) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}

func (x Directory) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *Directory) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
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
	base.LogDebug(LogUFS, "chtimes %v", dst)
	localPath := dst.String()
	if err := os.Chtimes(localPath, mtime, mtime); err == nil {
		dst.Invalidate()
		return nil
	} else {
		return err
	}
}
func (ufs *UFSFrontEnd) Remove(dst Filename) error {
	defer dst.Invalidate()
	if err := os.Remove(dst.String()); err != nil {
		base.LogError(LogUFS, "%v", err)
		return err
	}
	return nil
}
func (ufs *UFSFrontEnd) Mkdir(dst Directory) {
	if err := ufs.MkdirEx(dst); err != nil {
		base.LogPanicErr(LogUFS, err)
	}
}
func (ufs *UFSFrontEnd) MkdirEx(dst Directory) error {
	localPath := dst.String()
	if st, err := os.Stat(localPath); st != nil && (err == nil || os.IsExist(err)) {
		if !st.IsDir() {
			base.LogDebug(LogUFS, "mkdir %v", dst)
			return fmt.Errorf("ufs: %q already exist, but is not a directory", dst)
		}
	} else {
		base.LogDebug(LogUFS, "mkdir %v", dst)
		defer dst.Invalidate()
		if err := os.MkdirAll(localPath, os.ModePerm); err != nil {
			return fmt.Errorf("ufs: mkdir %q got error %v", dst, err)
		}
	}
	return nil
}
func (ufs *UFSFrontEnd) CreateWriter(dst Filename) (*os.File, error) {
	defer dst.Invalidate()
	ufs.Mkdir(dst.Dirname)
	base.LogDebug(LogUFS, "create '%v'", dst)
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
	base.LogWarning(LogUFS, "CreateFile: caught %v while trying to create %v", err, dst)
	return err
}
func (ufs *UFSFrontEnd) Create(dst Filename, write func(io.Writer) error) error {
	return ufs.CreateFile(dst, func(f *os.File) error {
		return write(f)
	})
}
func (ufs *UFSFrontEnd) CreateBuffered(dst Filename, write func(io.Writer) error, pageAlloc base.BytesRecycler) error {
	return ufs.CreateFile(dst, func(w *os.File) (err error) {
		if base.EnableAsyncIO {
			asyncWriter := base.NewAsyncWriter(w, pageAlloc)
			defer func() {
				if er := asyncWriter.Close(); er != nil && err == nil {
					err = er
				}
			}()
			err = write(&asyncWriter)
			return
		} else {
			var buffered bufio.Writer
			buffered.Reset(w)
			if err := write(&buffered); err != nil {
				return err
			}
			return buffered.Flush()
		}
	})
}

type TemporaryFile struct {
	Path Filename
}

func (x TemporaryFile) Close() error   { return UFS.Remove(x.Path) }
func (x TemporaryFile) String() string { return x.Path.String() }

func (ufs *UFSFrontEnd) CreateTemp(prefix string, write func(io.Writer) error, pageAlloc base.BytesRecycler) (TemporaryFile, error) {
	randBytes := [16]byte{}
	rand.Read(randBytes[:])
	tmp := UFS.Transient.Folder(prefix).File(hex.EncodeToString(randBytes[:]))
	return TemporaryFile{tmp}, ufs.CreateBuffered(tmp, write, pageAlloc)
}

func (ufs *UFSFrontEnd) MTime(src Filename) time.Time {
	if info, err := src.Info(); err == nil {
		return info.ModTime()
	} else {
		base.LogPanicErr(LogUFS, err)
		return time.Time{}
	}
}
func (ufs *UFSFrontEnd) OpenFile(src Filename, read func(*os.File) error) error {
	input, err := os.Open(src.String())
	base.LogDebug(LogUFS, "open '%v'", src)

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

	base.LogWarning(LogUFS, "OpenFile: %v", err)
	return err
}
func (ufs *UFSFrontEnd) Open(src Filename, read func(io.Reader) error) error {
	return ufs.OpenFile(src, func(f *os.File) error {
		return read(f)
	})
}
func (ufs *UFSFrontEnd) OpenBuffered(src Filename, read func(io.Reader) error) error {
	return ufs.OpenFile(src, func(r *os.File) (err error) {
		if base.EnableAsyncIO {
			asyncReader := base.NewAsyncReader(r, base.TransientPage4KiB)
			defer func() {
				if er := asyncReader.Close(); er != nil && err == nil {
					err = er
				}
			}()
			err = read(&asyncReader)
			return
		} else {
			var buffered bufio.Reader
			buffered.Reset(r)
			return read(&buffered)
		}
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
		if info, err := src.Info(); info.Size() < int64(base.TransientPage1MiB.Stride()) {
			base.LogPanicIfFailed(LogUFS, err)

			transient := base.TransientPage1MiB.Allocate()
			defer base.TransientPage1MiB.Release(transient)
			return useBuffer(*transient)

		} else {
			// for large files we revert to a dedicated allocation
			largeBuffer := make([]byte, info.Size())
			return useBuffer(largeBuffer) // don't want to keep large allocations alive
		}
	})
}
func (ufs *UFSFrontEnd) ReadLines(src Filename, line func(string) error) error {
	return ufs.Open(src, func(rd io.Reader) error {
		base.LogDebug(LogUFS, "read lines '%v'", src)

		buf := base.TransientPage64KiB.Allocate()
		defer base.TransientPage64KiB.Release(buf)

		scanner := bufio.NewScanner(rd)
		scanner.Buffer(*buf, len(*buf)/2)

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
		base.LogDebug(LogUFS, "scan '%v' with regexp %v", src, re)

		buf := base.TransientPage64KiB.Allocate()
		defer base.TransientPage64KiB.Release(buf)
		capacity := len(*buf) / 2

		scanner := bufio.NewScanner(rd)
		scanner.Buffer(*buf, capacity)
		scanner.Split(base.SplitRegex(re, capacity))

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
	defer func() {
		src.Invalidate()
		dst.Invalidate()
	}()
	base.LogDebug(LogUFS, "rename file '%v' to '%v'", src, dst)
	return os.Rename(src.String(), dst.String())
}
func (ufs *UFSFrontEnd) Copy(src, dst Filename) error {
	ufs.Mkdir(dst.Dirname)
	defer dst.Invalidate()

	base.LogDebug(LogUFS, "copy file '%v' to '%v'", src, dst)
	return ufs.OpenFile(src, func(r *os.File) error {
		info, err := r.Stat()
		FileInfos.SetFileInfo(src, info, err)
		if err != nil {
			return err
		}

		return ufs.CreateFile(dst, func(w *os.File) error {
			if err := base.SetMTime(w, info.ModTime()); err == nil {
				return base.CopyWithProgress(dst.Basename, info.Size(), w, r)
			} else {
				return err
			}
		})
	})
}
func (ufs *UFSFrontEnd) Crc32(src Filename) (checksum uint32, err error) {
	base.LogDebug(LogUFS, "crc32 file %q", src)
	err = ufs.OpenFile(src, func(f *os.File) error {
		stat, err := f.Stat()
		FileInfos.SetFileInfo(src, stat, err)
		if err != nil {
			return err
		}

		totalSize := stat.Size()
		pageAlloc := base.GetBytesRecyclerBySize(totalSize)

		crc := crc32.NewIEEE()
		if _, err = base.TransientIoCopy(crc, f, pageAlloc, true); err == nil {
			checksum = crc.Sum32()
		}
		return err
	})
	return
}
func (ufs *UFSFrontEnd) Fingerprint(src Filename, seed base.Fingerprint) (base.Fingerprint, error) {
	base.LogDebug(LogUFS, "fingerprint file %q", src)
	var fingerprint base.Fingerprint
	if err := ufs.OpenFile(src, func(f *os.File) error {
		stat, err := f.Stat()
		FileInfos.SetFileInfo(src, stat, err)
		if err != nil {
			return err
		}

		totalSize := stat.Size()
		pageAlloc := base.GetBytesRecyclerBySize(totalSize)
		fingerprint, err = base.ReaderFingerprint(f, seed, pageAlloc, totalSize > int64(pageAlloc.Stride()))
		return err
	}); err != nil {
		return base.Fingerprint{}, err
	}
	return fingerprint, nil
}

func (ufs *UFSFrontEnd) MountOutputDir(output Directory) error {
	base.LogVerbose(LogUFS, "mount output directory %q", output)
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
	base.LogVerbose(LogUFS, "mount root directory %q", root)
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
	base.LogPanicIfFailed(LogUFS, err)

	ufs.Executable = MakeFilename(executable)
	if !ufs.Executable.Exists() {
		ufs.Executable = ufs.Executable.ReplaceExt(".exe")
	}
	if !ufs.Executable.Exists() {
		base.LogPanic(LogUFS, "executable path %q does not point to a valid file", ufs.Executable)
	}

	base.LogVeryVerbose(LogUFS, "mount executable file %q", ufs.Executable)

	ufs.Internal, err = ufs.GetCallerFolder(0)
	base.LogPanicIfFailed(LogUFS, err)

	ufs.Internal = ufs.Internal.Parent().Folder("internal")
	base.LogVeryVerbose(LogUFS, "mount internal directory %q", ufs.Internal)

	ufs.Root, err = ufs.GetWorkingDir()
	base.LogPanicIfFailed(LogUFS, err)

	err = ufs.MountRootDirectory(ufs.Root)
	base.LogPanicIfFailed(LogUFS, err)

	return ufs
}

func MakeGlobRegexpExpr(glob ...string) string {
	if len(glob) == 0 {
		return ".*"
	}
	var expr strings.Builder
	expr.WriteString("(?i)(")
	for i, it := range glob {
		it = regexp.QuoteMeta(it)
		it = strings.ReplaceAll(it, "\\?", ".")
		it = strings.ReplaceAll(it, "\\*", ".*?")
		it = strings.ReplaceAll(it, "/", "[\\\\/]")
		it = "(" + it + ")"
		if i > 0 {
			expr.WriteString("|")
		}
		expr.WriteString(it)
	}
	expr.WriteString(")")
	return expr.String()
}
func MakeGlobRegexp(glob ...string) *regexp.Regexp {
	if len(glob) == 0 {
		return nil
	}
	return regexp.MustCompile(MakeGlobRegexpExpr(glob...))
}

/***************************************
 * UFS serialization and transformation
 ***************************************/

func (x *Filename) Serialize(ar base.Archive) {
	ar.Serializable(&x.Dirname)
	ar.String(&x.Basename)
}

func (x *Directory) Serialize(ar base.Archive) {
	ar.String(&x.Path)
}

func MakeDirSet(root Directory, paths ...string) (result DirSet) {
	return base.Map(func(x string) Directory {
		return root.AbsoluteFolder(x).Normalize()
	}, paths...)
}

func MakeFileSet(root Directory, paths ...string) (result FileSet) {
	return base.Map(func(x string) Filename {
		return root.AbsoluteFile(x).Normalize()
	}, paths...)
}
