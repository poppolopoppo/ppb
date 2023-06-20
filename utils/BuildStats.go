package utils

import (
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

func (g *buildEvents) OnBuildGraphStart(e base.EventDelegate[BuildGraph]) base.DelegateHandle {
	return g.onBuildGraphStartEvent.Add(e)
}
func (g *buildEvents) OnBuildNodeStart(e base.EventDelegate[BuildNodeEvent]) base.DelegateHandle {
	return g.onBuildNodeStartEvent.Add(e)
}
func (g *buildEvents) OnBuildNodeFinished(e base.EventDelegate[BuildNodeEvent]) base.DelegateHandle {
	return g.onBuildNodeFinishedEvent.Add(e)
}
func (g *buildEvents) OnBuildGraphFinished(e base.EventDelegate[BuildGraph]) base.DelegateHandle {
	return g.onBuildGraphFinishedEvent.Add(e)
}

func (g *buildEvents) onBuildGraphStart_ThreadSafe() base.ProgressScope {
	g.barrier.Lock()
	defer g.barrier.Unlock()

	if g.pbar == nil {
		g.pbar = base.LogSpinner("Build Graph ")
	}
	return g.pbar
}
func (g *buildEvents) onBuildGraphFinished_ThreadSafe() {
	g.barrier.Lock()
	defer g.barrier.Unlock()

	if g.pbar != nil {
		g.pbar.Close()
		g.pbar = nil
	}
}

func (g *buildEvents) onBuildNodeStart_ThreadSafe(graph *buildGraph, node *buildNode) {
	if g.numRunningTasks.Add(1) == 1 {
		g.onBuildGraphStartEvent.Invoke(graph)
	}

	base.LogDebug(LogBuildEvent, "%v -> %T: build start", node.BuildAlias, node.Buildable)

	if base.EnableInteractiveShell() {
		g.onBuildGraphStart_ThreadSafe()

		if g.pbar != nil {
			g.pbar.Grow(1)
			g.pbar.Log("Built %d / %d nodes", g.pbar.Progress(), g.pbar.Len())
		}
	}

	if g.onBuildNodeStartEvent.Bound() {
		g.onBuildNodeStartEvent.Invoke(BuildNodeEvent{
			Alias:     node.BuildAlias,
			Node:      node,
			Buildable: node.Buildable,
		})
	}
}
func (g *buildEvents) onBuildNodeFinished_ThreadSafe(graph *buildGraph, node *buildNode) {
	if g.onBuildNodeFinishedEvent.Bound() {
		g.onBuildNodeFinishedEvent.Invoke(BuildNodeEvent{
			Alias:     node.BuildAlias,
			Node:      node,
			Buildable: node.Buildable,
		})
	}

	base.LogDebug(LogBuildEvent, "%v -> %T: build finished", node.BuildAlias, node.Buildable)

	if g.numRunningTasks.Add(-1) == 0 {
		g.onBuildGraphFinishedEvent.Invoke(graph)
		g.onBuildGraphFinished_ThreadSafe()
	}

	if base.EnableInteractiveShell() && g.pbar != nil {
		g.pbar.Inc()
		g.pbar.Log("Built %d / %d nodes", g.pbar.Progress(), g.pbar.Len())
	}
}
