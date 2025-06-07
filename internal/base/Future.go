package base

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

var LogFuture = NewLogCategory("Future")

type Future[T any] interface {
	Done() chan struct{}
	Join() Result[T]
}

/***************************************
 * Result[T]
 ***************************************/

type Result[S any] interface {
	Success() S
	Failure() error
	Get() (S, error)
}

type result[S any] struct {
	success S
	failure error
}

func (r result[S]) Get() (S, error) {
	return r.success, r.failure
}
func (r result[S]) Success() S {
	LogPanicIfFailed(LogFuture, r.failure)
	return r.success
}
func (r result[S]) Failure() error {
	return r.failure
}
func (r result[S]) String() string {
	if r.failure == nil {
		return fmt.Sprint(r.success)
	} else {
		return r.failure.Error()
	}
}

/***************************************
 * Sync Future (for debug)
 ***************************************/

type debugCallstack struct {
	pcs [10]uintptr
	len int
}

func (x *debugCallstack) capture() {
	x.len = runtime.Callers(3, x.pcs[:])
}

func (x *debugCallstack) String() string {
	sb := strings.Builder{}
	frame := runtime.CallersFrames(x.pcs[:x.len])
	for {
		frame, more := frame.Next()
		fmt.Fprintf(&sb, "%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			break
		}
	}
	return sb.String()
}

//lint:ignore U1000 ignore unused function
type sync_future[T any] struct {
	await   func() (T, error)
	result  *result[T]
	debug   []fmt.Stringer
	barrier sync.Mutex

	make_callstack debugCallstack
	join_callstack debugCallstack
}

//lint:ignore U1000 ignore unused function
func make_sync_future[T any](f func() (T, error), debug ...fmt.Stringer) Future[T] {
	AssertErr(func() error {
		if f != nil {
			return nil
		}
		return fmt.Errorf("invalid future!\n%s", MakeStringerSet(debug...).Join("\n"))
	})
	future := &sync_future[T]{
		await:  f,
		result: nil,
		debug:  debug,
	}
	future.make_callstack.capture()
	return future
}

//lint:ignore U1000 ignore unused function
func (future *sync_future[T]) Done() chan struct{} {
	return nil
}

//lint:ignore U1000 ignore unused function
func (future *sync_future[T]) Join() Result[T] {
	future.barrier.Lock()
	defer future.barrier.Unlock()

	if future.result == nil {
		await := future.await
		AssertErr(func() error {
			if await != nil {
				return nil
			}
			return fmt.Errorf("future reentrancy: await=%p!\n%s\n-> created by:\n%s\n<- joined by:\n%s", await, MakeStringerSet(future.debug...).Join("\n"), &future.make_callstack, &future.join_callstack)
		})
		future.join_callstack.capture()
		future.await = nil
		value, err := await()
		future.result = &result[T]{
			success: value,
			failure: err,
		}
	}
	return future.result
}

/***************************************
 * Async Future (using goroutine)
 ***************************************/

//lint:ignore U1000 ignore unused function
type async_future[T any] struct {
	done   chan struct{}
	result result[T]
}

// each future owns its channel to allow better ParallelJoin_Async
func MakeAsyncFuture[T any](f func() (T, error)) Future[T] {
	return make_async_future(f)
}
func MakeGlobalWorkerFuture[T any](f func(ThreadContext) (T, error), priority TaskPriority, debugId ThreadPoolDebugId) Future[T] {
	return MakeWorkerFuture(GetGlobalThreadPool(), f, priority, debugId)
}
func MakeWorkerFuture[T any](pool ThreadPool, f func(ThreadContext) (T, error), priority TaskPriority, debugId ThreadPoolDebugId) Future[T] {
	future := &async_future[T]{done: make(chan struct{})}
	pool.Queue(func(tc ThreadContext) {
		future.invoke(func() (T, error) {
			return f(tc)
		})
	}, priority, debugId)
	return future
}

//lint:ignore U1000 ignore unused function
func make_async_future[T any](f func() (T, error)) Future[T] {
	return (&async_future[T]{done: make(chan struct{})}).run_in_background(f)
}

//lint:ignore U1000 ignore unused function
func (future *async_future[T]) invoke(f func() (T, error)) {
	defer close(future.done)
	future.result.success, future.result.failure = f()
}

//lint:ignore U1000 ignore unused function
func (future *async_future[T]) Done() chan struct{} {
	return future.done
}

//lint:ignore U1000 ignore unused function
func (future *async_future[T]) Join() Result[T] {
	<-future.done
	return future.result
}

//lint:ignore U1000 ignore unused function
func (future *async_future[T]) run_in_background(f func() (T, error)) *async_future[T] {
	go future.invoke(f)
	return future
}

/***************************************
 * Map Future
 ***************************************/

type map_future_result[OUT, IN any] struct {
	inner     Result[IN]
	transform func(IN) (OUT, error)
}

func (x map_future_result[OUT, IN]) Success() OUT {
	result, err := x.transform(x.inner.Success())
	LogPanicIfFailed(LogFuture, err)
	return result
}
func (x map_future_result[OUT, IN]) Failure() error {
	return x.inner.Failure()
}
func (x map_future_result[OUT, IN]) Get() (OUT, error) {
	if result, err := x.inner.Get(); err == nil {
		return x.transform(result)
	} else {
		var none OUT
		return none, err
	}
}

type map_future[OUT, IN any] struct {
	inner     Future[IN]
	transform func(IN) (OUT, error)
}

func MapFuture[OUT, IN any](future Future[IN], transform func(IN) (OUT, error)) Future[OUT] {
	return map_future[OUT, IN]{inner: future, transform: transform}
}
func (x map_future[OUT, IN]) Done() chan struct{} {
	return x.inner.Done()
}
func (x map_future[OUT, IN]) Join() Result[OUT] {
	return map_future_result[OUT, IN]{
		inner:     x.inner.Join(),
		transform: x.transform,
	}
}

/***************************************
 * Future Literal
 ***************************************/

// one closed channel shared along all future literals
var future_literal_done = Memoize(func() chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
})

type future_literal[T any] struct {
	immediate result[T]
}

func (x future_literal[T]) Done() chan struct{} {
	return future_literal_done() // always returns immediately without blocking
}
func (x future_literal[T]) Join() Result[T] {
	return &x.immediate
}

func MakeFutureLiteral[T any](value T) Future[T] {
	return future_literal[T]{
		immediate: result[T]{
			success: value,
			failure: nil,
		},
	}
}

/***************************************
 * Future Error
 ***************************************/

func MakeFutureError[T any](err error) Future[T] {
	return future_literal[T]{
		immediate: result[T]{
			failure: err,
		},
	}
}

/***************************************
 * Parallel Helpers
 ***************************************/

func ParallelJoin_Sync[T any](each func(int, T) error, futures ...Future[T]) (lastError error) {
	for i, future := range futures {
		if value, err := future.Join().Get(); err == nil {
			if err = each(i, value); err != nil {
				return err
			}
		} else {
			lastError = err
		}
	}
	return
}

const enableParallelJoin_Async = false // TODO: reflect.Select is allocating too much, after profiling it is not worth while

func ParallelJoin_Async[T any](each func(int, T) error, futures ...Future[T]) error {
	if len(futures) == 1 || !enableParallelJoin_Async {
		return ParallelJoin_Sync(each, futures...)
	}

	cases := make([]reflect.SelectCase, len(futures))
	for i, f := range futures {
		cases[i] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(f.Done()),
		}
	}

	for range futures {
		i, _, _ := reflect.Select(cases)

		if value, err := futures[i].Join().Get(); err == nil {
			if err = each(i, value); err != nil {
				return err
			}
		} else {
			return err
		}

		cases[i].Chan = reflect.ValueOf(nil)
	}

	return nil
}

func ParallelRange_Sync[IN any](each func(IN) error, in ...IN) error {
	for _, it := range in {
		if err := each(it); err != nil {
			return err
		}
	}
	return nil
}

func ParallelRange_Async[IN any](each func(IN) error, in ...IN) error {
	if len(in) == 1 {
		return each(in[0])
	}

	var firstErr unsafe.Pointer

	wg := sync.WaitGroup{}
	wg.Add(len(in))

	for _, it := range in {
		go func(input IN) {
			defer wg.Done()
			if err := each(input); err != nil {
				atomic.CompareAndSwapPointer(&firstErr, nil, unsafe.Pointer(&err))
			}
		}(it)
	}

	wg.Wait()
	if firstErr == nil {
		return nil
	} else {
		return *(*error)(firstErr)
	}
}

func ParallelMap_Sync[IN any, OUT any](each func(IN) (OUT, error), in ...IN) ([]OUT, error) {
	results := Map(func(x IN) OUT {
		result, err := each(x)
		if err != nil {
			LogPanicErr(LogFuture, err)
		}
		return result
	}, in...)
	return results, nil
}

func ParallelMap_Async[IN any, OUT any](each func(IN) (OUT, error), in ...IN) ([]OUT, error) {
	if len(in) == 1 {
		it, err := each(in[0])
		return []OUT{it}, err
	}

	var firstErr unsafe.Pointer

	wg := sync.WaitGroup{}
	wg.Add(len(in))

	results := make([]OUT, len(in))
	for i, it := range in {
		go func(id int, input IN) {
			if value, err := each(input); err == nil {
				results[id] = value
			} else {
				atomic.CompareAndSwapPointer(&firstErr, nil, unsafe.Pointer(&err))
			}
			wg.Done()
		}(i, it)
	}

	wg.Wait()
	if firstErr == nil {
		return results, nil
	} else {
		return results, *(*error)(firstErr)
	}
}
