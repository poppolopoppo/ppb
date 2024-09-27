package base

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

const EnableAsyncIO = true

var GetIOReadThreadPool = Memoize(func() (result ThreadPool) {
	result = NewFixedSizeThreadPool("IORead", 2)
	allThreadPools = append(allThreadPools, result)
	return
})
var GetIOWriteThreadPool = Memoize(func() (result ThreadPool) {
	result = NewFixedSizeThreadPool("IOWrite", 1)
	allThreadPools = append(allThreadPools, result)
	return
})

/***************************************
 * Async IO Copy
 ***************************************/

func AsyncTransientIoCopy(dst io.Writer, src io.Reader, pageAlloc BytesRecycler, priority TaskPriority) (n int64, err error) {
	err = WithAsyncWriter(dst, pageAlloc, priority, func(w io.Writer) error {
		return WithAsyncReader(src, pageAlloc, priority, func(r io.Reader) (er error) {
			n, er = io.Copy(w, r)
			return
		})
	})
	return
}

/***************************************
 * Async IO Reader
 ***************************************/

type AsyncReader struct {
	rd io.Reader
	al BytesRecycler

	buf asyncIOBlock
	cur int

	cancel chan struct{}
	queue  <-chan Optional[asyncIOBlock]
}

func NewAsyncReaderSize(reader io.Reader, totalSize int64, priority TaskPriority) AsyncReader {
	return NewAsyncReader(reader, GetBytesRecyclerBySize(totalSize), priority)
}
func NewAsyncReader(reader io.Reader, pageAlloc BytesRecycler, priority TaskPriority) AsyncReader {
	Assert(func() bool {
		if IsNil(reader) {
			return false
		}
		if _, ok := reader.(*bufio.Reader); ok {
			return false
		}
		if _, ok := reader.(*bufio.ReadWriter); ok {
			return false
		}
		return true
	})

	cancel := make(chan struct{})
	queue := make(chan Optional[asyncIOBlock])

	GetIOReadThreadPool().Queue(func(ThreadContext) {
		defer close(queue)
		for {
			select {
			case <-cancel:
				return
			default:
				var buf asyncIOBlock
				var err error
				buf.allocate(pageAlloc)
				buf.off, err = reader.Read(*buf.data)

				if buf.off > 0 {
					queue <- NewOption(buf)
				} else {
					buf.release(pageAlloc)
				}

				if err != nil {
					queue <- UnexpectedOption[asyncIOBlock](err)
					return
				}
			}
		}
	}, priority, ThreadPoolDebugId{Category: "AsyncRead", Arg: MakeStringer(func() string {
		return fmt.Sprintf("@%p <%T> blk=%d", reader, reader, pageAlloc.Stride())
	})})

	return AsyncReader{
		rd:     reader,
		al:     pageAlloc,
		cancel: cancel,
		queue:  queue,
	}
}

func WithAsyncReader(reader io.Reader, pageAlloc BytesRecycler, priority TaskPriority, scope func(io.Reader) error) error {
	if EnableAsyncIO {
		switch rd := reader.(type) {
		case *bufio.ReadWriter:
			return scope(rd)
		case *bufio.Reader:
			return scope(rd)
		case *bytes.Buffer:
			return scope(rd)
		case *AsyncReader:
			return scope(rd)
		default:
			asyncReader := NewAsyncReader(rd, pageAlloc, priority)
			defer asyncReader.Close()
			return scope(&asyncReader)
		}
	} else {
		return scope(reader)
	}
}

func (x *AsyncReader) String() string {
	return fmt.Sprintf("@%p <%T> blk=%d", x.rd, x.rd, x.al.Stride())
}

func (x *AsyncReader) retrieveNextBlock() (err error) {
	if x.buf.data != nil {
		x.buf.release(x.al)
	}
	x.cur = 0
	if read, more := <-x.queue; more {
		x.buf, err = read.Get()
	} else {
		err = io.EOF
	}
	return
}

func (x *AsyncReader) Read(p []byte) (n int, err error) {
	Assert(func() bool { return len(p) <= x.al.Stride() })
	if len(p) == 0 {
		return
	}

	for {
		if x.cur == x.buf.off {
			if err = x.retrieveNextBlock(); err != nil {
				return
			}
		}

		read := min(len(p)-n, x.buf.off-x.cur)
		copy(p[n:n+read], (*x.buf.data)[x.cur:x.cur+read])
		x.cur += read
		n += read

		if n == len(p) {
			return
		}
	}
}

func (x *AsyncReader) Close() (err error) {
	if x.rd == nil {
		return nil
	}

	// cancel remote task and release all read buffers in flight
	AssertNotIn(x.cancel, nil) // Close() already called?
	close(x.cancel)
	x.cancel = nil
	for {
		if err = x.retrieveNextBlock(); err != nil {
			break
		}
	}
	if err == io.EOF {
		err = nil
	}
	if x.buf.data != nil {
		x.buf.release(x.al)
	}
	x.cur = 0

	// close inner reader if it implements Close() method
	if readCloser, ok := x.rd.(io.ReadCloser); ok {
		if er := readCloser.Close(); er != nil && err == nil {
			err = er
		}
	}
	return
}

func asyncWriteTo(wr *AsyncWriter, rd *AsyncReader) (n int64, err error) {
	AssertIn(rd.al, wr.al)
	if err = wr.Flush(); err != nil {
		return
	}

	for {
		if rd.cur == rd.buf.off {
			if err = rd.retrieveNextBlock(); err != nil {
				if err == io.EOF {
					AssertIn(rd.buf.off, 0)
					err = nil
				}
				return
			}
		}

		n += int64(rd.buf.off)
		wr.asyncWriteBuf(rd.buf)

		rd.cur = 0
		rd.buf = asyncIOBlock{} // will be released by asyncWriteBuf()
	}
}

func (x *AsyncReader) WriteTo(writer io.Writer) (n int64, err error) {
	switch wr := writer.(type) {
	case *AsyncWriter:
		return asyncWriteTo(wr, x)
	default:
		for {
			if x.cur == x.buf.off {
				if err = x.retrieveNextBlock(); err != nil {
					if err == io.EOF {
						err = nil
					}
					return
				}
			}

			var written int
			written, err = wr.Write((*x.buf.data)[x.cur:x.buf.off])
			x.cur += written
			n += int64(written)

			if err != nil {
				return
			}
		}
	}
}

/***************************************
 * Async IO Writer
 ***************************************/

type AsyncWriter struct {
	wr  io.Writer
	al  BytesRecycler
	wg  sync.WaitGroup
	buf asyncIOBlock
	err atomic.Pointer[error]
	pri TaskPriority
}

func NewAsyncWriterSize(writer io.Writer, totalSize int64, priority TaskPriority) AsyncWriter {
	return NewAsyncWriter(writer, GetBytesRecyclerBySize(totalSize), priority)
}
func NewAsyncWriter(writer io.Writer, pageAlloc BytesRecycler, priority TaskPriority) AsyncWriter {
	Assert(func() bool {
		if IsNil(writer) {
			return false
		}
		if _, ok := writer.(*bufio.Writer); ok {
			return false
		}
		if _, ok := writer.(*bufio.ReadWriter); ok {
			return false
		}
		return true
	})

	return AsyncWriter{
		wr:  writer,
		al:  pageAlloc,
		pri: priority,
	}
}

func WithAsyncWriter(writer io.Writer, pageAlloc BytesRecycler, priority TaskPriority, scope func(io.Writer) error) error {
	if EnableAsyncIO {
		switch wr := writer.(type) {
		case *bufio.ReadWriter:
			return scope(wr)
		case *bufio.Writer:
			return scope(wr)
		case *bytes.Buffer:
			return scope(wr)
		case *AsyncWriter:
			return scope(wr)
		default:
			asyncWriter := NewAsyncWriter(wr, pageAlloc, priority)
			defer asyncWriter.Close()
			return scope(&asyncWriter)
		}
	} else {
		return scope(writer)
	}
}

func (x *AsyncWriter) asyncWriteBuf(buf asyncIOBlock) {
	Assert(func() bool { return buf.data != nil && buf.off > 0 })
	x.wg.Add(1)
	GetIOWriteThreadPool().Queue(func(tc ThreadContext) {
		defer func() {
			x.wg.Done()
			buf.release(x.al)
		}()
		for off := 0; off < buf.off; {
			if written, err := x.wr.Write((*buf.data)[off:buf.off]); err == nil {
				off += written
			} else {
				x.err.Store(&err)
			}
		}
	}, x.pri, ThreadPoolDebugId{Category: "AsyncWriteBuf", Arg: x})
}
func (x *AsyncWriter) asyncWriteRaw(p []byte) {
	x.wg.Add(1)
	GetIOWriteThreadPool().Queue(func(tc ThreadContext) {
		defer x.wg.Done()
		for off := 0; off < len(p); {
			if written, err := x.wr.Write(p[off:]); err == nil {
				off += written
			} else {
				x.err.Store(&err)
			}
		}
	}, x.pri, ThreadPoolDebugId{Category: "AsyncWriteRaw", Arg: x})
}

func (x *AsyncWriter) String() string {
	return fmt.Sprintf("@%p <%T> blk=%d", x.wr, x.wr, x.al.Stride())
}

func (x *AsyncWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	} else if len(p) > x.al.Stride() {
		x.asyncWriteRaw(p)
		return len(p), nil
	}

	if x.buf.data != nil && len(p)+x.buf.off > len(*x.buf.data) {
		x.asyncWriteBuf(x.buf)
		x.buf = asyncIOBlock{}
	}

	if x.buf.data == nil {
		x.buf.allocate(x.al)
	}

	written := copy((*x.buf.data)[x.buf.off:], p)
	x.buf.off += written
	return written, nil
}

func (x *AsyncWriter) Flush() error {
	if x.buf.off > 0 {
		x.asyncWriteBuf(x.buf)
		x.buf = asyncIOBlock{}
	} else if x.buf.data != nil {
		x.buf.release(x.al)
	}
	return nil
}

func (x *AsyncWriter) Close() (err error) {
	if err = x.Flush(); err != nil {
		return err
	}

	x.wg.Wait()

	if writeCloser, ok := x.wr.(io.WriteCloser); ok {
		err = writeCloser.Close()
	}

	if er := x.err.Load(); er != nil {
		err = *er
	}
	return
}

func (x *AsyncWriter) ReadFrom(reader io.Reader) (n int64, err error) {
	switch rd := reader.(type) {
	case *AsyncReader:
		return asyncWriteTo(x, rd)

	default:
		if err = x.Flush(); err != nil {
			return
		}

		for {
			x.buf.allocate(x.al)
			x.buf.off, err = rd.Read(*x.buf.data)

			n += int64(x.buf.off)
			x.Flush()

			if err != nil {
				if err == io.EOF {
					err = nil
				}
				return
			}
		}
	}
}

/***************************************
 * Async IO Block
 ***************************************/

type asyncIOBlock struct {
	data *[]byte
	off  int
}

func (x *asyncIOBlock) allocate(al BytesRecycler) {
	AssertIn(x.data, nil)
	x.data, x.off = al.Allocate(), 0
}
func (x *asyncIOBlock) release(al BytesRecycler) {
	AssertNotIn(x.data, nil)
	al.Release(x.data)
	x.data, x.off = nil, 0
}
