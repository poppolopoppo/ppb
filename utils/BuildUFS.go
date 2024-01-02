package utils

import "github.com/poppolopoppo/ppb/internal/base"

/***************************************
 * UFS Bindings for Build Graph
 ***************************************/

func (x *Filename) Alias() BuildAlias {
	return BuildAlias(x.String())
}
func (x Filename) Digest() (BuildStamp, error) {
	x.Invalidate()
	if info, err := x.Info(); err == nil {
		return MakeTimedBuildFingerprint(info.ModTime(), &x), nil
	} else {
		return BuildStamp{}, err
	}
}
func (x Filename) Build(bc BuildContext) error {
	x.Invalidate()
	if info, err := x.Info(); err == nil {
		bc.Annotate(base.SizeInBytes(info.Size()).String())
		bc.Timestamp(info.ModTime())
		return nil
	} else {
		return err
	}
}

func (x *Directory) Alias() BuildAlias {
	return BuildAlias(x.String())
}
func (x Directory) Build(bc BuildContext) error {
	x.Invalidate()
	if info, err := x.Info(); err == nil {
		bc.Timestamp(GetModificationTime(info))
		return nil
	} else {
		return err
	}
}

func BuildFile(source Filename, staticDeps ...BuildAlias) BuildFactoryTyped[*Filename] {
	return MakeBuildFactory(func(bi BuildInitializer) (Filename, error) {
		return SafeNormalize(source), bi.DependsOn(staticDeps...)
	})
}
func BuildDirectory(source Directory, staticDeps ...BuildAlias) BuildFactoryTyped[*Directory] {
	return MakeBuildFactory(func(bi BuildInitializer) (Directory, error) {
		base.Assert(func() bool { return source.Normalize().Equals(source) })
		return SafeNormalize(source), bi.DependsOn(staticDeps...)
	})
}
