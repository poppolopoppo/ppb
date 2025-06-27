package base

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRecycler_AllocateRelease(t *testing.T) {
	type testStruct struct{ x int }
	var released bool

	recycler := NewRecycler(
		func() testStruct { return testStruct{x: 42} },
		func(ts testStruct) { released = true },
	)

	item := recycler.Allocate()
	if item.x != 42 {
		t.Errorf("expected 42, got %d", item.x)
	}
	released = false
	recycler.Release(item)
	if !released {
		t.Error("release function was not called")
	}
}

func TestBytesRecycler_AllocateRelease(t *testing.T) {
	stride := 128
	recycler := newBytesRecycler(stride)
	buf := recycler.Allocate()
	if buf == nil || len(*buf) != stride {
		t.Errorf("expected buffer of length %d, got %v", stride, buf)
	}
	recycler.Release(buf)
}

func TestGetBytesRecyclerBySize(t *testing.T) {
	tests := []struct {
		size   int64
		expect int
	}{
		{100, TransientPage4KiB.Stride()},
		{65 << 10, TransientPage64KiB.Stride()},
		{300 << 10, TransientPage256KiB.Stride()},
		{2 << 20, TransientPage1MiB.Stride()},
		{5 << 20, TransientPage4MiB.Stride()},
	}
	for _, tt := range tests {
		r := GetBytesRecyclerBySize(tt.size)
		if r.Stride() != tt.expect {
			t.Errorf("for size %d, expected stride %d, got %d", tt.size, tt.expect, r.Stride())
		}
	}
}

func TestTransientBuffer(t *testing.T) {
	buf := TransientBuffer.Allocate()
	if buf == nil {
		t.Fatal("expected non-nil buffer")
	}
	buf.WriteString("hello")
	TransientBuffer.Release(buf)
	buf2 := TransientBuffer.Allocate()
	if buf2.Len() != 0 {
		t.Error("expected buffer to be reset")
	}
	TransientBuffer.Release(buf2)
}

func TestTransientIoCopy(t *testing.T) {
	src := strings.NewReader(strings.Repeat("a", 1024))
	dst := &bytes.Buffer{}
	pageAlloc := newBytesRecycler(256)
	n, err := TransientIoCopy(context.Background(), dst, src, pageAlloc, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1024 {
		t.Errorf("expected 1024 bytes copied, got %d", n)
	}
	if dst.Len() != 1024 {
		t.Errorf("expected dst.Len() == 1024, got %d", dst.Len())
	}
}

func TestTransientIoCopyWithProgress(t *testing.T) {
	src := strings.NewReader(strings.Repeat("b", 512))
	dst := &bytes.Buffer{}
	pageAlloc := newBytesRecycler(128)
	// ProgressScope and LogProgress/LogSpinner are assumed to be no-ops or mocked in test
	n, err := TransientIoCopyWithProgress(context.Background(), "test", 512, dst, src, pageAlloc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 512 {
		t.Errorf("expected 512 bytes copied, got %d", n)
	}
	if dst.Len() != 512 {
		t.Errorf("expected dst.Len() == 512, got %d", dst.Len())
	}
}
