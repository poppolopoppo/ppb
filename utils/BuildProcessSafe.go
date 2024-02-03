package utils

import (
	"sync"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"

	"github.com/danjacques/gofslock/fslock"
)

/***************************************
 * ProcessSafeBuildGraph: don't load build database unless needed, and only 1 process is allowed to open the database
 ***************************************/

type ProcessSafeBuildGraph struct {
	barrier    sync.Mutex
	buildGraph BuildGraph
	globalLock fslock.Handle

	onBuildGraphLoadedEvent base.PublicEvent[BuildGraph]
	onBuildGraphSavedEvent  base.PublicEvent[BuildGraph]
}

func (x *ProcessSafeBuildGraph) Available() bool {
	x.barrier.Lock()
	defer x.barrier.Unlock()
	return !base.IsNil(x.buildGraph)
}
func (x *ProcessSafeBuildGraph) Get() BuildGraph {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	if base.IsNil(x.buildGraph) {
		x.buildGraph = NewBuildGraph(GetCommandFlags())
		if err := x.loadBuildGraph(CommandEnv); err != nil {
			switch err {
			case fslock.ErrLockHeld:
				base.LogPanic(LogBuildGraph, "build graph database is already locked by another process")
			default:
				base.LogError(LogBuildGraph, "failed to load build graph database: %v", err)
			}
		}
		x.buildGraph.PostLoad()
	}
	return x.buildGraph
}
func (x *ProcessSafeBuildGraph) Join() error {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	if base.IsNil(x.buildGraph) {
		return nil
	}
	return x.buildGraph.Join()
}
func (x *ProcessSafeBuildGraph) Save(env *CommandEnvT) error {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	if base.IsNil(x.buildGraph) {
		return nil
	}
	return x.saveBuildGraph(env)
}

func (x *ProcessSafeBuildGraph) OnBuildGraphLoaded(e base.EventDelegate[BuildGraph]) error {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	if base.IsNil(x.buildGraph) {
		x.onBuildGraphLoadedEvent.Add(e)
	} else {
		return e(x.buildGraph)
	}
	return nil
}
func (x *ProcessSafeBuildGraph) OnBuildGraphSaved(e base.EventDelegate[BuildGraph]) error {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	x.onBuildGraphSavedEvent.Add(e)
	return nil
}

func (x *ProcessSafeBuildGraph) saveBuildGraph(env *CommandEnvT) (err error) {
	defer func() {
		base.LogTrace(LogBuildGraph, "unlocking database file %q", env.databasePath)
		if er := x.globalLock.Unlock(); er != nil && err == nil {
			err = er
		}
	}()

	if !base.IsNil(env.lastPanic.Load()) {
		base.LogTrace(LogCommand, "won't save build graph since a panic occured")
	} else if x.buildGraph.Dirty() {
		benchmark := base.LogBenchmark(LogCommand, "saving build graph to '%v'...", env.databasePath)
		defer benchmark.Close()

		err = UFS.SafeCreate(env.databasePath, x.buildGraph.Save)
	} else {
		base.LogTrace(LogCommand, "skipped saving unmodified build graph")
	}

	err = x.onBuildGraphSavedEvent.FireAndForget(x.buildGraph)
	return
}
func (x *ProcessSafeBuildGraph) loadBuildGraph(env *CommandEnvT) (err error) {
	base.AssertIn(x.globalLock, nil)

	base.LogTrace(LogBuildGraph, "locking database file %q", env.databasePath)
	x.globalLock, err = fslock.Lock(env.databasePath.String() + ".lock")
	if err != nil {
		return err
	}

	benchmark := base.LogBenchmark(LogCommand, "loading build graph from '%v'...", env.databasePath)
	defer benchmark.Close()

	err = UFS.OpenBuffered(env.databasePath, x.buildGraph.Load)

	if err == nil {
		err = x.onBuildGraphLoadedEvent.FireAndForget(x.buildGraph)
	} else {
		x.buildGraph.(*buildGraph).makeDirty("corrupted database")
	}
	return
}

func (x *ProcessSafeBuildGraph) PrintSummary(startedAt time.Time, level base.LogLevel) {
	if !base.IsNil(x.buildGraph) {
		x.buildGraph.PrintSummary(startedAt, level)
	}
}
