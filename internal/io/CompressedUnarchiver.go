package io

import (
	"archive/tar"
	"fmt"
	"io"
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

type readerSeeker interface {
	io.Reader
	io.ReaderAt
	io.Seeker
}

type osFileWithProgress struct {
	fd *os.File
	pg base.ProgressScope
}

func (x osFileWithProgress) Read(p []byte) (n int, err error) {
	n, err = x.fd.Read(p)
	x.pg.Add(int64(n))
	return
}

func (x osFileWithProgress) Seek(offset int64, whence int) (n int64, err error) {
	n, err = x.fd.Seek(offset, whence)
	x.pg.Set(n)
	return
}

func (x osFileWithProgress) ReadAt(p []byte, off int64) (n int, err error) {
	n, err = x.fd.ReadAt(p, off)
	x.pg.Set(off + int64(n))
	return
}

func openFileWithProgressBar(src utils.Filename, read func(rd readerSeeker, size int64, pg base.ProgressScope) error) error {
	return utils.UFS.OpenFile(src, func(fd *os.File) error {
		st, err := fd.Stat()
		if err != nil {
			return err
		}
		size := st.Size()
		if base.EnableInteractiveShell() {
			pg := base.LogProgress(0, size, utils.MakeShortUserFriendlyPath(src).String())
			defer pg.Close()
			rd := osFileWithProgress{fd: fd, pg: pg}
			return read(&rd, size, pg)
		} else {
			pg := base.NewDummyLogProgress()
			return read(fd, size, pg)
		}
	})
}

type archiveHeader struct {
	relativePath string
	archiveMtime time.Time
	archiveCrc32 base.Optional[uint32]
}

func openArchive(src utils.Filename, read func(ar archiver.File, header archiveHeader) error) error {
	return openFileWithProgressBar(src, func(fd readerSeeker, size int64, pg base.ProgressScope) error {
		arch, err := archiver.ByExtension(src.Basename)
		if err != nil {
			return err
		}
		ar := arch.(archiver.Reader)

		if err := ar.Open(fd, size); err != nil {
			return err
		}
		defer ar.Close()

		yield := func(rd archiver.File) error {
			defer rd.Close()
			if rd.IsDir() {
				return nil
			}

			var header archiveHeader
			// /!\ rd.Name() returns the basename of the file, but we need an absolute path
			// instead we need to cast explicitly src.Header to a concrete type...
			switch h := rd.Header.(type) {
			case *tar.Header:
				header.relativePath = h.Name
				header.archiveMtime = h.ModTime
			case zip.FileHeader:
				header.relativePath = h.Name
				header.archiveMtime = h.Modified
				header.archiveCrc32 = base.NewOption(h.CRC32)
			default:
				base.UnexpectedValuePanic(rd.Header, h)
			}

			pg.Log("unarchive: %s (%v)", header.relativePath, base.SizeInBytes(rd.FileInfo.Size()))
			base.LogVeryVerbose(LogCompressedArchive, "%s: %v", src.Relative(utils.UFS.Root), header.relativePath)

			return read(rd, header)
		}

		for {
			rd, err := ar.Read()
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
			if err := yield(rd); err != nil {
				return err
			}
		}
		return nil
	})
}

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

	if err := openArchive(x.Source, func(src archiver.File, header archiveHeader) error {
		if destination, extract := exportFilter(header.relativePath); extract {
			base.LogVerbose(LogCompressedArchive, "%v: extracting %q file", x.Source, header.relativePath)

			if !bc.GetBuildOptions().Force {
				if info, err := destination.Info(); err == nil {
					bSkipExtraction := false
					if info.ModTime().UTC() == header.archiveMtime.UTC() && info.Size() == src.Size() {
						if checkCrc32, err := header.archiveCrc32.Get(); err == nil {
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
				// extract file contents
				if _, err := base.TransientIoCopy(dst, src, base.TransientPage1MiB, false); err != nil {
					return err
				}
				// replicate modification time stored in archive
				if err := base.SetMTime(dst, header.archiveMtime); err != nil {
					return err
				}
				return nil
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
