package cmd

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"time"

	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

type completionArgsTraits[T base.Comparable[T]] interface {
	*T
	utils.PersistentVar
}

type CompletionArgs[T base.Comparable[T], P completionArgsTraits[T]] struct {
	Inputs   []T
	Output   utils.Filename
	Detailed utils.BoolVar
}

func (flags *CompletionArgs[T, P]) Flags(cfv utils.CommandFlagsVisitor) {
	cfv.Variable("Output", "optional output file", &flags.Output)
	cfv.Variable("l", "add more details to completion output", &flags.Detailed)
}

func openCompletion[T base.Comparable[T], P completionArgsTraits[T]](args *CompletionArgs[T, P], closure func(io.Writer) error) error {
	base.LogVerbose(utils.LogCommand, "completion input parameters = %v", args.Inputs)
	if args.Output.Basename != "" {
		base.LogInfo(utils.LogCommand, "export completion results to %q...", args.Output)
		return utils.UFS.CreateBuffered(args.Output, closure)
	} else {
		return closure(base.GetLogger())
	}
}
func printCompletion[T base.Comparable[T], P completionArgsTraits[T]](args *CompletionArgs[T, P], in []T) error {
	return openCompletion(args, func(w io.Writer) error {
		return filterCompletion(args, func(it T) (err error) {
			if args.Detailed.Get() {
				_, err = fmt.Fprint(w, base.PrettyPrint(P(&it)))
			} else {
				_, err = fmt.Fprintln(w, P(&it).String())
			}
			return
		}, in...)
	})
}
func filterCompletion[T base.Comparable[T], P completionArgsTraits[T]](completionArgs *CompletionArgs[T, P], output func(T) error, values ...T) error {
	args := completionArgs.Inputs

	sort.Slice(values, func(i, j int) bool {
		return values[i].Compare(values[j]) < 0
	})

	if len(args) > 0 {
		pbar := base.LogProgress(0, int64(len(args)), "filter-completion")
		defer pbar.Close()

		in := make([]string, len(values))
		for i, it := range values {
			in[i] = P(&it).String()
		}
		sort.Strings(in)

		for _, q := range args {
			glob := regexp.MustCompile(regexp.QuoteMeta(P(&q).String()))
			for i, it := range in {
				if glob.MatchString(it) {
					if err := output(values[i]); err != nil {
						return err
					}
				}
			}
			pbar.Inc()
		}
	} else {
		pbar := base.LogProgress(0, int64(len(values)), "filter-completion")
		defer pbar.Close()

		for _, it := range values {
			if err := output(it); err != nil {
				return err
			}
			pbar.Inc()
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

func newCompletionCommand[T base.Comparable[T], P completionArgsTraits[T]](
	category, name, description string,
	run func(utils.CommandContext, *CompletionArgs[T, P]) error,
) func() utils.CommandItem {
	completionArgs := &CompletionArgs[T, P]{}
	return utils.NewCommand(
		category, name, description,
		utils.OptionCommandParsableAccessor("CompletionArgs", "control completion command output", func() *CompletionArgs[T, P] { return completionArgs }),
		utils.OptionCommandConsumeMany[T, P]("Input", fmt.Sprintf("multiple [%s] command input", base.GetTypenameT[T]()), &completionArgs.Inputs, utils.COMMANDARG_OPTIONAL),
		utils.OptionCommandRun(func(cc utils.CommandContext) error {
			return run(cc, completionArgs)
		}))
}

var ListArtifacts = newCompletionCommand(
	"Metadata",
	"list-artifacts",
	"list all known artifacts",
	func(cc utils.CommandContext, args *CompletionArgs[utils.BuildAlias, *utils.BuildAlias]) error {
		bg := utils.CommandEnv.BuildGraph()
		return openCompletion(args, func(w io.Writer) error {
			return filterCompletion(args, func(a utils.BuildAlias) error {
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
			}, bg.Aliases()...)
		})
	})

var ListSourceFiles = newCompletionCommand(
	"Metadata",
	"list-source-files",
	"list all known source files",
	func(cc utils.CommandContext, args *CompletionArgs[utils.BuildAlias, *utils.BuildAlias]) error {
		bg := utils.CommandEnv.BuildGraph()
		return openCompletion(args, func(w io.Writer) error {
			return filterCompletion(args, func(a utils.BuildAlias) error {
				if node, err := bg.Expect(a); err == nil {
					switch buildable := node.GetBuildable().(type) {
					case utils.BuildableSourceFile:
						err = printFileCompletion(w, buildable.GetSourceFile(), args.Detailed.Get())
					}
					return err
				} else {
					return err
				}
			}, bg.Aliases()...)
		})
	})

var ListGeneratedFiles = newCompletionCommand(
	"Metadata",
	"list-generated-files",
	"list all known generated files",
	func(cc utils.CommandContext, args *CompletionArgs[utils.BuildAlias, *utils.BuildAlias]) error {
		bg := utils.CommandEnv.BuildGraph()
		return openCompletion(args, func(w io.Writer) error {
			return filterCompletion(args, func(a utils.BuildAlias) error {
				if node, err := bg.Expect(a); err == nil {
					switch buildable := node.GetBuildable().(type) {
					case utils.BuildableGeneratedFile:
						err = printFileCompletion(w, buildable.GetGeneratedFile(), args.Detailed.Get())
					}
					return err
				} else {
					return err
				}
			}, bg.Aliases()...)
		})
	})

var ListCommands = newCompletionCommand[utils.CommandName, *utils.CommandName](
	"Metadata",
	"list-commands",
	"list all available commands",
	func(cc utils.CommandContext, ca *CompletionArgs[utils.CommandName, *utils.CommandName]) error {
		return printCompletion(ca, utils.GetAllCommandNames())
	})

var ListPlatforms = newCompletionCommand[compile.PlatformAlias, *compile.PlatformAlias](
	"Metadata",
	"list-platforms",
	"list all available platforms",
	func(cc utils.CommandContext, ca *CompletionArgs[compile.PlatformAlias, *compile.PlatformAlias]) error {
		return printCompletion(ca, compile.GetAllPlatformAliases())
	})

var ListConfigs = newCompletionCommand[compile.ConfigurationAlias, *compile.ConfigurationAlias](
	"Metadata",
	"list-configs",
	"list all available configurations",
	func(cc utils.CommandContext, ca *CompletionArgs[compile.ConfigurationAlias, *compile.ConfigurationAlias]) error {
		return printCompletion(ca, compile.GetAllConfigurationAliases())
	})

var ListCompilers = newCompletionCommand[compile.CompilerName, *compile.CompilerName](
	"Metadata",
	"list-compilers",
	"list all available compilers",
	func(cc utils.CommandContext, ca *CompletionArgs[compile.CompilerName, *compile.CompilerName]) error {
		return printCompletion(ca, compile.AllCompilerNames.Slice())
	})

var ListModules = newCompletionCommand[compile.ModuleAlias, *compile.ModuleAlias](
	"Metadata",
	"list-modules",
	"list all available modules",
	func(cc utils.CommandContext, ca *CompletionArgs[compile.ModuleAlias, *compile.ModuleAlias]) error {
		bc := utils.CommandEnv.BuildGraph().GlobalContext()
		if moduleAliases, err := compile.NeedAllModuleAliases(bc); err == nil {
			return printCompletion(ca, moduleAliases)
		} else {
			return err
		}
	})

var ListNamespaces = newCompletionCommand[compile.NamespaceAlias, *compile.NamespaceAlias](
	"Metadata",
	"list-namespaces",
	"list all available namespaces",
	func(cc utils.CommandContext, ca *CompletionArgs[compile.NamespaceAlias, *compile.NamespaceAlias]) error {
		bc := utils.CommandEnv.BuildGraph().GlobalContext()
		moduleAliases, err := compile.NeedAllModuleAliases(bc)
		if err != nil {
			return err
		}

		namespaceAliases := base.NewSet[compile.NamespaceAlias]()
		for _, moduleAlias := range moduleAliases {
			namespaceAliases.AppendUniq(moduleAlias.NamespaceAlias)
		}

		return printCompletion(ca, namespaceAliases)
	})

var ListEnvironments = newCompletionCommand[compile.EnvironmentAlias, *compile.EnvironmentAlias](
	"Metadata",
	"list-environments",
	"list all compilation environments",
	func(cc utils.CommandContext, ca *CompletionArgs[compile.EnvironmentAlias, *compile.EnvironmentAlias]) error {
		return printCompletion(ca, compile.GetEnvironmentAliases())
	})

var ListTargets = newCompletionCommand[compile.TargetAlias, *compile.TargetAlias](
	"Metadata",
	"list-targets",
	"list all translated targets",
	func(cc utils.CommandContext, ca *CompletionArgs[compile.TargetAlias, *compile.TargetAlias]) error {
		units, err := compile.NeedAllBuildUnits(utils.CommandEnv.BuildGraph().GlobalContext())
		if err != nil {
			return err
		}
		aliases := base.Map(func(u *compile.Unit) compile.TargetAlias { return u.TargetAlias }, units...)
		return printCompletion(ca, aliases)
	})

var ListPrograms = newCompletionCommand[compile.TargetAlias, *compile.TargetAlias](
	"Metadata",
	"list-programs",
	"list all executable targets",
	func(cc utils.CommandContext, ca *CompletionArgs[compile.TargetAlias, *compile.TargetAlias]) error {
		units, err := compile.NeedAllBuildUnits(utils.CommandEnv.BuildGraph().GlobalContext())
		if err != nil {
			return err
		}

		executables := base.RemoveUnless(func(unit *compile.Unit) bool {
			return unit.Payload == compile.PAYLOAD_EXECUTABLE
		}, units...)

		aliases := base.Map(func(u *compile.Unit) compile.TargetAlias { return u.TargetAlias }, executables...)
		return printCompletion(ca, aliases)
	})

var ListPersistentData = newCompletionCommand[utils.StringVar, *utils.StringVar](
	"Metadata",
	"list-persistents",
	"list all persistent data",
	func(cc utils.CommandContext, ca *CompletionArgs[base.InheritableString, *base.InheritableString]) error {
		data := utils.CommandEnv.Persistent().PinData()
		return openCompletion(ca, func(w io.Writer) error {
			return filterCompletion(ca, func(is base.InheritableString) error {
				_, err := fmt.Fprintf(w, "%v=%v\n", is, data[is.Get()])
				return err
			}, base.Map(func(it string) (result utils.StringVar) {
				result.Assign(it)
				return
			}, base.Keys(data)...)...)
		})
	})

var ListModifiedFiles = newCompletionCommand[utils.Filename, *utils.Filename](
	"Metadata",
	"list-modified-files",
	"list modified files from source control",
	func(cc utils.CommandContext, ca *CompletionArgs[utils.Filename, *utils.Filename]) error {
		var repo utils.SourceControlRepositoryStatus
		if err := utils.GetSourceControlProvider().GetRepositoryStatus(&repo); err != nil {
			return err
		}

		return openCompletion(ca, func(w io.Writer) error {
			return filterCompletion(ca, func(file utils.Filename) error {
				return printFileCompletion(w, file, ca.Detailed.Get())
			}, base.Keys(repo.Files)...)
		})
	})
