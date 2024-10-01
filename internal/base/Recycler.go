package base

import (
	"bytes"
	"io"
	"sync"
)

/***************************************
 * Recycler[T] is a generic sync.Pool
 ***************************************/

type Recycler[T any] interface {
	Allocate() T
	Release(T)
}

type recyclerPool[T any] struct {
	pool      sync.Pool
	onRelease func(T)
}

func NewRecycler[T any](factory func() T, release func(T)) Recycler[T] {
	result := &recyclerPool[T]{}
	result.pool.New = func() any { return factory() }
	result.onRelease = release
	return result
}
func (x *recyclerPool[T]) Allocate() (result T) {
	result = x.pool.Get().(T)
	return
}
func (x *recyclerPool[T]) Release(item T) {
	x.onRelease(item)
	x.pool.Put(item)
}

/***************************************
 * Recycle temporary byte arrays
 ***************************************/

type bytesRecyclerPool struct {
	stride int
	pool   sync.Pool
}

type BytesRecycler interface {
	Stride() int
	Recycler[*[]byte]
}

func newBytesRecycler(stride int) BytesRecycler {
	result := new(bytesRecyclerPool)
	result.stride = stride
	result.pool.New = func() any {
		buf := make([]byte, result.stride)
		return &buf
	}
	return result
}
func (x *bytesRecyclerPool) Stride() int { return x.stride }
func (x *bytesRecyclerPool) Allocate() (item *[]byte) {
	item = x.pool.Get().(*[]byte)
	Assert(func() bool { return item != nil && len(*item) == x.stride })
	return
}
func (x *bytesRecyclerPool) Release(item *[]byte) {
	Assert(func() bool { return item != nil && len(*item) == x.stride })
	x.pool.Put(item)
}

var TransientPage1MiB = newBytesRecycler(1 << 20) // SHOULD BE EQUALS TO ONE OF PREDEFINED LZ4.BLOCKSIZE! (64KiB,256KiB,1MiB,4MiB)
var TransientPage256KiB = newBytesRecycler(256 << 10)
var TransientPage64KiB = newBytesRecycler(64 << 10)
var TransientPage4KiB = newBytesRecycler(4 << 10)

func GetBytesRecyclerBySize(size int64) BytesRecycler {
	pageAlloc := TransientPage4KiB
	if 2*size > int64(TransientPage64KiB.Stride()) {
		pageAlloc = TransientPage64KiB
		if 2*size > int64(TransientPage256KiB.Stride()) {
			pageAlloc = TransientPage256KiB
			if 2*size > int64(TransientPage1MiB.Stride()) {
				pageAlloc = TransientPage1MiB
			}
		}
	}
	return pageAlloc
}

/***************************************
 * Share LZ4 pool for 1MiB/64KiB blocks
 ***************************************/

// #TODO: lz4 recycler is private

// type bytesRecyclerPoolWrapper struct {
// 	stride int
// 	pool   *sync.Pool
// }

// func newBytesRecyclerWrapper(stride int, pool *sync.Pool) bytesRecyclerPoolWrapper {
// 	return bytesRecyclerPoolWrapper{stride: stride, pool: pool}
// }

// func (x bytesRecyclerPoolWrapper) Stride() int      { return x.stride }
// func (x bytesRecyclerPoolWrapper) Allocate() []byte { return x.pool.Get().([]byte) }
// func (x bytesRecyclerPoolWrapper) Release(p []byte) { x.pool.Put(p) }

// var TransientPage64KiB = newBytesRecyclerWrapper(int(lz4.Block64Kb), lz4.BlockPool64K)
// var TransientPage1MiB = newBytesRecyclerWrapper(int(lz4.Block1Mb), lz4.BlockPool1M)

/***************************************
 * Recycle bytes buffers
 ***************************************/

var TransientBuffer = NewRecycler(
	func() *bytes.Buffer { return &bytes.Buffer{} },
	func(b *bytes.Buffer) {
		b.Reset()
	})

/***************************************
 * Stream copy using previous recycler and asynchronous IO (when profitable)
 ***************************************/

// overlap read and write with double-buffering
const useTransientIoCopyOverIoCopy = true

// io copy with transient bytes to replace io.Copy()
func TransientIoCopy(dst io.Writer, src io.Reader, pageAlloc BytesRecycler, allowAsync bool) (size int64, err error) {
	if useTransientIoCopyOverIoCopy {
		return AsyncTransientIoCopy(dst, src, pageAlloc, TASKPRIORITY_NORMAL)
	} else {
		buf := pageAlloc.Allocate()
		defer pageAlloc.Release(buf)

		size, err = io.CopyBuffer(dst, src, *buf)
	}

	if err == io.EOF {
		err = nil
	}
	return
}

func TransientIoCopyWithProgress(context string, totalSize int64, dst io.Writer, src io.Reader, pageAlloc BytesRecycler) (size int64, err error) {
	var pbar ProgressScope
	if totalSize > 0 {
		pbar = LogProgress(0, totalSize, "copying %s -- %.3f MiB", context, float32(totalSize)/(1024*1024))
	} else {
		pbar = LogSpinner("copying %s -- unknown size", context)
	}
	defer pbar.Close()

	allowAsync := totalSize > int64(pageAlloc.Stride())
	return TransientIoCopy(NewObservableWriter(dst, func(w io.Writer) func(int64, error) error {
		return func(n int64, err error) error {
			pbar.Add(n)
			return err
		}
	}), src, pageAlloc, allowAsync)
}
