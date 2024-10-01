package utils

import (
	"fmt"
	"math"
	"strings"
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

func (x BuildStats) GetExclusiveEnd() time.Duration {
	return x.ExclusiveStart + x.Duration.Exclusive
}
func (x BuildStats) GetInclusiveEnd() time.Duration {
	return x.InclusiveStart + x.Duration.Inclusive
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
	onBuildGraphStartEvent    base.ConcurrentEvent[BuildGraphWritePort]
	onBuildGraphFinishedEvent base.ConcurrentEvent[BuildGraphWritePort]

	onBuildNodeStartEvent    base.ConcurrentEvent[BuildNodeEvent]
	onBuildNodeFinishedEvent base.ConcurrentEvent[BuildNodeEvent]
}

func newBuildEvents() (result buildEvents) {
	if base.EnableInteractiveShell() {
		var pbar base.ProgressScope

		result.onBuildGraphStartEvent.Add(func(bgwp BuildGraphWritePort) error {
			if !bgwp.PortFlags().Any(BUILDGRAPH_QUIET) {
				pbar = base.LogSpinner(bgwp.PortName().String())
			} else {
				pbar = nil
			}
			return nil
		})
		result.onBuildGraphFinishedEvent.Add(func(bgwp BuildGraphWritePort) (err error) {
			if !base.IsNil(pbar) {
				err = pbar.Close()
				pbar = nil
			}
			return
		})

		result.onBuildNodeStartEvent.Add(func(bne BuildNodeEvent) error {
			if !base.IsNil(pbar) {
				pbar.Grow(1)
				pbar.Log("Built %d / %d nodes (workload: %d)", pbar.Progress(), pbar.Len(), base.GetGlobalThreadPool().GetWorkload())
			}
			return nil
		})
		result.onBuildNodeFinishedEvent.Add(func(bne BuildNodeEvent) error {
			if !base.IsNil(pbar) {
				pbar.Inc()
				pbar.Log("Built %d / %d nodes (workload: %d)", pbar.Progress(), pbar.Len(), base.GetGlobalThreadPool().GetWorkload())
			}
			return nil
		})
	}
	return
}

func (g *buildEvents) OnBuildGraphStart(e base.EventDelegate[BuildGraphWritePort]) base.DelegateHandle {
	return g.onBuildGraphStartEvent.Add(e)
}
func (g *buildEvents) OnBuildNodeStart(e base.EventDelegate[BuildNodeEvent]) base.DelegateHandle {
	return g.onBuildNodeStartEvent.Add(e)
}
func (g *buildEvents) OnBuildNodeFinished(e base.EventDelegate[BuildNodeEvent]) base.DelegateHandle {
	return g.onBuildNodeFinishedEvent.Add(e)
}
func (g *buildEvents) OnBuildGraphFinished(e base.EventDelegate[BuildGraphWritePort]) base.DelegateHandle {
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

func (g *buildGraphWritePort) onBuildGraphStart_ThreadSafe() {
	base.LogDebug(LogBuildEvent, "build graph start <%v>", g.name)

	g.onBuildGraphStartEvent.Invoke(g)
}
func (g *buildGraphWritePort) onBuildGraphFinished_ThreadSafe() {
	base.LogDebug(LogBuildEvent, "build graph finished <%v>", g.name)

	g.onBuildGraphFinishedEvent.Invoke(g)
}

func (g *buildGraphWritePort) onBuildNodeStart_ThreadSafe(node *buildState) {
	base.LogDebug(LogBuildEvent, "<%v> %v -> %T: build start", g.name, node.BuildAlias, node.GetBuildable())

	g.onBuildNodeStartEvent.Invoke(BuildNodeEvent{
		Port: g,
		Node: node,
	})
}
func (g *buildGraphWritePort) onBuildNodeFinished_ThreadSafe(node *buildState) {
	base.LogDebug(LogBuildEvent, "<%v> %v -> %T: build finished", g.name, node.BuildAlias, node.GetBuildable())

	g.onBuildNodeFinishedEvent.Invoke(BuildNodeEvent{
		Port: g,
		Node: node,
	})
}

/***************************************
 * Build Summary
 ***************************************/

func (g *buildGraphWritePort) PrintSummary(startedAt time.Time, level base.LogLevel) {
	// Total duration (always)
	totalDuration := time.Since(startedAt)
	base.LogForwardf("\nGraph for %q took %.3f seconds to run", g.name, totalDuration.Seconds())

	// Build durationl (if something was built)
	if !level.IsVisible(base.LOG_INFO) {
		return
	}

	stats := g.GetAggregatedBuildStats()
	if stats.Count == 0 {
		return
	}

	base.LogForwardf("Took %.3f seconds to build %d nodes using %d threads (x%.2f)",
		stats.Duration.Exclusive.Seconds(), stats.Count,
		base.GetGlobalThreadPool().GetArity(),
		float32(stats.Duration.Exclusive)/float32(totalDuration))

	// Most expansive nodes built
	if !level.IsVisible(base.LOG_VERBOSE) {
		return
	}

	printNodeStatus := func(node BuildState) fmt.Stringer {
		return base.MakeStringer(func() (str string) {
			result, err := node.GetBuildResult()
			if err == nil {
				switch result.Status {
				case BUILDSTATUS_UNBUILT:
					return fmt.Sprint(base.ANSI_FG0_BLACK.String(), "??")
				case BUILDSTATUS_BUILT:
					return fmt.Sprint(base.ANSI_FG1_GREEN.String(), "OK")
				case BUILDSTATUS_UPDATED:
					return fmt.Sprint(base.ANSI_FG0_YELLOW.String(), "=>")
				case BUILDSTATUS_UPTODATE:
					return fmt.Sprint(base.ANSI_FG0_GREEN.String(), "OK")
				}
			} else {
				return fmt.Sprint(base.ANSI_FG1_RED, base.ANSI_BLINK1, "KO")
			}
			return
		})
	}

	base.LogForwardf("\nMost expansive nodes built:")

	for i, node := range g.GetMostExpansiveNodes(10, false) {
		ns, _ := g.GetBuildStats(node)

		fract := ns.Duration.Exclusive.Seconds() / stats.Duration.Exclusive.Seconds()

		// use percent of blocking duration
		sstep := base.Smootherstep(ns.Duration.Exclusive.Seconds() / totalDuration.Seconds())
		rowColor := base.NewColdHotColor(math.Sqrt(sstep))

		annotation := ""
		switch buildable := node.GetBuildable().(type) {
		case BuildableGeneratedFile:
			if info, err := buildable.GetGeneratedFile().Info(); err == nil {
				annotation = fmt.Sprintf(" (%v)", base.SizeInBytes(info.Size()))
			}
		case BuildableSourceFile:
			if info, err := buildable.GetSourceFile().Info(); err == nil {
				annotation = fmt.Sprintf(" (%v)", base.SizeInBytes(info.Size()))
			}
		}

		base.LogForwardf("%v[%02d] - %6.2f%% -  %7.3f  %7.3f  -- %s%v --  %s%v%v",
			rowColor.Quantize(true).Ansi(true),
			(i + 1),
			100.0*fract,
			ns.Duration.Exclusive.Seconds(),
			ns.Duration.Inclusive.Seconds(),
			printNodeStatus(node),
			rowColor.Quantize(true).Ansi(true),
			node.Alias(),
			annotation,
			base.ANSI_RESET)
	}

	// Critical path
	if !level.IsVisible(base.LOG_VERYVERBOSE) {
		return
	}

	criticalPath, maxTime := g.GetCriticalPathNodes()
	if len(criticalPath) < 2 {
		return
	}

	base.LogForwardf("\nCritical path: %5.3f s", maxTime.Seconds())

	for depth, node := range criticalPath {
		ns, _ := g.GetBuildStats(node)

		fract := ns.Duration.Exclusive.Seconds() / stats.Duration.Exclusive.Seconds()
		// use percent of blocking duration
		sstep := base.Smootherstep(ns.Duration.Exclusive.Seconds() / totalDuration.Seconds())
		rowColor := base.NewColdHotColor(math.Sqrt(sstep))

		annotation := ``
		switch buildable := node.GetBuildable().(type) {
		case BuildableGeneratedFile:
			if info, err := buildable.GetGeneratedFile().Info(); err == nil {
				annotation = fmt.Sprintf(" (%v)", base.SizeInBytes(info.Size()))
			}
		case BuildableSourceFile:
			if info, err := buildable.GetSourceFile().Info(); err == nil {
				annotation = fmt.Sprintf(" (%v)", base.SizeInBytes(info.Size()))
			}
		}

		base.LogForwardf("%v[%02d] - %6.2f%% -  %7.3f  %7.3f  -- %s%v --  %s%s%v%v",
			rowColor.Quantize(true).Ansi(true),
			depth,
			100.0*fract,
			ns.Duration.Exclusive.Seconds(),
			ns.Duration.Inclusive.Seconds(),
			printNodeStatus(node),
			rowColor.Quantize(true).Ansi(true),
			strings.Repeat(` `, depth),
			node.Alias(),
			annotation,
			base.ANSI_RESET)
	}
}
