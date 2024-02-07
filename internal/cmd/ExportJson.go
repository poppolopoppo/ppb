package cmd

import (
	"io"

	compile "github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

func completeJsonExport[T base.Comparable[T], P completionArgsTraits[T], OUTPUT any](cmd *utils.CommandEnvT, args *CompletionArgs[T, P], factory func(T) (OUTPUT, error), inputs ...T) error {
	return openCompletion(args, func(w io.Writer) error {
		return filterCompletion(args, func(it T) error {
			if output, err := factory(it); err == nil {
				return base.JsonSerialize(output, w, base.OptionJsonPrettyPrint(true))
			} else {
				return err
			}
		}, inputs...)
	})
}

var ExportConfig = newCompletionCommand[compile.ConfigurationAlias, *compile.ConfigurationAlias](
	"Export",
	"export-config",
	"export configuration to json",
	func(cc utils.CommandContext, ca *CompletionArgs[compile.ConfigurationAlias, *compile.ConfigurationAlias]) error {
		return completeJsonExport(utils.CommandEnv, ca, func(alias compile.ConfigurationAlias) (compile.Configuration, error) {
			result := compile.GetBuildConfig(alias).Build(utils.CommandEnv.BuildGraph())

			if err := result.Failure(); err == nil {
				return result.Success(), nil
			} else {
				return nil, err
			}
		}, compile.GetAllConfigurationAliases()...)
	})

var ExportPlatform = newCompletionCommand[compile.PlatformAlias, *compile.PlatformAlias](
	"Export",
	"export-platform",
	"export platform to json",
	func(cc utils.CommandContext, ca *CompletionArgs[compile.PlatformAlias, *compile.PlatformAlias]) error {
		return completeJsonExport(utils.CommandEnv, ca, func(alias compile.PlatformAlias) (compile.Platform, error) {
			result := compile.GetBuildPlatform(alias).Build(utils.CommandEnv.BuildGraph())

			if err := result.Failure(); err == nil {
				return result.Success(), nil
			} else {
				return nil, err
			}
		}, compile.GetAllPlatformAliases()...)
	})

var ExportModule = newCompletionCommand[compile.ModuleAlias, *compile.ModuleAlias](
	"Export",
	"export-module",
	"export parsed module to json",
	func(cc utils.CommandContext, ca *CompletionArgs[compile.ModuleAlias, *compile.ModuleAlias]) error {
		bc := utils.CommandEnv.BuildGraph().GlobalContext()
		moduleAliases, err := compile.NeedAllModuleAliases(bc)
		if err != nil {
			return err
		}

		return completeJsonExport(utils.CommandEnv, ca, func(moduleAlias compile.ModuleAlias) (compile.Module, error) {
			return compile.FindBuildModule(moduleAlias)
		}, moduleAliases...)
	})

var ExportNamespace = newCompletionCommand[compile.NamespaceAlias, *compile.NamespaceAlias](
	"Export",
	"export-namespace",
	"export parsed namespace to json",
	func(cc utils.CommandContext, ca *CompletionArgs[compile.NamespaceAlias, *compile.NamespaceAlias]) error {
		bc := utils.CommandEnv.BuildGraph().GlobalContext()
		namespaceAliases, err := compile.NeedAllBuildNamespaceAliases(bc)
		if err != nil {
			return err
		}

		return completeJsonExport(utils.CommandEnv, ca, func(na compile.NamespaceAlias) (compile.Namespace, error) {
			return compile.FindBuildNamespace(na)
		}, namespaceAliases...)
	})

var ExportNode = newCompletionCommand[utils.BuildAlias, *utils.BuildAlias](
	"Export",
	"export-node",
	"export build node to json",
	func(cc utils.CommandContext, ca *CompletionArgs[utils.BuildAlias, *utils.BuildAlias]) error {
		bg := utils.CommandEnv.BuildGraph()
		return completeJsonExport(utils.CommandEnv, ca, func(ba utils.BuildAlias) (utils.BuildNode, error) {
			return bg.Expect(ba)
		}, utils.CommandEnv.BuildGraph().Aliases()...)
	})

var ExportUnit = newCompletionCommand[compile.TargetAlias, *compile.TargetAlias](
	"Export",
	"export-unit",
	"export translated unit to json",
	func(cc utils.CommandContext, ca *CompletionArgs[compile.TargetAlias, *compile.TargetAlias]) error {
		unitAliases, err := compile.NeedAllUnitAliases(utils.CommandEnv.BuildGraph().GlobalContext())
		if err != nil {
			return err
		}

		return completeJsonExport(utils.CommandEnv, ca, func(ta compile.TargetAlias) (utils.Buildable, error) {
			return compile.FindBuildUnit(ta)
		}, unitAliases...)
	})
