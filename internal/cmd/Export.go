package cmd

import (
	"io"

	compile "github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

type ExportableNodeAlias interface {
	comparable
	utils.BuildAliasable
	utils.PersistentVar
}

type ExportNodeArgs[T comparable, E interface {
	*T
	ExportableNodeAlias
}] struct {
	Aliases base.SetT[T]
	Output  utils.Filename
	Minify  utils.BoolVar
}

func (x *ExportNodeArgs[T, E]) Flags(cfv utils.CommandFlagsVisitor) {
	cfv.Variable("Output", "optional output file", &x.Output)
	cfv.Variable("Minify", "produce mininal text output for exported nodes", &x.Minify)
}

func (x *ExportNodeArgs[T, E]) WithOutput(closure func(io.Writer) error) error {
	base.LogVerbose(utils.LogCommand, "export node aliases = %v", x.Aliases)
	if x.Output.Valid() {
		base.LogInfo(utils.LogCommand, "export node results to %q...", x.Output)
		return utils.UFS.CreateBuffered(x.Output, closure, base.TransientPage4KiB)
	} else {
		return closure(base.GetLogger())
	}
}

func newExportNodesCommand[T comparable, E interface {
	*T
	ExportableNodeAlias
}](category, name, description string, run func(utils.CommandContext, *ExportNodeArgs[T, E]) error) func() utils.CommandItem {
	args := new(ExportNodeArgs[T, E])
	return utils.NewCommand(category, name, description,
		utils.OptionCommandConsumeMany[T, E](base.GetTypenameT[T](), "select build nodes by their build aliases", args.Aliases.Ref()),
		utils.OptionCommandParsableAccessor("ExportNodeArgs", "options for commands exporting build graph nodes", func() *ExportNodeArgs[T, E] { return args }),
		utils.OptionCommandRun(func(cc utils.CommandContext) error {
			return run(cc, args)
		}))
}

type jsonExportYieldFunc = func(any) error

func newJsonExportCommand[T comparable, E interface {
	*T
	ExportableNodeAlias
}](category, name, description string, run func(utils.CommandContext, *ExportNodeArgs[T, E], jsonExportYieldFunc) error) func() utils.CommandItem {
	return newExportNodesCommand(category, name, description,
		func(cc utils.CommandContext, args *ExportNodeArgs[T, E]) error {
			var collected base.SetT[any]
			yield := func(it any) error {
				collected.Append(it)
				return nil
			}
			if err := run(cc, args, yield); err != nil {
				return err
			}

			return args.WithOutput(func(w io.Writer) error {
				return base.JsonSerialize(collected, w,
					base.OptionJsonPrettyPrint(!args.Minify.Get()))
			})
		})
}

var ExportConfig = newJsonExportCommand(
	"Export",
	"export-config",
	"export configuration to json",
	func(cc utils.CommandContext, args *ExportNodeArgs[compile.ConfigurationAlias, *compile.ConfigurationAlias], yield jsonExportYieldFunc) error {
		bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "ExportConfig"})
		defer bg.Close()

		for _, a := range args.Aliases {
			result := compile.GetBuildConfig(a).Build(bg)

			err := result.Failure()
			if err == nil {
				err = yield(result.Success())
			}
			if err != nil {
				return err
			}
		}
		return nil
	})

var ExportPlatform = newJsonExportCommand(
	"Export",
	"export-platform",
	"export platform to json",
	func(cc utils.CommandContext, args *ExportNodeArgs[compile.PlatformAlias, *compile.PlatformAlias], yield jsonExportYieldFunc) error {
		bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "ExportPlatform"})
		defer bg.Close()

		for _, a := range args.Aliases {
			result := compile.GetBuildPlatform(a).Build(bg)

			err := result.Failure()
			if err == nil {
				err = yield(result.Success())
			}
			if err != nil {
				return err
			}
		}
		return nil
	})

var ExportModule = newJsonExportCommand(
	"Export",
	"export-module",
	"export parsed module to json",
	func(cc utils.CommandContext, args *ExportNodeArgs[compile.ModuleAlias, *compile.ModuleAlias], yield jsonExportYieldFunc) error {
		bg := utils.CommandEnv.BuildGraph().OpenReadPort(base.ThreadPoolDebugId{Category: "ExportModule"})
		defer bg.Close()

		for _, a := range args.Aliases {
			module, err := compile.FindBuildModule(bg, a)
			if err == nil {
				err = yield(module)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})

var ExportNamespace = newJsonExportCommand(
	"Export",
	"export-namespace",
	"export parsed namespace to json",
	func(cc utils.CommandContext, args *ExportNodeArgs[compile.NamespaceAlias, *compile.NamespaceAlias], yield jsonExportYieldFunc) error {
		bg := utils.CommandEnv.BuildGraph().OpenReadPort(base.ThreadPoolDebugId{Category: "ExportNamespace"})
		defer bg.Close()

		for _, a := range args.Aliases {
			namespace, err := compile.FindBuildNamespace(bg, a)
			if err == nil {
				err = yield(namespace)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})

var ExportUnit = newJsonExportCommand(
	"Export",
	"export-unit",
	"export translated unit to json",
	func(cc utils.CommandContext, args *ExportNodeArgs[compile.TargetAlias, *compile.TargetAlias], yield jsonExportYieldFunc) error {
		bg := utils.CommandEnv.BuildGraph().OpenReadPort(base.ThreadPoolDebugId{Category: "ExportUnit"})
		defer bg.Close()

		for _, a := range args.Aliases {
			namespace, err := compile.FindBuildUnit(bg, a)
			if err == nil {
				err = yield(namespace)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})

var ExportNode = newJsonExportCommand(
	"Export",
	"export-node",
	"export build node to json",
	func(cc utils.CommandContext, args *ExportNodeArgs[utils.BuildAlias, *utils.BuildAlias], yield jsonExportYieldFunc) error {
		bg := utils.CommandEnv.BuildGraph().OpenReadPort(base.ThreadPoolDebugId{Category: "ExportNode"})
		defer bg.Close()

		for _, a := range args.Aliases {
			node, err := bg.Expect(a)
			if err == nil {
				err = yield(node)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
