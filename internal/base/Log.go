package base

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

/***************************************
 * Logger API
 ***************************************/

var LogGlobal = NewLogCategory("Global")

var gLogger Logger = NewLogger(DEBUG_ENABLED)

func GetLogger() Logger { return gLogger }
func SetLogger(logger Logger) (previous Logger) {
	previous = gLogger
	gLogger = logger
	return
}

func LogIf(level LogLevel, category *LogCategory, enabled bool, msg string, args ...interface{}) {
	if enabled {
		gLogger.Log(category, level, msg, args...)
	}
}

//// THOSE ARE DEFINED INSIDE Assert_Debug/Assert_NotDebug TO COMPILE OUT DEBUG/TRACE MESSAGES
// func LogDebug(category *LogCategory, msg string, args ...interface{}) {
// 	gLogger.Log(category, LOG_DEBUG, msg, args...)
// }
// func LogDebugIf(category *LogCategory, enabled bool, msg string, args ...interface{}) {
// 	LogIf(LOG_DEBUG, category, enabled, msg, args...)
// }
// func LogTrace(category *LogCategory, msg string, args ...interface{}) {
// 	gLogger.Log(category, LOG_TRACE, msg, args...)
// }

func LogVeryVerbose(category *LogCategory, msg string, args ...interface{}) {
	gLogger.Log(category, LOG_VERYVERBOSE, msg, args...)
}
func LogVerbose(category *LogCategory, msg string, args ...interface{}) {
	gLogger.Log(category, LOG_VERBOSE, msg, args...)
}
func LogInfo(category *LogCategory, msg string, args ...interface{}) {
	gLogger.Log(category, LOG_INFO, msg, args...)
}
func LogClaim(category *LogCategory, msg string, args ...interface{}) {
	gLogger.Log(category, LOG_CLAIM, msg, args...)
}

var logWarningsSeenOnce = NewSharedMapT[string, int]()

func LogWarning(category *LogCategory, msg string, args ...interface{}) {
	gLogger.Log(category, LOG_WARNING, msg, args...)
}
func LogWarningOnce(category *LogCategory, msg string, args ...interface{}) {
	if IsLogLevelActive(LOG_VERBOSE) {
		formattedMsg := fmt.Sprintf(msg, args...)
		if _, loaded := logWarningsSeenOnce.FindOrAdd(formattedMsg, 1); !loaded {
			gLogger.Log(category, LOG_WARNING, msg, args...)
		}
	}
}
func LogWarningVerbose(category *LogCategory, msg string, args ...interface{}) {
	if IsLogLevelActive(LOG_VERYVERBOSE) {
		gLogger.Log(category, LOG_WARNING, msg, args...)
	}
}

func LogError(category *LogCategory, msg string, args ...interface{}) {
	gLogger.Log(category, LOG_ERROR, msg, args...)
}
func LogFatal(msg string, args ...interface{}) {
	gLogger.Purge()
	log.Fatalf(msg, args...)
}

func LogPanic(category *LogCategory, msg string, args ...interface{}) {
	LogPanicErr(category, fmt.Errorf(msg, args...))
}
func LogPanicErr(category *LogCategory, err error) {
	LogError(category, "panic: caught error %v", err)
	Panic(err)
}
func LogPanicIfFailed(category *LogCategory, err error) {
	if err != nil {
		LogPanicErr(category, err)
	}
}

func LogForward(msg ...string) {
	gLogger.Forward(msg...)
}
func LogForwardln(msg ...string) {
	gLogger.Forwardln(msg...)
}
func LogForwardf(format string, args ...interface{}) {
	gLogger.Forwardf(format, args...)
}

func WithoutLog(block func()) {
	gLogger.WithoutPin(block)
}

func IsLogLevelActive(level LogLevel) bool {
	return gLogger.IsVisible(level)
}
func FlushLog() {
	gLogger.Flush()
}

/***************************************
 * Logger interface
 ***************************************/

type PinScope interface {
	Log(msg string, args ...interface{})
	Closable

	format(LogWriter)
}

type ProgressScope interface {
	Progress() int
	Len() int
	Grow(int)
	Add(int)
	Inc()
	Set(int)
	PinScope
}

type LogCategory struct {
	Name  string
	Level LogLevel
	Hash  uint64
	Color Color3b
}

type LogWriter interface {
	io.Writer
	io.StringWriter
}

type Logger interface {
	IsInteractive() bool
	IsVisible(LogLevel) bool

	SetLevel(LogLevel) LogLevel
	SetLevelMaximum(LogLevel) LogLevel
	SetLevelMinimum(LogLevel) LogLevel
	SetWarningAsError(bool)
	SetShowCategory(bool)
	SetShowTimestamp(bool)
	SetWriter(LogWriter)

	Forward(msg ...string)
	Forwardln(msg ...string)
	Forwardf(msg string, args ...interface{})

	Log(category *LogCategory, level LogLevel, msg string, args ...interface{})

	Pin(msg string, args ...interface{}) PinScope
	Progress(first, last int, msg string, args ...interface{}) ProgressScope
	WithoutPin(func())
	Close(PinScope) error

	Flush()   // wait for all pending all messages
	Purge()   // close the log and every on-going pins
	Refresh() // re-draw all pins, need for animated effects

}

func MakeError(msg string, args ...interface{}) error {
	//LogError(LogGlobal, msg, args...) # DONT -> this can lock recursively the logger
	return fmt.Errorf(msg, args...)
}

func MakeUnexpectedValueError(dst interface{}, any interface{}) error {
	return MakeError("unexpected <%T> value: %#v", dst, any)
}
func UnexpectedValuePanic(dst interface{}, any interface{}) {
	LogPanicErr(LogGlobal, MakeUnexpectedValueError(dst, any))
}

type LogForwardWriter struct{}

func (x LogForwardWriter) Write(buf []byte) (int, error) {
	LogForward(UnsafeStringFromBytes(buf))
	return len(buf), nil
}

func NewLogger(immediate bool) Logger {
	if immediate {
		return newImmediateLogger(newInteractiveLogger(newBasicLogger()))
	} else {
		return newDeferredLogger(newInteractiveLogger(newBasicLogger()))
	}
}

/***************************************
 * Log Manager
 ***************************************/

type LogManager struct {
	barrierRW  sync.RWMutex
	categories map[string]*LogCategory
}

var gLogManager = LogManager{
	categories: make(map[string]*LogCategory, 100),
}

func GetLogManager() *LogManager { return &gLogManager }

func (x *LogManager) SetCategoryLevel(name string, level LogLevel) {
	x.FindOrAddCategory(name).Level = level
}
func (x *LogManager) FindCategory(name string) *LogCategory {
	x.barrierRW.RLock()
	defer x.barrierRW.RUnlock()
	return x.categories[name]
}
func (x *LogManager) FindOrAddCategory(name string) (result *LogCategory) {
	if result = x.FindCategory(name); result == nil {
		x.barrierRW.Lock()
		defer x.barrierRW.Unlock()
		if result = x.categories[name]; result == nil {
			category := MakeLogCategory(name)
			result = &category
			x.categories[name] = result
		}
	}
	return
}

/***************************************
 * Log Category
 ***************************************/

func MakeLogCategory(name string) LogCategory {
	var hash uint64 = 14695981039346656037
	hash = Fnv1a(name, hash)

	rnd := rand.New(rand.NewSource(int64(hash)))

	nextFloat01 := func(r *rand.Rand) float64 {
		// see official comment in func (r *Rand) Float64() float64
		return float64(r.Int63n(1<<53)) / (1 << 53)
	}

	col := NewPastelizerColor(nextFloat01(rnd))

	return LogCategory{
		Name:  name,
		Level: LOG_FATAL,
		Hash:  hash,
		Color: col.Quantize(true),
	}
}

func NewLogCategory(name string) *LogCategory {
	return gLogManager.FindOrAddCategory(name)
}

/***************************************
 * Log level
 ***************************************/

type LogLevel int32

const (
	LOG_ALL LogLevel = iota
	LOG_DEBUG
	LOG_TRACE
	LOG_VERYVERBOSE
	LOG_VERBOSE
	LOG_INFO
	LOG_CLAIM
	LOG_WARNING
	LOG_ERROR
	LOG_FATAL
)

func (x LogLevel) IsVisible(level LogLevel) bool {
	return (int32(level) >= int32(x))
}
func (x LogLevel) Style(dst io.Writer) {
	switch x {
	case LOG_DEBUG:
		fmt.Fprint(dst, ANSI_FG0_MAGENTA, ANSI_ITALIC, ANSI_FAINT)
	case LOG_TRACE:
		fmt.Fprint(dst, ANSI_FG0_CYAN, ANSI_ITALIC, ANSI_FAINT)
	case LOG_VERYVERBOSE:
		fmt.Fprint(dst, ANSI_FG1_MAGENTA, ANSI_ITALIC, ANSI_ITALIC)
	case LOG_VERBOSE:
		fmt.Fprint(dst, ANSI_FG0_BLUE)
	case LOG_INFO:
		fmt.Fprint(dst, ANSI_FG1_WHITE, ANSI_BG0_BLACK)
	case LOG_CLAIM:
		fmt.Fprint(dst, ANSI_FG1_GREEN, ANSI_BG0_BLUE, ANSI_BOLD)
	case LOG_WARNING:
		fmt.Fprint(dst, ANSI_FG0_YELLOW)
	case LOG_ERROR:
		fmt.Fprint(dst, ANSI_FG1_RED, ANSI_BOLD)
	case LOG_FATAL:
		fmt.Fprint(dst, ANSI_FG1_WHITE, ANSI_BG0_RED, ANSI_BLINK0)
	default:
		UnexpectedValue(x)
	}
}
func (x LogLevel) Header(dst io.Writer) {
	switch x {
	// case LOG_DEBUG:
	// 	fmt.Fprint(dst, "   ")
	// case LOG_TRACE:
	// 	fmt.Fprint(dst, "  ")
	// case LOG_VERYVERBOSE:
	// 	fmt.Fprint(dst, "   ")
	// case LOG_VERBOSE:
	// 	fmt.Fprint(dst, "  ")
	// case LOG_INFO:
	// 	fmt.Fprint(dst, "  ")
	// case LOG_CLAIM:
	// 	fmt.Fprint(dst, "  ")
	// case LOG_WARNING:
	// 	fmt.Fprint(dst, "  ")
	// case LOG_ERROR:
	// 	fmt.Fprint(dst, "  ")
	// case LOG_FATAL:
	// 	fmt.Fprint(dst, "  ")
	case LOG_DEBUG:
		fmt.Fprint(dst, "  ~  ")
	case LOG_TRACE:
		fmt.Fprint(dst, "  .  ")
	case LOG_VERYVERBOSE:
		fmt.Fprint(dst, "     ")
	case LOG_VERBOSE:
		fmt.Fprint(dst, "  -  ")
	case LOG_INFO:
		fmt.Fprint(dst, " --- ")
	case LOG_CLAIM:
		fmt.Fprint(dst, " --> ")
	case LOG_WARNING:
		fmt.Fprint(dst, " /?\\ ")
	case LOG_ERROR:
		fmt.Fprint(dst, " /!\\ ")
	case LOG_FATAL:
		fmt.Fprint(dst, " [!] ")
	default:
		UnexpectedValue(x)
	}
}
func (x LogLevel) String() string {
	outp := strings.Builder{}
	x.Header(&outp)
	return outp.String()
}

/***************************************
 * Basic Logger
 ***************************************/

type basicLogPin struct{}

func (x basicLogPin) Log(string, ...interface{}) {}
func (x basicLogPin) Close() error               { return nil }
func (x basicLogPin) format(LogWriter)           {}

type basicLogProgress struct {
	basicLogPin
}

func (x basicLogProgress) Progress() int { return 0 }
func (x basicLogProgress) Len() int      { return 0 }
func (x basicLogProgress) Grow(int)      {}
func (x basicLogProgress) Add(int)       {}
func (x basicLogProgress) Inc()          {}
func (x basicLogProgress) Set(int)       {}

type basicLogger struct {
	MinimumLevel   LogLevel
	WarningAsError bool
	ShowCategory   bool
	ShowTimestamp  bool
	Writer         *bufio.Writer

	lastFlush time.Time
}

func newBasicLogger() *basicLogger {
	level := LOG_INFO
	if EnableDiagnostics() {
		level = LOG_ALL
	}

	return &basicLogger{
		MinimumLevel:   level,
		WarningAsError: false,
		ShowCategory:   true,
		ShowTimestamp:  false,
		Writer:         bufio.NewWriter(os.Stdout),
		lastFlush:      time.Now(),
	}
}

func (x *basicLogger) IsInteractive() bool {
	return false
}
func (x *basicLogger) IsVisible(level LogLevel) bool {
	return x.MinimumLevel.IsVisible(level)
}

func (x *basicLogger) SetLevel(level LogLevel) LogLevel {
	previous := x.MinimumLevel
	if level < LOG_FATAL {
		x.MinimumLevel = level
	} else {
		x.MinimumLevel = LOG_FATAL
	}
	return previous
}
func (x *basicLogger) SetLevelMinimum(level LogLevel) LogLevel {
	previous := x.MinimumLevel
	if level < LOG_FATAL && level < x.MinimumLevel {
		x.MinimumLevel = level
	}
	return previous
}
func (x *basicLogger) SetLevelMaximum(level LogLevel) LogLevel {
	previous := x.MinimumLevel
	if level < LOG_FATAL && level > x.MinimumLevel {
		x.MinimumLevel = level
	}
	return previous
}
func (x *basicLogger) SetWarningAsError(enabled bool) {
	x.WarningAsError = enabled
}
func (x *basicLogger) SetShowCategory(enabled bool) {
	x.ShowCategory = enabled
}
func (x *basicLogger) SetShowTimestamp(enabled bool) {
	x.ShowTimestamp = enabled
}
func (x *basicLogger) SetWriter(dst LogWriter) {
	Assert(func() bool { return !IsNil(dst) })
	x.Writer.Flush()
	x.Writer.Reset(dst)
}

func (x *basicLogger) Forward(msg ...string) {
	for _, it := range msg {
		x.Writer.WriteString(it)
	}
}
func (x *basicLogger) Forwardln(msg ...string) {
	if len(msg) == 0 {
		return
	}
	for _, it := range msg {
		x.Writer.WriteString(it)
	}
	if !strings.HasSuffix(msg[len(msg)-1], "\n") {
		x.Writer.WriteRune('\n')
	}

	x.flushLogToAvoidCropIFN()
}
func (x *basicLogger) Forwardf(msg string, args ...interface{}) {
	fmt.Fprintf(x.Writer, msg, args...)
	fmt.Fprintln(x.Writer, "")

	x.flushLogToAvoidCropIFN()
}

func (x *basicLogger) Log(category *LogCategory, level LogLevel, msg string, args ...interface{}) {
	// warning as error?
	if level == LOG_WARNING && x.WarningAsError {
		level = LOG_ERROR
	}

	// log level visible?
	if !x.IsVisible(level) && !category.Level.IsVisible(level) {
		return
	}

	// format message
	if x.ShowTimestamp {
		fmt.Fprintf(x.Writer, "%s%010.5f |%s  ", ANSI_FG1_BLACK, Elapsed().Seconds(), ANSI_RESET)
	}

	level.Style(x.Writer)
	level.Header(x.Writer)

	if x.ShowCategory {
		fmt.Fprintf(x.Writer, " %s%s%s%s: ", ANSI_RESET, category.Color.Ansi(true), category.Name, ANSI_RESET)
		level.Style(x.Writer)
	}

	fmt.Fprintf(x.Writer, msg, args...)

	x.Writer.WriteString(ANSI_RESET.String())
	x.Writer.WriteRune('\n')

	x.flushLogToAvoidCropIFN()
}

func (x *basicLogger) Pin(msg string, args ...interface{}) PinScope {
	return basicLogPin{} // see interactiveLogger struct
}
func (x *basicLogger) Progress(first, last int, msg string, args ...interface{}) ProgressScope {
	return basicLogProgress{} // see interactiveLogger struct
}
func (x *basicLogger) WithoutPin(block func()) {
	block() // see interactiveLogger struct
}
func (x *basicLogger) Close(pin PinScope) error {
	UnreachableCode() // see interactiveLogger structa
	return nil
}

func (x *basicLogger) Flush()   { x.Writer.Flush() }
func (x *basicLogger) Purge()   { x.Writer.Flush() }
func (x *basicLogger) Refresh() { x.Writer.Flush() }

func (x *basicLogger) flushLogToAvoidCropIFN() {
	if x.Writer.Available() < 200 {
		x.Writer.Flush()
	} else if time.Since(x.lastFlush) > 500*time.Millisecond {
		x.lastFlush = time.Now()
		x.Writer.Flush()
	}
}

/***************************************
 * Deferred Logger
 ***************************************/

type deferredPinScope struct {
	future Future[PinScope]
}

func (x deferredPinScope) Log(msg string, args ...interface{}) {
	x.future.Join().Success().Log(msg, args...)
}
func (x deferredPinScope) Close() error {
	result := x.future.Join()
	if err := result.Failure(); err == nil {
		return result.Success().Close()
	} else {
		return err
	}
}
func (x deferredPinScope) format(dst LogWriter) {
	x.future.Join().Success().format(dst)
}

type deferredProgressScope struct {
	future Future[ProgressScope]
}

func (x deferredProgressScope) Log(msg string, args ...interface{}) {
	x.future.Join().Success().Log(msg, args...)
}
func (x deferredProgressScope) Close() error {
	result := x.future.Join()
	if err := result.Failure(); err == nil {
		return result.Success().Close()
	} else {
		return err
	}
}
func (x deferredProgressScope) format(dst LogWriter) {
	x.future.Join().Success().format(dst)
}

func (x deferredProgressScope) Progress() int { return x.future.Join().Success().Progress() }
func (x deferredProgressScope) Len() int      { return x.future.Join().Success().Len() }

func (x deferredProgressScope) Grow(n int) {
	x.future.Join().Success().Grow(n)
}
func (x deferredProgressScope) Add(n int) {
	x.future.Join().Success().Add(n)
}
func (x deferredProgressScope) Inc() {
	x.future.Join().Success().Inc()
}
func (x deferredProgressScope) Set(v int) {
	x.future.Join().Success().Set(v)
}

type deferredLogger struct {
	logger  Logger
	thread  ThreadPool
	barrier *sync.Mutex
}

func newDeferredLogger(logger Logger) deferredLogger {
	barrier := &sync.Mutex{}
	return deferredLogger{
		logger:  logger,
		barrier: barrier,
		thread: NewFixedSizeThreadPoolEx("logger", 1,
			func(threadContext ThreadContext, tasks <-chan TaskFunc) {
				runTask := func(task TaskFunc) {
					barrier.Lock()
					defer barrier.Unlock()
					task(threadContext)
				}

				for quit := false; !quit; {
					if logger.IsInteractive() {
						// refresh pinned logs if no message output after a while
						select {
						case task := (<-tasks):
							if task != nil {
								runTask(task)
							} else {
								quit = true
							}
						case <-time.After(33 * time.Millisecond):
							logger.Refresh()
						}
					} else {
						if task := (<-tasks); task != nil {
							runTask(task)
						} else {
							quit = true // a nil task means quit
						}
					}
				}
			}),
	}
}

func (x deferredLogger) IsInteractive() bool {
	return x.logger.IsInteractive()
}
func (x deferredLogger) IsVisible(level LogLevel) bool {
	return x.logger.IsVisible(level)
}

func (x deferredLogger) SetLevel(level LogLevel) LogLevel {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.SetLevel(level)
}
func (x deferredLogger) SetLevelMinimum(level LogLevel) LogLevel {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.SetLevelMinimum(level)
}
func (x deferredLogger) SetLevelMaximum(level LogLevel) LogLevel {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.SetLevelMaximum(level)
}
func (x deferredLogger) SetWarningAsError(enabled bool) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.SetWarningAsError(enabled)
}
func (x deferredLogger) SetShowCategory(enabled bool) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.SetShowCategory(enabled)
}
func (x deferredLogger) SetShowTimestamp(enabled bool) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.SetShowTimestamp(enabled)
}
func (x deferredLogger) SetWriter(dst LogWriter) {
	x.thread.Queue(func(ThreadContext) {
		x.logger.SetWriter(dst)
	})
}

func (x deferredLogger) Forward(msg ...string) {
	x.thread.Queue(func(ThreadContext) {
		x.logger.Forward(msg...)
	})
}
func (x deferredLogger) Forwardln(msg ...string) {
	x.thread.Queue(func(ThreadContext) {
		x.logger.Forwardln(msg...)
	})
}
func (x deferredLogger) Forwardf(msg string, args ...interface{}) {
	x.thread.Queue(func(ThreadContext) {
		x.logger.Forwardf(msg, args...)
	})
}
func (x deferredLogger) Log(category *LogCategory, level LogLevel, msg string, args ...interface{}) {
	if x.logger.IsVisible(level) || category.Level.IsVisible(level) {
		x.thread.Queue(func(ThreadContext) {
			x.logger.Log(category, level, msg, args...)
		})
	}
	if level >= LOG_ERROR {
		x.thread.Join() // flush log when an error occurred
	}
}
func (x deferredLogger) Pin(msg string, args ...interface{}) PinScope {
	return deferredPinScope{
		future: MakeWorkerFuture(x.thread, func(ThreadContext) (PinScope, error) {
			pin := x.logger.Pin(msg, args...)
			return pin, nil
		})}
}
func (x deferredLogger) Progress(first, last int, msg string, args ...interface{}) ProgressScope {
	return deferredProgressScope{
		future: MakeWorkerFuture(x.thread, func(ThreadContext) (ProgressScope, error) {
			pin := x.logger.Progress(first, last, msg, args...)
			return pin, nil
		})}
}
func (x deferredLogger) WithoutPin(block func()) {
	x.thread.Queue(func(ThreadContext) {
		x.logger.WithoutPin(block)
	})
	x.thread.Join()
}
func (x deferredLogger) Close(pin PinScope) error {
	x.thread.Queue(func(ThreadContext) {
		err := x.logger.Close(pin)
		LogPanicIfFailed(LogGlobal, err)
	})
	return nil
}
func (x deferredLogger) Flush() {
	x.thread.Queue(func(ThreadContext) {
		x.logger.Flush()
	})
	x.thread.Join()
}
func (x deferredLogger) Purge() {
	x.thread.Queue(func(ThreadContext) {
		x.logger.Purge()
	})
	x.thread.Join()
}
func (x deferredLogger) Refresh() {
	x.thread.Queue(func(ThreadContext) {
		x.logger.Refresh()
	})
}

/***************************************
 * Immediate Logger
 ***************************************/

type immediateLogger struct {
	logger  Logger
	barrier *sync.Mutex
}

func newImmediateLogger(logger Logger) immediateLogger {
	barrier := &sync.Mutex{}
	return immediateLogger{
		logger:  logger,
		barrier: barrier,
	}
}

func (x immediateLogger) IsInteractive() bool {
	return x.logger.IsInteractive()
}
func (x immediateLogger) IsVisible(level LogLevel) bool {
	return x.logger.IsVisible(level)
}

func (x immediateLogger) SetLevel(level LogLevel) LogLevel {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.SetLevel(level)
}
func (x immediateLogger) SetLevelMinimum(level LogLevel) LogLevel {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.SetLevelMinimum(level)
}
func (x immediateLogger) SetLevelMaximum(level LogLevel) LogLevel {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.SetLevelMaximum(level)
}
func (x immediateLogger) SetWarningAsError(enabled bool) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.SetWarningAsError(enabled)
}
func (x immediateLogger) SetShowCategory(enabled bool) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.SetShowCategory(enabled)
}
func (x immediateLogger) SetShowTimestamp(enabled bool) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.SetShowTimestamp(enabled)
}
func (x immediateLogger) SetWriter(dst LogWriter) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.SetWriter(dst)
}

func (x immediateLogger) Forward(msg ...string) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.Forward(msg...)
}
func (x immediateLogger) Forwardln(msg ...string) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.Forwardln(msg...)
}
func (x immediateLogger) Forwardf(msg string, args ...interface{}) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.Forwardf(msg, args...)
}
func (x immediateLogger) Log(category *LogCategory, level LogLevel, msg string, args ...interface{}) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	if category.Level.IsVisible(level) || x.logger.IsVisible(level) {
		x.logger.Log(category, level, msg, args...)
	}
}
func (x immediateLogger) Pin(msg string, args ...interface{}) PinScope {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.Pin(msg, args...)
}
func (x immediateLogger) Progress(first, last int, msg string, args ...interface{}) ProgressScope {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.Progress(first, last, msg, args...)
}
func (x immediateLogger) WithoutPin(block func()) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.WithoutPin(block)
}
func (x immediateLogger) Close(pin PinScope) error {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.Close(pin)
}
func (x immediateLogger) Flush() {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.Flush()
}
func (x immediateLogger) Purge() {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.Purge()
}
func (x immediateLogger) Refresh() {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.Refresh()
}

/***************************************
 * Interactive Logger
 ***************************************/

var enableInteractiveShell bool = true
var forceInteractiveShell bool = false

func EnableInteractiveShell() bool {
	return enableInteractiveShell
}
func SetEnableInteractiveShell(enabled bool) {
	if enableInteractiveShell {
		enabled = enabled || forceInteractiveShell
		enableInteractiveShell = enabled
	}
}

type interactiveLogPin struct {
	header atomic.Value
	writer func(LogWriter)

	tick      int
	first     int
	last      atomic.Int32
	progress  atomic.Int32
	startedAt time.Duration

	color Color3b
}

func (x *interactiveLogPin) reset() {
	x.header.Store("")
	x.writer = nil
	x.tick = 0
	x.startedAt = 0
	x.progress.Store(0)
}
func (x *interactiveLogPin) format(dst LogWriter) {
	if x.writer != nil {
		x.writer(dst)
	}
}

func (x *interactiveLogPin) Log(msg string, args ...interface{}) {
	x.header.Store(fmt.Sprintf(msg, args...) + " ")
}
func (x *interactiveLogPin) Close() error {
	return gLogger.Close(x)
}

func (x *interactiveLogPin) Progress() int {
	return int(x.progress.Load()) - x.first
}
func (x *interactiveLogPin) Len() int {
	return int(x.last.Load()) - x.first
}

func (x *interactiveLogPin) Grow(n int) {
	x.last.Add(int32(n))
}
func (x *interactiveLogPin) Add(n int) {
	x.progress.Add(int32(n))
}
func (x *interactiveLogPin) Inc() {
	x.progress.Add(1)
}
func (x *interactiveLogPin) Set(v int) {
	for {
		prev := x.progress.Load()
		if prev > int32(v) || x.progress.CompareAndSwap(prev, int32(v)) {
			break
		}
	}
}

type interactiveLogger struct {
	messages SetT[*interactiveLogPin]
	inflight int
	maxLen   int

	recycler  Recycler[*interactiveLogPin]
	transient bytes.Buffer
	*basicLogger
}

func newInteractiveLogger(basic *basicLogger) *interactiveLogger {
	return &interactiveLogger{
		messages:    make([]*interactiveLogPin, 0, runtime.NumCPU()),
		inflight:    0,
		maxLen:      0,
		basicLogger: basic,
		recycler: NewRecycler(
			func() *interactiveLogPin {
				return new(interactiveLogPin)
			},
			func(ip *interactiveLogPin) {
				ip.reset()
			}),
	}
}
func (x *interactiveLogger) IsInteractive() bool {
	return true
}
func (x *interactiveLogger) Forward(msg ...string) {
	x.WithoutPin(func() {
		x.basicLogger.Forward(msg...)
	})
}
func (x *interactiveLogger) Forwardln(msg ...string) {
	x.WithoutPin(func() {
		x.basicLogger.Forwardln(msg...)
	})
}
func (x *interactiveLogger) Forwardf(msg string, args ...interface{}) {
	x.WithoutPin(func() {
		x.basicLogger.Forwardf(msg, args...)
	})
}
func (x *interactiveLogger) Log(category *LogCategory, level LogLevel, msg string, args ...interface{}) {
	x.WithoutPin(func() {
		x.basicLogger.Log(category, level, msg, args...)
	})
}
func (x *interactiveLogger) Pin(msg string, args ...interface{}) PinScope {
	if enableInteractiveShell {
		pin := x.recycler.Allocate()
		pin.Log(msg, args...)
		pin.startedAt = Elapsed()
		pin.color = NewPastelizerColor(rand.Float64()).Quantize(true)
		pin.writer = pin.writeLogHeader

		x.messages.Append(pin)
		x.attachMessages()
		return pin
	}
	return basicLogPin{}
}
func (x *interactiveLogger) Progress(first, last int, msg string, args ...interface{}) ProgressScope {
	if enableInteractiveShell {
		pin := x.recycler.Allocate()
		pin.Log(msg, args...)
		pin.startedAt = Elapsed()
		pin.first = first
		pin.last.Store(int32(last))
		pin.progress.Store(0)
		pin.color = NewPastelizerColor(rand.Float64()).Quantize(true)
		pin.writer = pin.writeLogProgress

		x.messages.Append(pin)
		x.attachMessages()
		return pin
	}
	return basicLogProgress{}
}
func (x *interactiveLogger) WithoutPin(block func()) {
	if x.hasInflightMessages() {
		x.detachMessages()
		block()
		x.attachMessages()
	} else {
		block()
	}
}
func (x *interactiveLogger) Close(scope PinScope) error {
	if !IsNil(scope) {
		pin := scope.(*interactiveLogPin)
		x.messages.Remove(pin)
		x.recycler.Release(pin)
	}
	return nil
}
func (x *interactiveLogger) Flush() {
	x.basicLogger.Flush()
}
func (x *interactiveLogger) Purge() {
	x.detachMessages()
	x.basicLogger.Purge()
}
func (x *interactiveLogger) Refresh() {
	if x.hasInflightMessages() {
		x.detachMessages()
		x.basicLogger.Refresh()
		x.attachMessages()
	}
}

func (x *interactiveLogger) hasInflightMessages() bool {
	return x.inflight > 0
}
func (x *interactiveLogger) attachMessages() bool {
	if x.inflight != 0 || x.messages.Empty() {
		return false
	}

	x.inflight = len(x.messages)
	x.maxLen = 0

	// format pins in memory
	defer x.transient.Reset()
	fmt.Fprintln(&x.transient, "")

	for _, it := range x.messages {
		if it != nil {
			offset := x.transient.Len()

			fmt.Fprint(&x.transient, "\r", ANSI_ERASE_END_LINE.Always(), it.color.Ansi(true))
			{
				it.format(&x.transient)
			}
			fmt.Fprintln(&x.transient, ANSI_RESET.Always())

			if len := int(x.transient.Len() - offset); x.maxLen < len {
				x.maxLen = len
			}
		} else {
			x.inflight--
			UnreachableCode()
		}
	}

	// write all output with 1 call
	os.Stderr.Write(x.transient.Bytes())

	return true
}
func (x *interactiveLogger) detachMessages() bool {
	if x.inflight == 0 {
		return false
	}

	// format pins in memory
	defer x.transient.Reset()

	fmt.Fprint(&x.transient,
		ANSI_ERASE_ALL_LINE.Always(),
		"\033[", x.inflight+1, "F", // move cursor up  # lines
		ANSI_ERASE_SCREEN_FROM_CURSOR.Always())

	// write all output with 1 call
	os.Stderr.Write(x.transient.Bytes())

	x.inflight = 0
	return true
}

/***************************************
 * Log Progress
 ***************************************/

func writeLogCropped(dst io.Writer, buf []byte, capacity int, in string) {
	i := int(Elapsed().Seconds() * 20)
	if i < 0 {
		i = -i
	}
	for w := 0; w < capacity; i++ {
		ci := i % len(in)
		ch := in[ci]
		switch ch {
		case '\r', '\n':
			continue
		case '\t':
			ch = ' '
			fallthrough
		default:
			buf[w] = ch
			w++
		}
	}
	dst.Write(buf)
}

func (x *interactiveLogPin) writeLogHeader(lw LogWriter) {
	const width = 100
	buf := [width]byte{}
	if value := x.header.Load(); !IsNil(value) {
		writeLogCropped(lw, buf[:], width, value.(string))
	} else {
		writeLogCropped(lw, buf[:], width, "")
	}
}

func (x *interactiveLogPin) writeLogProgress(lw LogWriter) {
	const width = 50

	header := [25]byte{}
	if value := x.header.Load(); !IsNil(value) {
		writeLogCropped(lw, header[:], len(header), value.(string))
	} else {
		writeLogCropped(lw, header[:], len(header), "")
	}
	lw.WriteString(" ")

	progress := int(x.progress.Load())
	last := int(x.last.Load())

	duration := Elapsed() - x.startedAt
	t := float64(duration.Seconds()+float64(x.color.R)) * 5.0

	if x.first < last {
		// progress-bar (%)

		lw.WriteString(ANSI_FG1_WHITE.String())
		ff := float64(progress-x.first) / float64(last-x.first)
		ff = math.Max(0.0, math.Min(1.0, ff)) * (width - 1)
		f0 := math.Floor(ff)
		fi := int(f0)
		ff -= f0

		colorF := x.color.Unquantize(true)

		pgChars := []rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}

		for i := 0; i < width; i++ {
			ft := Smootherstep(math.Cos(t*1.5+float64(i)/(width-1)*math.Pi)*0.5 + 0.5)
			mi := 0.5

			fg := colorF.Brightness(ft*.15 + mi - .05).Quantize(true)
			bg := colorF.Brightness(ft*.09 + .30).Quantize(true)

			var ch rune

			if i < fi {
				ch = pgChars[len(pgChars)-1]

			} else if i == fi {
				ch = pgChars[int(ff*float64(len(pgChars)-1))]

			} else {
				ch = pgChars[0]
			}

			fmt.Fprint(lw, bg.Ansi(false), fg.Ansi(true), string(ch)) //`▁`)
		}

		lw.WriteString(ANSI_RESET.String())
		fmt.Fprintf(lw, " %6.2f%% ", ff*100) // Sprintf() escaping hell

		numElts := float32(progress - x.first)
		eltUnit := ""
		if numElts > 5000 {
			eltUnit = "K"
			numElts /= 1000
		}
		if numElts > 5000 {
			eltUnit = "M"
			numElts /= 1000
		}

		eltPerSec := numElts / float32(duration.Seconds()+0.00001)
		lw.WriteString(ANSI_FG1_YELLOW.String())
		fmt.Fprintf(lw, " %.3f %s/s", eltPerSec, eltUnit)

	} else {
		// spinner (?)

		lw.WriteString(ANSI_FG0_CYAN.String())

		pattern := []string{` `, `▁`, `▂`, `▃`, `▄`, `▅`, `▆`, `▇`}

		colorF := x.color.Unquantize(true)

		const phi float64 = 1.0 / 1.61803398875
		const gravitationalConst float64 = 9.81

		for i := 0; i < width; i++ {
			lx := math.Pi * 100 * float64(i) / (width - 1)

			var waves_x, waves_y float64

			var steepness float64 = 1.0
			var wavelength float64 = 133.0

			for i := 0; i < 6; i++ {
				k := 2.0 * math.Pi / wavelength
				c := math.Sqrt(float64(gravitationalConst / k))
				a := steepness / k
				f := k * (lx - c*t*steepness*3)

				waves_x += a * math.Sin(f)
				waves_y += a
				steepness *= 0.9
				wavelength *= phi
			}

			f := (0.5*waves_x/waves_y + 0.5)
			if f > 1 {
				f = 1
			} else if f < 0 {
				f = 0
			}

			c := int(math.Round(float64(len(pattern)-1) * f))

			fg := colorF.Brightness(Smootherstep(f)*.4 + .2).Quantize(true)
			bg := colorF.Brightness((math.Cos(t+float64(i)/(width-1)*math.Pi)*0.5+0.5)*0.2 + 0.3).Quantize(true)

			fmt.Fprint(lw, bg.Ansi(false), fg.Ansi(true), pattern[c])
		}

		lw.WriteString(ANSI_RESET.String())
		lw.WriteString(ANSI_FG0_GREEN.String())
		fmt.Fprintf(lw, " %6.2fs ", duration.Seconds())
	}

	if progress == last {
		lw.WriteString("DONE")
	}
}

/***************************************
 * Logger helpers
 ***************************************/

func PurgePinnedLogs() {
	gLogger.Purge()
}

func LogProgress(first, last int, msg string, args ...interface{}) ProgressScope {
	return gLogger.Progress(first, last, msg, args...)
}
func LogSpinner(msg string, args ...interface{}) ProgressScope {
	return gLogger.Progress(1, 0, msg, args...)
}

type BenchmarkLog struct {
	category  *LogCategory
	message   string
	startedAt time.Duration
}

func (x BenchmarkLog) Close() time.Duration {
	duration := Elapsed() - x.startedAt
	LogVeryVerbose(x.category, "benchmark: %10v   %s", duration, x.message)
	return duration
}
func LogBenchmark(category *LogCategory, msg string, args ...interface{}) BenchmarkLog {
	formatted := fmt.Sprintf(msg, args...) // before measured scope
	return BenchmarkLog{
		category:  category,
		message:   formatted,
		startedAt: Elapsed(),
	}
}

type lambdaStringer func() string

func (x lambdaStringer) String() string {
	return x()
}
func MakeStringer(fn func() string) fmt.Stringer {
	return lambdaStringer(fn)
}

func CopyWithProgress(context string, totalSize int64, dst io.Writer, src io.Reader) (err error) {
	if enableInteractiveShell {
		return TransientIoCopyWithProgress(context, totalSize, dst, src)
	} else {
		_, err := TransientIoCopy(dst, src)
		return err
	}
}

func CopyWithSpinner(context string, dst io.Writer, src io.Reader) (err error) {
	return CopyWithProgress(context, 0, dst, src)
}

type writerWithProgress struct {
	wr   io.Writer
	pbar ProgressScope
}

func (x writerWithProgress) Write(p []byte) (int, error) {
	n, err := x.wr.Write(p)
	x.pbar.Add(n)
	return n, err
}

func TransientIoCopyWithProgress(context string, totalSize int64, dst io.Writer, src io.Reader) (err error) {
	var pbar ProgressScope
	if totalSize > 0 {
		pbar = LogProgress(0, int(totalSize), "copying %s -- %.3f MiB", context, float32(totalSize)/(1024*1024))
	} else {
		pbar = LogSpinner("copying %s -- unknown size", context)
	}
	defer pbar.Close()

	_, err = TransientIoCopy(writerWithProgress{
		wr:   dst,
		pbar: pbar,
	}, src)
	return
}
