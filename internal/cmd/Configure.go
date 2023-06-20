package cmd

import (

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/compile"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var CommandConfigure = NewCommand(
	"Configure",
	"configure",
	"parse json input files and generate compilation graph",
	OptionCommandAllCompilationFlags(),
	OptionCommandRun(func(cc CommandContext) error {
		LogClaim(LogCommand, "configure compilation graph with %q as root", CommandEnv.RootFile())

		bg := CommandEnv.BuildGraph()
		_, err := NeedAllTargetActions(bg.GlobalContext())
		return err
	}),
)
