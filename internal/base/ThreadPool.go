package base

import (
	"runtime"
	"sync"
	"sync/atomic"
)

type TaskPriority byte

const (
	TASKPRIORITY_HIGH TaskPriority = iota
	TASKPRIORITY_LOW
)

type TaskFunc func(ThreadContext)

type ThreadContext interface {
	GetThreadId() int32
	GetThreadPool() ThreadPool
}

type ThreadPool interface {
	GetArity() int
	GetName() string
	GetWorkload() int

	Queue(TaskFunc, TaskPriority)
	Join()
	Resize(int)
}

var allThreadPools = []ThreadPool{}

func JoinAllThreadPools() {
	for _, pool := range allThreadPools {
		pool.Join()
	}
}

/***************************************
 * Global Thread Pool
 ***************************************/

var GetGlobalThreadPool = Memoize(func() (result ThreadPool) {
	result = NewFixedSizeThreadPool("global", runtime.NumCPU()-1)
	allThreadPools = append(allThreadPools, result)
	return
})

/***************************************
 * Thread Context
 ***************************************/

type threadContext struct {
	threadId   int32
	threadPool ThreadPool
}

func NewThreadContext(threadPool ThreadPool, threadId int32) ThreadContext {
	return threadContext{
		threadId:   threadId,
		threadPool: threadPool,
	}
}
func (x threadContext) GetThreadId() int32        { return x.threadId }
func (x threadContext) GetThreadPool() ThreadPool { return x.threadPool }

/***************************************
 * Fixed Size Thread Pool
 ***************************************/

type fixedSizeThreadPool struct {
	give       [2]chan TaskFunc
	loop       func(fswp *fixedSizeThreadPool, i int)
	name       string
	numWorkers int
	workload   atomic.Int32
}

func NewFixedSizeThreadPool(name string, numWorkers int) ThreadPool {
	return newFixedSizeThreadPoolImpl(name, numWorkers, func(fswp *fixedSizeThreadPool, i int) {
		threadContext := onWorkerThreadStart(fswp, i)
		defer onWorkerThreadStop(fswp, i)

		fswp.threadLoop(threadContext)
	})
}
func NewFixedSizeThreadPoolEx(name string, numWorkers int, loop func(ThreadContext, <-chan TaskFunc, <-chan TaskFunc)) ThreadPool {
	return newFixedSizeThreadPoolImpl(name, numWorkers, func(fswp *fixedSizeThreadPool, i int) {
		threadContext := onWorkerThreadStart(fswp, i)
		defer onWorkerThreadStop(fswp, i)

		loop(threadContext, fswp.give[TASKPRIORITY_HIGH], fswp.give[TASKPRIORITY_LOW])
	})
}
func newFixedSizeThreadPoolImpl(name string, numWorkers int, loop func(*fixedSizeThreadPool, int)) ThreadPool {
	pool := &fixedSizeThreadPool{
		loop:       loop,
		name:       name,
		numWorkers: numWorkers,
	}
	pool.give[TASKPRIORITY_HIGH] = make(chan TaskFunc)
	pool.give[TASKPRIORITY_LOW] = make(chan TaskFunc)

	for i := 0; i < pool.numWorkers; i++ {
		workerIndex := i
		go pool.loop(pool, workerIndex)
	}
	runtime.SetFinalizer(pool, func(pool *fixedSizeThreadPool) {
		pool.Join()
	})
	return pool
}
func (x *fixedSizeThreadPool) GetName() string  { return x.name }
func (x *fixedSizeThreadPool) GetArity() int    { return x.numWorkers }
func (x *fixedSizeThreadPool) GetWorkload() int { return int(x.workload.Load()) }
func (x *fixedSizeThreadPool) Queue(task TaskFunc, priority TaskPriority) {
	if task != nil {
		x.give[priority] <- task
	}
}
func (x *fixedSizeThreadPool) Close() {
	for i := 0; i < x.numWorkers; i++ {
		x.give[TASKPRIORITY_LOW] <- nil // push a nil task to kill the future
	}
}
func (x *fixedSizeThreadPool) Join() {
	wg := sync.WaitGroup{}
	wg.Add(x.numWorkers)

	for i := 0; i < x.numWorkers; i++ {
		x.Queue(func(ThreadContext) {
			wg.Done()
			wg.Wait()
		}, TASKPRIORITY_LOW)
	}

	wg.Wait()
}
func (x *fixedSizeThreadPool) Resize(n int) {
	delta := n - x.numWorkers
	if delta == 0 {
		return
	}

	if delta > 0 {
		for i := 0; i < delta; i++ {
			workerIndex := x.numWorkers + i
			go x.loop(x, workerIndex) // create a new worker
		}
	} else {
		for i := 0; i < -delta; i++ {
			x.give[TASKPRIORITY_LOW] <- nil // push a nil task to kill a worker goroutine
		}
	}

	x.numWorkers += delta
}
func onWorkerThreadStart(pool ThreadPool, workerIndex int) ThreadContext {
	// LockOSThread wires the calling goroutine to its current operating system thread.
	// The calling goroutine will always execute in that thread,
	// and no other goroutine will execute in it,
	// until the calling goroutine has made as many calls to
	// UnlockOSThread as to LockOSThread.
	// If the calling goroutine exits without unlocking the thread,
	// the thread will be terminated.
	runtime.LockOSThread()

	return NewThreadContext(pool, int32(workerIndex))
}
func onWorkerThreadStop(pool ThreadPool, workerIndex int) {
	//runtime.UnlockOSThread() // let acquired thread die with the pool
}
func (x *fixedSizeThreadPool) threadLoop(threadContext ThreadContext) {
	for {
		var task TaskFunc
		select {
		case task = <-x.give[TASKPRIORITY_HIGH]: // high priority first
		case task = <-x.give[TASKPRIORITY_LOW]: // low priority only if high if empty
		}

		x.workload.Add(1)
		task(threadContext)
		x.workload.Add(-1)
	}
}
