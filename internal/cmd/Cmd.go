package cmd

//lint:ignore ST1001 ignore dot imports warning
import (
	. "github.com/poppolopoppo/ppb/internal/base"
)

func InitCmd() {
	RegisterSerializable(&BffBuilder{})
	RegisterSerializable(&SlnAdditionalOptions{})
	RegisterSerializable(&SlnSolution{})
	RegisterSerializable(&SlnSolutionConfig{})
	RegisterSerializable(&SlnSolutionDependencies{})
	RegisterSerializable(&SlnSolutionFolder{})
	RegisterSerializable(&SlnSolutionBuilder{})
	RegisterSerializable(&VcxAdditionalOptions{})
	RegisterSerializable(&VcxFileType{})
	RegisterSerializable(&VcxProject{})
	RegisterSerializable(&VcxProjectBuilder{})
	RegisterSerializable(&VcxProjectConfig{})
	RegisterSerializable(&VcxProjectImport{})
	RegisterSerializable(&VscodeBuilder{})
}
