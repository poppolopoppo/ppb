package compile

import (
	"io"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/internal/io"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * Compilation Database
 ***************************************/

// https://clang.llvm.org/docs/JSONCompilationDatabase.html
type CompileCommand struct {
	Directory Directory `json:"directory"`
	File      Filename  `json:"file"`
	Output    Filename  `json:"output"`
	Arguments StringSet `json:"arguments"`
}

type CompilationDatabase []CompileCommand

func (x *CompilationDatabase) Append(cmd CompileCommand) {
	Assert(func() bool {
		for _, it := range *x {
			if it.File == cmd.File {
				LogError(LogCompile, "input file already present in compiledb: %v\n\told output: %v\n\tnew output: %v", cmd.File, it.Output, cmd.Output)
				return false
			}
		}
		return true
	})
	LogTrace(LogCompile, "append %v to compiledb (%v)", cmd.File, cmd.Output)
	*x = append(*x, cmd)
}

/***************************************
 * Compilation Database Builder
 ***************************************/

func BuildCompilationDatabase(ea EnvironmentAlias) BuildFactoryTyped[*CompilationDatabaseBuilder] {
	return MakeBuildFactory(func(bi BuildInitializer) (CompilationDatabaseBuilder, error) {
		outputDir := UFS.Intermediate.Folder(ea.PlatformName).Folder(ea.ConfigName)
		return CompilationDatabaseBuilder{
			EnvironmentAlias: ea,
			OutputFile:       outputDir.AbsoluteFile("compile_commands.json"),
		}, CreateDirectory(bi, outputDir)
	})
}

type CompilationDatabaseBuilder struct {
	EnvironmentAlias
	OutputFile Filename
}

func (x *CompilationDatabaseBuilder) Alias() BuildAlias {
	return MakeBuildAlias("CompileDb", x.EnvironmentAlias.String())
}
func (x *CompilationDatabaseBuilder) Build(bc BuildContext) error {
	LogVerbose(LogCommand, "generate compilation database for %v in %q...", x.EnvironmentAlias, x.OutputFile)

	moduleAliases, err := NeedAllModuleAliases(bc)
	if err != nil {
		return err
	}

	LogTrace(LogCompile, "retrieved %q modules", moduleAliases)

	// need to depends from the compiler
	if _, err := GetCompileEnvironment(x.EnvironmentAlias).Need(bc); err != nil {
		return nil
	}

	expandedActions := make(ActionSet, 0, len(moduleAliases))

	err = ForeachTargetActions(x.EnvironmentAlias, func(targetActions *TargetActions) error {
		if err := bc.NeedBuildable(targetActions); err != nil {
			return err
		}

		LogTrace(LogCompile, "retrieved target actions %q with %d payloads", targetActions.Alias(), targetActions.PresentPayloads.Len())

		actions, err := targetActions.GetOutputActions()
		if err != nil {
			return err
		}

		LogTrace(LogCompile, "retrieved %d output actions for target %q", len(actions), targetActions.Alias())

		if err = actions.ExpandDependencies(&expandedActions); err != nil {
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

		commandArgs := make([]string, len(rules.Arguments)+1)
		commandArgs[0] = rules.Executable.String()
		for j, arg := range rules.Arguments {
			commandArgs[j+1] = arg
		}

		actionCmd := CompileCommand{
			Directory: rules.WorkingDir,
			File:      rules.Inputs[0],
			Output:    rules.Outputs[0],
			Arguments: commandArgs,
		}

		database.Append(actionCmd)

		for _, input := range action.GetAction().Inputs {
			if unityFile, err := FindUnityFile(input); err == nil {
				LogVerbose(LogCompile, "expand unity file %q action inputs for compilation database", unityFile.Alias())

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

	err = UFS.SafeCreate(x.OutputFile, func(w io.Writer) error {
		return JsonSerialize(database, w, OptionJsonPrettyPrint(true))
	})
	if err != nil {
		return err
	}

	return bc.OutputFile(x.OutputFile)
}
func (x *CompilationDatabaseBuilder) Serialize(ar Archive) {
	ar.Serializable(&x.EnvironmentAlias)
	ar.Serializable(&x.OutputFile)
}
