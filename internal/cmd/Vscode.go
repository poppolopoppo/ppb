package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/compile"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/internal/io"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var Vscode = NewCommand(
	"Configure",
	"vscode",
	"generate workspace for Visual Studio Code",
	OptionCommandParsableAccessor("solution_flags", "solution generation option", GetSolutionFlags),
	OptionCommandRun(func(cc CommandContext) error {
		outputDir := UFS.Root.Folder(".vscode")
		LogClaim(LogCommand, "generating VSCode workspace in '%v'", outputDir)

		result := BuildVscode(outputDir).Build(CommandEnv.BuildGraph(), OptionBuildForce)
		return result.Failure()
	}))

/***************************************
 * Visual Studio Code workspace generation
 ***************************************/

func BuildVscode(outputDir Directory) BuildFactoryTyped[*VscodeBuilder] {
	return MakeBuildFactory(func(bi BuildInitializer) (VscodeBuilder, error) {
		return VscodeBuilder{
			Version:   VscodeBuilderVersion,
			OutputDir: outputDir,
		}, CreateDirectory(bi, outputDir)
	})
}

var VscodeBuilderVersion = "VscodeBuilder-1-0-0"

type VscodeBuilder struct {
	Version   string
	OutputDir Directory
}

func (vsc *VscodeBuilder) Alias() BuildAlias {
	return MakeBuildAlias("Vscode", vsc.OutputDir.String())
}
func (vsc *VscodeBuilder) Serialize(ar Archive) {
	ar.String(&vsc.Version)
	ar.Serializable(&vsc.OutputDir)
}
func (vsc *VscodeBuilder) Build(bc BuildContext) error {
	LogVerbose(LogCommand, "build vscode configuration in '%v'...", vsc.OutputDir)

	modules, err := NeedAllBuildModules(bc)
	if err != nil {
		return err
	}

	platform, err := GeLocalHostBuildPlatform().Need(bc)
	if err != nil {
		return err
	}
	compiler, err := platform.GetCompiler().Need(bc)
	if err != nil {
		return err
	}

	c_cpp_properties_json := vsc.OutputDir.File("c_cpp_properties.json")
	LogTrace(LogCommand, "generating vscode c/c++ properties in '%v'", c_cpp_properties_json)
	if err := vsc.c_cpp_properties(bc, c_cpp_properties_json); err != nil {
		return err
	}

	moduleAliases := Map(func(m Module) ModuleAlias { return m.GetModule().ModuleAlias }, modules...)

	tasks_json := vsc.OutputDir.File("tasks.json")
	LogTrace(LogCommand, "generating vscode build tasks in '%v'", tasks_json)
	if err := vsc.tasks(moduleAliases, tasks_json); err != nil {
		return err
	}

	progamAliases := Map(func(m Module) ModuleAlias { return m.GetModule().ModuleAlias },
		RemoveUnless(func(m Module) bool { return m.GetModule().ModuleType == MODULE_PROGRAM }, modules...)...)

	launch_json := vsc.OutputDir.File("launch.json")
	LogTrace(LogCommand, "generating vscode launch configuratiosn in '%v'", launch_json)
	if err := vsc.launch_configs(progamAliases, compiler, launch_json); err != nil {
		return err
	}

	return bc.OutputFile(c_cpp_properties_json, tasks_json, launch_json)
}

func sanitizeEnvironmentDefines(defines StringSet) (StringSet, error) {
	ignoreds := make(map[string]string, len(defines))
	keys := make(map[string]string, len(defines))
	for _, it := range defines {
		args := strings.Split(it, "=")
		if len(args) > 2 {
			return StringSet{}, fmt.Errorf("invalid define '%s'", it)
		}

		if len(args) == 2 {
			if _, ok := ignoreds[args[0]]; ok {
				// already ignored divergent define
			} else if _, ok := keys[args[0]]; ok {
				ignoreds[args[0]] = args[1]
				delete(keys, args[0])
			} else {
				keys[args[0]] = args[1]
			}
		} else {
			keys[args[0]] = ""
		}
	}

	result := make(StringSet, 0, len(keys))
	for key, value := range keys {
		if len(value) == 0 {
			result.Append(key)
		} else {
			result.Append(fmt.Sprint(key, "=", value))
		}
	}

	return result, nil
}

func (vsc *VscodeBuilder) c_cpp_properties(bc BuildContext, outputFile Filename) error {
	configurations := []JsonMap{}

	err := ForeachCompileEnvironment(func(u BuildFactoryTyped[*CompileEnv]) error {
		env, err := u.Need(bc)
		if err != nil {
			return err
		}

		var intelliSenseMode string
		switch env.GetPlatform().Os {
		case "Linux":
			intelliSenseMode = fmt.Sprintf("linux-%s-x64", env.CompilerAlias.CompilerFamily)
		case "Windows":
			intelliSenseMode = fmt.Sprintf("windows-%s-x64", env.CompilerAlias.CompilerFamily)
		default:
			UnexpectedValue(env.GetPlatform().Os)
		}

		compileDb, err := BuildCompilationDatabase(env.EnvironmentAlias).Need(bc)
		if err != nil {
			return err
		}

		units, err := NeedAllBuildUnits(bc)
		if err != nil {
			return err
		}

		defines := StringSet{}
		includePaths := DirSet{}
		for _, u := range units {
			defines.AppendUniq(u.Defines...)
			includePaths.AppendUniq(u.IncludePaths...)
		}

		defines, err = sanitizeEnvironmentDefines(defines)
		if err != nil {
			return nil
		}

		configurations = append(configurations, JsonMap{
			"name":             env.EnvironmentAlias.String(),
			"compilerPath":     env.GetCompiler().Executable.String(),
			"compileCommands":  compileDb.OutputFile,
			"cStandard":        "c17",
			"cppStandard":      strings.ToLower(env.GetCpp(nil).CppStd.String()),
			"defines":          defines,
			"includePath":      includePaths,
			"intelliSenseMode": intelliSenseMode,
			"browse": JsonMap{
				"path":                          includePaths,
				"limitSymbolsToIncludedHeaders": true,
				"databaseFilename":              env.IntermediateDir().File("vscode-vc.db"),
			},
		})

		return nil
	})

	if err != nil {
		return err
	}

	return UFS.SafeCreate(outputFile, func(w io.Writer) error {
		return JsonSerialize(JsonMap{
			"version":        4,
			"configurations": configurations,
		}, w)
	})
}
func (vsc *VscodeBuilder) tasks(moduleAliases ModuleAliases, outputFile Filename) error {
	var problemMatcher string
	switch CurrentHost().Id {
	case HOST_LINUX, HOST_DARWIN:
		problemMatcher = "$gcc"
	case HOST_WINDOWS:
		problemMatcher = "$msCompile"
	default:
		return MakeUnexpectedValueError(problemMatcher, CurrentHost().Id)
	}

	buildCommand := "build"
	if solutionFlags := GetSolutionFlags(); solutionFlags.FASTBuild.Get() {
		buildCommand = "fbuild"
	}

	tasks := Map(func(moduleAliases ModuleAlias) JsonMap {
		label := moduleAliases.String()
		return JsonMap{
			"label":   label,
			"command": UFS.Executable.String(),
			"args":    []string{buildCommand, "-Ide", label + "-${command:cpptools.activeConfigName}"},
			"options": JsonMap{
				"cwd": UFS.Root,
			},
			"group": JsonMap{
				"kind":      "build",
				"isDefault": true,
			},
			"presentation": JsonMap{
				"clear":  true,
				"echo":   true,
				"reveal": "always",
				"focus":  false,
				"panel":  "dedicated",
			},
			"problemMatcher": problemMatcher,
		}
	}, moduleAliases...)

	return UFS.SafeCreate(outputFile, func(w io.Writer) error {
		return JsonSerialize(JsonMap{
			"version": "2.0.0",
			"tasks":   tasks,
		}, w)
	})
}
func (vsc *VscodeBuilder) launch_configs(programAliases ModuleAliases, compiler Compiler, outputFile Filename) error {
	var debuggerType string
	switch CurrentHost().Id {
	case HOST_LINUX, HOST_DARWIN:
		debuggerType = "cppdbg"
	case HOST_WINDOWS:
		debuggerType = "cppvsdbg"
	default:
		UnexpectedValue(CurrentHost().Id)
	}

	// create a launch single launch configuration per executable
	// configuration is deduced from selection in vscode
	configurations := Map(func(programAlias ModuleAlias) JsonMap {
		alias := programAlias.String()
		executable := SanitizePath(alias, '-')

		environment := []JsonMap{}
		for _, it := range compiler.GetCompiler().Environment {
			environment = append(environment, JsonMap{
				"name":  it.Name.String(),
				"value": strings.Join(it.Values, ";"),
			})
		}

		cfg := JsonMap{
			"name":        alias,
			"type":        debuggerType,
			"request":     "launch",
			"program":     UFS.Binaries.File(executable + "-${command:cpptools.activeConfigName}" + compiler.Extname(PAYLOAD_EXECUTABLE)),
			"args":        []string{},
			"stopAtEntry": false,
			"cwd":         UFS.Binaries,
			"environment": environment,
		}

		return cfg
	}, programAliases...)

	// append configurations for debugging the build system
	configurations = append(configurations,
		JsonMap{
			"name":    fmt.Sprint("Build ", CommandEnv.Prefix()),
			"type":    "go",
			"cwd":     UFS.Root,
			"request": "launch",
			"mode":    "auto",
			"program": UFS.Caller,
			"args":    "${input:buildPromptCommand} ${input:buildPromptArgument} -Color",
		},
		JsonMap{
			"name":       fmt.Sprint("Build ", CommandEnv.Prefix(), " (Debug)"),
			"type":       "go",
			"cwd":        UFS.Root,
			"request":    "launch",
			"mode":       "auto",
			"program":    UFS.Caller,
			"buildFlags": "-tags=ppb_debug",
			"args":       "${input:buildPromptCommand} ${input:buildPromptArgument} -Color",
		},
		JsonMap{
			"name":       fmt.Sprint("Build ", CommandEnv.Prefix(), " (Profiling)"),
			"type":       "go",
			"cwd":        UFS.Root,
			"request":    "launch",
			"mode":       "auto",
			"program":    UFS.Caller,
			"buildFlags": "-tags=ppb_profiling",
			"args":       "${input:buildPromptCommand} ${input:buildPromptArgument} -Color -q",
		},
		JsonMap{
			"name":       fmt.Sprint("Build ", CommandEnv.Prefix(), " (Trace)"),
			"type":       "go",
			"cwd":        UFS.Root,
			"request":    "launch",
			"mode":       "auto",
			"program":    UFS.Caller,
			"buildFlags": "-tags=ppb_trace",
			"args":       "${input:buildPromptCommand} ${input:buildPromptArgument} -Color",
		})

	allCommands := Commands.Keys()
	sort.Strings(allCommands)

	inputs := []JsonMap{
		{
			"id":          "buildPromptCommand",
			"type":        "pickString",
			"options":     allCommands,
			"description": "build command",
		},
		{
			"id":          "buildPromptArgument",
			"type":        "promptString",
			"description": "command argument",
		},
	}

	return UFS.SafeCreate(outputFile, func(w io.Writer) error {
		return JsonSerialize(JsonMap{
			"version":        "0.2.0",
			"configurations": configurations,
			"inputs":         inputs,
		}, w)
	})
}

// func (vsc *VscodeBuilder) make_compiledb(env EnvironmentAlias, output Filename) error {
// 	LogTrace(LogCommand, "generating compile commands '%v' for <%v> environment...", output, env)

// 	fbuildArgs := FBuildArgs{
// 		BffInput: GetBffArgs().BffOutput,
// 	}

// 	// #TODO: use internal compiledb targets instead of running FBuild, which is much slower
// 	fbuildExec := MakeFBuildExecutor(&fbuildArgs, "-compdb", "-nounity", env.String())
// 	fbuildExec.Capture = IsLogLevelActive(LOG_VERYVERBOSE)

// 	if err := fbuildExec.Run(); err != nil {
// 		return err
// 	}

// 	compiledb := UFS.Root.File("compile_commands.json")
// 	if _, err := compiledb.Info(); err != nil {
// 		LogError(LogCommand, "can't find generated compilation commands in '%v'", compiledb)
// 		return err
// 	}

// 	return UFS.Rename(compiledb, output)
// }
