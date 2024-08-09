package base

import "io"

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

func ArchiveFileRead(reader io.Reader, scope func(ar Archive), flags ...ArchiveFlag) (file ArchiveFile, err error) {
	return file, ArchiveBinaryRead(reader, func(ar Archive) {
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
	}, flags...)
}
func ArchiveFileWrite(writer io.Writer, scope func(ar Archive)) (err error) {
	return ArchiveBinaryWrite(writer, func(ar Archive) {
		file := NewArchiveFile()
		ar.Serializable(&file)
		if err := ar.Error(); err == nil {
			scope(NewArchiveGuard(ar))
		}
	})
}

/***************************************
 * CompressedArchiveFile
 ***************************************/

func CompressedArchiveFileRead(reader io.Reader, scope func(ar Archive), compression ...CompressionOptionFunc) (file ArchiveFile, err error) {
	return ArchiveFileRead(NewCompressedReader(reader, compression...), scope)
}
func CompressedArchiveFileWrite(writer io.Writer, scope func(ar Archive), compression ...CompressionOptionFunc) (err error) {
	return ArchiveFileWrite(NewCompressedWriter(writer, compression...), scope)
}
