package base

import (
	"bytes"
	"io"
	"sync"
	"unsafe"
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

const SMALL_PAGE_CAPACITY = (4 << 10) // 4 KiB
const LARGE_PAGE_CAPACITY = (1 << 20) // 1 MiB - SHOULD BE EQUALS TO ONE OF PREDEFINED LZ4.BLOCKSIZE! (64KiB,256KiB,1MiB,4MiB)

// recycle temporary buffers
var TransientLargePage = NewRecycler(
	func() []byte { return make([]byte, LARGE_PAGE_CAPACITY) },
	func([]byte) {})
var TransientSmallPage = NewRecycler(
	func() []byte { return make([]byte, SMALL_PAGE_CAPACITY) },
	func([]byte) {})

// recycle byte buffers
var TransientBuffer = NewRecycler(
	func() *bytes.Buffer { return &bytes.Buffer{} },
	func(b *bytes.Buffer) {
		b.Reset()
	})

func UnsafeBytesFromString(in string) []byte {
	return unsafe.Slice(unsafe.StringData(in), len(in))
}
func UnsafeStringFromBytes(raw []byte) string {
	// from func (strings.Builder) String() string
	return unsafe.String(unsafe.SliceData(raw), len(raw))
}
func UnsafeStringFromBuffer(buf *bytes.Buffer) string {
	// from func (strings.Builder) String() string
	return UnsafeStringFromBytes(buf.Bytes())
}

// recycle channels for Future[T]
var AnyChannels = NewRecycler(
	func() chan any { return make(chan any, 1) },
	func(chan any) {})

const useTransientIoCopyOverIoCopy = true
const useTransientIoCopyAsynchronous = true

// overlap read and write with double-buffering
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
			buf = TransientLargePage.Allocate()
			defer TransientLargePage.Release(buf) // first time where defer scope is handy :p
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

			if i == 0 && view.size < len(buf) {
				// do not create channels and goroutine if the first read already exhausted the stream
				// ONLY SAFE FOR THE FIRST BLOCK AND NEED TO VALIDATE EOF TO BE SURE
				if er == nil {
					// try to read the remaining part of the buffer to check for EOF
					nr, er = src.Read(buf[view.size:])
					if nr > 0 {
						readerSize += int64(view.size)
						Assert(func() bool { return view.size <= len(view.buf) })
					}
				}
				if er != nil {
					if ew := write_block(view); ew != nil {
						er = ew
					}
					buf = nil // skip async writer launchpad
				}
			}

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

	if writerErr != nil {
		return writerSize, writerErr
	} else {
		AssertIn(readerSize, writerSize)
		return writerSize, readerErr
	}
}

// io copy with transient bytes to replace io.Copy()
func TransientIoCopy(dst io.Writer, src io.Reader) (size int64, err error) {
	if useTransientIoCopyOverIoCopy {
		if useTransientIoCopyAsynchronous {
			return AsyncTransientIoCopy(dst, src)
		} else {
			// From io.Copy(), but with TransientBytes recycler:
			/*if wt, ok := src.(io.WriterTo); ok {
				// If the reader has a WriteTo method, use it to do the copy.
				// Avoids an allocation and a copy.
				_, err = wt.WriteTo(dst)
			} else if rt, ok := dst.(io.ReaderFrom); ok {
				// Similarly, if the writer has a ReadFrom method, use it to do the copy.
				_, err = rt.ReadFrom(src)
			} else*/{
				// io.Copy() will make a temporary allocation, and we have a recycler for this
				buf := TransientLargePage.Allocate()
				defer TransientLargePage.Release(buf)

				for {
					nr, er := src.Read(buf)
					if nr > 0 {
						nw, ew := dst.Write(buf[0:nr])
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
		}
	} else {
		buf := TransientLargePage.Allocate()
		defer TransientLargePage.Release(buf)

		size, err = io.CopyBuffer(dst, src, buf)
	}

	if err == io.EOF {
		err = nil
	}
	return
}
