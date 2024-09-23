package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

type CompletionArgs struct {
	GlobPatterns []utils.StringVar
	Output       utils.Filename
	Detailed     utils.BoolVar
}

func (flags *CompletionArgs) Flags(cfv utils.CommandFlagsVisitor) {
	cfv.Variable("Output", "optional output file", &flags.Output)
	cfv.Variable("l", "add more details to completion output", &flags.Detailed)
}

func openCompletion(args *CompletionArgs, closure func(io.Writer) error) error {
	base.LogVerbose(utils.LogCommand, "completion input parameters = %v", args.GlobPatterns)
	if args.Output.Valid() {
		base.LogInfo(utils.LogCommand, "export completion results to %q...", args.Output)
		return utils.UFS.CreateBuffered(args.Output, closure)
	} else {
		return closure(base.GetLogger())
	}
}
func printCompletion(args *CompletionArgs, in base.StringSet) error {
	return openCompletion(args, func(w io.Writer) error {
		return filterCompletion(args, func(it string) (err error) {
			_, err = fmt.Fprintln(w, it)
			return
		}, in...)
	})
}
func filterCompletion(args *CompletionArgs, output func(string) error, values ...string) error {
	sort.Strings(values)

	if len(args.GlobPatterns) > 0 {
		keys := base.MakeStringerSet(args.GlobPatterns...)
		keys.Sort()

		globRE := utils.MakeGlobRegexp(keys...)
		if globRE == nil {
			return fmt.Errorf("invalid regular expression: %q", keys)
		}

		for _, it := range values {
			if globRE.MatchString(it) {
				if err := output(it); err != nil {
					return err
				}
			}
		}

	} else {
		for _, it := range values {
			if err := output(it); err != nil {
				return err
			}
		}
	}

	return nil
}

func printFileCompletion(w io.Writer, path utils.Filename, detailed bool) error {
	pathColor := base.NewColorFromStringHash(path.Ext())

	if detailed {
		var (
			fileMode os.FileMode
			fileSize base.SizeInBytes
			fileTime time.Time
		)

		modeColor := base.NewColorFromHash(uint64(fileMode))
		sizeColor := base.ANSI_FG1_YELLOW
		timeColor := base.ANSI_FG1_BLUE

		if info, err := path.Info(); err == nil {
			fileMode = info.Mode()
			fileSize = base.SizeInBytes(info.Size())
			fileTime = info.ModTime()
		} else {
			modeColor = modeColor.Brightness(0.25)
			pathColor = pathColor.Brightness(0.25)
			sizeColor = base.ANSI_FG0_YELLOW
			timeColor = base.ANSI_FG0_BLUE
		}

		_, err := fmt.Fprintf(w, "%v%v %v%10v %v%v  %v%v%v\n",
			modeColor.Quantize(true).Ansi(true),
			fileMode,
			sizeColor,
			fileSize,
			timeColor,
			fileTime.Format(time.Stamp),
			pathColor.Quantize(true).Ansi(true),
			path,
			base.ANSI_RESET)
		return err
	} else {
		_, err := fmt.Fprintln(w, pathColor.Quantize(true).Ansi(true), path.String(), base.ANSI_RESET)
		return err
	}
}

func newCompletionCommand(
	category, name, description string,
	run func(utils.CommandContext, *CompletionArgs) error,
) func() utils.CommandItem {
	completionArgs := &CompletionArgs{}
	return utils.NewCommand(
		category, name, description,
		utils.OptionCommandParsableAccessor("CompletionArgs", "control completion command output", func() *CompletionArgs { return completionArgs }),
		utils.OptionCommandConsumeMany("GlobPatterns", "multiple command input", &completionArgs.GlobPatterns, utils.COMMANDARG_OPTIONAL),
		utils.OptionCommandRun(func(cc utils.CommandContext) error {
			return run(cc, completionArgs)
		}))
}

var ListArtifacts = newCompletionCommand(
	"Metadata",
	"list-artifacts",
	"list all known artifacts",
	func(cc utils.CommandContext, args *CompletionArgs) error {
		bg := utils.CommandEnv.BuildGraph().OpenReadPort(base.ThreadPoolDebugId{Category: "ListArtifacts"})
		defer bg.Close()
		return openCompletion(args, func(w io.Writer) error {
			return filterCompletion(args, func(in string) error {
				var a utils.BuildAlias
				if err := a.Set(in); err != nil {
					return err
				}
				if args.Detailed.Get() {
					if node, err := bg.Expect(a); err == nil {
						_, err = fmt.Fprintf(w, "%v --> %v (%T)\n", node.GetBuildStamp(), a, node.GetBuildable())
						return err
					} else {
						return err
					}
				} else {
					_, err := fmt.Fprintln(w, a)
					return err
				}
			}, base.MakeStringerSet(bg.Aliases()...)...)
		})
	})

var ListSourceFiles = newCompletionCommand(
	"Metadata",
	"list-source-files",
	"list all known source files",
	func(cc utils.CommandContext, args *CompletionArgs) error {
		bg := utils.CommandEnv.BuildGraph().OpenReadPort(base.ThreadPoolDebugId{Category: "ListSourceFiles"})
		defer bg.Close()
		return openCompletion(args, func(w io.Writer) error {
			var files utils.FileSet
			bg.Range(func(ba utils.BuildAlias, bn utils.BuildNode) error {
				if it, ok := bn.GetBuildable().(utils.BuildableSourceFile); ok {
					files.AppendUniq(it.GetSourceFile())
				}
				return nil
			})

			return filterCompletion(args, func(in string) error {
				var file utils.Filename
				if err := file.Set(in); err != nil {
					return err
				}
				return printFileCompletion(w, file, args.Detailed.Get())
			}, base.MakeStringerSet(files...)...)
		})
	})

var ListGeneratedFiles = newCompletionCommand(
	"Metadata",
	"list-generated-files",
	"list all known generated files",
	func(cc utils.CommandContext, args *CompletionArgs) error {
		bg := utils.CommandEnv.BuildGraph().OpenReadPort(base.ThreadPoolDebugId{Category: "ListGeneratedFiles"})
		defer bg.Close()
		return openCompletion(args, func(w io.Writer) error {
			var files utils.FileSet
			bg.Range(func(ba utils.BuildAlias, bn utils.BuildNode) error {
				if it, ok := bn.GetBuildable().(utils.BuildableGeneratedFile); ok {
					files.AppendUniq(it.GetGeneratedFile())
				}
				return nil
			})

			return filterCompletion(args, func(in string) error {
				var file utils.Filename
				if err := file.Set(in); err != nil {
					return err
				}
				return printFileCompletion(w, file, args.Detailed.Get())
			}, base.MakeStringerSet(files...)...)
		})
	})

var ListModifiedFiles = newCompletionCommand(
	"Metadata",
	"list-modified-files",
	"list modified files from source control",
	func(cc utils.CommandContext, ca *CompletionArgs) error {
		var repo utils.SourceControlRepositoryStatus
		if err := utils.GetSourceControlProvider().GetRepositoryStatus(&repo); err != nil {
			return err
		}

		return openCompletion(ca, func(w io.Writer) error {
			return filterCompletion(ca, func(in string) error {
				var file utils.Filename
				if err := file.Set(in); err != nil {
					return err
				}
				return printFileCompletion(w, file, ca.Detailed.Get())
			}, base.MakeStringerSet(base.Keys(repo.Files)...)...)
		})
	})

var ListCommands = newCompletionCommand(
	"Metadata",
	"list-commands",
	"list all available commands",
	func(cc utils.CommandContext, ca *CompletionArgs) error {
		return printCompletion(ca, base.MakeStringerSet(utils.GetAllCommandNames()...))
	})

var ListPlatforms = newCompletionCommand(
	"Metadata",
	"list-platforms",
	"list all available platforms",
	func(cc utils.CommandContext, ca *CompletionArgs) error {
		bg := utils.CommandEnv.BuildGraph().OpenReadPort(base.ThreadPoolDebugId{Category: "ListPlatforms"})
		defer bg.Close()
		return printCompletion(ca, base.MakeStringerSet(compile.GetAllPlatformAliases(bg)...))
	})

var ListConfigs = newCompletionCommand(
	"Metadata",
	"list-configs",
	"list all available configurations",
	func(cc utils.CommandContext, ca *CompletionArgs) error {
		return printCompletion(ca, base.MakeStringerSet(compile.GetAllConfigurationAliases()...))
	})

var ListCompilers = newCompletionCommand(
	"Metadata",
	"list-compilers",
	"list all available compilers",
	func(cc utils.CommandContext, ca *CompletionArgs) error {
		return printCompletion(ca, base.MakeStringerSet(compile.AllCompilerNames.Slice()...))
	})

var ListModules = newCompletionCommand(
	"Metadata",
	"list-modules",
	"list all available modules",
	func(cc utils.CommandContext, ca *CompletionArgs) error {
		bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "ListModules"})
		defer bg.Close()
		if moduleAliases, err := compile.NeedAllModuleAliases(bg.GlobalContext()); err == nil {
			return printCompletion(ca, base.MakeStringerSet(moduleAliases...))
		} else {
			return err
		}
	})

var ListNamespaces = newCompletionCommand(
	"Metadata",
	"list-namespaces",
	"list all available namespaces",
	func(cc utils.CommandContext, ca *CompletionArgs) error {
		bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "ListNamespaces"})
		defer bg.Close()

		moduleAliases, err := compile.NeedAllModuleAliases(bg.GlobalContext())
		if err != nil {
			return err
		}

		namespaceAliases := base.NewSet[compile.NamespaceAlias]()
		for _, moduleAlias := range moduleAliases {
			namespaceAliases.AppendUniq(moduleAlias.NamespaceAlias)
		}

		return printCompletion(ca, base.MakeStringerSet(namespaceAliases...))
	})

var ListEnvironments = newCompletionCommand(
	"Metadata",
	"list-environments",
	"list all compilation environments",
	func(cc utils.CommandContext, ca *CompletionArgs) error {
		return printCompletion(ca, base.MakeStringerSet(compile.GetEnvironmentAliases()...))
	})

var ListTargets = newCompletionCommand(
	"Metadata",
	"list-targets",
	"list all translated targets",
	func(cc utils.CommandContext, ca *CompletionArgs) error {
		bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "ListTargets"})
		defer bg.Close()

		units, err := compile.NeedAllBuildUnits(bg.GlobalContext())
		if err != nil {
			return err
		}
		aliases := base.Map(func(u *compile.Unit) compile.TargetAlias { return u.TargetAlias }, units...)
		return printCompletion(ca, base.MakeStringerSet(aliases...))
	})

var ListPrograms = newCompletionCommand(
	"Metadata",
	"list-programs",
	"list all executable targets",
	func(cc utils.CommandContext, ca *CompletionArgs) error {
		bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "ListPrograms"})
		defer bg.Close()

		units, err := compile.NeedAllBuildUnits(bg.GlobalContext())
		if err != nil {
			return err
		}

		executables := base.RemoveUnless(func(unit *compile.Unit) bool {
			return unit.Payload == compile.PAYLOAD_EXECUTABLE
		}, units...)

		aliases := base.Map(func(u *compile.Unit) compile.TargetAlias { return u.TargetAlias }, executables...)
		return printCompletion(ca, base.MakeStringerSet(aliases...))
	})

var ListPersistentData = newCompletionCommand(
	"Metadata",
	"list-persistents",
	"list all persistent data",
	func(cc utils.CommandContext, ca *CompletionArgs) error {
		data := utils.CommandEnv.Persistent().PinData()
		return openCompletion(ca, func(w io.Writer) error {
			return filterCompletion(ca, func(is string) error {
				_, err := fmt.Fprintf(w, "%v=%v\n", is, data[is])
				return err
			}, base.Keys(data)...)
		})
	})
