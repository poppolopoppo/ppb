package io

import (
	"context"
	"fmt"
	"os"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"

	"github.com/mholt/archives"
)

var LogCompressedArchive = base.NewLogCategory("CompressedArchive")

type CompressedArchiveFromDownload struct {
	Download   *Downloader
	ExtractDir utils.Directory
}

func (x CompressedArchiveFromDownload) Equals(other CompressedArchiveFromDownload) bool {
	return (x.Download == other.Download && x.ExtractDir.Equals(other.ExtractDir))
}

func BuildCompressedUnarchiver(src utils.Filename, dst utils.Directory, acceptList base.Regexp, staticDeps ...utils.BuildAlias) utils.BuildFactoryTyped[*CompressedUnarchiver] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (CompressedUnarchiver, error) {
		err := CreateDirectory(bi, dst)
		return CompressedUnarchiver{
				Source:      src,
				Destination: dst,
				AcceptList:  acceptList,
			}, base.AnyError(
				err,
				bi.NeedFiles(src),
				bi.DependsOn(staticDeps...))
	})
}
func BuildCompressedArchiveExtractorFromDownload(prms CompressedArchiveFromDownload, acceptList base.Regexp) utils.BuildFactoryTyped[*CompressedUnarchiver] {
	return BuildCompressedUnarchiver(
		prms.Download.Destination,
		prms.ExtractDir,
		acceptList,
		prms.Download.Alias())
}

/***************************************
 * Compressed Archive Extractor
 ***************************************/

type CompressedUnarchiver struct {
	Source         utils.Filename
	Destination    utils.Directory
	AcceptList     base.Regexp
	ExtractedFiles utils.FileSet
}

func (x *CompressedUnarchiver) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("Unarchiver", x.Destination.String())
}
func (x *CompressedUnarchiver) Build(bc utils.BuildContext) error {
	x.ExtractedFiles.Clear()

	exportFilter := func(s string) (utils.Filename, bool) {
		dst := x.Destination.AbsoluteFile(s)
		match := true
		if x.AcceptList.Valid() {
			match = x.AcceptList.MatchString(s)
		}
		if match {
			x.ExtractedFiles.Append(dst)
			return dst, true
		}
		return dst, false
	}

	if err := utils.UFS.OpenLogProgress(x.Source, func(fd utils.FileProgress, wholeSize int64, pg base.ProgressScope) error {
		format, stream, err := archives.Identify(bc, x.Source.String(), fd)
		if err != nil {
			return err
		}

		if ex, ok := format.(archives.Extractor); ok {
			err = ex.Extract(bc, stream, func(ctx context.Context, srcInfo archives.FileInfo) error {
				if srcInfo.IsDir() {
					return nil
				}

				pg.Log("unarchive: %s (%v)", srcInfo.NameInArchive, base.SizeInBytes(srcInfo.Size()))

				if destination, extract := exportFilter(srcInfo.NameInArchive); extract {
					base.LogVerbose(LogCompressedArchive, "%v: extracting %q file", x.Source.Relative(utils.UFS.Root), srcInfo.NameInArchive)

					src, err := srcInfo.Open()
					if err != nil {
						return err
					}
					defer src.Close()

					return utils.UFS.CreateFile(destination, func(dst *os.File) error {
						// extract file contents
						if _, err := base.TransientIoCopy(bc, dst, src, base.GetBytesRecyclerBySize(wholeSize), false); err != nil {
							return err
						}
						// replicate modification time stored in archive
						if err := base.SetMTime(dst, srcInfo.ModTime()); err != nil {
							return err
						}
						return nil
					})
				}
				return nil
			})
		} else {
			err = archives.NoMatch
		}
		return err
	}); err != nil {
		return err
	}
	base.AssertErr(func() error {
		if len(x.ExtractedFiles) > 0 {
			return nil
		}
		return fmt.Errorf("%v: did not extract any files", x.Source)
	})

	if err := bc.OutputFile(x.ExtractedFiles...); err != nil {
		return err
	}

	// avoid re-extracting after each rebuild
	bc.Annotate(
		utils.AnnocateBuildCommentf("%d files", len(x.ExtractedFiles)),
		utils.AnnocateBuildTimestamp(utils.UFS.MTime(x.Source)))
	return nil
}
func (x *CompressedUnarchiver) Serialize(ar base.Archive) {
	ar.Serializable(&x.Source)
	ar.Serializable(&x.Destination)
	ar.Serializable(&x.AcceptList)
	ar.Serializable(&x.ExtractedFiles)
}
