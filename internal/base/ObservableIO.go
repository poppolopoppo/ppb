package base

import "io"

// can be done, but we lose all incremental updates and just get one progress message...
const enableObservableReaderFromWriterTo = false

/***************************************
 * Observable Writer
 ***************************************/

type ObservableWriterFunc = func(io.Writer) func(int64, error) error

type ObservableWriter struct {
	io.Writer
	OnWrite ObservableWriterFunc
}

type ObsersvableReaderFrom struct {
	ObservableWriter
}

func NewObservableWriter(w io.Writer, onWrite ObservableWriterFunc) io.Writer {
	Assert(func() bool { return w != nil })
	if onWrite == nil {
		return w
	}
	result := ObservableWriter{
		Writer:  w,
		OnWrite: onWrite,
	}
	if enableObservableReaderFromWriterTo {
		if _, ok := w.(io.ReaderFrom); ok {
			return ObsersvableReaderFrom{
				ObservableWriter: result,
			}
		}
	}
	return result
}

func (x ObservableWriter) Flush() error {
	return FlushWriterIFP(x.Writer)
}
func (x ObservableWriter) Close() error {
	return CloseWriterIFP(x.Writer)
}
func (x *ObservableWriter) Reset(w io.Writer) error {
	if err := FlushWriterIFP(x.Writer); err != nil {
		return err
	}
	if rst, ok := x.Writer.(WriteReseter); ok {
		return rst.Reset(w)
	} else {
		x.Writer = w
	}
	return nil
}
func (x ObservableWriter) Write(buf []byte) (n int, err error) {
	onWrite := x.OnWrite(x.Writer)
	n, err = x.Writer.Write(buf)
	if er := onWrite(int64(n), err); er != nil {
		err = er
	}
	return
}

func (x ObsersvableReaderFrom) ReadFrom(r io.Reader) (n int64, err error) {
	onWrite := x.OnWrite(x.Writer)
	n, err = x.Writer.(io.ReaderFrom).ReadFrom(r)
	if er := onWrite(n, err); er != nil {
		err = er
	}
	return
}

/***************************************
 * Observable Reader
 ***************************************/

type ObservableReaderFunc = func(io.Reader) func(int64, error) error

type ObservableReader struct {
	io.Reader
	OnRead ObservableReaderFunc
}

type ObservableWriterTo struct {
	ObservableReader
}

func NewObservableReader(r io.Reader, onRead ObservableReaderFunc) io.Reader {
	Assert(func() bool { return r != nil })
	Assert(func() bool { return onRead != nil })
	if onRead == nil {
		return r
	}
	result := ObservableReader{
		Reader: r,
		OnRead: onRead,
	}
	if enableObservableReaderFromWriterTo {
		if _, ok := r.(io.WriterTo); ok {
			return ObservableWriterTo{
				ObservableReader: result,
			}
		}
	}
	return result
}
func (x ObservableReader) Close() error {
	return CloseReaderIFP(x.Reader)
}
func (x *ObservableReader) Reset(r io.Reader) error {
	if rst, ok := x.Reader.(ReadReseter); ok {
		return rst.Reset(r)
	} else {
		x.Reader = r
	}
	return nil
}
func (x ObservableReader) Read(buf []byte) (n int, err error) {
	onRead := x.OnRead(x.Reader)
	n, err = x.Reader.Read(buf)
	if er := onRead(int64(n), err); er != nil {
		err = er
	}
	return
}

func (x ObservableWriterTo) WriteTo(w io.Writer) (n int64, err error) {
	onRead := x.OnRead(x.Reader)
	n, err = x.Reader.(io.WriterTo).WriteTo(w)
	if er := onRead(n, err); er != nil {
		err = er
	}
	return
}
