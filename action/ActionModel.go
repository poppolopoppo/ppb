package action

import (
	"fmt"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
)

type CommandRules struct {
	Arguments   base.StringSet
	Environment internal_io.ProcessEnvironment
	Executable  utils.Filename
	WorkingDir  utils.Directory
}

func (x *CommandRules) Serialize(ar base.Archive) {
	ar.Serializable(&x.Arguments)
	ar.Serializable(&x.Environment)
	ar.Serializable(&x.Executable)
	ar.Serializable(&x.WorkingDir)
}
func (x *CommandRules) String() string {
	oss := strings.Builder{}
	fmt.Fprintf(&oss, "%q", x.Executable)
	for _, arg := range x.Arguments {
		fmt.Fprintf(&oss, " %q", arg)
	}
	return oss.String()
}

type ArtifactRules struct {
	InputFiles      utils.FileSet
	DependencyFiles utils.FileSet
	OutputFiles     utils.FileSet
}

func (x *ArtifactRules) Serialize(ar base.Archive) {
	ar.Serializable(&x.InputFiles)
	ar.Serializable(&x.DependencyFiles)
	ar.Serializable(&x.OutputFiles)
}

type ActionModel struct {
	Command CommandRules

	DynamicInputs     ActionSet
	DynamicInputFiles utils.FileSet
	StaticInputFiles  utils.FileSet

	ExportFile utils.Filename
	OutputFile utils.Filename
	ExtraFiles utils.FileSet

	Options       OptionFlags
	Prerequisites ActionSet
	StaticDeps    utils.BuildAliases
}

func (x *ActionModel) GetCommandInputFiles() (results utils.FileSet) {
	results = x.StaticInputFiles.Concat(x.DynamicInputFiles...)
	if len(x.DynamicInputs) > 0 {
		results.Append(x.DynamicInputs.GetExportFiles()...)
	}
	return
}

func (x *ActionModel) CreateActionRules() ActionRules {
	base.Assert(x.ExportFile.Valid)

	rules := ActionRules{
		CommandRules:  x.Command,
		OutputFiles:   utils.FileSet{x.OutputFile}.Concat(x.ExtraFiles...),
		Prerequisites: x.Prerequisites.Aliases(),
		Options:       x.Options,
	}
	rules.OutputFiles.Sort()

	if index, ok := base.IndexOf(x.ExportFile, rules.OutputFiles...); ok {
		rules.ExportIndex = int32(index)
	} else {
		rules.ExportIndex = int32(len(rules.OutputFiles))
		rules.OutputFiles.Append(x.ExportFile)
		rules.OutputFiles.Sort()
	}

	return rules
}

func BuildAction(model *ActionModel, factory func(*ActionModel) (Action, error)) utils.BuildFactoryTyped[Action] {
	base.Assert(model.ExportFile.Valid)
	base.Assert(model.OutputFile.Valid)

	return utils.WrapBuildFactory(func(bi utils.BuildInitializer) (Action, error) {
		// track static input files
		if err := bi.NeedFiles(model.StaticInputFiles...); err != nil {
			return nil, err
		}

		// track dynamic inputs
		if err := bi.DependsOn(model.DynamicInputs.Aliases()...); err != nil {
			return nil, err
		}

		// track executable file
		if err := bi.NeedFiles(model.Command.Executable); err != nil {
			return nil, err
		}

		// track static dependencies
		if err := bi.DependsOn(model.StaticDeps...); err != nil {
			return nil, err
		}

		// create output directories
		outputDirs := utils.DirSet{model.OutputFile.Dirname}
		outputDirs.AppendUniq(model.ExportFile.Dirname)
		for _, filename := range model.ExtraFiles {
			if dir := filename.Dirname; !outputDirs.Contains(dir) {
				outputDirs.Append(dir)
			}
		}
		for _, dir := range outputDirs {
			if _, err := internal_io.BuildDirectoryCreator(dir).Need(bi); err != nil {
				return nil, err
			}
		}

		// finally, converts model to action (losing some infos which will be restructed from dependencies)
		return factory(model)
	})
}
