package cmd

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"

	compile "github.com/poppolopoppo/ppb/compile"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

type CompletionArgs struct {
	Inputs []StringVar
	Output Filename
}

var GetCompletionArgs = NewCommandParsableFlags(&CompletionArgs{})

func OptionCommandCompletionArgs() CommandOptionFunc {
	return OptionCommandItem(func(ci CommandItem) {
		ci.Options(
			OptionCommandParsableAccessor("CompletionArgs", "control completion command output", GetCompletionArgs),
			OptionCommandConsumeMany("Input", "multiple command input", &GetCompletionArgs().Inputs, COMMANDARG_OPTIONAL))
	})
}

func (flags *CompletionArgs) Flags(cfv CommandFlagsVisitor) {
	cfv.Variable("Output", "optional output file", &flags.Output)
}

func openCompletion(args *CompletionArgs, closure func(io.Writer) error) error {
	LogVerbose(LogCommand, "completion input parameters = %v", args.Inputs)
	if args.Output.Basename != "" {
		LogInfo(LogCommand, "export completion results to %q...", args.Output)
		return UFS.CreateBuffered(args.Output, closure)
	} else {
		return closure(os.Stdout)
	}
}
func printCompletion(args *CompletionArgs, in []string) error {
	return openCompletion(args, func(w io.Writer) error {
		filterCompletion(args, func(s string) {
			WithoutLog(func() {
				fmt.Fprintln(w, s)
			})
		}, in...)
		return nil
	})
}
func mapCompletion[T any](completionArgs *CompletionArgs, output func(string), values map[string]T) {
	filterCompletion(completionArgs, output, Keys(values)...)
}
func filterCompletion(completionArgs *CompletionArgs, output func(string), in ...string) {
	sort.Strings(in)
	args := completionArgs.Inputs

	if len(args) > 0 {
		pbar := LogProgress(0, len(args), "filter-completion")
		defer pbar.Close()

		for _, q := range args {
			glob := regexp.MustCompile(regexp.QuoteMeta(q.Get()))
			for _, x := range in {
				if glob.MatchString(x) {
					output(x)
				}
			}
			pbar.Inc()
		}
	} else {
		pbar := LogProgress(0, len(in), "filter-completion")
		defer pbar.Close()

		for _, x := range in {
			output(x)
			pbar.Inc()
		}
	}
}

var ListArtifacts = NewCommand(
	"Metadata",
	"list-artifacts",
	"list all known artifacts",
	OptionCommandCompletionArgs(),
	OptionCommandRun(func(cc CommandContext) error {
		bg := CommandEnv.BuildGraph()
		args := GetCompletionArgs()
		return openCompletion(args, func(w io.Writer) error {
			filterCompletion(args, func(s string) {
				a := BuildAlias(s)
				node := bg.Find(a)

				WithoutLog(func() {
					fmt.Fprintf(w, "%v --> %v (%T)\n", node.GetBuildStamp(), a, node.GetBuildable())
				})
			}, Stringize(bg.Aliases()...)...)
			return nil
		})
	}))

var ListCommands = NewCommand(
	"Metadata",
	"list-commands",
	"list all available commands",
	OptionCommandCompletionArgs(),
	OptionCommandRun(func(cc CommandContext) error {
		return printCompletion(GetCompletionArgs(), Commands.Keys())
	}))

var ListPlatforms = NewCommand(
	"Metadata",
	"list-platforms",
	"list all available platforms",
	OptionCommandCompletionArgs(),
	OptionCommandRun(func(cc CommandContext) error {
		return printCompletion(GetCompletionArgs(), compile.AllPlatforms.Keys())
	}))

var ListConfigs = NewCommand(
	"Metadata",
	"list-configs",
	"list all available configurations",
	OptionCommandCompletionArgs(),
	OptionCommandRun(func(cc CommandContext) error {
		return printCompletion(GetCompletionArgs(), compile.AllConfigurations.Keys())
	}))

var ListCompilers = NewCommand(
	"Metadata",
	"list-compilers",
	"list all available compilers",
	OptionCommandCompletionArgs(),
	OptionCommandRun(func(cc CommandContext) error {
		return printCompletion(GetCompletionArgs(), compile.AllCompilers.Slice())
	}))

var ListModules = NewCommand(
	"Metadata",
	"list-modules",
	"list all available modules",
	OptionCommandCompletionArgs(),
	OptionCommandRun(func(cc CommandContext) error {
		bc := CommandEnv.BuildGraph().GlobalContext()
		if moduleAliases, err := compile.NeedAllModuleAliases(bc); err == nil {
			return printCompletion(GetCompletionArgs(), Stringize(moduleAliases...))
		} else {
			return err
		}
	}))

var ListNamespaces = NewCommand(
	"Metadata",
	"list-namespaces",
	"list all available namespaces",
	OptionCommandCompletionArgs(),
	OptionCommandRun(func(cc CommandContext) error {
		bc := CommandEnv.BuildGraph().GlobalContext()
		moduleAliases, err := compile.NeedAllModuleAliases(bc)
		if err != nil {
			return err
		}

		namespaceAliases := NewSet[compile.NamespaceAlias]()
		for _, moduleAlias := range moduleAliases {
			namespaceAliases.AppendUniq(moduleAlias.NamespaceAlias)
		}

		return printCompletion(GetCompletionArgs(), Stringize(namespaceAliases...))
	}))

var ListEnvironments = NewCommand(
	"Metadata",
	"list-environments",
	"list all compilation environments",
	OptionCommandCompletionArgs(),
	OptionCommandRun(func(cc CommandContext) error {
		return printCompletion(GetCompletionArgs(), Stringize(compile.GetEnvironmentAliases()...))
	}))

var ListTargets = NewCommand(
	"Metadata",
	"list-targets",
	"list all translated targets",
	OptionCommandCompletionArgs(),
	OptionCommandRun(func(cc CommandContext) error {
		units, err := compile.NeedAllBuildUnits(CommandEnv.BuildGraph().GlobalContext())
		if err != nil {
			return err
		}
		aliases := Map(func(u *compile.Unit) string { return u.TargetAlias.String() }, units...)
		return printCompletion(GetCompletionArgs(), aliases)
	}))

var ListPrograms = NewCommand(
	"Metadata",
	"list-programs",
	"list all executable targets",
	OptionCommandCompletionArgs(),
	OptionCommandRun(func(cc CommandContext) error {
		units, err := compile.NeedAllBuildUnits(CommandEnv.BuildGraph().GlobalContext())
		if err != nil {
			return err
		}

		executables := RemoveUnless(func(unit *compile.Unit) bool {
			return unit.Payload == compile.PAYLOAD_EXECUTABLE
		}, units...)

		aliases := Map(func(u *compile.Unit) string { return u.TargetAlias.String() }, executables...)
		return printCompletion(GetCompletionArgs(), aliases)
	}))

var ListPersistentData = NewCommand(
	"Metadata",
	"list-persistents",
	"list all persistent data",
	OptionCommandCompletionArgs(),
	OptionCommandRun(func(cc CommandContext) error {
		args := GetCompletionArgs()
		data := CommandEnv.Persistent().PinData()
		return openCompletion(args, func(w io.Writer) error {
			mapCompletion(args, func(s string) {
				WithoutLog(func() {
					fmt.Printf("%v=%v\n", s, data[s])
				})
			}, data)
			return nil
		})
	}))

var ListModifiedFiles = NewCommand(
	"Metadata",
	"list-modified-files",
	"list modified files from source control",
	OptionCommandCompletionArgs(),
	OptionCommandRun(func(cc CommandContext) error {
		bg := CommandEnv.BuildGraph()
		result := BuildSourceControlModifiedFiles(UFS.Source).Build(bg)
		return printCompletion(GetCompletionArgs(), Stringize(result.Success().ModifiedFiles...))
	}))
