package cmd

import (

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/compile"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var CommandCompileDb = NewCommand(
	"Configure",
	"compiledb",
	"generate json compilation database",
	OptionCommandAllCompilationFlags(),
	OptionCommandRun(func(cc CommandContext) error {
		LogClaim(LogCommand, "generation json compilation database in %q", UFS.Intermediate)

		bg := CommandEnv.BuildGraph()

		for _, future := range Map(func(ea EnvironmentAlias) Future[*CompilationDatabaseBuilder] {
			return BuildCompilationDatabase(ea).Prepare(bg)
		}, GetEnvironmentAliases()...) {
			result := future.Join()
			if err := result.Failure(); err != nil {
				return err
			}
		}

		return nil
	}),
)

// #TODO: add `importdb` command to import compilation database as build actions
