package utils

import (
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
)

/***************************************
 * UFS Bindings for Build Graph
 ***************************************/

func (x *Directory) Alias() BuildAlias {
	return MakeBuildAlias("UFS", x.Path)
}

func (x *Filename) Alias() BuildAlias {
	return MakeBuildAlias("UFS", x.Dirname.Path, x.Basename)
}

/***************************************
 * Track file modification and source control state
 ***************************************/

type FileDependency struct {
	Filename
	Size          int64
	SourceControl SourceControlState
}

func (x *FileDependency) GetSourceFile() Filename {
	return x.Filename
}
func (x *FileDependency) Build(bc BuildContext) (err error) {
	x.Size = 0
	x.SourceControl = SOURCECONTROL_IGNORED

	var modTime time.Time
	*x, modTime, err = buildFileWithoutDeps(x.Filename)
	if err != nil {
		return err
	}

	bc.Annotate(
		AnnocateBuildCommentWith(base.SizeInBytes(x.Size)),
		AnnocateBuildTimestamp(modTime))
	if !x.SourceControl.Ignored() {
		bc.Annotate(AnnocateBuildCommentWith(x.SourceControl))
	}
	return nil
}
func (x *FileDependency) Serialize(ar Archive) {
	ar.Serializable(&x.Filename)
	ar.Int64(&x.Size)
}

func BuildFile(source Filename, staticDeps ...BuildAlias) BuildFactoryTyped[*FileDependency] {
	return MakeBuildFactory(func(bi BuildInitializer) (FileDependency, error) {
		return FileDependency{
			Filename: SafeNormalize(source),
		}, bi.DependsOn(staticDeps...)
	})
}
func PrepareOutputFile(bg BuildGraph, source Filename, staticDeps BuildAliases, options ...BuildOptionFunc) (*FileDependency, error) {
	return BuildFile(source, staticDeps...).Init(bg, options...)
}

func buildFileWithoutDeps(path Filename) (FileDependency, time.Time, error) {
	path.Invalidate() // if node is rebuilt, then file has been force created

	info, err := path.Info()
	if err != nil {
		return FileDependency{}, time.Time{}, err
	}

	file := FileDependency{
		Filename: path,
		Size:     info.Size(),
	}

	if scm := GetSourceControlProvider(); scm.IsInRepository(path) {
		fileStatus := SourceControlFileStatus{Path: path}
		if err = scm.GetFileStatus(&fileStatus); err == nil {
			file.SourceControl = fileStatus.State
		}
	}

	return file, info.ModTime(), err
}
func buildFileStampWithoutDeps(path Filename) (BuildStamp, error) {
	if file, modTime, err := buildFileWithoutDeps(path); err == nil {
		return MakeTimedBuildFingerprint(modTime, &file), nil
	} else {
		return BuildStamp{}, err
	}
}

/***************************************
 * Track file creation
 ***************************************/

type DirectoryDependency struct {
	Directory
}

func (x *DirectoryDependency) GetSourceDirectory() Directory {
	return x.Directory
}
func (x *DirectoryDependency) Build(bc BuildContext) error {
	x.Invalidate() // if node is rebuilt, then directory has been force created

	info, err := x.Info()
	if err != nil {
		return err
	}

	bc.Annotate(AnnocateBuildTimestamp(info.ModTime()))
	return nil
}

func BuildDirectory(source Directory, staticDeps ...BuildAlias) BuildFactoryTyped[*DirectoryDependency] {
	return MakeBuildFactory(func(bi BuildInitializer) (DirectoryDependency, error) {
		base.Assert(func() bool { return source.Normalize().Equals(source) })
		return DirectoryDependency{
			Directory: SafeNormalize(source),
		}, bi.DependsOn(staticDeps...)
	})
}
