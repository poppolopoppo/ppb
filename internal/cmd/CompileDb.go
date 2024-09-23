package cmd

import (
	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"

	"github.com/poppolopoppo/ppb/utils"
)

var CommandCompileDb = utils.NewCommand(
	"Configure",
	"compiledb",
	"generate json compilation database",
	compile.OptionCommandAllCompilationFlags(),
	utils.OptionCommandRun(func(cc utils.CommandContext) error {
		base.LogClaim(utils.LogCommand, "generation json compilation database in %q", utils.UFS.Intermediate)

		bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "Configure"})
		defer bg.Close()

		for _, future := range base.Map(func(ea compile.EnvironmentAlias) base.Future[*compile.CompilationDatabaseBuilder] {
			return compile.BuildCompilationDatabase(ea).Prepare(bg)
		}, compile.GetEnvironmentAliases()...) {
			result := future.Join()
			if err := result.Failure(); err != nil {
				return err
			}
		}

		return nil
	}),
)

// #TODO: add `importdb` command to import compilation database as build actions
