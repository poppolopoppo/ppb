package cmd

import (
	"github.com/poppolopoppo/ppb/internal/base"
)

func InitCmd() {
	base.RegisterSerializable(&BffBuilder{})
	base.RegisterSerializable(&SlnAdditionalOptions{})
	base.RegisterSerializable(&SlnSolution{})
	base.RegisterSerializable(&SlnSolutionConfig{})
	base.RegisterSerializable(&SlnSolutionDependencies{})
	base.RegisterSerializable(&SlnSolutionFolder{})
	base.RegisterSerializable(&SlnSolutionBuilder{})
	base.RegisterSerializable(&VcxAdditionalOptions{})
	base.RegisterSerializable(&VcxFileType{})
	base.RegisterSerializable(&VcxProject{})
	base.RegisterSerializable(&VcxProjectBuilder{})
	base.RegisterSerializable(&VcxProjectConfig{})
	base.RegisterSerializable(&VcxProjectImport{})
	base.RegisterSerializable(&VscodeBuilder{})
}
