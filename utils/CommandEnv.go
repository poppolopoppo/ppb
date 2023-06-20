package utils

import (
	"fmt"
	"math"
	"runtime"
	"sync"
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
	Summary:        base.MakeBoolVar(base.DEBUG_ENABLED),
	WarningAsError: base.INHERITABLE_FALSE,
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
	cfv.Variable("LogImmediate", "disable buffering of log messages", &flags.LogImmediate)
	cfv.Variable("LogFile", "output log to specified file (default: stdout)", &flags.LogFile)
	cfv.Variable("OutputDir", "override default output directory", &flags.OutputDir)
	cfv.Variable("RootDir", "override root directory", &flags.RootDir)
	cfv.Variable("Summary", "print build graph execution summary when build finished", &flags.Summary)
	cfv.Variable("WX", "consider warnings as errors", &flags.WarningAsError)
}
func (flags *CommandFlags) Apply() {
	base.SetEnableDiagnostics(flags.Diagnostics.Get())
	base.GetLogger().SetShowTimestamp(flags.Timestamp.Get())

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
		base.SetEnableInteractiveShell(false)
	}
	if flags.Debug.Get() {
		base.GetLogger().SetLevel(base.LOG_DEBUG)
		base.SetEnableDiagnostics(true)
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

	// queue print summary if specified on command-line
	if flags.Summary.Get() {
		CommandEnv.onExit.Add(func(cet *CommandEnvT) error {
			base.PurgePinnedLogs()
			cet.lazyBuildGraph.PrintSummary(cet.startedAt)
			return nil
		})
	}

	if !flags.Color.IsInheritable() {
		base.SetEnableAnsiColor(flags.Color.Get())
	}

	if flags.Ide.Get() {
		base.GetLogger().SetLevelMinimum(base.LOG_INFO)
		base.SetEnableAnsiColor(false)
		base.SetEnableInteractiveShell(false)
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
	prefix         string
	lazyBuildGraph lazyBuildGraph
	persistent     *persistentData
	rootFile       Filename
	startedAt      time.Time

	configPath   Filename
	databasePath Filename

	onExit base.ConcurrentEvent[*CommandEnvT]

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

	base.OnPanic = CommandEnv.OnPanic

	CommandEnv.commandLines = NewCommandLine(CommandEnv.persistent, args)

	// parse global flags early-on
	base.LogPanicIfFailed(LogCommand, PrepareCommands(CommandEnv.commandLines, &CommandEnv.commandEvents))

	// apply global command flags early-on
	GetCommandFlags().Apply()

	// use UFS.Output only after having parsed -OutputDir/RootDir= flags
	CommandEnv.configPath = UFS.Output.File(fmt.Sprint(".", prefix, "-config.json"))
	CommandEnv.databasePath = UFS.Output.File(fmt.Sprint(".", prefix, "-cache.db"))
	CommandEnv.rootFile = UFS.Source.File(prefix + "-namespace.json")

	base.LogVerbose(LogCommand, "will load config from %q", CommandEnv.configPath)
	base.LogVerbose(LogCommand, "will load database from %q", CommandEnv.databasePath)
	base.LogVerbose(LogCommand, "will load modules from %q", CommandEnv.rootFile)

	runtime.SetFinalizer(CommandEnv, func(env *CommandEnvT) {
		env.onExit.FireAndForget(env)
	})

	return CommandEnv
}
func (env *CommandEnvT) Prefix() string             { return env.prefix }
func (env *CommandEnvT) BuildGraph() BuildGraph     { return env.lazyBuildGraph.Get() }
func (env *CommandEnvT) Persistent() PersistentData { return env.persistent }
func (env *CommandEnvT) ConfigPath() Filename       { return env.configPath }
func (env *CommandEnvT) DatabasePath() Filename     { return env.databasePath }
func (env *CommandEnvT) RootFile() Filename         { return env.rootFile }
func (env *CommandEnvT) StartedAt() time.Time       { return env.startedAt }
func (env *CommandEnvT) BuildTime() time.Time       { return PROCESS_INFO.Timestamp }

func (env *CommandEnvT) SetRootFile(rootFile Filename) {
	base.LogVerbose(LogCommand, "set root file to %q", rootFile)
	env.rootFile = rootFile
}

func (env *CommandEnvT) OnBuildGraphLoaded(e base.EventDelegate[BuildGraph]) base.DelegateHandle {
	return env.lazyBuildGraph.OnBuildGraphLoaded.Add(e)
}
func (env *CommandEnvT) OnBuildGraphSaved(e base.EventDelegate[BuildGraph]) base.DelegateHandle {
	return env.lazyBuildGraph.OnBuildGraphSaved.Add(e)
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
	if env.lastPanic == nil {

		env.lastPanic = err
		env.commandEvents.OnPanic.Invoke(err)
		return base.PANIC_ABORT
	}
	return base.PANIC_REENTRANCY // a fatal error was already reported
}

func (env *CommandEnvT) Run() error {
	// prepare specified commands
	for _, cl := range env.commandLines {
		if err := env.commandEvents.Parse(cl); err != nil {
			return err
		}
	}

	defer func() {
		base.JoinAllThreadPools()
		env.onExit.FireAndForget(env)
		base.PurgePinnedLogs()
	}()

	// check if any command was successfully parsed
	if !env.commandEvents.Bound() {
		base.LogWarning(LogCommand, "missing argument, use `help` to learn about command usage")
		return nil
	}

	err := env.commandEvents.Run()

	if er := env.lazyBuildGraph.Join(); err != nil && err == nil {
		err = er
	}
	return err
}
func (env *CommandEnvT) LoadConfig() error {
	benchmark := base.LogBenchmark(LogCommand, "loading config from '%v'...", env.configPath)
	defer benchmark.Close()

	return UFS.OpenBuffered(env.configPath, env.persistent.Deserialize)
}
func (env *CommandEnvT) SaveConfig() error {
	benchmark := base.LogBenchmark(LogCommand, "saving config to '%v'...", env.configPath)
	defer benchmark.Close()

	return UFS.SafeCreate(env.configPath, env.persistent.Serialize)
}
func (env *CommandEnvT) SaveBuildGraph() error {
	return env.lazyBuildGraph.Save(env)
}

/***************************************
 * Lazy build graph: don't load build database unless needed
 ***************************************/

type lazyBuildGraph struct {
	barrier    sync.Mutex
	buildGraph BuildGraph

	OnBuildGraphLoaded base.ConcurrentEvent[BuildGraph]
	OnBuildGraphSaved  base.ConcurrentEvent[BuildGraph]
}

func (x *lazyBuildGraph) Get() BuildGraph {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	if base.IsNil(x.buildGraph) {
		x.buildGraph = NewBuildGraph(GetCommandFlags())
		if err := x.loadBuildGraph(CommandEnv); err != nil {
			base.LogError(LogBuildGraph, "failed to load build graph database: %v", err)
		}
		x.buildGraph.PostLoad()
	}
	return x.buildGraph
}
func (x *lazyBuildGraph) Join() error {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	if base.IsNil(x.buildGraph) {
		return nil
	}
	return x.buildGraph.Join()
}
func (x *lazyBuildGraph) Save(env *CommandEnvT) error {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	if base.IsNil(x.buildGraph) {
		return nil
	}
	return x.saveBuildGraph(env)
}
func (x *lazyBuildGraph) saveBuildGraph(env *CommandEnvT) error {
	if env.lastPanic != nil {
		base.LogTrace(LogCommand, "won't save build graph since a panic occured")
	} else if x.buildGraph.Dirty() {
		benchmark := base.LogBenchmark(LogCommand, "saving build graph to '%v'...", env.databasePath)
		defer benchmark.Close()

		return UFS.SafeCreate(env.databasePath, x.buildGraph.Save)
	} else {
		base.LogTrace(LogCommand, "skipped saving unmodified build graph")
	}
	return x.OnBuildGraphSaved.Invoke(x.buildGraph)
}
func (x *lazyBuildGraph) loadBuildGraph(env *CommandEnvT) error {
	benchmark := base.LogBenchmark(LogCommand, "loading build graph from '%v'...", env.databasePath)
	defer benchmark.Close()

	err := UFS.OpenBuffered(env.databasePath, x.buildGraph.Load)

	if err == nil {
		err = x.OnBuildGraphLoaded.Invoke(x.buildGraph)
	} else {
		x.buildGraph.(*buildGraph).makeDirty()
	}
	return err
}

func (x *lazyBuildGraph) PrintSummary(startedAt time.Time) {
	if base.IsNil(x.buildGraph) {
		return
	}

	totalDuration := time.Since(startedAt)
	base.LogForwardf("\nProgram took %.3f seconds to run", totalDuration.Seconds())

	stats := x.buildGraph.GetBuildStats()
	if stats.Count == 0 {
		return
	}

	base.LogForwardf("Took %.3f seconds to build %d nodes using %d threads (x%.2f)",
		stats.Duration.Exclusive.Seconds(), stats.Count, runtime.GOMAXPROCS(0),
		float32(stats.Duration.Exclusive)/float32(totalDuration))

	base.LogForwardf("\nMost expansive nodes built:")

	colorHot := base.Color3b{R: 255, G: 128, B: 128}.Unquantize(true)
	colorCold := base.Color3b{R: 128, G: 128, B: 128}.Unquantize(true)
	for i, node := range x.buildGraph.GetMostExpansiveNodes(10, false) {
		ns := node.GetBuildStats()
		fract := ns.Duration.Exclusive.Seconds() / stats.Duration.Exclusive.Seconds()

		sstep := base.Smootherstep(math.Sqrt(ns.Duration.Exclusive.Seconds() / totalDuration.Seconds())) // use percent of blocking duration

		rowColor := colorCold.Lerp(colorHot, sstep)
		rowColor = rowColor.Brightness(0.45 + 0.15*sstep)

		base.LogForwardf("%v[%02d] - %6.2f%% -  %6.3f  %6.3f  --  %s%v",
			rowColor.Quantize(true).Ansi(true),
			(i + 1),
			100.0*fract,
			ns.Duration.Exclusive.Seconds(),
			ns.Duration.Inclusive.Seconds(),
			node.Alias(),
			base.ANSI_RESET)
	}
}
