package utils

import (
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
)

/***************************************
 * Command Flags
 ***************************************/

type CommandFlags struct {
	Force          BoolVar
	Purge          BoolVar
	Quiet          BoolVar
	Verbose        BoolVar
	Trace          BoolVar
	VeryVerbose    BoolVar
	Debug          BoolVar
	Timestamp      BoolVar
	Diagnostics    BoolVar
	Jobs           IntVar
	Color          BoolVar
	Ide            BoolVar
	LogAll         base.StringSet
	LogImmediate   BoolVar
	LogFile        Filename
	OutputDir      Directory
	RootDir        Directory
	Summary        BoolVar
	WarningAsError BoolVar
}

var GetCommandFlags = NewGlobalCommandParsableFlags("global command options", &CommandFlags{
	Force:          base.INHERITABLE_FALSE,
	Purge:          base.INHERITABLE_FALSE,
	Quiet:          base.INHERITABLE_FALSE,
	Verbose:        base.INHERITABLE_FALSE,
	Trace:          base.INHERITABLE_FALSE,
	VeryVerbose:    base.INHERITABLE_FALSE,
	Debug:          base.MakeBoolVar(base.DEBUG_ENABLED),
	Diagnostics:    base.MakeBoolVar(base.DEBUG_ENABLED),
	Jobs:           base.INHERIT_VALUE,
	Color:          base.INHERITABLE_INHERIT,
	Ide:            base.INHERITABLE_FALSE,
	Timestamp:      base.INHERITABLE_FALSE,
	Summary:        base.INHERITABLE_FALSE,
	WarningAsError: base.INHERITABLE_FALSE,
})

func (flags *CommandFlags) Flags(cfv CommandFlagsVisitor) {
	cfv.Variable("f", "force build even if up-to-date", &flags.Force)
	cfv.Variable("F", "force build and ignore cache", &flags.Purge)
	cfv.Variable("j", "override number of worker threads (default: numCpu-1)", &flags.Jobs)
	cfv.Variable("q", "disable all messages", &flags.Quiet)
	cfv.Variable("v", "turn on verbose mode", &flags.Verbose)
	cfv.Variable("t", "print more informations about progress", &flags.Trace)
	cfv.Variable("V", "turn on very verbose mode", &flags.VeryVerbose)
	if base.DEBUG_ENABLED {
		cfv.Variable("d", "turn on debug assertions and more log", &flags.Debug)
	}
	cfv.Variable("T", "turn on timestamp logging", &flags.Timestamp)
	cfv.Variable("X", "turn on diagnostics mode", &flags.Diagnostics)
	cfv.Variable("Color", "control ansi color output in log messages", &flags.Color)
	cfv.Variable("Ide", "set output to IDE mode (disable interactive shell)", &flags.Ide)
	cfv.Variable("LogAll", "force to output all messages for given log categories", &flags.LogAll)
	cfv.Variable("LogImmediate", "disable buffering of log messages", &flags.LogImmediate)
	cfv.Variable("LogFile", "output log to specified file (default: stdout)", &flags.LogFile)
	cfv.Variable("OutputDir", "override default output directory", &flags.OutputDir)
	cfv.Variable("RootDir", "override root directory", &flags.RootDir)
	cfv.Variable("Summary", "print build graph execution summary when build finished", &flags.Summary)
	cfv.Variable("WX", "consider warnings as errors", &flags.WarningAsError)
}
func (flags *CommandFlags) Apply() {
	for _, category := range flags.LogAll {
		base.GetLogManager().SetCategoryLevel(category, base.LOG_ALL)
	}

	if flags.LogImmediate.Get() {
		base.SetLogger(base.NewLogger(true))
	}

	if flags.LogFile.Valid() {
		if outp, err := UFS.CreateWriter(flags.LogFile); err == nil {
			base.SetEnableInteractiveShell(false)
			base.GetLogger().SetWriter(outp)
		} else {
			base.LogPanicErr(LogCommand, err)
		}
	}

	base.SetEnableDiagnostics(flags.Diagnostics.Get())
	base.GetLogger().SetShowTimestamp(flags.Timestamp.Get())

	if flags.Ide.Get() {
		base.SetEnableAnsiColor(false)
		base.SetEnableInteractiveShell(false)
	}

	if flags.Debug.Get() {
		base.GetLogger().SetLevel(base.LOG_DEBUG)
		base.SetEnableDiagnostics(true)
	}

	if flags.Verbose.Get() {
		base.GetLogger().SetLevel(base.LOG_VERBOSE)
	}
	if flags.Trace.Get() {
		base.GetLogger().SetLevel(base.LOG_TRACE)
	}
	if flags.VeryVerbose.Get() {
		base.GetLogger().SetLevel(base.LOG_VERYVERBOSE)
	}
	if flags.Quiet.Get() {
		base.GetLogger().SetLevel(base.LOG_ERROR)
	}
	if flags.WarningAsError.Get() {
		base.GetLogger().SetWarningAsError(true)
	}

	if flags.Purge.Get() {
		base.LogTrace(LogCommand, "build will be forced due to '-F' command-line option")
		flags.Force.Enable()
	}
	if flags.Force.Get() {
		base.LogTrace(LogCommand, "fbuild will be forced due to '-f' command-line option")
	}

	if flags.Summary.Get() || (flags.Ide.Get() && !flags.Quiet.Get()) {
		CommandEnv.onExit.Add(func(cet *CommandEnvT) error {
			base.PurgePinnedLogs()
			if flags.Summary.Get() {
				// queue print summary if specified on command-line
				cet.buildGraph.PrintSummary(cet.startedAt, base.LOG_ALL)
			} else {
				// ide mode only prints execution time as a feedback for process termination
				cet.buildGraph.PrintSummary(cet.startedAt, base.LOG_CLAIM)
			}
			return nil
		})
	}

	if !flags.Color.IsInheritable() {
		base.SetEnableAnsiColor(flags.Color.Get())
	}

	if flags.RootDir.Valid() {
		base.LogPanicIfFailed(LogCommand, UFS.MountRootDirectory(flags.RootDir))
	}

	if flags.OutputDir.Valid() {
		UFS.MountOutputDir(flags.OutputDir)
	}

	if !flags.Jobs.IsInheritable() && flags.Jobs.Get() > 0 {
		base.GetGlobalThreadPool().Resize(flags.Jobs.Get())
	}
}

/***************************************
 * Command Env
 ***************************************/

type CommandEnvT struct {
	prefix     string
	buildGraph GlobalBuildGraph
	persistent *persistentData
	rootFile   Filename
	startedAt  time.Time

	configPath   Filename
	databasePath Filename

	onExit base.ConcurrentEvent[*CommandEnvT]

	commandEvents CommandEvents
	commandLines  []CommandLine

	lastPanic atomic.Value
}

var CommandEnv *CommandEnvT

func InitCommandEnv(prefix string, args []string, startedAt time.Time) *CommandEnvT {
	CommandEnv = &CommandEnvT{
		prefix:     prefix,
		persistent: NewPersistentMap(prefix),
		startedAt:  startedAt,
	}

	base.OnPanic = CommandEnv.OnPanic

	CommandEnv.commandLines = NewCommandLine(CommandEnv.persistent, args)

	// parse global flags early-on
	for i, cl := range CommandEnv.commandLines {
		if err := GlobalParsableFlags.Parse(cl); err != nil {
			base.LogPanicIfFailed(LogCommand, err)
		}
		if cl.Empty() { // remove empty command-lines
			CommandEnv.commandLines = append(CommandEnv.commandLines[0:i], CommandEnv.commandLines[i+1:]...)
		}
	}
	CommandEnv.commandEvents.Add(&GlobalParsableFlags)

	// apply global command flags early-on
	GetCommandFlags().Apply()

	// use UFS.Output only after having parsed -OutputDir/RootDir= flags
	CommandEnv.configPath = UFS.Output.File(fmt.Sprint(".", prefix, "-config.json"))
	CommandEnv.databasePath = UFS.Output.File(fmt.Sprint(".", prefix, "-cache.db"))
	CommandEnv.rootFile = UFS.Source.File(prefix + "-namespace.json")

	base.LogVerbose(LogCommand, "will load config from %q", CommandEnv.configPath)
	base.LogVerbose(LogCommand, "will load database from %q", CommandEnv.databasePath)
	base.LogVerbose(LogCommand, "will load modules from %q", CommandEnv.rootFile)

	CommandEnv.onExit.Add(func(*CommandEnvT) error {
		return FileInfos.PrintStats(base.GetLogger())
	})

	// creates a 'listener' on a new goroutine which will notify the
	// program if it receives an interrupt from the OS. We then handle this by calling
	// our clean up procedure and exiting the program.
	go func() {
		const maxBeforePanic = 5
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		for i := 0; i < maxBeforePanic; i++ {
			<-c

			// intercepting the event allows to die gracefully by waiting running jobs
			base.LogWarning(LogUtils, "\r- Ctrl+C pressed in Terminal, aborting (%d/%d)", i+1, maxBeforePanic)
			// child processes also received the signal, and we rely on them dying to quit the program instead of calling os.Exit(0) here
			CommandEnv.Abort()
		}

		CommandPanicF("Ctrl+C pressed %d times in Terminal, panic", maxBeforePanic)
	}()

	return CommandEnv
}
func (env *CommandEnvT) Prefix() string             { return env.prefix }
func (env *CommandEnvT) BuildGraph() BuildGraph     { return env.buildGraph.Get(env) }
func (env *CommandEnvT) Persistent() PersistentData { return env.persistent }
func (env *CommandEnvT) ConfigPath() Filename       { return env.configPath }
func (env *CommandEnvT) DatabasePath() Filename     { return env.databasePath }
func (env *CommandEnvT) RootFile() Filename         { return env.rootFile }
func (env *CommandEnvT) StartedAt() time.Time       { return env.startedAt }
func (env *CommandEnvT) BuildTime() time.Time       { return GetProcessInfo().Timestamp }

func (env *CommandEnvT) SetRootFile(rootFile Filename) {
	base.LogVerbose(LogCommand, "set root file to %q", rootFile)
	env.rootFile = rootFile
}

func (env *CommandEnvT) OnBuildGraphLoaded(e base.EventDelegate[BuildGraph]) error {
	return env.buildGraph.OnBuildGraphLoaded(e)
}
func (env *CommandEnvT) OnBuildGraphSaved(e base.EventDelegate[BuildGraph]) error {
	return env.buildGraph.OnBuildGraphSaved(e)
}

func (env *CommandEnvT) OnExit(e base.EventDelegate[*CommandEnvT]) base.DelegateHandle {
	return env.onExit.Add(e)
}
func (env *CommandEnvT) RemoveOnExit(h base.DelegateHandle) bool {
	return env.onExit.Remove(h)
}

func CommandPanicF(msg string, args ...interface{}) {
	CommandPanic(fmt.Errorf(msg, args...))
}
func CommandPanic(err error) {
	base.Panic(err)
}

// don't save the db when panic occured
func (env *CommandEnvT) OnPanic(err error) base.PanicResult {
	if env.lastPanic.CompareAndSwap(nil, err) {
		env.commandEvents.OnPanic.Invoke(err)
		return base.PANIC_ABORT
	}
	return base.PANIC_REENTRANCY // a fatal error was already reported
}

func (env *CommandEnvT) Abort() {
	env.buildGraph.Abort()
}

func (env *CommandEnvT) Close() error {
	defer base.PurgePinnedLogs()
	return env.buildGraph.Close()
}

func (env *CommandEnvT) Run(defaults ...base.AnyDelegate) error {
	if err := env.loadConfig(); err != nil {
		base.LogWarning(LogCommand, "failed to load config: %v", err)
	}

	// prepare specified commands
	for _, cl := range env.commandLines {
		if err := env.commandEvents.Parse(cl); err != nil {
			return err
		}
	}

	// append input events only if user provider no command-line input
	if !env.commandEvents.OnRun.Bound() {
		for _, event := range defaults {
			env.commandEvents.OnRun.Add(event)
		}
	}

	defer func() {
		base.JoinAllThreadPools()
		env.onExit.FireAndForget(env)
	}()

	// check if any command was successfully parsed
	if !env.commandEvents.Bound() {
		base.LogWarning(LogCommand, "missing argument, use `help` to learn about command usage")
		return nil
	}

	err := env.commandEvents.Run()

	if er := env.buildGraph.Join(); er != nil && err == nil {
		err = er
	}
	if er := env.saveConfig(); er != nil && err == nil {
		err = er
	}
	if er := env.buildGraph.Save(env); er != nil && err == nil {
		err = er
	}
	return err
}

func (env *CommandEnvT) loadConfig() error {
	benchmark := base.LogBenchmark(LogCommand, "loading config from '%v'...", env.configPath)
	defer benchmark.Close()

	return UFS.OpenBuffered(env.configPath, env.persistent.Deserialize)
}
func (env *CommandEnvT) saveConfig() error {
	if !env.persistent.Dirty() {
		base.LogTrace(LogCommand, "skipped saving unmodified config")
		return nil
	}
	benchmark := base.LogBenchmark(LogCommand, "saving config to '%v'...", env.configPath)
	defer benchmark.Close()

	return UFS.SafeCreate(env.configPath, env.persistent.Serialize)
}
