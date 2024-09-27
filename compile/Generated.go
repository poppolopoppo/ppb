package compile

import (
	"io"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"

	internal_io "github.com/poppolopoppo/ppb/internal/io"
)

/***************************************
 * Generated
 ***************************************/

func MakeGeneratedAlias(output utils.Filename) utils.BuildAlias {
	return utils.MakeBuildAlias("Generated", output.Dirname.Path, output.Basename)
}

type Generated interface {
	Generate(utils.BuildContext, *BuildGenerated, io.Writer) error
	base.Serializable
}

type BuildGenerated struct {
	OutputFile utils.Filename
	Generated
}

func (x *BuildGenerated) Alias() utils.BuildAlias {
	return MakeGeneratedAlias(x.OutputFile)
}
func (x *BuildGenerated) Build(bc utils.BuildContext) error {
	err := utils.UFS.CreateBuffered(x.OutputFile, func(w io.Writer) error {
		return x.Generate(bc, x, w)
	}, base.TransientPage4KiB)
	if err == nil {
		err = bc.OutputFile(x.OutputFile)
	}
	return err
}
func (x *BuildGenerated) Serialize(ar base.Archive) {
	ar.Serializable(&x.OutputFile)
	base.SerializeExternal(ar, &x.Generated)
}

/***************************************
 * Generator
 ***************************************/

type Generator interface {
	CreateGenerated(unit *Unit, output utils.Filename) Generated
	base.Serializable
}

type GeneratorList []GeneratorRules

func (list *GeneratorList) Append(it ...GeneratorRules) {
	*list = append(*list, it...)
}
func (list *GeneratorList) Prepend(it ...GeneratorRules) {
	*list = append(it, *list...)
}
func (list *GeneratorList) Serialize(ar base.Archive) {
	base.SerializeSlice(ar, (*[]GeneratorRules)(list))
}

type GeneratorRules struct {
	GeneratedName string
	Visibility    VisibilityType
	Generator
}

func (rules *GeneratorRules) GetGenerator() *GeneratorRules {
	return rules
}
func (rules *GeneratorRules) Serialize(ar base.Archive) {
	ar.String(&rules.GeneratedName)
	ar.Serializable(&rules.Visibility)
	base.SerializeExternal(ar, &rules.Generator)
}
func (rules *GeneratorRules) GetGenerateDir(unit *Unit) utils.Directory {
	result := unit.GeneratedDir
	switch rules.Visibility {
	case PRIVATE:
		result = result.Folder("Private")
	case PUBLIC, RUNTIME:
		result = result.Folder("Public")
	default:
		base.UnexpectedValue(rules.Visibility)
	}
	return result
}
func (rules *GeneratorRules) GetGenerateFile(unit *Unit) utils.Filename {
	return rules.GetGenerateDir(unit).AbsoluteFile(rules.GeneratedName)
}

func (rules *GeneratorRules) CreateGenerated(bc utils.BuildContext, module Module, unit *Unit) (*BuildGenerated, error) {
	outputFile := rules.GetGenerateFile(unit)
	generated := &BuildGenerated{
		OutputFile: outputFile,
		Generated:  rules.Generator.CreateGenerated(unit, outputFile),
	}

	err := bc.OutputNode(utils.WrapBuildFactory(func(bi utils.BuildInitializer) (*BuildGenerated, error) {
		return generated, bi.NeedFactories(internal_io.BuildDirectoryCreator(outputFile.Dirname))
	}))
	return generated, err
}
