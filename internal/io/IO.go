package io

import (
	"fmt"
	"io"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

var LogIO = base.NewLogCategory("IO")

func InitIO() {
	base.LogTrace(LogIO, "internal/io.Init()")

	base.RegisterSerializable(&CompressedUnarchiver{})
	base.RegisterSerializable(&Downloader{})

	base.RegisterSerializable(&DirectoryCreator{})
	base.RegisterSerializable(&DirectoryGlob{})
	base.RegisterSerializable(&DirectoryList{})
	base.RegisterSerializable(&FileDigest{})
}

/***************************************
 * Observable Writer
 ***************************************/

type ObservableWriterFunc = func(w io.Writer, buf []byte) (int, error)

type ObservableWriter struct {
	io.Writer
	OnWrite ObservableWriterFunc
}

func NewObservableWriter(w io.Writer, onWrite ObservableWriterFunc) ObservableWriter {
	base.Assert(func() bool { return w != nil })
	base.Assert(func() bool { return onWrite != nil })
	return ObservableWriter{
		Writer:  w,
		OnWrite: onWrite,
	}
}

func (x ObservableWriter) Flush() error {
	return base.FlushWriterIFP(x.Writer)
}
func (x ObservableWriter) Close() error {
	if cls, ok := x.Writer.(io.WriteCloser); ok {
		return cls.Close()
	}
	return nil
}
func (x ObservableWriter) Reset(w io.Writer) error {
	if err := base.FlushWriterIFP(x.Writer); err != nil {
		return err
	}
	if rst, ok := x.Writer.(base.WriteReseter); ok {
		return rst.Reset(w)
	}
	return nil
}
func (x ObservableWriter) Write(buf []byte) (int, error) {
	if x.OnWrite != nil {
		return x.OnWrite(x.Writer, buf)
	} else {
		return x.Writer.Write(buf)
	}
}

/***************************************
 * Observable Reader
 ***************************************/

type ObservableReaderFunc = func(r io.Reader, buf []byte) (int, error)

type ObservableReader struct {
	io.Reader
	OnRead ObservableReaderFunc
}

func NewObservableReader(r io.Reader, onRead ObservableReaderFunc) ObservableReader {
	base.Assert(func() bool { return r != nil })
	base.Assert(func() bool { return onRead != nil })
	return ObservableReader{
		Reader: r,
		OnRead: onRead,
	}
}
func (x ObservableReader) Close() error {
	if cls, ok := x.Reader.(io.ReadCloser); ok {
		return cls.Close()
	}
	return nil
}
func (x ObservableReader) Reset(r io.Reader) error {
	if rst, ok := x.Reader.(base.ReadReseter); ok {
		return rst.Reset(r)
	}
	return nil
}
func (x ObservableReader) Read(buf []byte) (int, error) {
	if x.OnRead != nil {
		return x.OnRead(x.Reader, buf)
	} else {
		return x.Reader.Read(buf)
	}
}

/***************************************
 * File Digest
 ***************************************/

func DigestFile(bc utils.BuildContext, source utils.Filename) (base.Fingerprint, error) {
	file, err := BuildFileDigest(source).Need(bc)
	return file.Digest, err
}

type FileDigest struct {
	Source utils.Filename
	Digest base.Fingerprint
}

func BuildFileDigest(source utils.Filename) utils.BuildFactoryTyped[*FileDigest] {
	base.Assert(func() bool { return source.Valid() })
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (FileDigest, error) {
		return FileDigest{
			Source: utils.SafeNormalize(source),
		}, bi.NeedFiles(source)
	})
}

func PrepareFileDigests(bg utils.BuildGraph, n int, filenames func(int) utils.Filename, options ...utils.BuildOptionFunc) []base.Future[*FileDigest] {
	results := make([]base.Future[*FileDigest], n)
	for i := range results {
		results[i] = base.MapFuture(
			utils.PrepareBuildFactory(bg, BuildFileDigest(filenames(i)), options...),
			func(in utils.BuildResult) (*FileDigest, error) {
				return in.Buildable.(*FileDigest), nil
			})
	}

	return results
}

func (x *FileDigest) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("UFS", "Digest", x.Source.Dirname.Path, x.Source.Basename)
}
func (x *FileDigest) Build(bc utils.BuildContext) (err error) {
	x.Digest, err = utils.FileFingerprint(x.Source, base.Fingerprint{} /* no seed here */)
	base.LogTrace(utils.LogUFS, "file digest %s for %q", x.Digest, x.Source)
	return
}
func (x *FileDigest) Serialize(ar base.Archive) {
	ar.Serializable(&x.Source)
	ar.Serializable(&x.Digest)
}

/***************************************
 * Directory Creator
 ***************************************/

func CreateDirectory(bc utils.BuildInitializer, source utils.Directory) error {
	_, err := BuildDirectoryCreator(source).Need(bc)
	return err
}

type DirectoryCreator struct {
	Source utils.Directory
}

func BuildDirectoryCreator(source utils.Directory) utils.BuildFactoryTyped[*DirectoryCreator] {
	base.Assert(func() bool { return source.Valid() })
	return utils.MakeBuildFactory(func(init utils.BuildInitializer) (DirectoryCreator, error) {
		return DirectoryCreator{
			Source: utils.SafeNormalize(source),
		}, nil
	})
}

func (x *DirectoryCreator) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("UFS", "Create", x.Source.Path)
}
func (x *DirectoryCreator) Build(bc utils.BuildContext) error {
	if err := bc.OutputNode(utils.BuildDirectory(x.Source)); err != nil {
		return err
	}

	return utils.UFS.MkdirEx(x.Source)
}
func (x *DirectoryCreator) Serialize(ar base.Archive) {
	ar.Serializable(&x.Source)
}

/***************************************
 * Directory List
 ***************************************/

func ListDirectory(bc utils.BuildContext, source utils.Directory) (utils.FileSet, error) {
	factory := BuildDirectoryList(source)
	if list, err := factory.Need(bc); err == nil {
		return list.Results, nil
	} else {
		return utils.FileSet{}, err
	}
}

type DirectoryList struct {
	Source  utils.Directory
	Results utils.FileSet
}

func BuildDirectoryList(source utils.Directory) utils.BuildFactoryTyped[*DirectoryList] {
	base.Assert(func() bool { return source.Valid() })
	return utils.MakeBuildFactory(func(init utils.BuildInitializer) (DirectoryList, error) {
		return DirectoryList{
			Source:  utils.SafeNormalize(source),
			Results: utils.FileSet{},
		}, init.NeedDirectories(source)
	})
}

func (x *DirectoryList) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("UFS", "List", x.Source.Path)
}
func (x *DirectoryList) Build(bc utils.BuildContext) error {
	x.Results = utils.FileSet{}

	if info, err := x.Source.Info(); err == nil {
		bc.Timestamp(info.ModTime())
	} else {
		return err
	}

	var err error
	x.Results, err = x.Source.Files()
	if err != nil {
		return err
	}

	for i, filename := range x.Results {
		filename = utils.SafeNormalize(filename)
		x.Results[i] = filename

		if err := bc.NeedFiles(filename); err != nil {
			return err
		}
	}

	bc.Annotate(fmt.Sprintf("%d files", len(x.Results)))
	return nil
}
func (x *DirectoryList) Serialize(ar base.Archive) {
	ar.Serializable(&x.Source)
	ar.Serializable(&x.Results)
}

/***************************************
 * Directory Glob
 ***************************************/

func GlobDirectory(
	bc utils.BuildContext,
	source utils.Directory,
	includedGlobs base.StringSet,
	excludedGlobs base.StringSet,
	excludedFiles utils.FileSet) (utils.FileSet, error) {
	factory := BuildDirectoryGlob(source, includedGlobs, excludedGlobs, excludedFiles)
	if glob, err := factory.Need(bc); err == nil {
		return glob.Results, nil
	} else {
		return utils.FileSet{}, err
	}
}

type DirectoryGlob struct {
	Source        utils.Directory
	IncludedGlobs base.StringSet
	ExcludedGlobs base.StringSet
	ExcludedFiles utils.FileSet
	Results       utils.FileSet
}

func BuildDirectoryGlob(
	source utils.Directory,
	includedGlobs base.StringSet,
	excludedGlobs base.StringSet,
	excludedFiles utils.FileSet) utils.BuildFactoryTyped[*DirectoryGlob] {
	base.Assert(func() bool { return source.Valid() })
	base.Assert(func() bool { return len(includedGlobs) > 0 })
	return utils.MakeBuildFactory(func(init utils.BuildInitializer) (DirectoryGlob, error) {
		return DirectoryGlob{
			Source:        utils.SafeNormalize(source),
			IncludedGlobs: includedGlobs,
			ExcludedGlobs: excludedGlobs,
			ExcludedFiles: excludedFiles,
			Results:       utils.FileSet{},
		}, nil //init.NeedDirectories(source) // no dependency so to be built every-time
	})
}

func (x *DirectoryGlob) Alias() utils.BuildAlias {
	var bb utils.BuildAliasBuilder
	utils.MakeBuildAliasBuilder(&bb, "UFS", len(x.Source.Path)+1+len(x.Source.Basename())+4)

	bb.WriteString('/', "Glob")
	bb.WriteString('/', x.Source.Path)
	bb.WriteString('/', x.Source.Basename())

	for i, it := range x.IncludedGlobs {
		if i == 0 {
			bb.WriteString('|', it)
		} else {
			bb.WriteString(';', it)
		}
	}

	for i, it := range x.ExcludedGlobs {
		if i == 0 {
			bb.WriteString('|', it)
		} else {
			bb.WriteString(';', it)
		}
	}

	for i, it := range x.ExcludedFiles {
		if i == 0 {
			bb.WriteString('|', it.Dirname.Path)
		} else {
			bb.WriteString(';', it.Dirname.Path)
		}
		bb.WriteString('/', it.Basename)
	}

	return bb.Alias()
}
func (x *DirectoryGlob) Build(bc utils.BuildContext) error {
	x.Results = utils.FileSet{}

	if dirInfo, err := x.Source.Info(); err == nil {
		bc.Timestamp(dirInfo.ModTime())
	} else {
		return err
	}

	includeRE := utils.MakeGlobRegexp(x.IncludedGlobs...)
	excludeRE := utils.MakeGlobRegexp(x.ExcludedGlobs...)
	if includeRE == nil {
		includeRE = utils.MakeGlobRegexp("*")
	}

	err := x.Source.MatchFilesRec(func(f utils.Filename) error {
		f = utils.SafeNormalize(f)
		if !x.ExcludedFiles.Contains(f) {
			if excludeRE == nil || !excludeRE.MatchString(f.String()) {
				x.Results.Append(f)
			}
		}
		return nil
	}, includeRE)

	if err == nil {
		bc.Annotate(fmt.Sprintf("%d files", len(x.Results)))
	}

	return err
}
func (x *DirectoryGlob) Serialize(ar base.Archive) {
	ar.Serializable(&x.Source)
	ar.Serializable(&x.IncludedGlobs)
	ar.Serializable(&x.ExcludedGlobs)
	ar.Serializable(&x.ExcludedFiles)
	ar.Serializable(&x.Results)
}
