package base

import (
	"context"
	"io"
)

/***************************************
 * ArchiveFile
 ***************************************/

type ArchiveFile struct {
	Magic   FourCC
	Version FourCC
	Tags    []FourCC
}

var ArchiveFileMagic FourCC = MakeFourCC('A', 'R', 'B', 'F')
var ArchiveFileVersion FourCC = MakeFourCC('1', '0', '0', '1')

var ArchiveTags = []FourCC{}

func MakeArchiveTag(tag FourCC) FourCC {
	AssertNotIn(tag, ArchiveTags...)
	ArchiveTags = append(ArchiveTags, tag)
	return tag
}
func MakeArchiveTagIf(tag FourCC, enabled bool) (emptyTag FourCC) {
	if enabled {
		return MakeArchiveTag(tag)
	}
	return emptyTag
}

func NewArchiveFile() ArchiveFile {
	return ArchiveFile{
		Magic:   ArchiveFileMagic,
		Version: ArchiveFileVersion,
		Tags:    ArchiveTags,
	}
}
func (x *ArchiveFile) Serialize(ar Archive) {
	ar.Serializable(&x.Magic)
	ar.Serializable(&x.Version)
	SerializeSlice(ar, &x.Tags)

	// forward serialized tags to the archive
	ar.SetTags(x.Tags...)
}

func ArchiveFileRead(reader io.Reader, scope func(ar Archive), flags ArchiveFlags) (file ArchiveFile, err error) {
	err = WithArchiveBinaryReader(reader, func(ar Archive) (err error) {
		ar.Serializable(&file)
		if err := ar.Error(); err == nil {
			if file.Magic != ArchiveFileMagic {
				ar.OnErrorf("archive: invalid file magic (%q != %q)", file.Magic, ArchiveFileMagic)
			}
			if file.Version > ArchiveFileVersion {
				ar.OnErrorf("archive: newer file version (%q > %q)", file.Version, ArchiveFileVersion)
			}
			if file.Version < ArchiveFileVersion {
				ar.OnErrorf("archive: older file version (%q < %q)", file.Version, ArchiveFileVersion)
			}
			if err = ar.Error(); err == nil {
				scope(NewArchiveGuard(ar))
			}
		}
		return
	}, flags)
	return
}
func ArchiveFileWrite(writer io.Writer, scope func(ar Archive), flags ArchiveFlags) (err error) {
	return WithArchiveBinaryWriter(writer, func(ar Archive) (err error) {
		file := NewArchiveFile()
		ar.Serializable(&file)
		if err = ar.Error(); err == nil {
			scope(NewArchiveGuard(ar))
		}
		return
	}, flags)
}

/***************************************
 * CompressedArchiveFile
 ***************************************/

func CompressedArchiveFileRead(ctx context.Context, reader io.Reader, scope func(ar Archive), pageAlloc BytesRecycler, priority TaskPriority, flags ArchiveFlags, compression ...CompressionOptionFunc) (file ArchiveFile, err error) {
	err = WithCompressedReader(reader, func(cr io.Reader) error {
		return WithAsyncReader(ctx, cr, pageAlloc, priority, func(ar io.Reader) (err error) {
			file, err = ArchiveFileRead(ar, scope, flags)
			return err
		})
	}, compression...)
	return
}
func CompressedArchiveFileWrite(writer io.Writer, scope func(ar Archive), pageAlloc BytesRecycler, priority TaskPriority, flags ArchiveFlags, compression ...CompressionOptionFunc) error {
	return WithCompressedWriter(writer, func(cw io.Writer) error {
		return WithAsyncWriter(cw, pageAlloc, priority, func(aw io.Writer) error {
			return ArchiveFileWrite(aw, scope, flags)
		})
	}, compression...)
}
