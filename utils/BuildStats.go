package utils

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
)

type BuildStats struct {
	InclusiveStart time.Duration
	ExclusiveStart time.Duration
	Duration       struct {
		Inclusive time.Duration
		Exclusive time.Duration
	}
	Count int32
}

/***************************************
 * Build Stats
 ***************************************/

func StartBuildStats() (result BuildStats) {
	result.startTimer()
	return
}
func (x *BuildStats) Append(other *BuildStats) {
	other.stopTimer()
	x.atomic_add(other)
}

func (x *BuildStats) atomic_add(other *BuildStats) {
	if atomic.AddInt32(&x.Count, other.Count) == other.Count {
		x.InclusiveStart = other.InclusiveStart
		x.ExclusiveStart = other.ExclusiveStart
	}

	atomic.AddInt64((*int64)(&x.Duration.Inclusive), int64(other.Duration.Inclusive))
	atomic.AddInt64((*int64)(&x.Duration.Exclusive), int64(other.Duration.Exclusive))
}
func (x *BuildStats) add(other *BuildStats) {
	if x.Count == 0 {
		x.InclusiveStart = other.InclusiveStart
		x.ExclusiveStart = other.ExclusiveStart
	}

	x.Count += other.Count
	x.Duration.Inclusive += other.Duration.Inclusive
	x.Duration.Exclusive += other.Duration.Exclusive
}
func (x *BuildStats) startTimer() {
	x.Count++
	x.InclusiveStart = base.Elapsed()
	x.ExclusiveStart = x.InclusiveStart
}
func (x *BuildStats) stopTimer() {
	elapsed := base.Elapsed()
	x.Duration.Inclusive += (elapsed - x.InclusiveStart)
	x.Duration.Exclusive += (elapsed - x.ExclusiveStart)
}
func (x *BuildStats) pauseTimer() {
	x.Duration.Exclusive += (base.Elapsed() - x.ExclusiveStart)
}
func (x *BuildStats) resumeTimer() {
	x.ExclusiveStart = base.Elapsed()
}

/***************************************
 * Build Events
 ***************************************/

type buildEvents struct {
	onBuildGraphStartEvent    base.ConcurrentEvent[BuildGraph]
	onBuildGraphFinishedEvent base.ConcurrentEvent[BuildGraph]

	onBuildNodeStartEvent    base.ConcurrentEvent[BuildNode]
	onBuildNodeFinishedEvent base.ConcurrentEvent[BuildNode]

	graphEventBarrier sync.Mutex
	numRunningTasks   int32
}

func newBuildEvents() (result buildEvents) {
	result.numRunningTasks = -1 // 0 is reserved when running

	if base.EnableInteractiveShell() {
		var pbar base.ProgressScope

		result.onBuildGraphStartEvent.Add(func(bg BuildGraph) error {
			pbar = base.LogSpinner("Build Graph ")
			return nil
		})
		result.onBuildNodeStartEvent.Add(func(bn BuildNode) error {
			pbar.Grow(1)
			pbar.Log("Built %d / %d nodes", pbar.Progress(), pbar.Len())
			return nil
		})
		result.onBuildNodeFinishedEvent.Add(func(bn BuildNode) error {
			pbar.Inc()
			pbar.Log("Built %d / %d nodes", pbar.Progress(), pbar.Len())
			return nil
		})
		result.onBuildGraphFinishedEvent.Add(func(bg BuildGraph) error {
			return pbar.Close()
		})
	}
	return
}

func (g *buildGraph) hasRunningTasks() bool {
	return atomic.LoadInt32(&g.numRunningTasks) >= 0
}

func (g *buildEvents) OnBuildGraphStart(e base.EventDelegate[BuildGraph]) base.DelegateHandle {
	base.Assert(func() bool { return atomic.LoadInt32(&g.numRunningTasks) == -1 })
	return g.onBuildGraphStartEvent.Add(e)
}
func (g *buildEvents) OnBuildNodeStart(e base.EventDelegate[BuildNode]) base.DelegateHandle {
	base.Assert(func() bool { return atomic.LoadInt32(&g.numRunningTasks) == -1 })
	return g.onBuildNodeStartEvent.Add(e)
}
func (g *buildEvents) OnBuildNodeFinished(e base.EventDelegate[BuildNode]) base.DelegateHandle {
	base.Assert(func() bool { return atomic.LoadInt32(&g.numRunningTasks) == -1 })
	return g.onBuildNodeFinishedEvent.Add(e)
}
func (g *buildEvents) OnBuildGraphFinished(e base.EventDelegate[BuildGraph]) base.DelegateHandle {
	base.Assert(func() bool { return atomic.LoadInt32(&g.numRunningTasks) == -1 })
	return g.onBuildGraphFinishedEvent.Add(e)
}

func (g *buildEvents) RemoveOnBuildGraphStart(h base.DelegateHandle) bool {
	base.Assert(func() bool { return atomic.LoadInt32(&g.numRunningTasks) == -1 })
	return g.onBuildGraphStartEvent.Remove(h)
}
func (g *buildEvents) RemoveOnBuildNodeStart(h base.DelegateHandle) bool {
	base.Assert(func() bool { return atomic.LoadInt32(&g.numRunningTasks) == -1 })
	return g.onBuildNodeStartEvent.Remove(h)
}
func (g *buildEvents) RemoveOnBuildNodeFinished(h base.DelegateHandle) bool {
	base.Assert(func() bool { return atomic.LoadInt32(&g.numRunningTasks) == -1 })
	return g.onBuildNodeFinishedEvent.Remove(h)
}
func (g *buildEvents) RemoveOnBuildGraphFinished(h base.DelegateHandle) bool {
	base.Assert(func() bool { return atomic.LoadInt32(&g.numRunningTasks) == -1 })
	return g.onBuildGraphFinishedEvent.Remove(h)
}

func (g *buildEvents) onBuildNodeStart_ThreadSafe(graph *buildGraph, node *buildNode) {
	base.LogDebug(LogBuildEvent, "%v -> %T: build start", node.BuildAlias, node.GetBuildable())

	if atomic.LoadInt32(&g.numRunningTasks) == -1 {
		g.onBuildGraphStart_ThreadSafe(graph)
	}

	atomic.AddInt32(&g.numRunningTasks, 1)

	g.onBuildNodeStartEvent.Invoke(node)
}
func (g *buildEvents) onBuildNodeFinished_ThreadSafe(graph *buildGraph, node *buildNode) {
	base.LogDebug(LogBuildEvent, "%v -> %T: build finished", node.BuildAlias, node.GetBuildable())

	atomic.AddInt32(&g.numRunningTasks, -1)

	g.onBuildNodeFinishedEvent.Invoke(node)

	if atomic.LoadInt32(&g.numRunningTasks) == 0 {
		g.onBuildGraphFinished_ThreadSafe(graph)
	}
}

func (g *buildEvents) onBuildGraphStart_ThreadSafe(graph *buildGraph) {
	g.graphEventBarrier.Lock()
	defer g.graphEventBarrier.Unlock()

	if atomic.LoadInt32(&g.numRunningTasks) == -1 {
		g.onBuildGraphStartEvent.Invoke(graph)
		atomic.StoreInt32(&g.numRunningTasks, 0)
	}
}
func (g *buildEvents) onBuildGraphFinished_ThreadSafe(graph *buildGraph) {
	g.graphEventBarrier.Lock()
	defer g.graphEventBarrier.Unlock()

	if atomic.LoadInt32(&g.numRunningTasks) == 0 {
		g.onBuildGraphFinishedEvent.Invoke(graph)
		atomic.StoreInt32(&g.numRunningTasks, -1)
	}
}

/***************************************
 * Build Summary
 ***************************************/

func (g *buildGraph) PrintSummary(startedAt time.Time) {
	totalDuration := time.Since(startedAt)
	base.LogForwardf("\nProgram took %.3f seconds to run", totalDuration.Seconds())

	stats := g.GetBuildStats()
	if stats.Count == 0 {
		return
	}

	base.LogForwardf("Took %.3f seconds to build %d nodes using %d threads (x%.2f)",
		stats.Duration.Exclusive.Seconds(), stats.Count,
		base.GetGlobalThreadPool().GetArity(),
		float32(stats.Duration.Exclusive)/float32(totalDuration))

	base.LogForwardf("\nMost expansive nodes built:")

	for i, node := range g.GetMostExpansiveNodes(10, false) {
		ns := node.GetBuildStats()
		fract := ns.Duration.Exclusive.Seconds() / stats.Duration.Exclusive.Seconds()

		sstep := base.Smootherstep(ns.Duration.Exclusive.Seconds() / totalDuration.Seconds()) // use percent of blocking duration
		rowColor := base.NewColdHotColor(math.Sqrt(sstep))

		annotation := ""
		switch buildable := node.GetBuildable().(type) {
		case BuildableGeneratedFile:
			if info, err := buildable.GetGeneratedFile().Info(); err == nil {
				annotation = fmt.Sprintf("  (%v)", base.SizeInBytes(info.Size()))
			}
		case BuildableSourceFile:
			if info, err := buildable.GetSourceFile().Info(); err == nil {
				annotation = fmt.Sprintf("  (%v)", base.SizeInBytes(info.Size()))
			}
		}

		base.LogForwardf("%v[%02d] - %6.2f%% -  %6.3f  %6.3f  --  %s%v%v",
			rowColor.Quantize(true).Ansi(true),
			(i + 1),
			100.0*fract,
			ns.Duration.Exclusive.Seconds(),
			ns.Duration.Inclusive.Seconds(),
			node.Alias(),
			annotation,
			base.ANSI_RESET)
	}
}
