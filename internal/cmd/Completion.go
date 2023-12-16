package cmd

import (
	"fmt"
	"io"
	"regexp"
	"sort"

	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

type completionArgsTraits[T base.Comparable[T]] interface {
	*T
	utils.PersistentVar
}

type CompletionArgs[T base.Comparable[T], P completionArgsTraits[T]] struct {
	Inputs []T
	Output utils.Filename
}

func (flags *CompletionArgs[T, P]) Flags(cfv utils.CommandFlagsVisitor) {
	cfv.Variable("Output", "optional output file", &flags.Output)
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
		filterCompletion(args, func(it T) error {
			_, err := fmt.Fprintln(w, P(&it).String())
			return err
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
					output(values[i])
				}
			}
			pbar.Inc()
		}
	} else {
		pbar := base.LogProgress(0, int64(len(values)), "filter-completion")
		defer pbar.Close()

		for _, it := range values {
			output(it)
			pbar.Inc()
		}
	}
}

func newCompletionCommand[T base.Comparable[T], P completionArgsTraits[T]](
	category, name, description string,
	run func(utils.CommandContext, *CompletionArgs[T, P]) error) func() utils.CommandItem {
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
			filterCompletion(args, func(a utils.BuildAlias) error {
				if node, err := bg.Expect(a); err == nil {
					_, err = fmt.Fprintf(w, "%v --> %v (%T)\n", node.GetBuildStamp(), a, node.GetBuildable())
					return err
				} else {
					return err
				}
			}, bg.Aliases()...)
			return nil
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
			filterCompletion(ca, func(is base.InheritableString) error {
				_, err := fmt.Fprintf(w, "%v=%v\n", is, data[is.Get()])
				return err
			}, base.Map(func(it string) (result utils.StringVar) {
				result.Assign(it)
				return
			}, base.Keys(data)...)...)
			return nil
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
		return printCompletion(ca, base.Keys(repo.Files))
	})
