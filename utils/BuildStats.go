package utils

import (
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

	onBuildNodeStartEvent    base.ConcurrentEvent[BuildNodeEvent]
	onBuildNodeFinishedEvent base.ConcurrentEvent[BuildNodeEvent]

	onBuildGraphStartOnce    sync.Once
	onBuildGraphFinishedOnce sync.Once
}

func newBuildEvents() (result buildEvents) {
	if base.EnableInteractiveShell() {
		var pbar base.ProgressScope
		result.onBuildGraphStartEvent.Add(func(bg BuildGraph) error {
			pbar = base.LogSpinner("Build Graph ")
			return nil
		})
		result.onBuildNodeStartEvent.Add(func(bne BuildNodeEvent) error {
			pbar.Grow(1)
			pbar.Log("Built %d / %d nodes", pbar.Progress(), pbar.Len())
			return nil
		})
		result.onBuildNodeFinishedEvent.Add(func(bne BuildNodeEvent) error {
			pbar.Inc()
			pbar.Log("Built %d / %d nodes", pbar.Progress(), pbar.Len())
			return nil
		})
		result.onBuildGraphFinishedEvent.Add(func(bg BuildGraph) error {
			err := pbar.Close()
			pbar = nil
			return err
		})
	}
	return
}

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

func (g *buildEvents) RemoveOnBuildGraphStart(h base.DelegateHandle) bool {
	return g.onBuildGraphStartEvent.Remove(h)
}
func (g *buildEvents) RemoveOnBuildNodeStart(h base.DelegateHandle) bool {
	return g.onBuildNodeStartEvent.Remove(h)
}
func (g *buildEvents) RemoveOnBuildNodeFinished(h base.DelegateHandle) bool {
	return g.onBuildNodeFinishedEvent.Remove(h)
}
func (g *buildEvents) RemoveOnBuildGraphFinished(h base.DelegateHandle) bool {
	return g.onBuildGraphFinishedEvent.Remove(h)
}

func (g *buildEvents) onBuildNodeStart_ThreadSafe(graph *buildGraph, node *buildNode) {
	g.onBuildGraphStartOnce.Do(func() {
		g.onBuildGraphStartEvent.Invoke(graph)
		g.onBuildGraphFinishedOnce = sync.Once{}
	})

	base.LogDebug(LogBuildEvent, "%v -> %T: build start", node.BuildAlias, node.Buildable)

	g.onBuildNodeStartEvent.Invoke(BuildNodeEvent{
		Alias:     node.BuildAlias,
		Node:      node,
		Buildable: node.Buildable,
	})
}
func (g *buildEvents) onBuildNodeFinished_ThreadSafe(graph *buildGraph, node *buildNode) {
	base.LogDebug(LogBuildEvent, "%v -> %T: build finished", node.BuildAlias, node.Buildable)

	g.onBuildNodeFinishedEvent.Invoke(BuildNodeEvent{
		Alias:     node.BuildAlias,
		Node:      node,
		Buildable: node.Buildable,
	})

	g.onBuildGraphFinishedOnce.Do(func() {
		g.onBuildGraphFinishedEvent.Invoke(graph)
		g.onBuildGraphStartOnce = sync.Once{}
	})
}
