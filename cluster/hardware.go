package cluster

import (
	"context"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

/***************************************
 * Peer Avaibility
 ***************************************/

type PeerAvaibility struct {
	AvailableThreads       atomic.Int32
	AvailableVirtualMemory atomic.Uint64

	averageCpu MovingAverage
	averageMem MovingAverage

	lastUpdate time.Time
	idleSince  time.Time
}

func newPeerAvaibility() PeerAvaibility {
	return PeerAvaibility{
		averageCpu: NewMovingAverage(0),
		averageMem: NewMovingAverage(0),
		idleSince:  time.Now(),
	}
}
func (x *PeerAvaibility) GetAverageCpuUsage() float64 { return x.averageCpu.EstimatedLevel() }
func (x *PeerAvaibility) GetAverageMemUsage() float64 { return x.averageMem.EstimatedLevel() }

func (x *PeerAvaibility) ReadyForWork() bool {
	return x.AvailableThreads.Load() > 0
}
func (x *PeerAvaibility) UpdateResources(ctx context.Context, worker *WorkerFlags, hw *PeerHardware, numJobsInFlight int32) (changed bool, err error) {
	if x.lastUpdate == (time.Time{}) {
		changed = true
	}
	x.lastUpdate = time.Now()

	var vm *mem.VirtualMemoryStat
	if vm, err = mem.VirtualMemory(); err != nil {
		return
	}

	x.AvailableVirtualMemory.Store(vm.Available)
	if vm.Available < uint64(worker.MinFreeMemory.Get()) {
		changed = x.AvailableThreads.Swap(0) != 0
		return
	}

	var cpuUsages []float64
	cpuUsages, err = cpu.PercentWithContext(ctx, 50*time.Millisecond, false)
	if err != nil {
		return
	}

	x.averageCpu.Majorate(cpuUsages[0] / 100.0)
	x.averageMem.Majorate(vm.UsedPercent / 100.0)

	usedPercent := x.GetAverageCpuUsage()
	if usedPercent > float64(worker.IdleThreshold.Get()) {
		x.idleSince = time.Now()
	}

	var numThreads int32
	switch worker.Mode {
	case PEERMODE_DISABLED:
		// no resource available
		changed = x.AvailableThreads.Swap(0) != 0
		x.AvailableVirtualMemory.Store(0)
		return

	case PEERMODE_DEDICATED:
		// consume all resources available
		numThreads = hw.Threads - numJobsInFlight

	case PEERMODE_PROPORTIONAL, PEERMODE_IDLE:
		// consume only available resources
		if worker.Mode == PEERMODE_IDLE && time.Since(x.idleSince) > time.Duration(worker.IdleCooldown)*time.Second {
			numThreads = 0
		} else {
			numThreads = hw.Cores - int32(float64(hw.Cores)*usedPercent) - numJobsInFlight
		}

	default:
		UnexpectedValuePanic(worker.Mode, worker.Mode)
	}

	// clamp available threads with user given peer.MaxThreads
	if worker.MaxThreads > 0 && numThreads > int32(worker.MaxThreads) {
		numThreads = int32(worker.MaxThreads)
	}
	if numThreads < 0 {
		numThreads = 0
	}

	// signal when a worker was unavailable and became available, or vice-versa
	changed = (x.AvailableThreads.Swap(numThreads) > 0) != (numThreads > 0)
	return
}

/***************************************
 * Peer Hardware
 ***************************************/

type PeerHardware struct {
	Arch          string
	CpuName       string
	Vendor        string
	Cores         int32
	Threads       int32
	MaxClock      int32
	VirtualMemory uint64
}

func CurrentPeerHardware() (hw PeerHardware, err error) {
	defer LogBenchmark(LogCluster, "CurrentPeerHardware").Close()

	hw.Arch = runtime.GOARCH

	var cpuInfos []cpu.InfoStat
	if cpuInfos, err = cpu.Info(); err != nil {
		return
	}

	mainCpu := cpuInfos[0]

	hw.CpuName = strings.TrimSpace(mainCpu.ModelName)
	hw.Vendor = strings.TrimSpace(mainCpu.VendorID)
	hw.MaxClock = int32(mainCpu.Mhz)

	var numberOfPhysicalProcessors int
	if numberOfPhysicalProcessors, err = cpu.Counts(false); err != nil {
		return
	}

	hw.Cores = int32(numberOfPhysicalProcessors)

	var numberOfLogicalProcessors int
	if numberOfLogicalProcessors, err = cpu.Counts(true); err != nil {
		return
	}

	hw.Threads = int32(numberOfLogicalProcessors)

	var vm *mem.VirtualMemoryStat
	if vm, err = mem.VirtualMemory(); err != nil {
		return
	}

	hw.VirtualMemory = vm.Total
	LogVerbose(LogCluster, "local peer hardware: %v", &hw)
	return
}
func (x *PeerHardware) String() string {
	return PrettyPrint(x)
}
