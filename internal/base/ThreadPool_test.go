package base

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFixedSizeThreadPool_BasicQueueAndJoin(t *testing.T) {
	pool := NewFixedSizeThreadPool("TestPool", 2)
	defer pool.Join()

	var counter int32
	wg := sync.WaitGroup{}
	numTasks := 10
	wg.Add(numTasks)

	for i := 0; i < numTasks; i++ {
		pool.Queue(func(ctx ThreadContext) {
			atomic.AddInt32(&counter, 1)
			wg.Done()
		}, TASKPRIORITY_NORMAL, ThreadPoolDebugId{Category: "Test"})
	}

	wg.Wait()
	if counter != int32(numTasks) {
		t.Errorf("Expected counter to be %d, got %d", numTasks, counter)
	}
}

func TestFixedSizeThreadPool_QueuePriorities(t *testing.T) {
	pool := NewFixedSizeThreadPool("PriorityPool", 1)
	defer pool.Join()

	result := make([]string, 0, 4)
	mu := sync.Mutex{}
	wg := sync.WaitGroup{}
	wg.Add(1)

	pool.Queue(func(ctx ThreadContext) {
		mu.Lock()
		result = append(result, "start")
		wg.Wait()
		mu.Unlock()
	}, TASKPRIORITY_HIGH, ThreadPoolDebugId{Category: "Start"})

	pool.Queue(func(ctx ThreadContext) {
		mu.Lock()
		result = append(result, "low")
		mu.Unlock()
	}, TASKPRIORITY_LOW, ThreadPoolDebugId{Category: "Low"})

	pool.Queue(func(ctx ThreadContext) {
		mu.Lock()
		result = append(result, "normal")
		mu.Unlock()
	}, TASKPRIORITY_NORMAL, ThreadPoolDebugId{Category: "Normal"})

	pool.Queue(func(ctx ThreadContext) {
		mu.Lock()
		result = append(result, "high")
		mu.Unlock()
	}, TASKPRIORITY_HIGH, ThreadPoolDebugId{Category: "High"})

	wg.Done()
	pool.Join()

	mu.Lock()
	defer mu.Unlock()
	if len(result) < 4 || result[0] != "start" || result[1] != "high" || result[2] != "normal" || result[3] != "low" {
		t.Errorf("Expected high priority to run first, got %v", result)
	}
}

func TestFixedSizeThreadPool_Resize(t *testing.T) {
	pool := NewFixedSizeThreadPool("ResizePool", 1)
	defer pool.Join()

	if pool.GetArity() != 1 {
		t.Errorf("Expected arity 1, got %d", pool.GetArity())
	}
	pool.Resize(3)
	if pool.GetArity() != 3 {
		t.Errorf("Expected arity 3 after resize, got %d", pool.GetArity())
	}
	pool.Resize(2)
	if pool.GetArity() != 2 {
		t.Errorf("Expected arity 2 after resize, got %d", pool.GetArity())
	}
}

func TestThreadContext_Properties(t *testing.T) {
	pool := NewFixedSizeThreadPool("CtxPool", 1)
	defer pool.Join()
	ctx := NewThreadContext(pool, 42)
	if ctx.GetThreadId() != 42 {
		t.Errorf("Expected thread id 42, got %d", ctx.GetThreadId())
	}
	if ctx.GetThreadPool() != pool {
		t.Errorf("Expected thread pool to match")
	}
	if ctx.GetThreadDebugName() != "CtxPool#42" {
		t.Errorf("Expected debug name 'CtxPool#42', got '%s'", ctx.GetThreadDebugName())
	}
}

func TestThreadPoolDebugId_String(t *testing.T) {
	id1 := ThreadPoolDebugId{Category: "Cat", Arg: nil}
	if id1.String() != "Cat" {
		t.Errorf("Expected 'Cat', got '%s'", id1.String())
	}
	id2 := ThreadPoolDebugId{Category: "", Arg: nil}
	if id2.String() != "" {
		t.Errorf("Expected '', got '%s'", id2.String())
	}
	id3 := ThreadPoolDebugId{Category: "Cat", Arg: MakeStringer(func() string { return "err" })}
	if id3.String() != "Cat, err" {
		t.Errorf("Expected 'Cat, err', got '%s'", id3.String())
	}
}

func TestJoinAllThreadPools(t *testing.T) {
	// This test ensures JoinAllThreadPools does not panic
	pool := NewFixedSizeThreadPool("JoinAll", 1)
	defer pool.Join()
	JoinAllThreadPools()
}

func TestFixedSizeThreadPool_OnWorkStartAndFinished(t *testing.T) {
	pool := NewFixedSizeThreadPool("EventPool", 1)
	defer pool.Join()

	started := make(chan struct{}, 1)
	finished := make(chan struct{}, 1)

	handleStart := pool.OnWorkStart(func(e ThreadPoolWorkEvent) error {
		started <- struct{}{}
		return nil
	})
	handleFinish := pool.OnWorkFinished(func(e ThreadPoolWorkEvent) error {
		finished <- struct{}{}
		return nil
	})

	pool.Queue(func(ctx ThreadContext) {}, TASKPRIORITY_NORMAL, ThreadPoolDebugId{Category: "Event"})

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("OnWorkStart not called")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("OnWorkFinished not called")
	}

	if !pool.RemoveOnWorkStart(handleStart) {
		t.Error("Failed to remove OnWorkStart handler")
	}
	if !pool.RemoveOnWorkFinished(handleFinish) {
		t.Error("Failed to remove OnWorkFinished handler")
	}
}
