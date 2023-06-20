package utils

import (
	"runtime"
	"sync"
	"sync/atomic"
)

var LogWorkerPool = NewLogCategory("WorkerPool")

type TaskFunc func(ThreadContext)

type ThreadContext interface {
	GetThreadId() int32
	GetThreadPool() ThreadPool
}

type ThreadPool interface {
	GetArity() int
	GetName() string
	GetWorkload() int

	Queue(TaskFunc)
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
 * Fixed Size Thread Pool
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

type fixedSizeThreadPool struct {
	give       chan TaskFunc
	name       string
	numWorkers int
	workload   atomic.Int32
}

func NewFixedSizeThreadPool(name string, numWorkers int) ThreadPool {
	return NewFixedSizeThreadPoolEx(name, numWorkers, func(fswp *fixedSizeThreadPool, i int) {
		fswp.threadLoop(i)
	})
}
func NewFixedSizeThreadPoolEx(name string, numWorkers int, loop func(*fixedSizeThreadPool, int)) ThreadPool {
	pool := &fixedSizeThreadPool{
		give:       make(chan TaskFunc, 8192),
		name:       name,
		numWorkers: numWorkers,
	}
	for i := 0; i < pool.numWorkers; i++ {
		workerIndex := i
		go loop(pool, workerIndex)
	}
	runtime.SetFinalizer(pool, func(pool *fixedSizeThreadPool) {
		pool.Join()
	})
	return pool
}
func (x *fixedSizeThreadPool) GetName() string  { return x.name }
func (x *fixedSizeThreadPool) GetArity() int    { return x.numWorkers }
func (x *fixedSizeThreadPool) GetWorkload() int { return int(x.workload.Load()) }
func (x *fixedSizeThreadPool) Queue(task TaskFunc) {
	// Assert(func() bool { return task != nil })
	x.give <- task
}
func (x *fixedSizeThreadPool) Close() {
	for i := 0; i < x.numWorkers; i++ {
		x.give <- nil // push a nil task to kill the future
	}
}
func (x *fixedSizeThreadPool) Join() {
	wg := sync.WaitGroup{}
	wg.Add(x.numWorkers)

	for i := 0; i < x.numWorkers; i++ {
		x.Queue(func(ThreadContext) {
			wg.Done()
			wg.Wait()
		})
	}

	wg.Wait()
}
func (x *fixedSizeThreadPool) Resize(n int) {
	Assert(func() bool { return n > 0 })

	delta := n - x.numWorkers
	if delta == 0 {
		return
	}

	LogTrace(LogWorkerPool, "resizing %q pool from %d to %d worker threads", x.name, x.numWorkers, n)

	if delta > 0 {
		for i := 0; i < delta; i++ {
			workerIndex := x.numWorkers + i
			go x.threadLoop(workerIndex) // create a new worker
		}
	} else {
		for i := 0; i < -delta; i++ {
			x.give <- nil // push a nil task to kill a worker goroutine
		}
	}

	x.numWorkers += delta
}
func onWorkerThreadStart(pool ThreadPool, workerIndex int) {
	// LockOSThread wires the calling goroutine to its current operating system thread.
	// The calling goroutine will always execute in that thread,
	// and no other goroutine will execute in it,
	// until the calling goroutine has made as many calls to
	// UnlockOSThread as to LockOSThread.
	// If the calling goroutine exits without unlocking the thread,
	// the thread will be terminated.
	runtime.LockOSThread()
}
func onWorkerThreadStop(pool ThreadPool, workerIndex int) {
	//runtime.UnlockOSThread() // let acquired thread die with the pool
}
func (x *fixedSizeThreadPool) threadLoop(workerIndex int) {
	onWorkerThreadStart(x, workerIndex)
	defer onWorkerThreadStop(x, workerIndex)

	threadContext := NewThreadContext(x, int32(workerIndex))
	for {
		if task := (<-x.give); task != nil {
			x.workload.Add(1)
			task(threadContext)
			x.workload.Add(-1)
		} else {
			break
		}
	}
}
