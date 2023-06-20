package io

import (
	"archive/tar"
	"os"

	"github.com/klauspost/compress/zip"
	"github.com/mholt/archiver/v3"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var LogCompressedArchive = NewLogCategory("CompressedArchive")

type CompressedArchiveFromDownload struct {
	Download   *Downloader
	ExtractDir Directory
}

func (x CompressedArchiveFromDownload) Equals(other CompressedArchiveFromDownload) bool {
	return (x.Download == other.Download && x.ExtractDir.Equals(other.ExtractDir))
}

func BuildCompressedUnarchiver(src Filename, dst Directory, acceptList StringSet, staticDeps ...BuildAliasable) BuildFactoryTyped[*CompressedUnarchiver] {
	return MakeBuildFactory(func(bi BuildInitializer) (CompressedUnarchiver, error) {
		err := CreateDirectory(bi, dst)
		return CompressedUnarchiver{
				Source:      src,
				Destination: dst,
				AcceptList:  acceptList,
			}, AnyError(
				err,
				bi.NeedFile(src),
				bi.NeedBuildable(staticDeps...))
	})
}
func BuildCompressedArchiveExtractorFromDownload(prms CompressedArchiveFromDownload, acceptList StringSet) BuildFactoryTyped[*CompressedUnarchiver] {
	return BuildCompressedUnarchiver(
		prms.Download.Destination,
		prms.ExtractDir,
		acceptList,
		prms.Download)
}

/***************************************
 * Compressed Archive Extractor
 ***************************************/

type CompressedUnarchiver struct {
	Source         Filename
	Destination    Directory
	AcceptList     StringSet
	ExtractedFiles FileSet
}

func (x *CompressedUnarchiver) Alias() BuildAlias {
	return MakeBuildAlias("Unarchiver", x.Destination.String())
}
func (x *CompressedUnarchiver) Build(bc BuildContext) error {
	x.ExtractedFiles.Clear()

	globRe := MakeGlobRegexp(x.AcceptList.Slice()...)
	exportFilter := func(s string) (Filename, bool) {
		dst := x.Destination.AbsoluteFile(s)
		match := true
		if globRe != nil {
			match = globRe.MatchString(s)
		}
		if match {
			x.ExtractedFiles.Append(dst)
			return dst, true
		}
		return dst, false
	}

	if err := archiver.Walk(x.Source.String(), func(src archiver.File) error {
		defer src.Close()
		if src.IsDir() {
			return nil
		}

		// /!\ src.Name() returns the basename of the file, but we need an absolute path
		// instead we need to cast explicitly src.Header to a concrete type...
		var relativePath string
		switch header := src.Header.(type) {
		case *tar.Header:
			relativePath = header.Name
		case zip.FileHeader:
			relativePath = header.Name
		default:
			UnexpectedValuePanic(src.Header, header)
		}

		if destination, extract := exportFilter(relativePath); extract {
			LogDebug(LogCompressedArchive, "%v: extracting %q file", x.Source, relativePath)
			return UFS.CreateFile(destination, func(dst *os.File) error {
				_, err := dst.ReadFrom(src)
				return err
			})
		}

		return nil
	}); err != nil {
		return err
	}
	AssertMessage(func() bool { return len(x.ExtractedFiles) > 0 }, "%v: did not extract any files!", x.Source)

	for _, f := range x.ExtractedFiles {
		if err := bc.OutputFile(f); err != nil {
			return err
		}
	}

	// avoid re-extracting after each rebuild
	bc.Timestamp(UFS.MTime(x.Source))
	return nil
}
func (x *CompressedUnarchiver) Serialize(ar Archive) {
	ar.Serializable(&x.Source)
	ar.Serializable(&x.Destination)
	ar.Serializable(&x.AcceptList)
	ar.Serializable(&x.ExtractedFiles)
}
