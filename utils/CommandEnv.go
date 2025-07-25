package utils

import (
	"context"
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
	Color          base.AnsiColorMode
	Ide            BoolVar
	LogAll         base.LogCategorySet
	LogMute        base.LogCategorySet
	LogImmediate   BoolVar
	LogFile        Filename
	OutputDir      Directory
	RootDir        Directory
	StopOnError    BoolVar
	Summary        BoolVar
	WarningAsError BoolVar
	ErrorAsPanic   BoolVar
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
	Jobs:           base.InheritableInt(base.INHERIT_VALUE),
	Color:          base.ANSICOLOR_INHERIT,
	Ide:            base.INHERITABLE_INHERIT,
	Timestamp:      base.INHERITABLE_FALSE,
	StopOnError:    base.INHERITABLE_FALSE,
	Summary:        base.INHERITABLE_FALSE,
	WarningAsError: base.INHERITABLE_FALSE,
	ErrorAsPanic:   base.INHERITABLE_FALSE,
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
	cfv.Variable("LogMute", "force mute all messages for given log categories", &flags.LogMute)
	cfv.Variable("LogImmediate", "disable buffering of log messages", &flags.LogImmediate)
	cfv.Variable("LogFile", "output log to specified file (default: stdout)", &flags.LogFile)
	cfv.Variable("OutputDir", "override default output directory", &flags.OutputDir)
	cfv.Variable("RootDir", "override root directory", &flags.RootDir)
	cfv.Variable("StopOnError", "interrupt build process immediately when an error occurred", &flags.StopOnError)
	cfv.Variable("Summary", "print build graph execution summary when build finished", &flags.Summary)
	cfv.Variable("WX", "consider warnings as errors", &flags.WarningAsError)
	cfv.Variable("EX", "consider errors as panics", &flags.ErrorAsPanic)
}
func (flags *CommandFlags) Apply() error {
	for _, category := range flags.LogAll {
		if err := base.GetLogManager().SetCategoryLevel(category, base.LOG_ALL); err != nil {
			return err
		}
	}
	for _, category := range flags.LogMute {
		if err := base.GetLogManager().SetCategoryLevel(category, base.LOG_ERROR); err != nil {
			return err
		}
	}

	if flags.LogImmediate.Get() {
		base.SetLogger(base.NewLogger(true))
	}

	if flags.LogFile.Valid() {
		if outp, err := UFS.CreateWriter(flags.LogFile); err == nil {
			base.SetAnsiColorMode(base.ANSICOLOR_DISABLED)
			base.SetEnableInteractiveShell(false)
			base.GetLogger().SetWriter(outp)
		} else {
			return err
		}
	}

	base.SetEnableDiagnostics(flags.Diagnostics.Get())
	base.GetLogger().SetShowTimestamp(flags.Timestamp.Get())

	if flags.Ide.Get() {
		base.SetAnsiColorMode(base.ANSICOLOR_DISABLED)
		base.SetEnableInteractiveShell(false)
		flags.StopOnError.Enable()
	}

	if flags.Debug.Get() {
		base.SetLogVisibleLevel(base.LOG_DEBUG)
		base.SetEnableDiagnostics(true)
	}

	if !flags.Color.IsInheritable() {
		base.SetAnsiColorMode(flags.Color)
	}

	if flags.Verbose.Get() {
		base.SetLogVisibleLevel(base.LOG_VERBOSE)
	}
	if flags.Trace.Get() {
		base.SetLogVisibleLevel(base.LOG_TRACE)
	}
	if flags.VeryVerbose.Get() {
		base.SetLogVisibleLevel(base.LOG_VERYVERBOSE)
	}
	if flags.Quiet.Get() {
		base.SetLogVisibleLevel(base.LOG_ERROR)
	}
	if flags.WarningAsError.Get() {
		base.SetLogWarningAsError(true)
	}
	if flags.ErrorAsPanic.Get() {
		base.SetLogErrorAsPanic(true)
	}

	if flags.Purge.Get() {
		base.LogVeryVerbose(LogCommand, "build will be forced due to '-F' command-line option")
		flags.Force.Enable()
	}
	if flags.Force.Get() {
		base.LogVeryVerbose(LogCommand, "build will be forced due to '-f' command-line option")
	}

	if flags.Summary.Get() || (flags.Ide.Get() && !flags.Quiet.Get()) {
		var buildSummaries []BuildSummary
		CommandEnv.OnExit(func(cet *CommandEnvT) error {
			base.PurgePinnedLogs()

			// ide mode only prints execution time as a feedback for process termination
			logLevel := base.LOG_CLAIM
			if flags.Summary.Get() {
				// queue print summary if specified on command-line
				logLevel = base.LOG_ALL
			}

			for _, it := range buildSummaries {
				it.PrintSummary(logLevel)
			}

			return nil
		})
		CommandEnv.OnBuildGraphLoaded(func(bg BuildGraph) error {
			var startedAt time.Time
			bg.OnBuildGraphStart(func(_ BuildGraphWritePort) error {
				startedAt = time.Now()
				return nil
			})
			bg.OnBuildGraphFinished(func(port BuildGraphWritePort) error {
				if !port.PortFlags().Any(BUILDGRAPH_QUIET) {
					buildSummaries = append(buildSummaries, port.RecordSummary(startedAt))
				}
				return nil
			})
			return nil
		})
	}

	if flags.RootDir.Valid() {
		if err := UFS.MountRootDirectory(flags.RootDir); err != nil {
			return err
		}
	}

	if flags.OutputDir.Valid() {
		if err := UFS.MountOutputDir(flags.OutputDir); err != nil {
			return err
		}
	}

	if !flags.Jobs.IsInheritable() && flags.Jobs.Get() > 0 {
		base.LogVeryVerbose(LogCommand, "limit concurrency to %d simultaneous jobs", flags.Jobs.Get())
		base.GetGlobalThreadPool().Resize(flags.Jobs.Get())
	}

	return nil
}

/***************************************
 * Command Env
 ***************************************/

type CommandEnvT struct {
	prefix     string
	buildGraph GlobalBuildGraph
	persistent *persistentData
	startedAt  time.Time

	configPath   Filename
	databasePath Filename

	onExit base.ConcurrentEvent[*CommandEnvT]

	commandEvents CommandEvents
	commandLines  []CommandLine

	lastPanic atomic.Pointer[error]
}

var CommandEnv *CommandEnvT

func InitCommandEnv(prefix string, args []string, startedAt time.Time) (*CommandEnvT, error) {
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
			return nil, err
		}
		if cl.Empty() { // remove empty command-lines
			CommandEnv.commandLines = append(CommandEnv.commandLines[0:i], CommandEnv.commandLines[i+1:]...)
		}
	}
	CommandEnv.commandEvents.Add(&GlobalParsableFlags)

	// apply global command flags early-on
	if err := GetCommandFlags().Apply(); err != nil {
		return nil, err
	}

	// use UFS.Output only after having parsed -OutputDir/RootDir= flags
	CommandEnv.configPath = UFS.Output.File(fmt.Sprint(".", prefix, "-config.json"))
	CommandEnv.databasePath = UFS.Output.File(fmt.Sprint(".", prefix, "-cache.db"))

	base.LogVerbose(LogCommand, "will load config from %q", CommandEnv.configPath)
	base.LogVerbose(LogCommand, "will load database from %q", CommandEnv.databasePath)

	if GetCommandFlags().Summary.Get() {
		CommandEnv.onExit.Add(func(*CommandEnvT) error {
			return FileInfos.PrintStats(base.GetLogger())
		})
	}

	// creates a 'listener' on a new goroutine which will notify the
	// program if it receives an interrupt from the OS. We then handle this by calling
	// our clean up procedure and exiting the program.
	go CommandEnv.interuptHandler()

	return CommandEnv, nil
}

func (env *CommandEnvT) interuptHandler() {
	const maxBeforePanic = 3
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	for i := 0; i < maxBeforePanic; i++ {
		<-c

		err := fmt.Errorf("Ctrl+C pressed in Terminal, aborting (%d/%d)", i+1, maxBeforePanic)
		// intercepting the event allows to die gracefully by waiting running jobs
		base.LogWarning(LogUtils, "\r- %v", err)
		// child processes also received the signal, and we rely on them dying to quit the program instead of calling os.Exit(0) here
		env.Abort(err)
	}

	CommandPanicF("Ctrl+C pressed %d times in Terminal, panic", maxBeforePanic)
}

func (env *CommandEnvT) Prefix() string             { return env.prefix }
func (env *CommandEnvT) BuildGraph() BuildGraph     { return env.buildGraph.Get(env) }
func (env *CommandEnvT) Persistent() PersistentData { return env.persistent }
func (env *CommandEnvT) ConfigPath() Filename       { return env.configPath }
func (env *CommandEnvT) DatabasePath() Filename     { return env.databasePath }
func (env *CommandEnvT) StartedAt() time.Time       { return env.startedAt }
func (env *CommandEnvT) BuildTime() time.Time       { return GetProcessInfo().Timestamp }

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
	if base.IsNil(err) {
		return base.PANIC_HANDLED
	}
	if env.lastPanic.CompareAndSwap(nil, &err) {
		env.commandEvents.OnPanic.Invoke(err)
		return base.PANIC_ABORT
	}
	return base.PANIC_REENTRANCY // a fatal error was already reported
}

func (env *CommandEnvT) Abort(err error) {
	env.buildGraph.Abort(err)
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

	return UFS.OpenBuffered(context.TODO(), env.configPath, env.persistent.Deserialize)
}
func (env *CommandEnvT) saveConfig() error {
	if !env.persistent.Dirty() {
		base.LogTrace(LogCommand, "skipped saving unmodified config")
		return nil
	}
	benchmark := base.LogBenchmark(LogCommand, "saving config to '%v'...", env.configPath)
	defer benchmark.Close()

	return UFS.Create(env.configPath, env.persistent.Serialize)
}
