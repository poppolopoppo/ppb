package windows

import (
	"io"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"
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
	action.ActionRules
	SourceDependenciesFile utils.Filename
}

func NewMsvcSourceDependenciesAction(rules *action.ActionRules, output utils.Filename) *MsvcSourceDependenciesAction {
	result := &MsvcSourceDependenciesAction{
		ActionRules:            *rules,
		SourceDependenciesFile: output,
	}

	result.Arguments.Append("/sourceDependencies", utils.MakeLocalFilename(output))
	result.Extras.Append(output)
	return result
}

func (x *MsvcSourceDependenciesAction) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("Action", "Msvc", x.Outputs.Join(";"))
}
func (x *MsvcSourceDependenciesAction) Build(bc utils.BuildContext) error {
	// compile the action with /sourceDependencies
	if err := x.ActionRules.Build(bc); err != nil {
		return err
	}

	// track json file as an output dependency (check file exists)
	if err := bc.OutputFile(x.SourceDependenciesFile); err != nil {
		return err
	}

	// parse source dependencies outputted by cl.exe
	var sourceDeps MsvcSourceDependencies
	if err := utils.UFS.OpenBuffered(x.SourceDependenciesFile, sourceDeps.Load); err != nil {
		return err
	}

	// add all parsed filenames as dynamic dependencies: when a header is modified, this action will have to be rebuild
	dependentFiles := sourceDeps.Files()
	base.LogDebug(LogWindows, "sourceDependencies: parsed output in %q\n%v", x.SourceDependenciesFile, base.MakeStringer(func() string {
		return base.PrettyPrint(dependentFiles)
	}))

	if flags := action.GetActionFlags(); flags.ShowFiles.Get() {
		for _, file := range dependentFiles {
			base.LogForwardf("%v: [%s]  %s", base.MakeStringer(func() string {
				return x.Alias().String()
			}), internal_io.FILEACCESS_READ, file)
		}
	}

	return bc.NeedFiles(dependentFiles...)
}
func (x *MsvcSourceDependenciesAction) Serialize(ar base.Archive) {
	ar.Serializable(&x.ActionRules)
	ar.Serializable(&x.SourceDependenciesFile)
}
