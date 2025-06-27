package base

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"math"
	"os"
	"runtime"
	"sort"
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

const LOGGER_REFRESH_PERIOD = 80 * time.Millisecond

func GetLogger() Logger { return gLogger }
func SetLogger(logger Logger) (previous Logger) {
	previous = gLogger
	gLogger = logger
	return
}

//// THOSE ARE DEFINED INSIDE Assert_Debug/Assert_NotDebug TO COMPILE OUT DEBUG/TRACE MESSAGES
// func LogDebug(category *LogCategory, msg string, args ...interface{}) {
// 	gLogger.Log(category, LOG_DEBUG, msg, args...)
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
	if !gLogWarningAsError {
		gLogger.Log(category, LOG_WARNING, msg, args...)
	} else {
		LogError(category, msg, args...)
	}
}
func LogWarningOnce(category *LogCategory, msg string, args ...interface{}) {
	if IsLogLevelActive(LOG_VERBOSE) {
		formattedMsg := fmt.Sprintf(msg, args...)
		if _, loaded := logWarningsSeenOnce.FindOrAdd(formattedMsg, 1); !loaded {
			LogWarning(category, msg, args...)
		}
	}
}
func LogWarningVerbose(category *LogCategory, msg string, args ...interface{}) {
	if IsLogLevelActive(LOG_VERBOSE) {
		LogWarning(category, msg, args...)
	}
}

func LogError(category *LogCategory, msg string, args ...interface{}) {
	gLogger.Log(category, LOG_ERROR, msg, args...)

	if gLogErrorAsPanic {
		gLogger.Flush()
		//LogPanic(category, msg, args...)
		panic(fmt.Errorf(msg, args...))
	}
}
func LogFatal(msg string, args ...interface{}) {
	gLogger.Purge()
	log.Fatalf(msg, args...)
}

func LogPanic(category *LogCategory, msg string, args ...interface{}) {
	LogPanicErr(category, fmt.Errorf(msg, args...))
}
func LogPanicErr(category *LogCategory, err error) {
	LogError(category, "💀 panic: caught error %v", err)
	FlushLog()
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

func IsLogLevelActive(level LogLevel) bool {
	return gLogger.IsVisible(level)
}
func FlushLog() {
	gLogger.Flush()
}

var gLogWarningAsError bool = false

func SetLogWarningAsError(enabled bool) {
	gLogWarningAsError = enabled
}

var gLogErrorAsPanic bool = false

func SetLogErrorAsPanic(enabled bool) {
	gLogErrorAsPanic = enabled
}

func SetLogVisibleLevel(level LogLevel) {
	gLogger.SetLevel(level)
}

/***************************************
 * Logger interface
 ***************************************/

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
	SetShowCategory(bool)
	SetShowTimestamp(bool)
	SetWriter(LogWriter)

	Forward(msg ...string)
	Forwardln(msg ...string)
	Forwardf(msg string, args ...interface{})

	Log(category *LogCategory, level LogLevel, msg string, args ...interface{})

	Write(buf []byte) (int, error)

	Pin(msg string, args ...interface{}) PinScope
	Progress(options ...ProgressOptionFunc) ProgressScope
	Close(PinScope) error

	Flush()   // wait for all pending all messages
	Purge()   // close the log and every on-going pins
	Refresh() // re-draw all pins, need for animated effects
}

type PinScope interface {
	Log(msg string, args ...interface{})
	Closable

	format(LogWriter)
}

type ProgressScope interface {
	Progress() int64
	Len() int64
	Grow(int64)
	Add(int64)
	Inc()
	Set(int64)
	PinScope
}

type ProgressOptions struct {
	Text        string
	First, Last int64
	Color       Color3b
}

type ProgressOptionFunc func(*ProgressOptions)

func ProgressOptionText(text string) ProgressOptionFunc {
	return func(po *ProgressOptions) {
		po.Text = text
	}
}
func ProgressOptionFormat(msg string, args ...interface{}) ProgressOptionFunc {
	return func(po *ProgressOptions) {
		po.Text = fmt.Sprintf(msg, args...)
	}
}
func ProgressOptionRange(first, last int64) ProgressOptionFunc {
	return func(po *ProgressOptions) {
		po.First = first
		po.Last = last
	}
}
func ProgressOptionColor(color Color3b) ProgressOptionFunc {
	return func(po *ProgressOptions) {
		po.Color = color
	}
}

/***************************************
 * Errors
 ***************************************/

func MakeError(msg string, args ...interface{}) error {
	//LogError(LogGlobal, msg, args...) # DONT -> this can lock recursively the logger
	return fmt.Errorf(msg, args...)
}

func MakeUnexpectedValueError(dst interface{}, any interface{}) (err error) {
	err = MakeError("unexpected <%T> value: %#v", dst, any)
	AssertErr(func() error {
		return err
	})
	return
}
func UnexpectedValuePanic(dst interface{}, any interface{}) {
	LogPanicErr(LogGlobal, MakeUnexpectedValueError(dst, any))
}

/***************************************
 * Log Forward Writer
 ***************************************/

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

func (x *LogManager) SetCategoryLevel(name LogCategoryName, level LogLevel) error {
	if category := x.FindOrAddCategory(name); category != nil {
		category.Level = level
		return nil
	} else {
		return fmt.Errorf("unknown log category: %q", name)
	}
}
func (x *LogManager) FindCategory(name LogCategoryName) *LogCategory {
	x.barrierRW.RLock()
	defer x.barrierRW.RUnlock()
	return x.categories[name.String()]
}
func (x *LogManager) FindOrAddCategory(name LogCategoryName) (result *LogCategory) {
	if result = x.FindCategory(name); result == nil {
		categoryKey := name.String()
		x.barrierRW.Lock()
		defer x.barrierRW.Unlock()
		if result = x.categories[categoryKey]; result == nil {
			category := MakeLogCategory(categoryKey)
			result = &category
			x.categories[categoryKey] = result
		}
	}
	return
}
func (x *LogManager) CategoryRange(each func(*LogCategory) error) error {
	x.barrierRW.RLock()
	defer x.barrierRW.RUnlock()
	for _, category := range x.categories {
		if err := each(category); err != nil {
			return err
		}
	}
	return nil
}

/***************************************
 * Log Category
 ***************************************/

func MakeLogCategory(name string) LogCategory {
	sum64a := fnv.New64a()
	sum64a.Write(UnsafeBytesFromString(name))
	sum64a.Write(UnsafeBytesFromString("%%cateogry"))
	hash := sum64a.Sum64()
	return LogCategory{
		Name:  name,
		Level: LOG_FATAL,
		Hash:  hash,
		Color: NewColorFromHash(hash).Quantize(),
	}
}

func NewLogCategory(name string) *LogCategory {
	return gLogManager.FindOrAddCategory(LogCategoryName{InheritableString: InheritableString(name)})
}

type LogCategoryName struct {
	InheritableString
}

type LogCategorySet = InheritableSlice[LogCategoryName, *LogCategoryName]

func (x LogCategoryName) Equals(o LogCategoryName) bool {
	return x.InheritableString.Equals(o.InheritableString)
}
func (x LogCategoryName) AutoComplete(in AutoComplete) {
	err := GetLogManager().CategoryRange(func(lc *LogCategory) error {
		in.Add(lc.Name, lc.Level.String())
		return nil
	})
	if err != nil {
		Panic(err)
	}
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
		fmt.Fprint(dst, ANSI_FG1_WHITE)
	case LOG_CLAIM:
		fmt.Fprint(dst, ANSI_FG1_GREEN, ANSI_BOLD)
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
	case LOG_DEBUG:
		fmt.Fprint(dst, "🐜 ")
	case LOG_TRACE:
		fmt.Fprint(dst, "👣 ")
	case LOG_VERYVERBOSE:
		fmt.Fprint(dst, "👥 ")
	case LOG_VERBOSE:
		fmt.Fprint(dst, "🗣️ ")
	case LOG_INFO:
		fmt.Fprint(dst, "🔹 ")
	case LOG_CLAIM:
		fmt.Fprint(dst, "❇️ ")
	case LOG_WARNING:
		fmt.Fprint(dst, "⚠️ ")
	case LOG_ERROR:
		fmt.Fprint(dst, "❌ ")
	case LOG_FATAL:
		fmt.Fprint(dst, "💀 ")
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

func (x basicLogProgress) Progress() int64 { return 0 }
func (x basicLogProgress) Len() int64      { return 0 }
func (x basicLogProgress) Grow(int64)      {}
func (x basicLogProgress) Add(int64)       {}
func (x basicLogProgress) Inc()            {}
func (x basicLogProgress) Set(int64)       {}

func NewDummyLogProgress() ProgressScope {
	return basicLogProgress{}
}

type basicLogger struct {
	MinimumLevel  LogLevel
	ShowCategory  bool
	ShowTimestamp bool
	Writer        LogWriter

	lastFlush time.Time
}

func newBasicLogger() *basicLogger {
	level := LOG_INFO
	if EnableDiagnostics() {
		level = LOG_ALL
	}

	logger := basicLogger{
		MinimumLevel:  level,
		ShowCategory:  true,
		ShowTimestamp: false,
		Writer:        os.Stdout,
		lastFlush:     time.Now(),
	}

	return &logger
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
func (x *basicLogger) SetShowCategory(enabled bool) {
	x.ShowCategory = enabled
}
func (x *basicLogger) SetShowTimestamp(enabled bool) {
	x.ShowTimestamp = enabled
}
func (x *basicLogger) SetWriter(dst LogWriter) {
	Assert(func() bool { return !IsNil(dst) })
	x.Flush()
	x.Writer = dst
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
		fmt.Fprintln(x.Writer, "")
	}

	x.Flush()
}
func (x *basicLogger) Forwardf(msg string, args ...interface{}) {
	fmt.Fprintf(x.Writer, msg, args...)
	fmt.Fprintln(x.Writer, "")

	x.Flush()
}

func (x *basicLogger) Log(category *LogCategory, level LogLevel, msg string, args ...interface{}) {
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

	fmt.Fprintln(x.Writer, ANSI_RESET.String())

	x.Flush()
}

func (x *basicLogger) Write(buf []byte) (int, error) {
	return x.Writer.Write(buf)
}

func (x *basicLogger) Pin(msg string, args ...interface{}) PinScope {
	return basicLogPin{} // see interactiveLogger struct
}
func (x *basicLogger) Progress(opts ...ProgressOptionFunc) ProgressScope {
	return basicLogProgress{} // see interactiveLogger struct
}
func (x *basicLogger) Close(pin PinScope) error {
	UnreachableCode() // see interactiveLogger structa
	return nil
}

func (x *basicLogger) Flush() {
	if err := FlushWriterIFP(x.Writer); err != nil {
		panic(err)
	}
}
func (x *basicLogger) Purge()   { x.Flush() }
func (x *basicLogger) Refresh() { x.Flush() }

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

func (x deferredProgressScope) Progress() int64 { return x.future.Join().Success().Progress() }
func (x deferredProgressScope) Len() int64      { return x.future.Join().Success().Len() }

func (x deferredProgressScope) Grow(n int64) {
	x.future.Join().Success().Grow(n)
}
func (x deferredProgressScope) Add(n int64) {
	x.future.Join().Success().Add(n)
}
func (x deferredProgressScope) Inc() {
	x.future.Join().Success().Inc()
}
func (x deferredProgressScope) Set(v int64) {
	x.future.Join().Success().Set(v)
}

type deferredLogger struct {
	logger  Logger
	thread  ThreadPool
	barrier *sync.Mutex
}

func newDeferredLogger(logger Logger) *deferredLogger {
	barrier := &sync.Mutex{}
	return &deferredLogger{
		logger:  logger,
		barrier: barrier,
		thread: NewFixedSizeThreadPoolEx("logger", 1,
			func(threadContext ThreadContext, queue TaskPriorityQueue) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				if logger.IsInteractive() {
					go func() {
						for {
							select {
							case <-ctx.Done():
								return
							case <-time.After(LOGGER_REFRESH_PERIOD):
								queue.Push(TaskQueued{
									Func: func(_ ThreadContext) {
										logger.Refresh()
									},
									DebugId: ThreadPoolDebugId{Category: "logger.Refresh"},
								}, TASKPRIORITY_LOW)
							}
						}
					}()
				}

				for {
					if task := queue.Pop(); task.Func != nil {
						barrier.Lock()
						task.Func(threadContext)
						barrier.Unlock()
					} else {
						break
					}
				}
			})}
}

func (x *deferredLogger) IsInteractive() bool {
	return x.logger.IsInteractive()
}
func (x *deferredLogger) IsVisible(level LogLevel) bool {
	return x.logger.IsVisible(level)
}

func (x *deferredLogger) SetLevel(level LogLevel) LogLevel {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.SetLevel(level)
}
func (x *deferredLogger) SetLevelMinimum(level LogLevel) LogLevel {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.SetLevelMinimum(level)
}
func (x *deferredLogger) SetLevelMaximum(level LogLevel) LogLevel {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.SetLevelMaximum(level)
}
func (x *deferredLogger) SetShowCategory(enabled bool) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.SetShowCategory(enabled)
}
func (x *deferredLogger) SetShowTimestamp(enabled bool) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.SetShowTimestamp(enabled)
}
func (x *deferredLogger) SetWriter(dst LogWriter) {
	x.thread.Queue(func(ThreadContext) {
		x.logger.SetWriter(dst)
	}, TASKPRIORITY_NORMAL, ThreadPoolDebugId{})
}

func (x *deferredLogger) Forward(msg ...string) {
	x.thread.Queue(func(ThreadContext) {
		x.logger.Forward(msg...)
	}, TASKPRIORITY_LOW, ThreadPoolDebugId{})
}
func (x *deferredLogger) Forwardln(msg ...string) {
	x.thread.Queue(func(ThreadContext) {
		x.logger.Forwardln(msg...)
	}, TASKPRIORITY_LOW, ThreadPoolDebugId{})
}
func (x *deferredLogger) Forwardf(msg string, args ...interface{}) {
	x.thread.Queue(func(ThreadContext) {
		x.logger.Forwardf(msg, args...)
	}, TASKPRIORITY_LOW, ThreadPoolDebugId{})
}
func (x *deferredLogger) Log(category *LogCategory, level LogLevel, msg string, args ...interface{}) {
	if x.logger.IsVisible(level) || category.Level.IsVisible(level) {
		x.thread.Queue(func(ThreadContext) {
			x.logger.Log(category, level, msg, args...)
		}, TASKPRIORITY_LOW, ThreadPoolDebugId{})
	}
	if level >= LOG_ERROR {
		x.thread.Join() // flush log when an error occurred
	}
}
func (x *deferredLogger) Write(buf []byte) (n int, err error) {
	x.thread.Queue(func(tc ThreadContext) {
		n, err = x.logger.Write(buf)
		if err == nil {
			x.logger.Flush()
		}
	}, TASKPRIORITY_NORMAL, ThreadPoolDebugId{})
	x.thread.Join()
	return
}
func (x *deferredLogger) Pin(msg string, args ...interface{}) PinScope {
	return deferredPinScope{
		future: MakeWorkerFuture(x.thread, func(ThreadContext) (PinScope, error) {
			pin := x.logger.Pin(msg, args...)
			return pin, nil
		}, TASKPRIORITY_HIGH, ThreadPoolDebugId{})}
}
func (x *deferredLogger) Progress(opts ...ProgressOptionFunc) ProgressScope {
	return deferredProgressScope{
		future: MakeWorkerFuture(x.thread, func(ThreadContext) (ProgressScope, error) {
			pin := x.logger.Progress(opts...)
			return pin, nil
		}, TASKPRIORITY_HIGH, ThreadPoolDebugId{})}
}
func (x *deferredLogger) Close(pin PinScope) error {
	x.thread.Queue(func(ThreadContext) {
		if err := x.logger.Close(pin); err != nil {
			Panic(err)
		}
	}, TASKPRIORITY_LOW, ThreadPoolDebugId{})
	return nil
}
func (x *deferredLogger) Flush() {
	x.thread.Queue(func(ThreadContext) {
		x.logger.Flush()
	}, TASKPRIORITY_LOW, ThreadPoolDebugId{})
	x.thread.Join()
}
func (x *deferredLogger) Purge() {
	x.thread.Queue(func(ThreadContext) {
		x.logger.Purge()
	}, TASKPRIORITY_LOW, ThreadPoolDebugId{})
	x.thread.Join()
}
func (x *deferredLogger) Refresh() {
	x.thread.Queue(func(ThreadContext) {
		x.logger.Refresh()
	}, TASKPRIORITY_LOW, ThreadPoolDebugId{})
}

/***************************************
 * Immediate Logger
 ***************************************/

type immediateLogger struct {
	logger  Logger
	barrier *sync.Mutex
}

func newImmediateLogger(logger Logger) *immediateLogger {
	barrier := &sync.Mutex{}
	return &immediateLogger{
		logger:  logger,
		barrier: barrier,
	}
}

func (x *immediateLogger) IsInteractive() bool {
	return x.logger.IsInteractive()
}
func (x *immediateLogger) IsVisible(level LogLevel) bool {
	return x.logger.IsVisible(level)
}

func (x *immediateLogger) SetLevel(level LogLevel) LogLevel {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.SetLevel(level)
}
func (x *immediateLogger) SetLevelMinimum(level LogLevel) LogLevel {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.SetLevelMinimum(level)
}
func (x *immediateLogger) SetLevelMaximum(level LogLevel) LogLevel {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.SetLevelMaximum(level)
}
func (x *immediateLogger) SetShowCategory(enabled bool) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.SetShowCategory(enabled)
}
func (x *immediateLogger) SetShowTimestamp(enabled bool) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.SetShowTimestamp(enabled)
}
func (x *immediateLogger) SetWriter(dst LogWriter) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.SetWriter(dst)
}

func (x *immediateLogger) Forward(msg ...string) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.Forward(msg...)
}
func (x *immediateLogger) Forwardln(msg ...string) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.Forwardln(msg...)
}
func (x *immediateLogger) Forwardf(msg string, args ...interface{}) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.Forwardf(msg, args...)
}
func (x *immediateLogger) Log(category *LogCategory, level LogLevel, msg string, args ...interface{}) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	if category.Level.IsVisible(level) || x.logger.IsVisible(level) {
		x.logger.Log(category, level, msg, args...)
	}
}
func (x *immediateLogger) Write(buf []byte) (int, error) {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.Write(buf)
}
func (x *immediateLogger) Pin(msg string, args ...interface{}) PinScope {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.Pin(msg, args...)
}
func (x *immediateLogger) Progress(opts ...ProgressOptionFunc) ProgressScope {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.Progress(opts...)
}
func (x *immediateLogger) Close(pin PinScope) error {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return x.logger.Close(pin)
}
func (x *immediateLogger) Flush() {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.Flush()
}
func (x *immediateLogger) Purge() {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.Purge()
}
func (x *immediateLogger) Refresh() {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	x.logger.Refresh()
}

/***************************************
 * Interactive Logger
 ***************************************/

var enableInteractiveShell bool = true

func EnableInteractiveShell() bool {
	return enableInteractiveShell
}
func SetEnableInteractiveShell(enabled bool) {
	if enableInteractiveShell {
		enableInteractiveShell = enabled
	}
}

type interactiveLogPin struct {
	header atomic.Value
	writer func(LogWriter)

	tick      int
	first     int64
	last      atomic.Int64
	progress  atomic.Int64
	startedAt time.Duration
	avgSpeed  float64

	color Color3b
}

func (x *interactiveLogPin) isProgressBar() bool {
	return x.first <= x.last.Load()
}

func (x *interactiveLogPin) reset() {
	x.header.Store("")
	x.writer = nil
	x.tick = 0
	x.startedAt = 0
	x.avgSpeed = 0
	x.progress.Store(0)
}
func (x *interactiveLogPin) format(dst LogWriter) {
	if x.writer != nil {
		x.writer(dst)
	}
}

func (x *interactiveLogPin) Log(msg string, args ...interface{}) {
	var text string
	if len(args) > 0 {
		text = fmt.Sprintf(msg+" ", args...)
	} else {
		text = msg + " "
	}
	x.header.Store(text)
}
func (x *interactiveLogPin) Close() error {
	return gLogger.Close(x)
}

func (x *interactiveLogPin) Progress() int64 {
	return x.progress.Load() - x.first
}
func (x *interactiveLogPin) Len() int64 {
	return x.last.Load() - x.first
}

func (x *interactiveLogPin) Grow(n int64) {
	x.last.Add(n)
}
func (x *interactiveLogPin) Add(n int64) {
	x.progress.Add(n)
}
func (x *interactiveLogPin) Inc() {
	x.progress.Add(1)
}
func (x *interactiveLogPin) Set(v int64) {
	for {
		prev := x.progress.Load()
		if prev > v || x.progress.CompareAndSwap(prev, v) {
			break
		}
	}
}

var interactiveLoggerOutput = os.Stderr

type interactiveWriter struct {
	logger *interactiveLogger
	output LogWriter
}

func (x *interactiveWriter) Write(buf []byte) (n int, err error) {
	if x.logger.hasInflightMessages() {
		x.logger.detachMessages()
		n, err = x.output.Write(buf)
		if err == nil {
			err = FlushWriterIFP(x.output)
		}
		x.logger.attachMessages()
		x.logger.lastRefresh = time.Now()
	} else {
		n, err = x.output.Write(buf)
	}
	return
}

type interactiveLogger struct {
	messages    SetT[*interactiveLogPin]
	inflight    int
	lastRefresh time.Time

	colors    ColorGenerator
	recycler  Recycler[*interactiveLogPin]
	transient bytes.Buffer
	*basicLogger
}

func newInteractiveLogger(basic *basicLogger) *interactiveLogger {
	result := &interactiveLogger{
		messages:    make([]*interactiveLogPin, 0, runtime.NumCPU()),
		inflight:    0,
		colors:      MakeColorGenerator(),
		basicLogger: basic,
		recycler: NewRecycler(
			func() *interactiveLogPin {
				return new(interactiveLogPin)
			},
			func(ip *interactiveLogPin) {
				ip.reset()
			}),
	}
	result.basicLogger.Writer = bufio.NewWriterSize(&interactiveWriter{
		logger: result,
		output: basic.Writer,
	}, 4096)
	interactiveLoggerOutput.WriteString(ANSI_HIDE_CURSOR.Always())
	return result
}
func (x *interactiveLogger) IsInteractive() bool {
	return true
}
func (x *interactiveLogger) Forward(msg ...string) {
	x.basicLogger.Forward(msg...)
}
func (x *interactiveLogger) Forwardln(msg ...string) {
	x.basicLogger.Forwardln(msg...)
}
func (x *interactiveLogger) Forwardf(msg string, args ...interface{}) {
	x.basicLogger.Forwardf(msg, args...)
}
func (x *interactiveLogger) Log(category *LogCategory, level LogLevel, msg string, args ...interface{}) {
	x.basicLogger.Log(category, level, msg, args...)
}
func (x *interactiveLogger) Pin(msg string, args ...interface{}) PinScope {
	if EnableInteractiveShell() {
		pin := x.recycler.Allocate()
		pin.Log(msg, args...)
		pin.first = 1 // considered as a spinner
		pin.startedAt = Elapsed()
		pin.color = x.colors.Next().Quantize()
		pin.writer = pin.writeLogHeader

		x.refreshMessages(func() {
			x.messages.Append(pin)
		})
		return pin
	}
	return basicLogPin{}
}
func (x *interactiveLogger) Progress(opts ...ProgressOptionFunc) ProgressScope {
	if EnableInteractiveShell() {
		po := ProgressOptions{}
		po.Color = x.colors.Next().Quantize()
		for _, it := range opts {
			it(&po)
		}

		pin := x.recycler.Allocate()
		pin.Log(po.Text)
		pin.startedAt = Elapsed()
		pin.first = po.First
		pin.last.Store(po.Last)
		pin.progress.Store(po.First)
		pin.color = po.Color
		pin.writer = pin.writeLogProgress

		x.refreshMessages(func() {
			x.messages.Append(pin)
		})
		return pin
	}
	return basicLogProgress{}
}
func (x *interactiveLogger) Close(scope PinScope) error {
	if !IsNil(scope) {
		x.refreshMessages(func() {
			pin := scope.(*interactiveLogPin)
			x.messages.Remove(pin)
			x.recycler.Release(pin)
		})
	}
	return nil
}
func (x *interactiveLogger) Refresh() {
	if x.hasInflightMessages() {
		x.refreshMessages(nil)
	}
}
func (x *interactiveLogger) Flush() {
	x.basicLogger.Flush()
}
func (x *interactiveLogger) Purge() {
	x.basicLogger.Purge()
	x.detachMessages()
	interactiveLoggerOutput.WriteString(ANSI_SHOW_CURSOR.Always())
}
func (x *interactiveLogger) Write(buf []byte) (n int, err error) {
	return x.basicLogger.Write(buf)
}

func (x *interactiveLogger) refreshMessages(inner func()) {
	now := time.Now()
	if now.Sub(x.lastRefresh) < LOGGER_REFRESH_PERIOD {
		if inner != nil {
			inner()
		}
		return
	}
	x.lastRefresh = now

	defer x.transient.Reset()
	prepareDetachMessages(&x.transient, x.inflight)
	if inner != nil {
		inner()
	}
	x.inflight = prepareAttachMessages(&x.transient, x.messages...)
	interactiveLoggerOutput.Write(x.transient.Bytes())
	FlushWriterIFP(interactiveLoggerOutput)
}
func (x *interactiveLogger) hasInflightMessages() bool {
	return x.inflight > 0
}

func prepareAttachMessages(buf LogWriter, messages ...*interactiveLogPin) (inflight int) {
	sort.SliceStable(messages, func(i, j int) bool {
		a := messages[i]
		b := messages[j]
		if a.isProgressBar() != b.isProgressBar() {
			return a.isProgressBar() && !b.isProgressBar()
		} else {
			return a.startedAt < b.startedAt
		}
	})

	inflight = 1 + len(messages)
	buf.WriteString(ANSI_DISABLE_LINE_WRAPPING.Always())

	fmt.Fprintln(buf, "\033[2K\r") // spacer line

	for i := range messages {
		it := messages[len(messages)-1-i]

		if i > 0 && it.isProgressBar() && !messages[len(messages)-i].isProgressBar() {
			fmt.Fprintln(buf, "\033[2K\r") // spacer line
			inflight++
		}

		fmt.Fprint(buf, "\033[2K\r", it.color.Ansi(true)) // Clear line and set color
		{
			it.format(buf)
		}
		fmt.Fprintln(buf, ANSI_RESET.Always()) // Reset color
	}

	buf.WriteString(ANSI_RESTORE_LINE_WRAPPING.Always())
	return
}
func prepareDetachMessages(buf LogWriter, inflight int) {
	if inflight > 0 {
		fmt.Fprint(buf,
			ANSI_DISABLE_LINE_WRAPPING.Always(),
			ANSI_ERASE_ALL_LINE.Always(),
			"\033[", inflight, "F", // move cursor up # lines
			ANSI_ERASE_SCREEN_FROM_CURSOR.Always(),
			ANSI_RESTORE_LINE_WRAPPING.Always())
	}
}

func (x *interactiveLogger) attachMessages() bool {
	if x.inflight != 0 || x.messages.Empty() {
		return false
	}

	// format pins in memory
	defer x.transient.Reset()
	x.inflight = prepareAttachMessages(&x.transient, x.messages...)

	// write all output with 1 call
	interactiveLoggerOutput.Write(x.transient.Bytes())

	return true
}
func (x *interactiveLogger) detachMessages() bool {
	if x.inflight == 0 {
		return false
	}

	// format pins in memory
	defer x.transient.Reset()
	prepareDetachMessages(&x.transient, x.inflight)

	// write all output with 1 call
	interactiveLoggerOutput.Write(x.transient.Bytes())

	x.inflight = 0
	return true
}

/***************************************
 * Log Progress
 ***************************************/

func writeLogCropped(dst LogWriter, capacity int, in string) {
	i := int(Elapsed().Seconds() * 13)
	if i < 0 {
		i = -i
	}
	for w := 0; w < capacity; i++ {
		ci := i % len(in)
		switch in[ci] {
		case '\r', '\n':
			continue
		case '\t':
			_, err := dst.WriteString(" ")
			if err != nil {
				panic(err)
			}
			w++
		default:
			_, err := dst.WriteString(in[ci : ci+1])
			if err != nil {
				panic(err)
			}
			w++
		}
	}
}

func (x *interactiveLogPin) writeLogHeader(lw LogWriter) {
	const width = 100

	if value := x.header.Load(); !IsNil(value) {
		writeLogCropped(lw, width, value.(string))
	} else {
		writeLogCropped(lw, width, "")
	}
}

var logProgressPattern = []string{" ", "▏", "▎", "▍", "▌", "▋", "▊", "▉", "▉", "█"}
var logSpinnerPattern = []string{" ⠏ ", " ⠛ ", " ⠹ ", " ⢸ ", " ⣰ ", " ⣤ ", " ⣆ ", " ⡇ "}

func (x *interactiveLogPin) writeLogProgress(lw LogWriter) {
	progress := x.progress.Load()
	last := x.last.Load()

	duration := max(Elapsed()-x.startedAt, 0)
	t := float64(duration.Seconds()+float64(x.color.R)) * 5.0

	const width = 50

	if x.isProgressBar() {
		// progress-bar (%)

		if value := x.header.Load(); !IsNil(value) {
			writeLogCropped(lw, 30, value.(string))
		} else {
			writeLogCropped(lw, 30, "")
		}

		lw.WriteString(" ")

		pf := float64(progress-x.first) / (1e-8 + float64(last-x.first))

		ff := math.Max(0.0, math.Min(1.0, pf)) * width
		f0 := math.Floor(ff)
		fi := int(f0)
		ff -= f0

		colorF := x.color.Unquantize()
		ft := 0.5 //Smootherstep(math.Cos(t*1.5)*0.5 + 0.5)
		mi := 0.5

		if ansiColorMode == ANSICOLOR_256COLORS {
			ft = 0.3 // avoid time animations with 256 bits color
		}

		fg := colorF.Brightness(ft*0.07 + mi - 0.05).Quantize()
		bg := colorF.Brightness(ft*0.05 + 0.28).Quantize()

		fmt.Fprint(lw, bg.Ansi(false), fg.Ansi(true))

		for i := 0; i < width; i++ {
			var ch string
			if i < fi {
				ch = logProgressPattern[len(logProgressPattern)-2]
			} else if i == fi {
				ch = logProgressPattern[int(math.Round(ff*float64(len(logProgressPattern)-1)))]
			} else {
				ch = logProgressPattern[0]
			}

			lw.WriteString(ch)
		}

		lw.WriteString(ANSI_RESET.String())
		fmt.Fprintf(lw, " %6.2f%% ", pf*100)

		if numElts := float64(progress - x.first); numElts > 0 {
			eltUnitDiv := 1.0
			eltUnitStr := ""
			if numElts > 5000 {
				eltUnitStr = "K"
				eltUnitDiv *= 1000
				numElts /= 1000
			}
			if numElts > 5000 {
				eltUnitStr = "M"
				eltUnitDiv *= 1000
				numElts /= 1000
			}

			if speed := float64(progress-x.first) / float64(duration.Seconds()+1e-6); x.avgSpeed == 0 {
				x.avgSpeed = speed
			} else {
				const alpha = 0.2
				x.avgSpeed = alpha*speed + (1-alpha)*x.avgSpeed
			}

			lw.WriteString(ANSI_FG0_YELLOW.String())
			fmt.Fprintf(lw, "%8.2f %s/s", x.avgSpeed/eltUnitDiv, eltUnitStr)

			// remove remaining time until estimation is changed to use a moving average of speed per item based of each item duration, instead of whole duration divided by count of actions
			// remainingTime := time.Duration(float64((last-progress)*int64(time.Second)) / (x.avgSpeed + 1e-6)).
			// 	Round(10 * time.Millisecond) // restrict precision

			// lw.WriteString(ANSI_FG0_GREEN.String())
			// fmt.Fprintf(lw, "  -%v", remainingTime)
		}

	} else {
		ti := int(math.Abs(t*3)) % len(logSpinnerPattern)

		fmt.Fprint(lw, logSpinnerPattern[ti])

		heat := Smootherstep(duration.Seconds() / Elapsed().Seconds()) // use percent of blocking duration
		fmt.Fprint(lw, FormatAnsiColdHotColor(heat, true))
		fmt.Fprintf(lw, "%6.2fs ", duration.Seconds())
		fmt.Fprint(lw, x.color.Ansi(true))

		var header string
		if it := x.header.Load(); !IsNil(it) {
			header = it.(string)
		} else {
			return
		}

		lw.WriteString(header)
	}
}

/***************************************
 * Logger helpers
 ***************************************/

func PurgePinnedLogs() {
	gLogger.Purge()
}

func LogProgress(first, last int64, msg string, args ...interface{}) ProgressScope {
	return gLogger.Progress(
		ProgressOptionRange(first, last),
		ProgressOptionFormat(msg, args...))
}
func LogProgressEx(opts ...ProgressOptionFunc) ProgressScope {
	return gLogger.Progress(opts...)
}
func LogSpinner(msg string, args ...interface{}) ProgressScope {
	return LogProgress(1, 0, msg, args...)
}
func LogSpinnerEx(opts ...ProgressOptionFunc) ProgressScope {
	return gLogger.Progress(append(opts, ProgressOptionRange(1, 0))...)
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

func CopyWithProgress(ctx context.Context, context string, totalSize int64, dst io.Writer, src io.Reader) (err error) {
	pageAlloc := GetBytesRecyclerBySize(totalSize)

	if EnableInteractiveShell() {
		_, err = TransientIoCopyWithProgress(ctx, context, totalSize, dst, src, pageAlloc)
	} else {
		_, err = TransientIoCopy(ctx, dst, src, pageAlloc, totalSize > int64(pageAlloc.Stride()))
	}
	return
}
