package cmd

import (
	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var CommandCompileDb = NewCommand(
	"Configure",
	"compiledb",
	"generate json compilation database",
	compile.OptionCommandAllCompilationFlags(),
	OptionCommandRun(func(cc CommandContext) error {
		base.LogClaim(LogCommand, "generation json compilation database in %q", UFS.Intermediate)

		bg := CommandEnv.BuildGraph()

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
