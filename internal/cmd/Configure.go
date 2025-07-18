package cmd

import (
	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

var CommandConfigure = utils.NewCommand(
	"Configure",
	"configure",
	"parse project configuration files and prepare build graph",
	compile.OptionCommandAllCompilationFlags(),
	utils.OptionCommandRun(func(cc utils.CommandContext) error {
		base.LogClaim(utils.LogCommand, "configure compilation graph with %q as root", utils.UFS.Root)

		bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "Configure"})
		defer bg.Close()

		_, err := compile.NeedAllTargetActions(bg.GlobalContext())
		return err
	}),
)
