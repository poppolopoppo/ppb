package io

import (
	"archive/tar"
	"fmt"
	"os"
	"time"

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

	if err := archiver.Walk(x.Source.String(), func(src archiver.File) error {
		defer src.Close()
		if src.IsDir() {
			return nil
		}

		// /!\ src.Name() returns the basename of the file, but we need an absolute path
		// instead we need to cast explicitly src.Header to a concrete type...
		var relativePath string
		var archiveMtime time.Time
		var archiveCrc32 base.Optional[uint32]
		switch header := src.Header.(type) {
		case *tar.Header:
			relativePath = header.Name
			archiveMtime = header.ModTime
		case zip.FileHeader:
			relativePath = header.Name
			archiveMtime = header.Modified
			archiveCrc32 = base.NewOption(header.CRC32)
		default:
			base.UnexpectedValuePanic(src.Header, header)
		}

		if destination, extract := exportFilter(relativePath); extract {
			base.LogDebug(LogCompressedArchive, "%v: extracting %q file", x.Source, relativePath)

			if !bc.GetBuildOptions().Force {
				if info, err := destination.Info(); err == nil {
					bSkipExtraction := false
					if info.ModTime().UTC() == archiveMtime.UTC() && info.Size() == src.Size() {
						if checkCrc32, err := archiveCrc32.Get(); err == nil {
							if localCrc32, err := utils.UFS.Crc32(destination); err == nil {
								bSkipExtraction = (localCrc32 == checkCrc32)
							}
						} else {
							bSkipExtraction = true
						}
					}
					if bSkipExtraction {
						base.LogVerbose(LogCompressedArchive, "skipping extraction of %q, since mod time and size perfectly match", destination)
						return nil
					}
				}
			}

			return utils.UFS.CreateFile(destination, func(dst *os.File) error {
				// replicate modification time stored in archive
				if err := base.SetMTime(dst, archiveMtime); err != nil {
					return err
				} else {
					// finally extract and copy file contents
					return base.CopyWithProgress(src.Name(), src.Size(), dst, src)
				}
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
	bc.Annotate(utils.AnnocateBuildTimestamp(utils.UFS.MTime(x.Source)))
	return nil
}
func (x *CompressedUnarchiver) Serialize(ar base.Archive) {
	ar.Serializable(&x.Source)
	ar.Serializable(&x.Destination)
	ar.Serializable(&x.AcceptList)
	ar.Serializable(&x.ExtractedFiles)
}
