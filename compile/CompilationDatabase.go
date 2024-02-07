package compile

import (
	"io"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * Compilation Database
 ***************************************/

// https://clang.llvm.org/docs/JSONCompilationDatabase.html
type CompileCommand struct {
	Directory utils.Directory `json:"directory"`
	File      utils.Filename  `json:"file"`
	Output    utils.Filename  `json:"output"`
	Arguments base.StringSet  `json:"arguments"`
}

type CompilationDatabase []CompileCommand

func (x *CompilationDatabase) Append(cmd CompileCommand) {
	base.Assert(func() bool {
		for _, it := range *x {
			if it.File == cmd.File {
				base.LogError(LogCompile, "input file already present in compiledb: %v\n\told output: %v\n\tnew output: %v", cmd.File, it.Output, cmd.Output)
				return false
			}
		}
		return true
	})
	base.LogTrace(LogCompile, "append %v to compiledb (%v)", cmd.File, cmd.Output)
	*x = append(*x, cmd)
}

/***************************************
 * Compilation Database Builder
 ***************************************/

func BuildCompilationDatabase(ea EnvironmentAlias) utils.BuildFactoryTyped[*CompilationDatabaseBuilder] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (CompilationDatabaseBuilder, error) {
		outputDir := utils.UFS.Intermediate.Folder(ea.PlatformName).Folder(ea.ConfigName)
		return CompilationDatabaseBuilder{
			EnvironmentAlias: ea,
			OutputFile:       outputDir.AbsoluteFile("compile_commands.json"),
		}, internal_io.CreateDirectory(bi, outputDir)
	})
}

type CompilationDatabaseBuilder struct {
	EnvironmentAlias
	OutputFile utils.Filename
}

func (x *CompilationDatabaseBuilder) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("CompileDb", x.EnvironmentAlias.PlatformName, x.EnvironmentAlias.ConfigName)
}
func (x *CompilationDatabaseBuilder) Build(bc utils.BuildContext) error {
	base.LogVerbose(utils.LogCommand, "generate compilation database for %v in %q...", x.EnvironmentAlias, x.OutputFile)

	moduleAliases, err := NeedAllModuleAliases(bc)
	if err != nil {
		return err
	}

	base.LogTrace(LogCompile, "retrieved %q modules", moduleAliases)

	// need to depends from the compiler
	if _, err := GetCompileEnvironment(x.EnvironmentAlias).Need(bc); err != nil {
		return nil
	}

	expandedActions := make(action.ActionSet, 0, len(moduleAliases))

	err = ForeachTargetActions(x.EnvironmentAlias, func(targetActions *TargetActions) error {
		if _, err := bc.NeedBuildable(targetActions); err != nil {
			return err
		}

		base.LogTrace(LogCompile, "retrieved target actions %q with %d payloads", targetActions.Alias(), targetActions.PresentPayloads.Len())

		actions, err := targetActions.GetOutputActions()
		if err != nil {
			return err
		}

		base.LogTrace(LogCompile, "retrieved %d output actions for target %q", len(actions), targetActions.Alias())

		if expandedActions, err = actions.ExpandDependencies(bc.BuildGraph()); err != nil {
			return err
		}

		return nil
	}, moduleAliases...)
	if err != nil {
		return err
	}

	var database CompilationDatabase

	for _, action := range expandedActions {
		rules := action.GetAction()

		inputFiles := rules.GetStaticInputFiles(bc.BuildGraph())
		if len(inputFiles) == 0 {
			continue // librarian or linker actions have dynamic inputs, but we are not interested in them here anyway
		}

		commandArgs := make([]string, len(rules.Arguments)+1)
		commandArgs[0] = rules.Executable.String()
		for j, arg := range rules.Arguments {
			commandArgs[j+1] = arg
		}

		actionCmd := CompileCommand{
			Directory: rules.WorkingDir,
			File:      inputFiles[0],
			Output:    rules.OutputFiles[0],
			Arguments: commandArgs,
		}

		database.Append(actionCmd)

		for _, input := range inputFiles {
			if unityFile, err := FindUnityFile(input); err == nil {
				base.LogVerbose(LogCompile, "expand unity file %q action inputs for compilation database", unityFile.Alias())

				for _, source := range unityFile.Inputs {
					if !unityFile.Excludeds.Contains(source) {
						database.Append(CompileCommand{
							Directory: actionCmd.Directory,
							File:      source,
							Output:    actionCmd.Output,
							Arguments: actionCmd.Arguments,
						})
					}
				}
			}
		}
	}

	err = utils.UFS.SafeCreate(x.OutputFile, func(w io.Writer) error {
		return base.JsonSerialize(database, w, base.OptionJsonPrettyPrint(true))
	})
	if err != nil {
		return err
	}

	return bc.OutputFile(x.OutputFile)
}
func (x *CompilationDatabaseBuilder) Serialize(ar base.Archive) {
	ar.Serializable(&x.EnvironmentAlias)
	ar.Serializable(&x.OutputFile)
}
