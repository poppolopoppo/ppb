package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var Vscode = NewCommand(
	"Configure",
	"vscode",
	"generate workspace for Visual Studio Code",
	OptionCommandRun(func(cc CommandContext) error {
		outputDir := UFS.Root.Folder(".vscode")
		base.LogClaim(LogCommand, "generating VSCode workspace in '%v'", outputDir)

		bg := CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "Vscode"})
		defer bg.Close()

		result := BuildVscode(outputDir).Build(bg, OptionBuildForce)
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
		}, internal_io.CreateDirectory(bi, outputDir)
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
func (vsc *VscodeBuilder) Serialize(ar base.Archive) {
	ar.String(&vsc.Version)
	ar.Serializable(&vsc.OutputDir)
}
func (vsc *VscodeBuilder) Build(bc BuildContext) error {
	base.LogVerbose(LogCommand, "build vscode configuration in '%v'...", vsc.OutputDir)

	modules, err := compile.NeedAllBuildModules(bc)
	if err != nil {
		return err
	}

	platform, err := compile.GeLocalHostBuildPlatform().Need(bc)
	if err != nil {
		return err
	}
	compiler, err := platform.GetCompiler().Need(bc)
	if err != nil {
		return err
	}

	moduleAliases := base.Map(func(m compile.Module) compile.ModuleAlias { return m.GetModule().ModuleAlias }, modules...)

	c_cpp_properties_json := vsc.OutputDir.File("c_cpp_properties.json")
	base.LogTrace(LogCommand, "generating vscode c/c++ properties in '%v'", c_cpp_properties_json)
	if err := vsc.c_cpp_properties(bc, moduleAliases, c_cpp_properties_json); err != nil {
		return err
	}

	tasks_json := vsc.OutputDir.File("tasks.json")
	base.LogTrace(LogCommand, "generating vscode build tasks in '%v'", tasks_json)
	if err := vsc.tasks(moduleAliases, tasks_json); err != nil {
		return err
	}

	progamAliases := base.Map(func(m compile.Module) compile.ModuleAlias { return m.GetModule().ModuleAlias },
		base.RemoveUnless(func(m compile.Module) bool { return m.GetModule().ModuleType == compile.MODULE_PROGRAM }, modules...)...)

	launch_json := vsc.OutputDir.File("launch.json")
	base.LogTrace(LogCommand, "generating vscode launch configuratiosn in '%v'", launch_json)
	if err := vsc.launch_configs(progamAliases, compiler, launch_json); err != nil {
		return err
	}

	return bc.OutputFile(c_cpp_properties_json, tasks_json, launch_json)
}

func sanitizeEnvironmentDefines(defines base.StringSet) (base.StringSet, error) {
	ignoreds := make(map[string]string, len(defines))
	keys := make(map[string]string, len(defines))
	for _, it := range defines {
		args := strings.Split(it, "=")
		if len(args) > 2 {
			return base.StringSet{}, fmt.Errorf("invalid define '%s'", it)
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

	result := make(base.StringSet, 0, len(keys))
	for key, value := range keys {
		if len(value) == 0 {
			result.Append(key)
		} else {
			result.Append(fmt.Sprint(key, "=", value))
		}
	}

	return result, nil
}

func (vsc *VscodeBuilder) c_cpp_properties(bc BuildContext, moduleAliases []compile.ModuleAlias, outputFile Filename) error {
	configurations := []base.JsonMap{}

	err := compile.ForeachCompileEnvironment(func(envFactory BuildFactoryTyped[*compile.CompileEnv]) error {
		env, err := envFactory.Need(bc)
		if err != nil {
			return err
		}

		var intelliSenseMode string
		switch env.GetPlatform(bc).Os {
		case "Linux":
			intelliSenseMode = fmt.Sprintf("linux-%s-x64", env.CompilerAlias.CompilerFamily)
		case "Windows":
			intelliSenseMode = fmt.Sprintf("windows-%s-x64", env.CompilerAlias.CompilerFamily)
		default:
			base.UnexpectedValue(env.GetPlatform(bc).Os)
		}

		compileDb, err := compile.BuildCompilationDatabase(env.EnvironmentAlias).Need(bc)
		if err != nil {
			return err
		}

		defines := base.StringSet{}
		includePaths := DirSet{}
		if err := compile.ForeachBuildUnits(bc, env.EnvironmentAlias, func(u *compile.Unit) error {
			defines.AppendUniq(u.Defines...)
			includePaths.AppendUniq(u.IncludePaths...)
			return nil
		}, moduleAliases...); err != nil {
			return err
		}

		defines, err = sanitizeEnvironmentDefines(defines)
		if err != nil {
			return nil
		}

		configurations = append(configurations, base.JsonMap{
			"name":             env.EnvironmentAlias.String(),
			"compilerPath":     env.GetCompiler(bc).Executable.String(),
			"compileCommands":  compileDb.OutputFile,
			"cStandard":        "c17",
			"cppStandard":      strings.ToLower(env.GetCpp(bc, nil).CppStd.String()),
			"defines":          defines,
			"includePath":      includePaths,
			"intelliSenseMode": intelliSenseMode,
			"browse": base.JsonMap{
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

	return UFS.CreateBuffered(outputFile, func(w io.Writer) error {
		return base.JsonSerialize(base.JsonMap{
			"version":        4,
			"configurations": configurations,
		}, w)
	}, base.TransientPage4KiB)
}
func (vsc *VscodeBuilder) tasks(moduleAliases compile.ModuleAliases, outputFile Filename) error {
	var problemMatcher string
	switch base.GetCurrentHost().Id {
	case base.HOST_LINUX, base.HOST_DARWIN:
		problemMatcher = "$gcc"
	case base.HOST_WINDOWS:
		problemMatcher = "$msCompile"
	default:
		return base.MakeUnexpectedValueError(problemMatcher, base.GetCurrentHost().Id)
	}

	const buildCommand = "build"
	tasks := base.Map(func(moduleAliases compile.ModuleAlias) base.JsonMap {
		label := moduleAliases.String()
		return base.JsonMap{
			"label":   label,
			"command": UFS.Executable.String(),
			"args":    []string{buildCommand, "-Ide", label + "-${command:cpptools.activeConfigName}"},
			"options": base.JsonMap{
				"cwd": UFS.Root,
			},
			"group": base.JsonMap{
				"kind":      "build",
				"isDefault": true,
			},
			"presentation": base.JsonMap{
				"clear":  true,
				"echo":   true,
				"reveal": "always",
				"focus":  false,
				"panel":  "dedicated",
			},
			"problemMatcher": problemMatcher,
		}
	}, moduleAliases...)

	return UFS.CreateBuffered(outputFile, func(w io.Writer) error {
		return base.JsonSerialize(base.JsonMap{
			"version": "2.0.0",
			"tasks":   tasks,
		}, w)
	}, base.TransientPage4KiB)
}
func (vsc *VscodeBuilder) launch_configs(programAliases compile.ModuleAliases, compiler compile.Compiler, outputFile Filename) error {
	var debuggerType string
	switch base.GetCurrentHost().Id {
	case base.HOST_LINUX, base.HOST_DARWIN:
		debuggerType = "cppdbg"
	case base.HOST_WINDOWS:
		debuggerType = "cppvsdbg"
	default:
		base.UnexpectedValue(base.GetCurrentHost().Id)
	}

	// create a launch single launch configuration per executable
	// configuration is deduced from selection in vscode
	configurations := base.Map(func(programAlias compile.ModuleAlias) base.JsonMap {
		alias := programAlias.String()
		executable := SanitizePath(alias, '-')

		environment := []base.JsonMap{}
		for _, it := range compiler.GetCompiler().Environment {
			environment = append(environment, base.JsonMap{
				"name":  it.Name.String(),
				"value": strings.Join(it.Values, ";"),
			})
		}

		cfg := base.JsonMap{
			"name":        alias,
			"type":        debuggerType,
			"request":     "launch",
			"program":     UFS.Binaries.File(executable + "-${command:cpptools.activeConfigName}" + compiler.Extname(compile.PAYLOAD_EXECUTABLE)),
			"args":        []string{},
			"stopAtEntry": false,
			"cwd":         UFS.Binaries,
			"environment": environment,
		}

		return cfg
	}, programAliases...)

	// append configurations for debugging the build system
	configurations = append(configurations,
		base.JsonMap{
			"name":    fmt.Sprint("Build ", CommandEnv.Prefix()),
			"type":    "go",
			"cwd":     UFS.Root,
			"request": "launch",
			"mode":    "auto",
			"program": UFS.Caller,
			"args":    "${input:buildPromptCommand} ${input:buildPromptArgument} -Color",
		},
		base.JsonMap{
			"name":       fmt.Sprint("Build ", CommandEnv.Prefix(), " (Debug)"),
			"type":       "go",
			"cwd":        UFS.Root,
			"request":    "launch",
			"mode":       "auto",
			"program":    UFS.Caller,
			"buildFlags": "-tags=ppb_debug,debug -pgo=off -gcflags=-N -gcflags=-w",
			"args":       "${input:buildPromptCommand} ${input:buildPromptArgument} -Color",
		},
		base.JsonMap{
			"name":       fmt.Sprint("Build ", CommandEnv.Prefix(), " (Profiling)"),
			"type":       "go",
			"cwd":        UFS.Root,
			"request":    "launch",
			"mode":       "auto",
			"program":    UFS.Caller,
			"buildFlags": "-tags=ppb_profiling",
			"args":       "${input:buildPromptCommand} ${input:buildPromptArgument} -Color -q",
		},
		base.JsonMap{
			"name":       fmt.Sprint("Build ", CommandEnv.Prefix(), " (Race)"),
			"type":       "go",
			"cwd":        UFS.Root,
			"request":    "launch",
			"mode":       "auto",
			"program":    UFS.Caller,
			"buildFlags": "-race",
			"env": base.JsonMap{
				"GORACE": "halt_on_error=true atexit_sleep_ms=10000",
			},
			"args": "${input:buildPromptCommand} ${input:buildPromptArgument} -Color",
		},
		base.JsonMap{
			"name":       fmt.Sprint("Build ", CommandEnv.Prefix(), " (Trace)"),
			"type":       "go",
			"cwd":        UFS.Root,
			"request":    "launch",
			"mode":       "auto",
			"program":    UFS.Caller,
			"buildFlags": "-tags=ppb_trace -pgo=off",
			"args":       "${input:buildPromptCommand} ${input:buildPromptArgument} -Color",
		})

	allCommands := AllCommands.Keys()
	sort.Strings(allCommands)

	inputs := []base.JsonMap{
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

	return UFS.CreateBuffered(outputFile, func(w io.Writer) error {
		return base.JsonSerialize(base.JsonMap{
			"version":        "0.2.0",
			"configurations": configurations,
			"inputs":         inputs,
		}, w)
	}, base.TransientPage4KiB)
}
