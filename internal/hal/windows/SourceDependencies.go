//go:build windows

package windows

import (
	"io"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * MsvcSourceDependencies
 ***************************************/

// https://learn.microsoft.com/en-us/cpp/build/reference/sourcedependencies?view=msvc-170

type MsvcSourceDependenciesImportModule struct {
	Name string
	BMI  utils.Filename
}
type MsvcSourceDependenciesImportHeaderUnit struct {
	Header utils.Filename
	BMI    utils.Filename
}
type MsvcSourceDependenciesData struct {
	Source              utils.Filename
	ProvidedModule      string
	PCH                 utils.Filename
	Includes            utils.FileSet
	ImportedModules     []MsvcSourceDependenciesImportModule
	ImportedHeaderUnits []MsvcSourceDependenciesImportHeaderUnit
}
type MsvcSourceDependencies struct {
	Version string
	Data    MsvcSourceDependenciesData
}

func (x *MsvcSourceDependencies) Load(r io.Reader) error {
	return base.JsonDeserialize(x, r)
}
func (x MsvcSourceDependencies) Files() (result []utils.Filename) {
	result = x.Data.Includes.Normalize()
	if x.Data.PCH.Valid() {
		result = append(result, x.Data.PCH.Normalize())
	}
	for _, module := range x.Data.ImportedModules {
		result = append(result, module.BMI.Normalize())
	}
	for _, header := range x.Data.ImportedHeaderUnits {
		result = append(result, header.Header.Normalize(), header.BMI.Normalize())
	}
	return
}

/***************************************
 * MsvcSourceDependenciesAction
 ***************************************/

type MsvcSourceDependenciesAction struct {
	SourceDependenciesFile utils.Filename
	action.ActionRules
}

func NewMsvcSourceDependenciesAction(model *action.ActionModel, output utils.Filename) *MsvcSourceDependenciesAction {
	result := &MsvcSourceDependenciesAction{
		ActionRules:            model.CreateActionRules(),
		SourceDependenciesFile: output,
	}

	allowRelativePath := result.Options.Has(action.OPT_ALLOW_RELATIVEPATH)

	result.Arguments.Append("/sourceDependencies", utils.MakeLocalFilenameIFP(output, allowRelativePath))
	result.OutputFiles.Append(output)
	return result
}

func (x *MsvcSourceDependenciesAction) Build(bc utils.BuildContext) error {
	// compile the action with /sourceDependencies
	return x.ActionRules.BuildWithSourceDependencies(bc, x)
}

func (x *MsvcSourceDependenciesAction) GetActionSourceDependencies(bc utils.BuildContext) (sourceFiles utils.FileSet, err error) {
	// track json file as an output dependency (check file exists)
	if err = bc.OutputFile(x.SourceDependenciesFile); err != nil {
		return
	}

	// parse source dependencies outputted by cl.exe
	var sourceDeps MsvcSourceDependencies
	if err = utils.UFS.OpenBuffered(x.SourceDependenciesFile, sourceDeps.Load); err != nil {
		return
	}

	// add all parsed filenames as dynamic dependencies: when a header is modified, this action will have to be rebuild
	dependentFiles := sourceDeps.Files()
	base.LogDebug(LogWindows, "sourceDependencies: parsed output in %q\n%v", x.SourceDependenciesFile, base.MakeStringer(func() string {
		return base.PrettyPrint(dependentFiles)
	}))

	return dependentFiles, nil
}

func (x *MsvcSourceDependenciesAction) Serialize(ar base.Archive) {
	ar.Serializable(&x.SourceDependenciesFile)
	ar.Serializable(&x.ActionRules)
}
