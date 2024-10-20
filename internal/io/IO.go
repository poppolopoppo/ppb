package io

import (
	"os"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

var LogIO = base.NewLogCategory("IO")

var IsOutputRedirectToPipe = isOutputRedirectToPipe()

func InitIO() {
	base.LogTrace(LogIO, "internal/io.Init()")

	// disable ansi colors when pipe output is detected
	if IsOutputRedirectToPipe {
		base.SetEnableAnsiColor(false)
		base.SetEnableInteractiveShell(false)
	}

	base.RegisterSerializable[CompressedUnarchiver]()
	base.RegisterSerializable[Downloader]()

	base.RegisterSerializable[DirectoryCreator]()
	base.RegisterSerializable[DirectoryGlob]()
	base.RegisterSerializable[DirectoryList]()
	base.RegisterSerializable[FileDigest]()
}

func isOutputRedirectToPipe() bool {
	o, _ := os.Stdout.Stat()
	return (o.Mode() & os.ModeCharDevice) != os.ModeCharDevice
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

func PrepareFileDigests(bg utils.BuildGraphWritePort, n int, filenames func(int) utils.Filename, options ...utils.BuildOptionFunc) []base.Future[*FileDigest] {
	results := make([]base.Future[*FileDigest], n)
	for i := range results {
		results[i] = BuildFileDigest(filenames(i)).Prepare(bg, options...)
	}
	return results
}

func (x *FileDigest) GetSourceFile() utils.Filename {
	return x.Source
}
func (x *FileDigest) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("UFS", "Digest", x.Source.Dirname.Path, x.Source.Basename)
}
func (x *FileDigest) Build(bc utils.BuildContext) (err error) {
	x.Digest, err = utils.UFS.Fingerprint(x.Source, base.Fingerprint{} /* no seed here */)
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

func (x *DirectoryCreator) GetSourceDirectory() utils.Directory {
	return x.Source
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

func (x *DirectoryList) GetSourceDirectory() utils.Directory {
	return x.Source
}
func (x *DirectoryList) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("UFS", "List", x.Source.Path)
}
func (x *DirectoryList) Build(bc utils.BuildContext) error {
	x.Results = utils.FileSet{}

	if info, err := x.Source.Info(); err == nil {
		bc.Annotate(utils.AnnocateBuildTimestamp(info.ModTime()))
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

	bc.Annotate(utils.AnnocateBuildCommentf("%d files", len(x.Results)))
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

	// make build alias determinist for DirectoryGlob
	includedGlobs.Sort()
	excludedGlobs.Sort()

	excludedFiles = excludedFiles.Normalize()
	excludedFiles.Sort()

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

func (x *DirectoryGlob) GetSourceDirectory() utils.Directory {
	return x.Source
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

	if info, err := x.Source.Info(); err == nil {
		bc.Annotate(utils.AnnocateBuildTimestamp(info.ModTime()))
	} else {
		return err
	}

	includeRE := utils.MakeGlobRegexp(x.IncludedGlobs...)
	excludeRE := utils.MakeGlobRegexp(x.ExcludedGlobs...)
	if !includeRE.Valid() {
		includeRE = utils.MakeGlobRegexp("*")
	}

	err := x.Source.MatchFilesRec(func(f utils.Filename) error {
		f = utils.SafeNormalize(f)
		if !x.ExcludedFiles.Contains(f) {
			if !excludeRE.Valid() || !excludeRE.MatchString(f.String()) {
				x.Results.Append(f)
			}
		}
		return nil
	}, includeRE)

	if err == nil {
		bc.Annotate(utils.AnnocateBuildCommentf("%d files", len(x.Results)))
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
