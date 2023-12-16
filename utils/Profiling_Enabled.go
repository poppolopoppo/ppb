//go:build ppb_profiling
// +build ppb_profiling

package utils

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/pkg/profile"
)

const PROFILING_ENABLED = true

var LogProfiling = NewLogCategory("Profiling")
var ProfilingTag = MakeArchiveTag(MakeFourCC('P', 'R', 'O', 'F'))

/***************************************
 * Profiling Mode
 ***************************************/

type ProfilingMode int32

const (
	PROFILING_BLOCK ProfilingMode = iota
	PROFILING_CPU
	PROFILING_GOROUTINE
	PROFILING_MEMORY
	PROFILING_MEMORYALLOC
	PROFILING_MEMORYHEAP
	PROFILING_MUTEX
	PROFILING_THREADCREATION
	PROFILING_TRACE
)

func ProfilingModes() []ProfilingMode {
	return []ProfilingMode{
		PROFILING_BLOCK,
		PROFILING_CPU,
		PROFILING_GOROUTINE,
		PROFILING_MEMORY,
		PROFILING_MEMORYALLOC,
		PROFILING_MEMORYHEAP,
		PROFILING_MUTEX,
		PROFILING_THREADCREATION,
		PROFILING_TRACE,
	}
}
func (x ProfilingMode) Mode() func(*profile.Profile) {
	switch x {
	case PROFILING_BLOCK:
		return profile.BlockProfile
	case PROFILING_CPU:
		return profile.CPUProfile
	case PROFILING_GOROUTINE:
		return profile.GoroutineProfile
	case PROFILING_MEMORY:
		return profile.MemProfile
	case PROFILING_MEMORYALLOC:
		return profile.MemProfileAllocs
	case PROFILING_MEMORYHEAP:
		return profile.MemProfileHeap
	case PROFILING_MUTEX:
		return profile.MutexProfile
	case PROFILING_THREADCREATION:
		return profile.ThreadcreationProfile
	case PROFILING_TRACE:
		return profile.TraceProfile
	default:
		UnexpectedValue(x)
		return nil
	}
}
func (x ProfilingMode) Equals(o ProfilingMode) bool {
	return (x == o)
}
func (x ProfilingMode) String() string {
	switch x {
	case PROFILING_BLOCK:
		return "BLOCK"
	case PROFILING_CPU:
		return "CPU"
	case PROFILING_GOROUTINE:
		return "GOROUTINE"
	case PROFILING_MEMORY:
		return "MEM"
	case PROFILING_MEMORYALLOC:
		return "MEMALLOC"
	case PROFILING_MEMORYHEAP:
		return "MEMHEAP"
	case PROFILING_MUTEX:
		return "MUTEX"
	case PROFILING_THREADCREATION:
		return "THREADCREATION"
	case PROFILING_TRACE:
		return "TRACE"
	default:
		UnexpectedValue(x)
		return ""
	}
}
func (x *ProfilingMode) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case PROFILING_BLOCK.String():
		*x = PROFILING_BLOCK
	case PROFILING_CPU.String():
		*x = PROFILING_CPU
	case PROFILING_GOROUTINE.String():
		*x = PROFILING_GOROUTINE
	case PROFILING_MEMORY.String():
		*x = PROFILING_MEMORY
	case PROFILING_MEMORYALLOC.String():
		*x = PROFILING_MEMORYALLOC
	case PROFILING_MEMORYHEAP.String():
		*x = PROFILING_MEMORYHEAP
	case PROFILING_MUTEX.String():
		*x = PROFILING_MUTEX
	case PROFILING_THREADCREATION.String():
		*x = PROFILING_THREADCREATION
	case PROFILING_TRACE.String():
		*x = PROFILING_TRACE
	default:
		err = MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *ProfilingMode) Serialize(ar Archive) {
	ar.Int32((*int32)(x))
}
func (x ProfilingMode) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *ProfilingMode) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}
func (x *ProfilingMode) AutoComplete(in AutoComplete) {
	for _, it := range ProfilingModes() {
		in.Add(it.String())
	}
}

type ProfilingFlags struct {
	Profiling ProfilingMode
}

var GetProflingFlags = NewGlobalCommandParsableFlags("profiling options", &ProfilingFlags{
	Profiling: PROFILING_CPU,
})

func (flags *ProfilingFlags) Flags(cfv CommandFlagsVisitor) {
	cfv.Variable("Profiling", "set profiling mode ["+JoinString(",", ProfilingModes()...)+"]", &flags.Profiling)
}

/***************************************
 * Profiling
 ***************************************/

var running_profiler interface {
	Stop()
}

func StartProfiling() func() {
	profiling := GetProflingFlags().Profiling
	LogWarning(LogProfiling, "use %v profiling mode", profiling)
	if profiling == PROFILING_CPU {
		runtime.SetCPUProfileRate(1000) // default is 100
	}
	running_profiler = profile.Start(
		profiling.Mode(),
		profile.NoShutdownHook,
		profile.ProfilePath("."))
	return PurgeProfiling
}

func PurgeProfiling() {
	if running_profiler != nil {
		running_profiler.Stop()
		if GetProflingFlags().Profiling == PROFILING_CPU {
			proc := exec.Command("sh", UFS.Scripts.File("flamegraph.sh").String())
			proc.Dir = UFS.Root.String()
			output, err := proc.Output()
			LogForward(UnsafeStringFromBytes(output))
			LogPanicIfFailed(LogProfiling, err)
		}
		running_profiler = nil
	}
}
