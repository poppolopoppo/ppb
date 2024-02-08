package cmd

import (
	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
)

var LogRun = base.NewLogCategory("Run")

type RunCommand struct {
	Program    compile.TargetAlias
	Arguments  []utils.StringVar
	Build      utils.BoolVar
	Debug      utils.BoolVar
	ShowOutput utils.BoolVar
}

var CommandRun = utils.NewCommandable(
	"Compilation",
	"run",
	"launch compiled program",
	&RunCommand{
		Debug: base.INHERITABLE_FALSE,
	})

func (x *RunCommand) Flags(cfv utils.CommandFlagsVisitor) {
	cfv.Variable("Build", "build the program before running it", &x.Build)
	cfv.Variable("Debug", "attach a debugger to the program", &x.Debug)
	cfv.Variable("ShowOutput", "capture output of the program", &x.ShowOutput)
}
func (x *RunCommand) Init(ci utils.CommandContext) error {
	ci.Options(
		utils.OptionCommandParsableFlags("RunCommand", "control compilation actions execution", x),
		compile.OptionCommandAllCompilationFlags(),
		utils.OptionCommandConsumeArg("TargetAlias", "build and execute specified target", &x.Program),
		utils.OptionCommandConsumeMany("Arguments", "pass given arguments to the program", &x.Arguments, utils.COMMANDARG_OPTIONAL),
	)
	return nil
}
func (x *RunCommand) Run(cc utils.CommandContext) error {
	base.LogClaim(utils.LogCommand, "run <%v>...", x.Program)

	bg := utils.CommandEnv.BuildGraph()

	// make sure selected program actions are generated
	_, asyncGenerate := bg.Build(&x.Program)
	if err := asyncGenerate.Join().Failure(); err != nil {
		return err
	}
	unit := asyncGenerate.Join().Success().Buildable.(*compile.Unit)

	// make sure selected executable is actually built (and up-to-date)
	if x.Build.Get() {
		if x.ShowOutput.Get() {
			base.LogVeryVerbose(LogRun, "building program")
		}

		_, asyncBuild := bg.Build(&unit.OutputFile)
		if err := asyncBuild.Join().Failure(); err != nil {
			return err
		}
	}

	if x.Debug.Get() {
		base.LogVeryVerbose(LogRun, "attaching debugger")
	}
	if x.ShowOutput.Get() {
		base.LogVeryVerbose(LogRun, "capturing output")
	}

	return internal_io.RunProcess(unit.OutputFile, base.MakeStringerSet(x.Arguments...),
		internal_io.OptionProcessAttachDebuggerIf(x.Debug.Get()),
		internal_io.OptionProcessCaptureOutputIf(x.ShowOutput.Get()),
		internal_io.OptionProcessWorkingDir(utils.UFS.Binaries))
}
