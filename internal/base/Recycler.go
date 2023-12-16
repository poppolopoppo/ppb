package base

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
)

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

// recycle temporary buffers
type bytesRecyclerPool struct {
	stride int
	pool   sync.Pool
}

type TransientPage struct {
	Raw []byte
}
type BytesRecycler interface {
	Stride() int
	Recycler[*TransientPage]
}

func newBytesRecycler(stride int) BytesRecycler {
	result := &bytesRecyclerPool{stride: stride}
	result.pool.New = func() any {
		return &TransientPage{
			Raw: make([]byte, result.stride),
		}
	}
	return result
}
func (x *bytesRecyclerPool) Stride() int { return x.stride }
func (x *bytesRecyclerPool) Allocate() *TransientPage {
	return x.pool.Get().(*TransientPage)
}
func (x *bytesRecyclerPool) Release(item *TransientPage) {
	Assert(func() bool { return len(item.Raw) == x.stride })
	x.pool.Put(item)
}

var TransientPage1MiB = newBytesRecycler(1 << 20) // SHOULD BE EQUALS TO ONE OF PREDEFINED LZ4.BLOCKSIZE! (64KiB,256KiB,1MiB,4MiB)
var TransientPage64KiB = newBytesRecycler(64 << 10)
var TransientPage4KiB = newBytesRecycler(4 << 10)

// recycle byte buffers
var TransientBuffer = NewRecycler(
	func() *bytes.Buffer { return &bytes.Buffer{} },
	func(b *bytes.Buffer) {
		b.Reset()
	})

// overlap read and write with double-buffering
const useTransientIoCopyOverIoCopy = true
const useTransientIoCopyAsynchronous = true

func AsyncTransientIoCopy(dst io.Writer, src io.Reader) (int64, error) {
	// pass a reusable buffer + size to keep og buffer size known
	type data_view struct {
		buf  []byte
		size int
	}

	// write source block to destination, checking for errors
	var writerErr error
	var writerSize int64
	write_block := func(view data_view) error {
		nw, ew := dst.Write(view.buf[:view.size])
		if nw != view.size {
			if ew == nil {
				ew = io.ErrShortWrite
			}
		}
		if nw > 0 {
			writerSize += int64(nw)
		}
		if ew == nil {
			return nil
		}
		writerErr = ew
		return ew
	}

	// spawns asynchronous writer goroutine, if needed
	var writerWg sync.WaitGroup
	var writerChannel chan data_view
	var readerChannel chan []byte
	launch_writer := func() {
		writerChannel = make(chan data_view, 2)
		readerChannel = make(chan []byte, 2)
		writerWg.Add(1)
		go func() {
			defer func() {
				close(readerChannel)
				writerWg.Done()
			}()
			for view := range writerChannel {
				er := write_block(view)
				readerChannel <- view.buf
				if er != nil {
					break
				}
			}
		}()
	}

	// read source stream synchronously and queue them to writer goroutine, with double-buffering if needed
	var readerErr error
	var readerSize int64
	for i := 0; ; i++ {
		var buf []byte
		if i < 2 {
			// lazily allocate 2 blocks, only 1 if no more is needed
			transientPage := TransientPage1MiB.Allocate()
			defer TransientPage1MiB.Release(transientPage) // first time where defer scope is handy :p
			buf = transientPage.Raw
		} else {
			// reuse already allocated blocks
			buf = <-readerChannel
		}

		// read from source
		nr, er := src.Read(buf)

		// check if something was read
		if nr > 0 {
			view := data_view{buf: buf, size: nr}
			readerSize += int64(view.size)

			// #TODO: reading 0 bytes won't return EOF, but reading >0 could trigger a blocking (particularly with a http download) and defeat the async goal of this function
			// if i == 0 && view.size < len(buf) {
			// 	// do not create channels and goroutine if the first read already exhausted the stream
			// 	// ONLY SAFE FOR THE FIRST BLOCK AND NEED TO VALIDATE EOF TO BE SURE
			// 	if er == nil {
			// 		// try to read the remaining part of the buffer to check for EOF
			// 		nr, er = src.Read(buf[view.size:view.size])
			// 		if nr > 0 {
			// 			view.size += nr
			// 			readerSize += int64(nr)

			// 			Assert(func() bool { return view.size <= len(view.buf) })
			// 		}
			// 	}
			// 	if er != nil {
			// 		if ew := write_block(view); ew != nil {
			// 			er = ew
			// 		}
			// 		buf = nil // skip async writer launchpad
			// 	}
			// }

			if buf != nil {
				// asynchronously write, allowing read/write overlap
				if writerChannel == nil {
					launch_writer()
				}
				writerChannel <- view
			}

			buf = nil // consumed, do not put back in pool
		}

		if er != nil {
			// EOF does not fail the function
			if er != io.EOF {
				readerErr = er
			}
			break
		} else if buf != nil {
			// put back buffer in free list if not consumed
			readerChannel <- buf
		}
	}

	if writerChannel != nil {
		// wait for asynchronous writer goroutine (if summoned)
		close(writerChannel)
		writerWg.Wait()
	}

	if writerErr == nil {
		if readerErr == nil && readerSize != writerSize {
			readerErr = fmt.Errorf("AsyncTransientIoCopy: read %d bytes, but wrote %d bytes", readerSize, writerSize)
		}
		return writerSize, readerErr
	}

	return writerSize, writerErr
}

// io copy with transient bytes to replace io.Copy()
func TransientIoCopy(dst io.Writer, src io.Reader) (size int64, err error) {
	if wt, ok := src.(io.WriterTo); ok {
		// If the reader has a WriteTo method, use it to do the copy.
		// Avoids an allocation and a copy.
		return wt.WriteTo(dst)
	} else if rt, ok := dst.(io.ReaderFrom); ok {
		hasNonGenericOverride := true
		IfWindows(func() {
			// os.File on Windows fallbacks on io.Copy, and we prefer our version in this case
			_, ok := dst.(*os.File)
			hasNonGenericOverride = !ok
		})
		if hasNonGenericOverride {
			// Similarly, if the writer has a ReadFrom method, use it to do the copy.
			return rt.ReadFrom(src)
		}
	}

	if useTransientIoCopyOverIoCopy {
		if useTransientIoCopyAsynchronous {
			return AsyncTransientIoCopy(dst, src)
		} else {
			// io.Copy() will make a temporary allocation, and we have a recycler for this
			buf := TransientPage1MiB.Allocate()
			defer TransientPage1MiB.Release(buf)

			for {
				nr, er := src.Read(buf.Raw)
				if nr > 0 {
					nw, ew := dst.Write(buf.Raw[0:nr])
					if nw < 0 || nr < nw {
						nw = 0
						if ew == nil {
							ew = io.ErrShortWrite
						}
					}
					size += int64(nw)
					if ew != nil {
						err = ew
						break
					}
					if nr != nw {
						err = io.ErrShortWrite
						break
					}
				}
				if er != nil {
					if er != io.EOF {
						err = er
					}
					break
				}
			}
		}
	} else {
		buf := TransientPage1MiB.Allocate()
		defer TransientPage1MiB.Release(buf)

		size, err = io.CopyBuffer(dst, src, buf.Raw)
	}

	if err == io.EOF {
		err = nil
	}
	return
}

func TransientIoCopyWithProgress(context string, totalSize int64, dst io.Writer, src io.Reader) (size int64, err error) {
	var pbar ProgressScope
	if totalSize > 0 {
		pbar = LogProgress(0, totalSize, "copying %s -- %.3f MiB", context, float32(totalSize)/(1024*1024))
	} else {
		pbar = LogSpinner("copying %s -- unknown size", context)
	}
	defer pbar.Close()

	if wt, ok := src.(io.WriterTo); ok {
		// If the reader has a WriteTo method, use it to do the copy.
		// Avoids an allocation and a copy.
		return wt.WriteTo(WriterWithProgress{writer: dst, pbar: pbar})
	} else if rt, ok := dst.(io.ReaderFrom); ok {
		hasNonGenericOverride := true
		IfWindows(func() {
			// os.File on Windows fallbacks on io.Copy, and we prefer our version in this case
			_, ok := dst.(*os.File)
			hasNonGenericOverride = !ok
		})
		if hasNonGenericOverride {
			// Similarly, if the writer has a ReadFrom method, use it to do the copy.
			return rt.ReadFrom(ReaderWithProgress{reader: src, pbar: pbar})
		}
	}

	return TransientIoCopy(WriterWithProgress{writer: dst, pbar: pbar}, src)
}
