package cmd

import (
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/compile"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var LogRun = NewLogCategory("Run")

type RunCommand struct {
	Program    TargetAlias
	Arguments  []StringVar
	Build      BoolVar
	Debug      BoolVar
	ShowOutput BoolVar
}

var CommandRun = NewCommandable(
	"Compilation",
	"run",
	"launch compiled program",
	&RunCommand{
		Debug: INHERITABLE_FALSE,
	})

func (x *RunCommand) Flags(cfv CommandFlagsVisitor) {
	cfv.Variable("Build", "build the program before running it", &x.Build)
	cfv.Variable("Debug", "attach a debugger to the program", &x.Debug)
	cfv.Variable("ShowOutput", "capture output of the program", &x.ShowOutput)
}
func (x *RunCommand) Init(ci CommandContext) error {
	ci.Options(
		OptionCommandParsableFlags("RunCommand", "control compilation actions execution", x),
		OptionCommandAllCompilationFlags(),
		OptionCommandConsumeArg("TargetAlias", "build and execute specified target", &x.Program),
		OptionCommandConsumeMany("Arguments", "pass given arguments to the program", &x.Arguments, COMMANDARG_OPTIONAL),
	)
	return nil
}
func (x *RunCommand) Run(cc CommandContext) error {
	LogClaim(LogCommand, "run <%v>...", x.Program)

	bg := CommandEnv.BuildGraph()

	// make sure selected program actions are generated
	_, asyncGenerate := bg.Build(&x.Program)
	if err := asyncGenerate.Join().Failure(); err != nil {
		return err
	}
	unit := asyncGenerate.Join().Success().Buildable.(*Unit)

	// make sure selected executable is actually built (and up-to-date)
	if x.Build.Get() {
		if x.ShowOutput.Get() {
			LogVeryVerbose(LogRun, "building program")
		}

		_, asyncBuild := bg.Build(&unit.OutputFile)
		if err := asyncBuild.Join().Failure(); err != nil {
			return err
		}
	}

	if x.Debug.Get() {
		LogVeryVerbose(LogRun, "attaching debugger")
	}
	if x.ShowOutput.Get() {
		LogVeryVerbose(LogRun, "capturing output")
	}

	return RunProcess(unit.OutputFile, Stringize(x.Arguments...),
		OptionProcessAttachDebuggerIf(x.Debug.Get()),
		OptionProcessCaptureOutputIf(x.ShowOutput.Get()),
		OptionProcessWorkingDir(UFS.Binaries))
}
