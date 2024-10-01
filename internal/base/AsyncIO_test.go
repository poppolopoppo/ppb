package base

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"golang.org/x/exp/rand"
)

func generateRandomData(t *testing.T, sz int) (result []byte) {
	result = make([]byte, sz)
	_, err := rand.New(rand.NewSource(42)).Read(result)
	if err != nil {
		t.Error(err)
	}
	return nil
}

func TestAsyncReaderEarlyClose(t *testing.T) {
	tmp := bytes.NewBuffer(generateRandomData(t, 218732))
	src := NewAsyncReader(tmp, TransientPage4KiB, TASKPRIORITY_NORMAL)
	if err := src.Close(); err != nil {
		t.Error(err)
	}
}

func TestAsyncReaderString(t *testing.T) {
	input := "this is test string"
	src := strings.NewReader(input)
	err := WithAsyncReader(src, TransientPage4KiB, TASKPRIORITY_NORMAL, func(r io.Reader) error {
		dst, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		if input != UnsafeStringFromBytes(dst) {
			return errors.New("written string doesn't match read one")
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func TestAsyncReaderLarge(t *testing.T) {
	input := generateRandomData(t, 218732)
	src := bytes.NewBuffer(input)
	err := WithAsyncReader(src, TransientPage4KiB, TASKPRIORITY_NORMAL, func(r io.Reader) error {
		dst, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		if !bytes.Equal(input, dst) {
			return errors.New("read string doesn't match read one")
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func TestAsyncWriterEarlyClose(t *testing.T) {
	tmp := bytes.Buffer{}
	dst := NewAsyncWriter(&tmp, TransientPage4KiB, TASKPRIORITY_NORMAL)
	if err := dst.Close(); err != nil {
		t.Error(err)
	}
}

func TestAsyncWriterString(t *testing.T) {
	input := "this is test string"
	dst := bytes.Buffer{}
	err := WithAsyncWriter(&dst, TransientPage4KiB, TASKPRIORITY_NORMAL, func(w io.Writer) error {
		_, err := w.Write(UnsafeBytesFromString(input))
		if err != nil {
			return err
		}
		if input != UnsafeStringFromBytes(dst.Bytes()) {
			return errors.New("written string doesn't match read one")
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func TestAsyncWriterLarge(t *testing.T) {
	input := generateRandomData(t, 218732)
	dst := bytes.Buffer{}
	err := WithAsyncWriter(&dst, TransientPage4KiB, TASKPRIORITY_NORMAL, func(w io.Writer) error {
		_, err := w.Write(input)
		if err != nil {
			return err
		}
		if !bytes.Equal(input, dst.Bytes()) {
			return errors.New("written string doesn't match read one")
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}
