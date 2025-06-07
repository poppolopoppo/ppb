package base

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	rand "math/rand/v2"
)

func generateRandomData(_ *testing.T, sz int) []byte {
	result := make([]byte, sz)
	rng := rand.NewPCG(42, 0xdeadbeef) // Use a fixed seed for reproducibility

	i := 0
	for ; i+8 <= sz; i += 8 {
		val := rng.Uint64()
		result[i+0] = byte(val)
		result[i+1] = byte(val >> 8)
		result[i+2] = byte(val >> 16)
		result[i+3] = byte(val >> 24)
		result[i+4] = byte(val >> 32)
		result[i+5] = byte(val >> 40)
		result[i+6] = byte(val >> 48)
		result[i+7] = byte(val >> 56)
	}
	if i < sz {
		val := rng.Uint64()
		for j := 0; i < sz; i, j = i+1, j+1 {
			result[i] = byte(val >> (8 * j))
		}
	}
	return result
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
