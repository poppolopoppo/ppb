package io

import (
	"archive/tar"
	"fmt"
	"os"

	"github.com/klauspost/compress/zip"
	"github.com/mholt/archiver/v3"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

var LogCompressedArchive = base.NewLogCategory("CompressedArchive")

type CompressedArchiveFromDownload struct {
	Download   *Downloader
	ExtractDir utils.Directory
}

func (x CompressedArchiveFromDownload) Equals(other CompressedArchiveFromDownload) bool {
	return (x.Download == other.Download && x.ExtractDir.Equals(other.ExtractDir))
}

func BuildCompressedUnarchiver(src utils.Filename, dst utils.Directory, acceptList base.StringSet, staticDeps ...utils.BuildAliasable) utils.BuildFactoryTyped[*CompressedUnarchiver] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (CompressedUnarchiver, error) {
		err := CreateDirectory(bi, dst)
		return CompressedUnarchiver{
				Source:      src,
				Destination: dst,
				AcceptList:  acceptList,
			}, base.AnyError(
				err,
				bi.NeedFiles(src),
				bi.NeedBuildables(staticDeps...))
	})
}
func BuildCompressedArchiveExtractorFromDownload(prms CompressedArchiveFromDownload, acceptList base.StringSet) utils.BuildFactoryTyped[*CompressedUnarchiver] {
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
	Source         utils.Filename
	Destination    utils.Directory
	AcceptList     base.StringSet
	ExtractedFiles utils.FileSet
}

func (x *CompressedUnarchiver) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("Unarchiver", x.Destination.String())
}
func (x *CompressedUnarchiver) Build(bc utils.BuildContext) error {
	x.ExtractedFiles.Clear()

	globRe := utils.MakeGlobRegexp(x.AcceptList.Slice()...)
	exportFilter := func(s string) (utils.Filename, bool) {
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
			base.UnexpectedValuePanic(src.Header, header)
		}

		if destination, extract := exportFilter(relativePath); extract {
			base.LogDebug(LogCompressedArchive, "%v: extracting %q file", x.Source, relativePath)
			return utils.UFS.CreateFile(destination, func(dst *os.File) error {
				_, err := base.TransientIoCopyWithProgress(src.Name(), src.Size(), dst, src)
				return err
			})
		}

		return nil
	}); err != nil {
		return err
	}
	base.AssertErr(func() error {
		if len(x.ExtractedFiles) > 0 {
			return nil
		}
		return fmt.Errorf("%v: did not extract any files", x.Source)
	})

	for _, f := range x.ExtractedFiles {
		if err := bc.OutputFile(f); err != nil {
			return err
		}
	}

	// avoid re-extracting after each rebuild
	bc.Timestamp(utils.UFS.MTime(x.Source))
	return nil
}
func (x *CompressedUnarchiver) Serialize(ar base.Archive) {
	ar.Serializable(&x.Source)
	ar.Serializable(&x.Destination)
	ar.Serializable(&x.AcceptList)
	ar.Serializable(&x.ExtractedFiles)
}
