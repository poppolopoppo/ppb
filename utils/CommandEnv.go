package utils

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"time"
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
	LogAll         StringSet
	LogFile        Filename
	OutputDir      Directory
	RootDir        Directory
	Summary        BoolVar
	WarningAsError BoolVar
}

var GetCommandFlags = NewGlobalCommandParsableFlags("global command options", &CommandFlags{
	Force:          INHERITABLE_FALSE,
	Purge:          INHERITABLE_FALSE,
	Quiet:          INHERITABLE_FALSE,
	Verbose:        INHERITABLE_FALSE,
	Trace:          INHERITABLE_FALSE,
	VeryVerbose:    INHERITABLE_FALSE,
	Debug:          MakeBoolVar(DEBUG_ENABLED),
	Diagnostics:    MakeBoolVar(DEBUG_ENABLED),
	Jobs:           INHERIT_VALUE,
	Color:          INHERITABLE_INHERIT,
	Ide:            INHERITABLE_FALSE,
	Timestamp:      INHERITABLE_FALSE,
	Summary:        MakeBoolVar(DEBUG_ENABLED),
	WarningAsError: INHERITABLE_FALSE,
})

func (flags *CommandFlags) Flags(cfv CommandFlagsVisitor) {
	cfv.Variable("f", "force build even if up-to-date", &flags.Force)
	cfv.Variable("F", "force build and ignore cache", &flags.Purge)
	cfv.Variable("j", "override number for worker threads (default: numCpu-1)", &flags.Jobs)
	cfv.Variable("q", "disable all messages", &flags.Quiet)
	cfv.Variable("v", "turn on verbose mode", &flags.Verbose)
	cfv.Variable("t", "print more informations about progress", &flags.Trace)
	cfv.Variable("V", "turn on very verbose mode", &flags.VeryVerbose)
	cfv.Variable("d", "turn on debug assertions and more log", &flags.Debug)
	cfv.Variable("T", "turn on timestamp logging", &flags.Timestamp)
	cfv.Variable("X", "turn on diagnostics mode", &flags.Diagnostics)
	cfv.Variable("Color", "control ansi color output in log messages", &flags.Color)
	cfv.Variable("Ide", "set output to IDE mode (disable interactive shell)", &flags.Ide)
	cfv.Variable("LogAll", "force to output all messages for given log categories", &flags.LogAll)
	cfv.Variable("LogFile", "output log to specified file (default: stdout)", &flags.LogFile)
	cfv.Variable("OutputDir", "override default output directory", &flags.OutputDir)
	cfv.Variable("RootDir", "override root directory", &flags.RootDir)
	cfv.Variable("Summary", "print build graph execution summary when build finished", &flags.Summary)
	cfv.Variable("WX", "consider warnings as errors", &flags.WarningAsError)
}
func (flags *CommandFlags) Apply() {
	SetEnableDiagnostics(flags.Diagnostics.Get())
	gLogger.SetShowTimestamp(flags.Timestamp.Get())

	for _, category := range flags.LogAll {
		gLogManager.SetCategoryLevel(category, LOG_ALL)
	}

	if flags.LogFile.Valid() {
		if outp, err := UFS.CreateWriter(flags.LogFile); err == nil {
			SetEnableInteractiveShell(false)
			gLogger.SetWriter(outp)
		} else {
			LogPanicErr(LogCommand, err)
		}
	}

	if flags.Verbose.Get() {
		gLogger.SetLevel(LOG_VERBOSE)
	}
	if flags.Trace.Get() {
		gLogger.SetLevel(LOG_TRACE)
	}
	if flags.VeryVerbose.Get() {
		gLogger.SetLevel(LOG_VERYVERBOSE)
	}
	if flags.Quiet.Get() {
		gLogger.SetLevel(LOG_ERROR)
		SetEnableInteractiveShell(false)
	}
	if flags.Debug.Get() {
		gLogger.SetLevel(LOG_DEBUG)
		SetEnableDiagnostics(true)
	}
	if flags.WarningAsError.Get() {
		gLogger.SetWarningAsError(true)
	}

	if flags.Purge.Get() {
		LogTrace(LogCommand, "build will be forced due to '-F' command-line option")
		flags.Force.Enable()
	}
	if flags.Force.Get() {
		LogTrace(LogCommand, "fbuild will be forced due to '-f' command-line option")
	}

	// queue print summary if specified on command-line
	if flags.Summary.Get() {
		CommandEnv.onExit.Add(func(cet *CommandEnvT) error {
			PurgePinnedLogs()
			printBuildGraphSummary(cet.startedAt, cet.buildGraph)
			return nil
		})
	}

	if !flags.Color.IsInheritable() {
		SetEnableAnsiColor(flags.Color.Get())
	}

	if flags.Ide.Get() {
		gLogger.SetLevelMinimum(LOG_INFO)
		SetEnableAnsiColor(false)
		SetEnableInteractiveShell(false)
	}

	if flags.RootDir.Valid() {
		LogPanicIfFailed(LogCommand, UFS.MountRootDirectory(flags.RootDir))
	}

	if flags.OutputDir.Valid() {
		UFS.MountOutputDir(flags.OutputDir)
	}

	if !flags.Jobs.IsInheritable() && flags.Jobs.Get() > 0 {
		GetGlobalThreadPool().Resize(flags.Jobs.Get())
	}
}

func printBuildGraphSummary(startedAt time.Time, g BuildGraph) {
	totalDuration := time.Since(startedAt)
	LogForwardf("\nProgram took %.3f seconds to run", totalDuration.Seconds())

	stats := g.GetBuildStats()
	if stats.Count == 0 {
		return
	}

	LogForwardf("Took %.3f seconds to build %d nodes using %d threads (x%.2f)",
		stats.Duration.Exclusive.Seconds(), stats.Count, runtime.GOMAXPROCS(0),
		float32(stats.Duration.Exclusive)/float32(totalDuration))

	LogForwardf("\nMost expansive nodes built:")

	colorHot := Color3b{255, 128, 128}.Unquantize()
	colorCold := Color3b{128, 128, 128}.Unquantize()
	for i, node := range g.GetMostExpansiveNodes(10, false) {
		ns := node.GetBuildStats()
		fract := ns.Duration.Exclusive.Seconds() / stats.Duration.Exclusive.Seconds()

		sstep := Smootherstep(math.Sqrt(ns.Duration.Exclusive.Seconds() / totalDuration.Seconds())) // use percent of blocking duration

		rowColor := colorCold.Lerp(colorHot, sstep)
		rowColor = rowColor.Brightness(0.45 + 0.15*sstep)

		LogForwardf("%v[%02d] - %6.2f%% -  %6.3f  %6.3f  --  %s%v",
			rowColor.Quantize().Ansi(true),
			(i + 1),
			100.0*fract,
			ns.Duration.Exclusive.Seconds(),
			ns.Duration.Inclusive.Seconds(),
			node.Alias(),
			ANSI_RESET)
	}
}

/***************************************
 * Command Env
 ***************************************/

type CommandEnvT struct {
	prefix     string
	buildGraph BuildGraph
	persistent *persistentData
	rootFile   Filename
	startedAt  time.Time

	configPath   Filename
	databasePath Filename

	onExit ConcurrentEvent[*CommandEnvT]

	commandEvents CommandEvents
	commandLines  []CommandLine

	lastPanic error
}

var CommandEnv *CommandEnvT

func InitCommandEnv(prefix string, args []string, startedAt time.Time) *CommandEnvT {
	CommandEnv = &CommandEnvT{
		prefix:     prefix,
		persistent: NewPersistentMap(prefix),
		startedAt:  startedAt,
		lastPanic:  nil,
	}

	CommandEnv.commandLines = NewCommandLine(CommandEnv.persistent, args)

	// parse global flags early-on
	for _, cl := range CommandEnv.commandLines {
		LogPanicIfFailed(LogCommand, GlobalParsableFlags.Parse(cl))
	}

	// apply global command flags early-on
	GetCommandFlags().Apply()

	// use UFS.Output only after having parsed -OutputDir/RootDir= flags
	CommandEnv.configPath = UFS.Output.File(fmt.Sprint(".", prefix, "-config.json"))
	CommandEnv.databasePath = UFS.Output.File(fmt.Sprint(".", prefix, "-cache.db"))
	CommandEnv.rootFile = UFS.Source.File(prefix + "-namespace.json")

	LogVerbose(LogCommand, "load config from %q", CommandEnv.configPath)
	LogVerbose(LogCommand, "load database from %q", CommandEnv.databasePath)
	LogVerbose(LogCommand, "load modules from %q", CommandEnv.rootFile)

	// finally create the build graph (empty)
	CommandEnv.buildGraph = NewBuildGraph(GetCommandFlags())

	runtime.SetFinalizer(CommandEnv, func(env *CommandEnvT) {
		env.onExit.FireAndForget(env)
	})

	return CommandEnv
}
func (env *CommandEnvT) Prefix() string             { return env.prefix }
func (env *CommandEnvT) BuildGraph() BuildGraph     { return env.buildGraph }
func (env *CommandEnvT) Persistent() PersistentData { return env.persistent }
func (env *CommandEnvT) ConfigPath() Filename       { return env.configPath }
func (env *CommandEnvT) DatabasePath() Filename     { return env.databasePath }
func (env *CommandEnvT) RootFile() Filename         { return env.rootFile }
func (env *CommandEnvT) StartedAt() time.Time       { return env.startedAt }
func (env *CommandEnvT) BuildTime() time.Time       { return PROCESS_INFO.Timestamp }

func (env *CommandEnvT) SetRootFile(rootFile Filename) {
	LogVerbose(LogCommand, "set root file to %q", rootFile)
	env.rootFile = rootFile
}

func (env *CommandEnvT) OnExit(e EventDelegate[*CommandEnvT]) DelegateHandle {
	return env.onExit.Add(e)
}
func (env *CommandEnvT) RemoveOnExit(h DelegateHandle) bool {
	return env.onExit.Remove(h)
}

func CommandPanicF(msg string, args ...interface{}) {
	CommandPanic(fmt.Errorf(msg, args...))
}
func CommandPanic(err error) {
	if CommandEnv == nil || CommandEnv.OnPanic(err) {
		panic(fmt.Errorf("%v%v%v[PANIC] %v%v",
			ANSI_FG1_RED, ANSI_BG1_WHITE, ANSI_BLINK0, err, ANSI_RESET))
	} else {
		panic(fmt.Errorf("panic reentrancy: %v", err))
	}
}

// don't save the db when panic occured
func (env *CommandEnvT) OnPanic(err error) bool {
	if env.lastPanic == nil {

		env.lastPanic = err
		env.commandEvents.OnPanic.Invoke(err)
		return true
	}
	return false // a fatal error was already reported
}

func (env *CommandEnvT) Run() error {
	env.buildGraph.PostLoad()

	// prepare specified commands
	for _, cl := range env.commandLines {
		if err := env.commandEvents.Parse(cl); err != nil {
			LogError(LogCommand, "%s", err)
			PrintCommandHelp(os.Stderr, false)
			return err
		}
	}

	defer func() {
		JoinAllThreadPools()
		env.onExit.FireAndForget(env)
		PurgePinnedLogs()
	}()

	// check if any command was successfully parsed
	if !env.commandEvents.Bound() {
		LogWarning(LogCommand, "missing argument, use `help` to learn about command usage")
		return nil
	}

	err := env.commandEvents.Run()

	if er := env.buildGraph.Join(); err != nil && err == nil {
		err = er
	}
	return err
}
func (env *CommandEnvT) LoadConfig() error {
	benchmark := LogBenchmark(LogCommand, "loading config from '%v'...", env.configPath)
	defer benchmark.Close()

	return UFS.OpenBuffered(env.configPath, env.persistent.Deserialize)
}
func (env *CommandEnvT) LoadBuildGraph() error {
	benchmark := LogBenchmark(LogCommand, "loading build graph from '%v'...", env.databasePath)
	defer benchmark.Close()

	err := UFS.OpenBuffered(env.databasePath, env.buildGraph.Load)
	if err != nil {
		env.buildGraph.(*buildGraph).makeDirty()
	}
	return err
}
func (env *CommandEnvT) SaveConfig() error {
	benchmark := LogBenchmark(LogCommand, "saving config to '%v'...", env.configPath)
	defer benchmark.Close()

	return UFS.SafeCreate(env.configPath, env.persistent.Serialize)
}
func (env *CommandEnvT) SaveBuildGraph() error {
	if env.lastPanic != nil {
		LogTrace(LogCommand, "won't save build graph since a panic occured")
	} else if env.buildGraph.Dirty() {
		benchmark := LogBenchmark(LogCommand, "saving build graph to '%v'...", env.databasePath)
		defer benchmark.Close()

		return UFS.SafeCreate(env.databasePath, env.buildGraph.Save)
	} else {
		LogTrace(LogCommand, "skipped saving unmodified build graph")
	}
	return nil
}
