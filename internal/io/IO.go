package io

import (
	"fmt"
	"io"
	"strings"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

func InitIO() {
	RegisterSerializable(&CompressedUnarchiver{})
	RegisterSerializable(&Downloader{})

	RegisterSerializable(&DirectoryCreator{})
	RegisterSerializable(&DirectoryGlob{})
	RegisterSerializable(&DirectoryList{})
	RegisterSerializable(&FileDigest{})
}

/***************************************
 * Cluster Observable Writer
 ***************************************/

type ObservableWriterFunc = func(w io.Writer, buf []byte) (int, error)

type ObservableWriter struct {
	io.WriteCloser
	OnWrite ObservableWriterFunc
}

func NewObservableWriter(w io.WriteCloser, onWrite ObservableWriterFunc) ObservableWriter {
	Assert(func() bool { return w != nil })
	Assert(func() bool { return onWrite != nil })
	return ObservableWriter{
		WriteCloser: w,
		OnWrite:     onWrite,
	}
}
func (x ObservableWriter) Flush() error {
	return FlushWriterIFP(x.WriteCloser)
}
func (x ObservableWriter) Reset(w io.Writer) error {
	if err := FlushWriterIFP(x.WriteCloser); err != nil {
		return err
	}
	if rst, ok := x.WriteCloser.(WriteReseter); ok {
		return rst.Reset(w)
	}
	return nil
}
func (x ObservableWriter) Write(buf []byte) (int, error) {
	if x.OnWrite != nil {
		return x.OnWrite(x.WriteCloser, buf)
	} else {
		return x.WriteCloser.Write(buf)
	}
}

/***************************************
 * Cluster Observable Reader
 ***************************************/

type ObservableReaderFunc = func(r io.Reader, buf []byte) (int, error)

type ObservableReader struct {
	io.ReadCloser
	OnRead ObservableReaderFunc
}

func NewObservableReader(r io.ReadCloser, onRead ObservableReaderFunc) ObservableReader {
	Assert(func() bool { return r != nil })
	Assert(func() bool { return onRead != nil })
	return ObservableReader{
		ReadCloser: r,
		OnRead:     onRead,
	}
}
func (x ObservableReader) Reset(r io.Reader) error {
	if rst, ok := x.ReadCloser.(ReadReseter); ok {
		return rst.Reset(r)
	}
	return nil
}
func (x ObservableReader) Read(buf []byte) (int, error) {
	if x.OnRead != nil {
		return x.OnRead(x.ReadCloser, buf)
	} else {
		return x.ReadCloser.Read(buf)
	}
}

/***************************************
 * File Digest
 ***************************************/

func DigestFile(bc BuildContext, source Filename) (Fingerprint, error) {
	file, err := BuildFileDigest(source).Need(bc)
	return file.Digest, err
}

type FileDigest struct {
	Source Filename
	Digest Fingerprint
}

func BuildFileDigest(source Filename) BuildFactoryTyped[*FileDigest] {
	Assert(func() bool { return source.Valid() })
	return MakeBuildFactory(func(bi BuildInitializer) (FileDigest, error) {
		source = source.Normalize()
		return FileDigest{
			Source: source,
		}, bi.NeedFile(source)
	})
}

func BuildFileDigests(bg BuildGraph, filenames []Filename, options ...BuildOptionFunc) Future[[]*FileDigest] {
	aliases := Map(func(f Filename) BuildAlias {
		fd, err := BuildFileDigest(f).Init(bg, options...)
		LogPanicIfFailed(LogUFS, err)
		return fd.Alias()
	}, filenames...)

	future := bg.BuildMany(aliases, options...)

	return MapFuture(future, func(results []BuildResult) ([]*FileDigest, error) {
		digests := make([]*FileDigest, len(results))
		for i, ret := range results {
			digests[i] = ret.Buildable.(*FileDigest)
		}
		return digests, nil
	})
}

func (x *FileDigest) Alias() BuildAlias {
	return MakeBuildAlias("UFS", "Digest", x.Source.String())
}
func (x *FileDigest) Build(bc BuildContext) (err error) {
	x.Digest, err = FileFingerprint(x.Source, Fingerprint{} /* no seed here */)
	LogTrace(LogUFS, "file digest %s for %q", x.Digest, x.Source)
	return
}
func (x *FileDigest) Serialize(ar Archive) {
	ar.Serializable(&x.Source)
	ar.Serializable(&x.Digest)
}

/***************************************
 * Directory Creator
 ***************************************/

func CreateDirectory(bc BuildInitializer, source Directory) error {
	_, err := BuildDirectoryCreator(source).Need(bc)
	return err
}

type DirectoryCreator struct {
	Source Directory
}

func BuildDirectoryCreator(source Directory) BuildFactoryTyped[*DirectoryCreator] {
	Assert(func() bool { return source.Valid() })
	return MakeBuildFactory(func(init BuildInitializer) (DirectoryCreator, error) {
		return DirectoryCreator{
			Source: source.Normalize(),
		}, nil
	})
}

func (x *DirectoryCreator) Alias() BuildAlias {
	return MakeBuildAlias("UFS", "Create", x.Source.String())
}
func (x *DirectoryCreator) Build(bc BuildContext) error {
	if err := bc.OutputNode(BuildDirectory(x.Source)); err != nil {
		return err
	}

	return UFS.MkdirEx(x.Source)
}
func (x *DirectoryCreator) Serialize(ar Archive) {
	ar.Serializable(&x.Source)
}

/***************************************
 * Directory List
 ***************************************/

func ListDirectory(bc BuildContext, source Directory) (FileSet, error) {
	factory := BuildDirectoryList(source)
	if list, err := factory.Need(bc); err == nil {
		return list.Results, nil
	} else {
		return FileSet{}, err
	}
}

type DirectoryList struct {
	Source  Directory
	Results FileSet
}

func BuildDirectoryList(source Directory) BuildFactoryTyped[*DirectoryList] {
	Assert(func() bool { return source.Valid() })
	return MakeBuildFactory(func(init BuildInitializer) (DirectoryList, error) {
		return DirectoryList{
			Source:  source.Normalize(),
			Results: FileSet{},
		}, init.NeedDirectory(source)
	})
}

func (x *DirectoryList) Alias() BuildAlias {
	return MakeBuildAlias("UFS", "List", x.Source.String())
}
func (x *DirectoryList) Build(bc BuildContext) error {
	x.Results = FileSet{}

	if info, err := x.Source.Info(); err == nil {
		bc.Timestamp(info.ModTime())
	} else {
		return err
	}

	x.Results = x.Source.Files()
	for i, filename := range x.Results {
		filename = filename.Normalize()
		x.Results[i] = filename

		if err := bc.NeedFile(filename); err != nil {
			return err
		}
	}

	bc.Annotate(fmt.Sprintf("%d files", len(x.Results)))
	return nil
}
func (x *DirectoryList) Serialize(ar Archive) {
	ar.Serializable(&x.Source)
	ar.Serializable(&x.Results)
}

/***************************************
 * Directory Glob
 ***************************************/

func GlobDirectory(
	bc BuildContext,
	source Directory,
	includedGlobs StringSet,
	excludedGlobs StringSet,
	excludedFiles FileSet) (FileSet, error) {
	factory := BuildDirectoryGlob(source, includedGlobs, excludedGlobs, excludedFiles)
	if glob, err := factory.Need(bc); err == nil {
		return glob.Results, nil
	} else {
		return FileSet{}, err
	}
}

type DirectoryGlob struct {
	Source        Directory
	IncludedGlobs StringSet
	ExcludedGlobs StringSet
	ExcludedFiles FileSet
	Results       FileSet
}

func BuildDirectoryGlob(
	source Directory,
	includedGlobs StringSet,
	excludedGlobs StringSet,
	excludedFiles FileSet) BuildFactoryTyped[*DirectoryGlob] {
	Assert(func() bool { return source.Valid() })
	Assert(func() bool { return len(includedGlobs) > 0 })
	return MakeBuildFactory(func(init BuildInitializer) (DirectoryGlob, error) {
		return DirectoryGlob{
			Source:        source.Normalize(),
			IncludedGlobs: includedGlobs,
			ExcludedGlobs: excludedGlobs,
			ExcludedFiles: excludedFiles,
			Results:       FileSet{},
		}, nil //init.NeedDirectory(source) // no dependency so to be built every-time
	})
}

func (x *DirectoryGlob) Alias() BuildAlias {
	return MakeBuildAlias("UFS", "Glob", strings.Join([]string{
		x.Source.String(),
		x.IncludedGlobs.Join(";"),
		x.ExcludedGlobs.Join(";"),
		x.ExcludedFiles.Join(";")},
		"|"))
}
func (x *DirectoryGlob) Build(bc BuildContext) error {
	x.Results = FileSet{}

	if dirInfo, err := x.Source.Info(); err == nil {
		bc.Timestamp(dirInfo.ModTime())
	} else {
		return err
	}

	includeRE := MakeGlobRegexp(x.IncludedGlobs...)
	excludeRE := MakeGlobRegexp(x.ExcludedGlobs...)
	if includeRE == nil {
		includeRE = MakeGlobRegexp("*")
	}

	err := x.Source.MatchFilesRec(func(f Filename) error {
		// f = f.Normalize()
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
func (x *DirectoryGlob) Serialize(ar Archive) {
	ar.Serializable(&x.Source)
	ar.Serializable(&x.IncludedGlobs)
	ar.Serializable(&x.ExcludedGlobs)
	ar.Serializable(&x.ExcludedFiles)
	ar.Serializable(&x.Results)
}
