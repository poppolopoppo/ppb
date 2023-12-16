package io

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

var LogChromeTrace = base.NewLogCategory("ChromeTrace")

/***************************************
 * Chrome Tracing flags
 ***************************************/

type ChromeTracingFlags struct {
	Enabled    utils.BoolVar
	OutputFile utils.Filename
}

var GetChromeTracingFlags = func() func() *ChromeTracingFlags {
	flags := &ChromeTracingFlags{}
	return utils.NewGlobalCommandParsableFlags(
		"chrome tracing options",
		flags,
		utils.OptionCommandPrepare(func(cc utils.CommandContext) error {
			if !flags.Enabled.Get() {
				return nil
			}

			if flags.OutputFile == (utils.Filename{}) {
				if workingDir, err := utils.UFS.GetWorkingDir(); err == nil {
					flags.OutputFile = workingDir.File("chrome_trace.json")
				} else {
					return err
				}
			}

			base.LogClaim(LogChromeTrace, "start chrome trace recording in %q", flags.OutputFile)

			utils.CommandEnv.OnBuildGraphLoaded(func(bg utils.BuildGraph) error {
				chromeTracingFile := NewThreadSafeChromeTracing(runtime.NumCPU())

				bg.OnBuildNodeStart(func(bn utils.BuildNode) error {
					chromeTracingFile.Event(ChromeTracingBegin(bn.Alias().String(), base.GetTypename(bn.GetBuildable())))
					return nil
				})

				bg.OnBuildNodeFinished(func(bn utils.BuildNode) error {
					chromeTracingFile.Event(ChromeTracingEnd(bn.Alias().String(), base.GetTypename(bn.GetBuildable())))
					return nil
				})

				utils.CommandEnv.OnExit(func(cet *utils.CommandEnvT) error {
					base.LogClaim(LogChromeTrace, "write chrome trace recording to %q", flags.OutputFile)

					return utils.UFS.CreateBuffered(flags.OutputFile, func(w io.Writer) error {
						return chromeTracingFile.ExportJson(w)
					})
				})
				return nil
			})

			return nil
		}))
}()

func (flags *ChromeTracingFlags) Flags(cfv utils.CommandFlagsVisitor) {
	cfv.Variable("ChromeTrace", fmt.Sprintf("enable chrome tracing export (default path: %q)", flags.OutputFile), &flags.Enabled)
	cfv.Variable("ChromeTraceFile", "save chrome tracing file at designated location", &flags.OutputFile)
}

/***************************************
 * Chrome Tracing file format
 * https://docs.google.com/document/d/1CvAClvFfyA5R-PhYUmn5OOQtYMH4h6I0nSsKchNAySU/preview#heading=h.yr4qxyxotyw
 ***************************************/

type ChromeTracing interface {
	Event(ChromeTracingEvent)
	ExportJson(w io.Writer) error
}

type ChromeTracingPhase rune

const (
	CHROMETRACING_PHASE_BEGIN              ChromeTracingPhase = 'B'
	CHROMETRACING_PHASE_END                ChromeTracingPhase = 'E'
	CHROMETRACING_PHASE_COMPLETE           ChromeTracingPhase = 'X'
	CHROMETRACING_PHASE_INSTANT            ChromeTracingPhase = 'i'
	CHROMETRACING_PHASE_COUNTER            ChromeTracingPhase = 'C'
	CHROMETRACING_PHASE_ASYNC_BEGIN        ChromeTracingPhase = 'b'
	CHROMETRACING_PHASE_ASYNC_INSTANT      ChromeTracingPhase = 'n'
	CHROMETRACING_PHASE_ASYNC_END          ChromeTracingPhase = 'e'
	CHROMETRACING_PHASE_FLOW_BEGIN         ChromeTracingPhase = 's'
	CHROMETRACING_PHASE_FLOW_STEP          ChromeTracingPhase = 't'
	CHROMETRACING_PHASE_FLOW_END           ChromeTracingPhase = 'f'
	CHROMETRACING_PHASE_SAMPLE             ChromeTracingPhase = 'p'
	CHROMETRACING_PHASE_OBJECT_CREATED     ChromeTracingPhase = 'N'
	CHROMETRACING_PHASE_OBJECT_SNAPSHOT    ChromeTracingPhase = 'O'
	CHROMETRACING_PHASE_OBJECT_DESTROYED   ChromeTracingPhase = 'D'
	CHROMETRACING_PHASE_METADATA           ChromeTracingPhase = 'M'
	CHROMETRACING_PHASE_MEMORYDUMP_GLOBAL  ChromeTracingPhase = 'V'
	CHROMETRACING_PHASE_MEMORYDUMP_PROCESS ChromeTracingPhase = 'v'
	CHROMETRACING_PHASE_MARK               ChromeTracingPhase = 'R'
	CHROMETRACING_PHASE_CLOCKSYNC          ChromeTracingPhase = 'c'
	CHROMETRACING_PHASE_CONTEXT            ChromeTracingPhase = ','
)

type ChromeTracingEvent struct {
	Name      string                 `json:"name"`
	Category  string                 `json:"cat"`
	Phase     ChromeTracingPhase     `json:"ph"`
	Timestamp int64                  `json:"ts"`
	Pid       int64                  `json:"pid"`
	Tid       int64                  `json:"tid"`
	Arguments map[string]interface{} `json:"args,omitempty"`
}

type ChromeTracingFile struct {
	TraceEvents     []ChromeTracingEvent `json:"traceEvents"`
	DisplayTimeUnit string               `json:"displayTimeUnit"`

	pid int64
}

func NewChromeTracingFile() *ChromeTracingFile {
	return &ChromeTracingFile{
		pid:             int64(os.Getpid()),
		DisplayTimeUnit: "ms",
	}
}
func (x *ChromeTracingFile) Event(e ChromeTracingEvent) {
	e.Pid = x.pid
	x.TraceEvents = append(x.TraceEvents, e)
}
func (x *ChromeTracingFile) ExportJson(w io.Writer) error {
	return base.JsonSerialize(x, w, base.OptionJsonPrettyPrint(false))
}

/***************************************
 * Chrome tracing phase enum
 ***************************************/

func (x ChromeTracingPhase) String() string {
	return string((rune)(x))
}
func (x ChromeTracingPhase) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}

/***************************************
 * Chrome trace events formating
 ***************************************/

const USE_THREADID_AS_TID = false // choose your poison between thread id or goroutine id: neither works well for scoping in GO due to how the scheduler works :'(

func NewChromeTracingEvent(name, category string, phase ChromeTracingPhase) ChromeTracingEvent {
	var tid int64
	if USE_THREADID_AS_TID {
		tid = int64(base.GetCurrentThreadId())
	} else {
		tid, _ = binary.Varint(bytes.Fields(debug.Stack())[1]) //,
	}
	return ChromeTracingEvent{
		//Pid: initialized with value cached in ChromeTracingFile
		Name:      name,
		Category:  category,
		Phase:     phase,
		Tid:       tid,
		Timestamp: base.Elapsed().Microseconds(),
	}
}
func ChromeTracingBegin(name, category string) ChromeTracingEvent {
	return NewChromeTracingEvent(name, category, CHROMETRACING_PHASE_BEGIN)
}
func ChromeTracingEnd(name, category string) ChromeTracingEvent {
	return NewChromeTracingEvent(name, category, CHROMETRACING_PHASE_END)
}

/***************************************
 * Thread-safe chrome tracing with sharding
 ***************************************/

type threadSafeChromeTracingShard struct {
	sync.Mutex
	ChromeTracingFile
}

type ThreadSafeChromeTracing struct {
	shards   []threadSafeChromeTracingShard
	revision atomic.Uint32
}

func NewThreadSafeChromeTracing(numShards int) *ThreadSafeChromeTracing {
	base.Assert(func() bool { return numShards > 0 })
	pid := int64(os.Getpid())
	return &ThreadSafeChromeTracing{
		shards: base.Range(func(int) (s threadSafeChromeTracingShard) {
			s.pid = pid
			s.DisplayTimeUnit = "ms"
			return
		}, numShards),
	}
}
func (x *ThreadSafeChromeTracing) Event(e ChromeTracingEvent) {
	n := x.revision.Add(1) % uint32(len(x.shards))
	shard := &x.shards[n]
	shard.Mutex.Lock()
	defer shard.Mutex.Unlock()
	shard.Event(e)
}
func (x *ThreadSafeChromeTracing) ExportJson(w io.Writer) error {
	var merged ChromeTracingFile

	base.Sum(func(i int) int {
		shard := &x.shards[i]
		shard.Mutex.Lock()
		defer shard.Mutex.Unlock()
		merged.pid = shard.pid
		merged.DisplayTimeUnit = shard.DisplayTimeUnit
		merged.TraceEvents = append(merged.TraceEvents, shard.TraceEvents...)
		return len(shard.TraceEvents)
	}, len(x.shards))

	sort.Slice(merged.TraceEvents, func(i, j int) bool {
		return merged.TraceEvents[i].Timestamp < merged.TraceEvents[j].Timestamp
	})

	return base.JsonSerialize(merged, w, base.OptionJsonPrettyPrint(false))
}
