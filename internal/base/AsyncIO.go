package base

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
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

func AsyncTransientIoCopy(ctx context.Context, dst io.Writer, src io.Reader, pageAlloc BytesRecycler, priority TaskPriority) (n int64, err error) {
	err = WithAsyncReaderFrom(dst, pageAlloc, priority, func(w io.Writer) error {
		return WithAsyncWriterTo(ctx, src, pageAlloc, priority, func(r io.Reader) (er error) {
			n, er = io.Copy(w, r)
			return
		})
	})
	return
}

/***************************************
 * WithAsyncReader
 ***************************************/

func alwaysWithAsyncReader(ctx context.Context, reader io.Reader, pageAlloc BytesRecycler, priority TaskPriority, scope func(io.Reader) error) error {
	asyncReader := NewAsyncReader(ctx, reader, pageAlloc, priority)
	defer asyncReader.Close()
	return scope(&asyncReader)
}

func WithAsyncWriterTo(ctx context.Context, reader io.Reader, pageAlloc BytesRecycler, priority TaskPriority, scope func(io.Reader) error) error {
	if EnableAsyncIO {
		var dispatch func(actual io.Reader) error
		dispatch = func(actual io.Reader) error {
			switch rd := actual.(type) {
			case ObservableReader:
				return dispatch(rd.Reader)
			case *os.File:
				return alwaysWithAsyncReader(ctx, reader, pageAlloc, priority, scope)
			case io.WriterTo:
				return scope(reader)
			case CompressedReader:
				return alwaysWithAsyncReader(ctx, reader, pageAlloc, priority, scope)
			default:
				return scope(reader)
			}
		}
		return dispatch(reader)
	} else {
		return scope(reader)
	}
}

func WithAsyncReader(ctx context.Context, reader io.Reader, pageAlloc BytesRecycler, priority TaskPriority, scope func(io.Reader) error) error {
	if EnableAsyncIO {
		var dispatch func(actual io.Reader) error
		dispatch = func(actual io.Reader) error {
			switch rd := actual.(type) {
			case ObservableReader:
				return dispatch(rd.Reader)
			case *os.File:
				return alwaysWithAsyncReader(ctx, reader, pageAlloc, priority, scope)
			case CompressedReader:
				return alwaysWithAsyncReader(ctx, reader, pageAlloc, priority, scope)
			default:
				return scope(reader)
			}
		}
		return dispatch(reader)
	} else {
		return scope(reader)
	}
}

/***************************************
 * WithAsyncWriter
 ***************************************/

func alwaysWithAsyncWriter(writer io.Writer, pageAlloc BytesRecycler, priority TaskPriority, scope func(io.Writer) error) error {
	asyncWriter := NewAsyncWriter(writer, pageAlloc, priority)
	defer asyncWriter.Close()
	if err := scope(&asyncWriter); err == nil {
		return asyncWriter.Flush()
	} else {
		return err
	}
}

func WithAsyncReaderFrom(writer io.Writer, pageAlloc BytesRecycler, priority TaskPriority, scope func(io.Writer) error) error {
	if EnableAsyncIO {
		var dispatch func(actual io.Writer) error
		dispatch = func(actual io.Writer) error {
			switch wr := actual.(type) {
			case ObservableWriter:
				return dispatch(wr.Writer)
			case *os.File:
				return alwaysWithAsyncWriter(writer, pageAlloc, priority, scope)
			case io.ReaderFrom:
				return scope(writer)
			case http.ResponseWriter:
				return alwaysWithAsyncWriter(writer, pageAlloc, priority, scope)
			case CompressedWriter:
				return alwaysWithAsyncWriter(writer, pageAlloc, priority, scope)
			default:
				return scope(writer)
			}
		}
		return dispatch(writer)
	} else {
		return scope(writer)
	}
}
func WithAsyncWriter(writer io.Writer, pageAlloc BytesRecycler, priority TaskPriority, scope func(io.Writer) error) error {
	if EnableAsyncIO {
		var dispatch func(actual io.Writer) error
		dispatch = func(actual io.Writer) error {
			switch wr := actual.(type) {
			case ObservableWriter:
				return dispatch(wr.Writer)
			case *os.File:
				return alwaysWithAsyncWriter(writer, pageAlloc, priority, scope)
			case http.ResponseWriter:
				return alwaysWithAsyncWriter(writer, pageAlloc, priority, scope)
			case CompressedWriter:
				return alwaysWithAsyncWriter(writer, pageAlloc, priority, scope)
			default:
				return scope(writer)
			}
		}
		return dispatch(writer)
	} else {
		return scope(writer)
	}
}

/***************************************
 * Async IO Reader
 ***************************************/

type AsyncReader struct {
	rd io.Reader
	al BytesRecycler

	buf asyncIOBlock
	cur int

	context context.Context
	cancel  context.CancelFunc
	queue   <-chan Optional[asyncIOBlock]
}

func NewAsyncReaderSize(ctx context.Context, reader io.Reader, totalSize int64, priority TaskPriority) AsyncReader {
	return NewAsyncReader(ctx, reader, GetBytesRecyclerBySize(totalSize), priority)
}
func NewAsyncReader(ctx context.Context, reader io.Reader, pageAlloc BytesRecycler, priority TaskPriority) AsyncReader {
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

	reader_ctx, cancel := context.WithCancel(ctx)
	queue := make(chan Optional[asyncIOBlock])

	GetIOReadThreadPool().Queue(func(ThreadContext) {
		defer close(queue)
		for {
			select {
			case <-reader_ctx.Done():
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
		rd:      reader,
		al:      pageAlloc,
		context: reader_ctx,
		cancel:  cancel,
		queue:   queue,
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
	AssertNotIn(x.context, nil)
	// cancel remote task and release all read buffers in flight
	x.cancel()
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
	x.context = nil
	x.cancel = nil
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

func (x *AsyncWriter) Err() (err error) {
	if er := x.err.Load(); er != nil {
		err = *er
	}
	return
}

func (x *AsyncWriter) String() string {
	return fmt.Sprintf("@%p <%T> blk=%d", x.wr, x.wr, x.al.Stride())
}

func (x *AsyncWriter) Write(p []byte) (written int, err error) {
	Assert(func() bool { return x.al.Stride() > len(p) })

	for len(p) > 0 {
		if x.buf.data == nil {
			x.buf.allocate(x.al)
		}

		n := min(len(*x.buf.data)-x.buf.off, len(p))
		copy((*x.buf.data)[x.buf.off:x.buf.off+n], p[:n])

		x.buf.off += n
		written += n
		p = p[n:]

		if x.buf.off == len(*x.buf.data) {
			x.asyncWriteBuf(x.buf)
			x.buf = asyncIOBlock{}
		}

		if err = x.Err(); err != nil {
			break
		}
	}
	return
}

func (x *AsyncWriter) Flush() error {
	if x.buf.off > 0 {
		x.asyncWriteBuf(x.buf)
		x.buf = asyncIOBlock{}
	} else if x.buf.data != nil {
		x.buf.release(x.al)
	}
	return x.Err()
}

func (x *AsyncWriter) Close() (err error) {
	x.Flush()
	x.wg.Wait()

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
