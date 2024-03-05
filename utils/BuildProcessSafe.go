package utils

import (
	"sync"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"

	"github.com/danjacques/gofslock/fslock"
)

/***************************************
 * GlobalBuildGraph: don't load build database unless needed
 ***************************************/

type GlobalBuildGraph struct {
	barrier   sync.Mutex
	protected *processSafeBuildGraph

	onBuildGraphLoadedEvent base.PublicEvent[BuildGraph]
	onBuildGraphSavedEvent  base.PublicEvent[BuildGraph]
}

func (x *GlobalBuildGraph) Close() (err error) {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	if x.protected != nil {
		return x.protected.Close()
	}
	return nil
}

func (x *GlobalBuildGraph) Available() bool {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	return x.protected != nil
}
func (x *GlobalBuildGraph) Get(env *CommandEnvT) BuildGraph {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	if x.protected == nil {
		var err error
		x.protected, err = newProcessSafeBuildGraph(env.databasePath)
		base.LogPanicIfFailed(LogBuildGraph, err)

		err = x.protected.LoadBuildGraph(env)
		if err != nil {
			base.LogError(LogBuildGraph, "failed to load build graph database: %v", err)
		}

		base.LogPanicIfFailed(LogBuildGraph, x.onBuildGraphLoadedEvent.FireAndForget(x.protected.BuildGraph))

		x.protected.BuildGraph.PostLoad()
	}

	return x.protected.BuildGraph
}
func (x *GlobalBuildGraph) Save(env *CommandEnvT) error {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	if x.protected == nil {
		return nil
	}

	err := x.protected.SaveBuildGraph(env)
	if err == nil {
		err = x.onBuildGraphSavedEvent.FireAndForget(x.protected.BuildGraph)
	}
	return err
}
func (x *GlobalBuildGraph) Abort() {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	if x.protected != nil {
		x.protected.BuildGraph.Abort()
	}
}
func (x *GlobalBuildGraph) Join() error {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	if x.protected != nil {
		return x.protected.BuildGraph.Join()
	}
	return nil
}

func (x *GlobalBuildGraph) OnBuildGraphLoaded(e base.EventDelegate[BuildGraph]) error {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	if x.protected == nil {
		x.onBuildGraphLoadedEvent.Add(e)
	} else {
		return e(x.protected.BuildGraph)
	}
	return nil
}
func (x *GlobalBuildGraph) OnBuildGraphSaved(e base.EventDelegate[BuildGraph]) error {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	x.onBuildGraphSavedEvent.Add(e)
	return nil
}

func (x *GlobalBuildGraph) PrintSummary(startedAt time.Time, level base.LogLevel) {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	if x.protected != nil {
		x.protected.BuildGraph.PrintSummary(startedAt, level)
	}
}

/***************************************
 * processSafeBuildGraph: only 1 process is allowed to open the database
 ***************************************/

type processSafeBuildGraph struct {
	BuildGraph BuildGraph
	GlobalLock fslock.Handle
}

func newProcessSafeBuildGraph(database Filename) (*processSafeBuildGraph, error) {
	if err := UFS.MkdirEx(database.Dirname); err != nil {
		return nil, err
	}

	globalLock, err := fslock.Lock(database.String())
	if err != nil {
		return nil, err
	}

	base.LogTrace(LogBuildGraph, "locked database file %q", globalLock.LockFile().Name())

	return &processSafeBuildGraph{
		GlobalLock: globalLock,
		BuildGraph: NewBuildGraph(GetCommandFlags()),
	}, nil
}
func (x *processSafeBuildGraph) Close() error {
	base.LogTrace(LogBuildGraph, "unlocking database file %q", x.GlobalLock.LockFile().Name())
	return x.GlobalLock.Unlock()
}

func (x *processSafeBuildGraph) SaveBuildGraph(env *CommandEnvT) (err error) {
	if !base.IsNil(env.lastPanic.Load()) {
		base.LogTrace(LogCommand, "won't save build graph since a panic occured")

	} else if x.BuildGraph.Dirty() {
		benchmark := base.LogBenchmark(LogCommand, "saving build graph to '%v'...", env.databasePath)
		defer benchmark.Close()

		handle := x.GlobalLock.LockFile()
		if _, err = handle.Seek(0, 0); err != nil {
			return
		}
		if err = handle.Truncate(0); err != nil {
			return
		}

		err = x.BuildGraph.Save(handle)
	} else {
		base.LogTrace(LogCommand, "skipped saving unmodified build graph")
	}
	return
}
func (x *processSafeBuildGraph) LoadBuildGraph(env *CommandEnvT) (err error) {
	benchmark := base.LogBenchmark(LogCommand, "loading build graph from '%v'...", env.databasePath)
	defer benchmark.Close()

	handle := x.GlobalLock.LockFile()
	if len, err := handle.Seek(0, 2); err != nil {
		return err
	} else if len == 0 {
		x.BuildGraph.(*buildGraph).makeDirty("new database")
		return nil
	}

	if _, err = handle.Seek(0, 0); err != nil {
		return
	}

	err = x.BuildGraph.Load(handle)
	if err != nil {
		x.BuildGraph.(*buildGraph).makeDirty(err.Error())
	}
	return
}
