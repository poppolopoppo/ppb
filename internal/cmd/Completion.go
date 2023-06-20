package cmd

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"

	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

type completionArgsTraits[T base.Comparable[T]] interface {
	*T
	PersistentVar
}

type CompletionArgs[T base.Comparable[T], P completionArgsTraits[T]] struct {
	Inputs []T
	Output Filename
}

func (flags *CompletionArgs[T, P]) Flags(cfv CommandFlagsVisitor) {
	cfv.Variable("Output", "optional output file", &flags.Output)
}

func openCompletion[T base.Comparable[T], P completionArgsTraits[T]](args *CompletionArgs[T, P], closure func(io.Writer) error) error {
	base.LogVerbose(LogCommand, "completion input parameters = %v", args.Inputs)
	if args.Output.Basename != "" {
		base.LogInfo(LogCommand, "export completion results to %q...", args.Output)
		return UFS.CreateBuffered(args.Output, closure)
	} else {
		return closure(os.Stdout)
	}
}
func printCompletion[T base.Comparable[T], P completionArgsTraits[T]](args *CompletionArgs[T, P], in []T) error {
	return openCompletion(args, func(w io.Writer) error {
		filterCompletion(args, func(it T) error {
			base.WithoutLog(func() {
				fmt.Fprintln(w, P(&it).String())
			})
			return nil
		}, in...)
		return nil
	})
}
func filterCompletion[T base.Comparable[T], P completionArgsTraits[T]](completionArgs *CompletionArgs[T, P], output func(T) error, values ...T) {
	args := completionArgs.Inputs

	sort.Slice(values, func(i, j int) bool {
		return values[i].Compare(values[j]) < 0
	})

	if len(args) > 0 {
		pbar := base.LogProgress(0, len(args), "filter-completion")
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
					output(values[i])
				}
			}
			pbar.Inc()
		}
	} else {
		pbar := base.LogProgress(0, len(values), "filter-completion")
		defer pbar.Close()

		for _, it := range values {
			output(it)
			pbar.Inc()
		}
	}
}

func newCompletionCommand[T base.Comparable[T], P completionArgsTraits[T]](
	category, name, description string,
	run func(CommandContext, *CompletionArgs[T, P]) error) func() CommandItem {
	completionArgs := &CompletionArgs[T, P]{}
	return NewCommand(
		category, name, description,
		OptionCommandParsableAccessor("CompletionArgs", "control completion command output", func() *CompletionArgs[T, P] { return completionArgs }),
		OptionCommandConsumeMany[T, P]("Input", fmt.Sprintf("multiple [%s] command input", base.GetTypenameT[T]()), &completionArgs.Inputs, COMMANDARG_OPTIONAL),
		OptionCommandRun(func(cc CommandContext) error {
			return run(cc, completionArgs)
		}))
}

var ListArtifacts = newCompletionCommand(
	"Metadata",
	"list-artifacts",
	"list all known artifacts",
	func(cc CommandContext, args *CompletionArgs[BuildAlias, *BuildAlias]) error {
		bg := CommandEnv.BuildGraph()
		return openCompletion(args, func(w io.Writer) error {
			filterCompletion(args, func(a BuildAlias) error {
				if node := bg.Find(a); node != nil {
					base.WithoutLog(func() {
						fmt.Fprintf(w, "%v --> %v (%T)\n", node.GetBuildStamp(), a, node.GetBuildable())
					})
					return nil
				} else {
					return utils.BuildableNotFound{Alias: a}
				}
			}, bg.Aliases()...)
			return nil
		})
	})

var ListCommands = newCompletionCommand[CommandName, *CommandName](
	"Metadata",
	"list-commands",
	"list all available commands",
	func(cc CommandContext, ca *CompletionArgs[CommandName, *CommandName]) error {
		return printCompletion(ca, GetAllCommandNames())
	})

var ListPlatforms = newCompletionCommand[compile.PlatformAlias, *compile.PlatformAlias](
	"Metadata",
	"list-platforms",
	"list all available platforms",
	func(cc CommandContext, ca *CompletionArgs[compile.PlatformAlias, *compile.PlatformAlias]) error {
		return printCompletion(ca, compile.GetAllPlatformAliases())
	})

var ListConfigs = newCompletionCommand[compile.ConfigurationAlias, *compile.ConfigurationAlias](
	"Metadata",
	"list-configs",
	"list all available configurations",
	func(cc CommandContext, ca *CompletionArgs[compile.ConfigurationAlias, *compile.ConfigurationAlias]) error {
		return printCompletion(ca, compile.GetAllConfigurationAliases())
	})

var ListCompilers = newCompletionCommand[compile.CompilerName, *compile.CompilerName](
	"Metadata",
	"list-compilers",
	"list all available compilers",
	func(cc CommandContext, ca *CompletionArgs[compile.CompilerName, *compile.CompilerName]) error {
		return printCompletion(ca, compile.AllCompilerNames.Slice())
	})

var ListModules = newCompletionCommand[compile.ModuleAlias, *compile.ModuleAlias](
	"Metadata",
	"list-modules",
	"list all available modules",
	func(cc CommandContext, ca *CompletionArgs[compile.ModuleAlias, *compile.ModuleAlias]) error {
		bc := CommandEnv.BuildGraph().GlobalContext()
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
	func(cc CommandContext, ca *CompletionArgs[compile.NamespaceAlias, *compile.NamespaceAlias]) error {
		bc := CommandEnv.BuildGraph().GlobalContext()
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
	func(cc CommandContext, ca *CompletionArgs[compile.EnvironmentAlias, *compile.EnvironmentAlias]) error {
		return printCompletion(ca, compile.GetEnvironmentAliases())
	})

var ListTargets = newCompletionCommand[compile.TargetAlias, *compile.TargetAlias](
	"Metadata",
	"list-targets",
	"list all translated targets",
	func(cc CommandContext, ca *CompletionArgs[compile.TargetAlias, *compile.TargetAlias]) error {
		units, err := compile.NeedAllBuildUnits(CommandEnv.BuildGraph().GlobalContext())
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
	func(cc CommandContext, ca *CompletionArgs[compile.TargetAlias, *compile.TargetAlias]) error {
		units, err := compile.NeedAllBuildUnits(CommandEnv.BuildGraph().GlobalContext())
		if err != nil {
			return err
		}

		executables := base.RemoveUnless(func(unit *compile.Unit) bool {
			return unit.Payload == compile.PAYLOAD_EXECUTABLE
		}, units...)

		aliases := base.Map(func(u *compile.Unit) compile.TargetAlias { return u.TargetAlias }, executables...)
		return printCompletion(ca, aliases)
	})

var ListPersistentData = newCompletionCommand[StringVar, *StringVar](
	"Metadata",
	"list-persistents",
	"list all persistent data",
	func(cc CommandContext, ca *CompletionArgs[base.InheritableString, *base.InheritableString]) error {
		data := CommandEnv.Persistent().PinData()
		return openCompletion(ca, func(w io.Writer) error {
			filterCompletion(ca, func(is base.InheritableString) error {
				base.WithoutLog(func() {
					fmt.Fprintf(w, "%v=%v\n", is, data[is.Get()])
				})
				return nil
			}, base.Map(func(it string) (result StringVar) {
				result.Assign(it)
				return
			}, base.Keys(data)...)...)
			return nil
		})
	})

var ListModifiedFiles = newCompletionCommand[Filename, *Filename](
	"Metadata",
	"list-modified-files",
	"list modified files from source control",
	func(cc CommandContext, ca *CompletionArgs[Filename, *Filename]) error {
		bg := CommandEnv.BuildGraph()
		result := BuildSourceControlModifiedFiles(UFS.Source).Build(bg)
		if err := result.Failure(); err == nil {
			return printCompletion(ca, result.Success().ModifiedFiles)
		} else {
			return err
		}
	})
