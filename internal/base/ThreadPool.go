package base

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
)

type TaskPriority byte

const (
	TASKPRIORITY_HIGH TaskPriority = iota
	TASKPRIORITY_NORMAL
	TASKPRIORITY_LOW
)

func (x TaskPriority) String() string {
	switch x {
	case TASKPRIORITY_HIGH:
		return "High"
	case TASKPRIORITY_NORMAL:
		return "Normal"
	case TASKPRIORITY_LOW:
		return "Low"
	}
	return ""
}

type TaskFunc func(ThreadContext)

type ThreadContext interface {
	GetThreadId() int32
	GetThreadPool() ThreadPool
	GetThreadDebugName() string
}

type ThreadPool interface {
	GetArity() int
	GetName() string
	GetWorkload() int

	Queue(task TaskFunc, priority TaskPriority, debugId ThreadPoolDebugId)
	Join()
	Resize(int)

	ThreadPoolEvents
}

/***************************************
 * Global Thread Pool
 ***************************************/

var GetGlobalThreadPool = Memoize(func() (result ThreadPool) {
	result = NewFixedSizeThreadPool("Global", runtime.NumCPU()-1)
	allThreadPools = append(allThreadPools, result)
	return
})

var allThreadPools = []ThreadPool{}

func GetAllThreadPools() []ThreadPool {
	return allThreadPools
}

// avoid allocating the global thread pool only to call Join() on it
func JoinAllThreadPools() {
	for _, pool := range allThreadPools {
		pool.Join()
	}
}

/***************************************
 * Thread Context
 ***************************************/

type threadContext struct {
	threadId        int32
	threadPool      ThreadPool
	threadDebugName string
}

func NewThreadContext(threadPool ThreadPool, threadId int32) ThreadContext {
	return threadContext{
		threadId:        threadId,
		threadPool:      threadPool,
		threadDebugName: fmt.Sprintf("%s#%d", threadPool.GetName(), threadId),
	}
}
func (x threadContext) GetThreadId() int32         { return x.threadId }
func (x threadContext) GetThreadPool() ThreadPool  { return x.threadPool }
func (x threadContext) GetThreadDebugName() string { return x.threadDebugName }

/***************************************
 * Thread Pool Events
 ***************************************/

type ThreadPoolDebugId struct {
	Category string
	Arg      fmt.Stringer
}

type ThreadPoolWorkEvent struct {
	Context  ThreadContext
	DebugId  ThreadPoolDebugId
	Priority TaskPriority
}

func (x ThreadPoolDebugId) String() string {
	if len(x.Category) > 0 {
		if IsNil(x.Arg) {
			return x.Category
		} else {
			return fmt.Sprint(x.Category, ", ", x.Arg.String())
		}
	} else if !IsNil(x.Arg) {
		return x.Arg.String()
	} else {
		return ""
	}
}

type ThreadPoolEvents interface {
	OnWorkStart(EventDelegate[ThreadPoolWorkEvent]) DelegateHandle
	OnWorkFinished(EventDelegate[ThreadPoolWorkEvent]) DelegateHandle

	RemoveOnWorkStart(DelegateHandle) bool
	RemoveOnWorkFinished(DelegateHandle) bool
}

/***************************************
 * Fixed Size Thread Pool
 ***************************************/

type TaskQueued struct {
	Func    TaskFunc
	DebugId ThreadPoolDebugId
}

type TaskQueuedAndPriority struct {
	TaskQueued
	Piority TaskPriority
}

type TaskPriorityQueue interface {
	Len() int
	Push(task TaskQueued, priority TaskPriority)
	Pop() TaskQueuedAndPriority
}

type taskPriorityQueue struct {
	mu       sync.Mutex
	nonEmpty *sync.Cond
	give     [3][]TaskQueued
	inflight int32
}

func (x *taskPriorityQueue) Len() int {
	return int(atomic.LoadInt32(&x.inflight))
}

func (x *taskPriorityQueue) Push(task TaskQueued, priority TaskPriority) {
	x.nonEmpty.L.Lock()
	x.give[int(priority)] = append(x.give[int(priority)], task)
	x.inflight += 1
	x.nonEmpty.L.Unlock()
	x.nonEmpty.Signal()
}

func (x *taskPriorityQueue) Pop() TaskQueuedAndPriority {
	x.nonEmpty.L.Lock()
	defer x.nonEmpty.L.Unlock()
	for {
		for i, q := range x.give {
			if len(q) > 0 {
				task := q[0]
				x.give[i] = q[1:]
				x.inflight -= 1
				return TaskQueuedAndPriority{
					TaskQueued: task,
					Piority:    TaskPriority(i),
				}
			}
		}

		x.nonEmpty.Wait()
	}
}

type fixedSizeThreadPool struct {
	name       string
	numWorkers int
	loop       func(fswp *fixedSizeThreadPool, i int)

	queue    taskPriorityQueue
	workload atomic.Int32

	onWorkStartEvent    ConcurrentEvent[ThreadPoolWorkEvent]
	onWorkFinishedEvent ConcurrentEvent[ThreadPoolWorkEvent]
}

func NewFixedSizeThreadPool(name string, numWorkers int) ThreadPool {
	return newFixedSizeThreadPoolImpl(name, numWorkers, func(fswp *fixedSizeThreadPool, i int) {
		threadContext := onWorkerThreadStart(fswp, i)
		defer onWorkerThreadStop(fswp, i)

		fswp.threadLoop(threadContext)
	})
}
func NewFixedSizeThreadPoolEx(name string, numWorkers int, loop func(ThreadContext, TaskPriorityQueue)) ThreadPool {
	return newFixedSizeThreadPoolImpl(name, numWorkers, func(fswp *fixedSizeThreadPool, i int) {
		threadContext := onWorkerThreadStart(fswp, i)
		defer onWorkerThreadStop(fswp, i)

		loop(threadContext, &fswp.queue)
	})
}
func newFixedSizeThreadPoolImpl(name string, numWorkers int, loop func(*fixedSizeThreadPool, int)) ThreadPool {
	pool := &fixedSizeThreadPool{
		loop:       loop,
		name:       name,
		numWorkers: numWorkers,
	}
	pool.queue = taskPriorityQueue{
		give: [3][]TaskQueued{
			make([]TaskQueued, 0, 33),
			make([]TaskQueued, 0, 33),
			make([]TaskQueued, 0, 33),
		},
	}
	pool.queue.nonEmpty = sync.NewCond(&pool.queue.mu)

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
func (x *fixedSizeThreadPool) Queue(task TaskFunc, priority TaskPriority, debugId ThreadPoolDebugId) {
	if task != nil {
		x.queue.Push(TaskQueued{
			Func:    task,
			DebugId: debugId,
		}, priority)
	}
}
func (x *fixedSizeThreadPool) Close() {
	for i := 0; i < x.numWorkers; i++ {
		x.queue.Push(TaskQueued{
			Func: nil, // push a nil task to kill the future
		}, TASKPRIORITY_LOW)
	}
}
func (x *fixedSizeThreadPool) Join() {
	wg := sync.WaitGroup{}
	wg.Add(x.numWorkers)

	for i := 0; i < x.numWorkers; i++ {
		x.Queue(func(ThreadContext) {
			wg.Done()
			wg.Wait()
		}, TASKPRIORITY_LOW, ThreadPoolDebugId{Category: "Queue.Join"})
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
			x.queue.Push(TaskQueued{
				Func: nil, // push a nil task to kill a worker goroutine
			}, TASKPRIORITY_LOW)
		}
	}

	x.numWorkers += delta
}

func (x *fixedSizeThreadPool) OnWorkStart(event EventDelegate[ThreadPoolWorkEvent]) DelegateHandle {
	return x.onWorkStartEvent.Add(event)
}
func (x *fixedSizeThreadPool) OnWorkFinished(event EventDelegate[ThreadPoolWorkEvent]) DelegateHandle {
	return x.onWorkFinishedEvent.Add(event)
}

func (x *fixedSizeThreadPool) RemoveOnWorkStart(handle DelegateHandle) bool {
	return x.onWorkStartEvent.Remove(handle)
}
func (x *fixedSizeThreadPool) RemoveOnWorkFinished(handle DelegateHandle) bool {
	return x.onWorkFinishedEvent.Remove(handle)
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
	_, _ = pool, workerIndex
	runtime.UnlockOSThread() // let acquired thread die with the pool
}

func (x *fixedSizeThreadPool) runTaskOnWorker(threadContext ThreadContext, task TaskQueuedAndPriority) {
	x.workload.Add(1)
	if x.onWorkStartEvent.Bound() {
		x.onWorkStartEvent.Invoke(ThreadPoolWorkEvent{
			Context:  threadContext,
			DebugId:  task.DebugId,
			Priority: task.Piority,
		})
	}
	defer func() {
		if x.onWorkFinishedEvent.Bound() {
			x.onWorkFinishedEvent.Invoke(ThreadPoolWorkEvent{
				Context:  threadContext,
				DebugId:  task.DebugId,
				Priority: task.Piority,
			})
		}
		x.workload.Add(-1)
	}()

	task.Func(threadContext)
}
func (x *fixedSizeThreadPool) threadLoop(threadContext ThreadContext) {
	for {
		task := x.queue.Pop()

		if task.Func == nil {
			break // worker was killed
		}

		x.runTaskOnWorker(threadContext, task)
	}
}
