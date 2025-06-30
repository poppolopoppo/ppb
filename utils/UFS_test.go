package utils

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
)

func TestUFS_Dir_File(t *testing.T) {
	tmpDir := t.TempDir()
	dir := UFS.Dir(tmpDir)
	if dir.String() != tmpDir {
		t.Errorf("UFS.Dir: expected %s, got %v", tmpDir, dir.String())
	}
	file := UFS.File(filepath.Join(tmpDir, "foo.txt"))
	if file.String() != filepath.Join(tmpDir, "foo.txt") {
		t.Errorf("UFS.File: expected %s, got %v", filepath.Join(tmpDir, "foo.txt"), file.String())
	}
}

func TestUFS_Mkdir_MkdirEx(t *testing.T) {
	tmpDir := t.TempDir()
	dir := UFS.Dir(filepath.Join(tmpDir, "subdir"))
	UFS.Mkdir(dir)
	if _, err := os.Stat(dir.String()); err != nil {
		t.Fatalf("UFS.Mkdir: expected directory to exist: %v", err)
	}
	// MkdirEx should not fail if directory already exists
	if err := UFS.MkdirEx(dir); err != nil {
		t.Errorf("UFS.MkdirEx: expected no error, got %v", err)
	}
}

func TestUFS_Create_CreateWriter_CreateFile_CreateBuffered(t *testing.T) {
	tmpDir := t.TempDir()
	file := UFS.File(filepath.Join(tmpDir, "file.txt"))
	content := []byte("hello world")
	// Create
	err := UFS.Create(file, func(w io.Writer) error {
		_, err := w.Write(content)
		return err
	})
	if err != nil {
		t.Fatalf("UFS.Create: %v", err)
	}
	// CreateWriter
	f, err := UFS.CreateWriter(file)
	if err != nil {
		t.Fatalf("UFS.CreateWriter: %v", err)
	}
	f.Write([]byte("!"))
	f.Close()
	// CreateFile
	file2 := UFS.File(filepath.Join(tmpDir, "file2.txt"))
	err = UFS.CreateFile(file2, func(f *os.File) error {
		_, err := f.Write([]byte("abc"))
		return err
	})
	if err != nil {
		t.Fatalf("UFS.CreateFile: %v", err)
	}
	// CreateBuffered
	file3 := UFS.File(filepath.Join(tmpDir, "file3.txt"))
	err = UFS.CreateBuffered(file3, func(w io.Writer) error {
		_, err := w.Write([]byte("buffered"))
		return err
	}, base.TransientPage4KiB)
	if err != nil {
		t.Fatalf("UFS.CreateBuffered: %v", err)
	}
}

func TestUFS_ReadAll_Read_ReadLines(t *testing.T) {
	tmpDir := t.TempDir()
	file := UFS.File(filepath.Join(tmpDir, "readme.txt"))
	lines := []string{"foo", "bar", "baz"}
	UFS.Create(file, func(w io.Writer) error {
		for _, l := range lines {
			w.Write([]byte(l + "\n"))
		}
		return nil
	})
	// ReadAll
	data, err := UFS.ReadAll(file)
	if err != nil {
		t.Fatalf("UFS.ReadAll: %v", err)
	}
	if !bytes.Contains(data, []byte("foo")) {
		t.Errorf("UFS.ReadAll: content mismatch")
	}
	// Read
	var got string
	err = UFS.Read(file, func(b []byte) error {
		got = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("UFS.Read: %v", err)
	}
	if !strings.Contains(got, "bar") {
		t.Errorf("UFS.Read: content mismatch")
	}
	// ReadLines
	var gotLines []string
	err = UFS.ReadLines(file, func(line string) error {
		gotLines = append(gotLines, line)
		return nil
	})
	if err != nil {
		t.Fatalf("UFS.ReadLines: %v", err)
	}
	if len(gotLines) != len(lines) {
		t.Errorf("UFS.ReadLines: expected %d lines, got %d", len(lines), len(gotLines))
	}
}

func TestUFS_Remove(t *testing.T) {
	tmpDir := t.TempDir()
	file := UFS.File(filepath.Join(tmpDir, "toremove.txt"))
	UFS.Create(file, func(w io.Writer) error { return nil })
	if err := UFS.Remove(file); err != nil {
		t.Fatalf("UFS.Remove: %v", err)
	}
	if _, err := os.Stat(file.String()); !os.IsNotExist(err) {
		t.Errorf("UFS.Remove: file still exists")
	}
}

func TestUFS_Touch_SetMTime(t *testing.T) {
	tmpDir := t.TempDir()
	file := UFS.File(filepath.Join(tmpDir, "touch.txt"))
	UFS.Create(file, func(w io.Writer) error { return nil })
	now := time.Now()
	if err := UFS.SetMTime(file, now); err != nil {
		t.Fatalf("UFS.SetMTime: %v", err)
	}
	if err := UFS.Touch(file); err != nil {
		t.Fatalf("UFS.Touch: %v", err)
	}
	info, err := os.Stat(file.String())
	if err != nil {
		t.Fatalf("os.Stat: %v", err)
	}
	if info.ModTime().Unix() == 0 {
		t.Errorf("UFS.SetMTime: modtime not set")
	}
}

func TestUFS_Rename_Copy(t *testing.T) {
	tmpDir := t.TempDir()
	file1 := UFS.File(filepath.Join(tmpDir, "file1.txt"))
	file2 := UFS.File(filepath.Join(tmpDir, "file2.txt"))
	content := []byte("abc")
	UFS.Create(file1, func(w io.Writer) error {
		_, err := w.Write(content)
		return err
	})
	// Rename
	if err := UFS.Rename(file1, file2); err != nil {
		t.Fatalf("UFS.Rename: %v", err)
	}
	if _, err := os.Stat(file1.String()); !os.IsNotExist(err) {
		t.Errorf("UFS.Rename: file1 still exists")
	}
	// Copy
	file3 := UFS.File(filepath.Join(tmpDir, "file3.txt"))
	if err := UFS.Copy(context.Background(), file2, file3, false); err != nil {
		t.Fatalf("UFS.Copy: %v", err)
	}
	data, err := UFS.ReadAll(file3)
	if err != nil {
		t.Fatalf("UFS.ReadAll after copy: %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Errorf("UFS.Copy: content mismatch")
	}
}

func TestUFS_CreateTemp(t *testing.T) {
	tmpDir := t.TempDir()
	UFS.Transient = UFS.Dir(tmpDir)
	temp, err := UFS.CreateTemp("prefix", func(w io.Writer) error {
		_, err := w.Write([]byte("tmp"))
		return err
	}, base.TransientPage4KiB)
	if err != nil {
		t.Fatalf("UFS.CreateTemp: %v", err)
	}
	if !temp.Path.Exists() {
		t.Errorf("UFS.CreateTemp: file does not exist")
	}
	temp.Close()
	if temp.Path.Exists() {
		t.Errorf("UFS.CreateTemp: file should be removed after Close")
	}
}

func TestUFS_Crc32(t *testing.T) {
	tmpDir := t.TempDir()
	file := UFS.File(filepath.Join(tmpDir, "crc.txt"))
	content := []byte("crc content")
	UFS.Create(file, func(w io.Writer) error {
		_, err := w.Write(content)
		return err
	})
	crc, err := UFS.Crc32(context.Background(), file)
	if err != nil {
		t.Fatalf("UFS.Crc32: %v", err)
	}
	if crc == 0 {
		t.Errorf("UFS.Crc32: expected non-zero checksum")
	}
}

func TestUFS_GetCallerFile(t *testing.T) {
	file, err := UFS.GetCallerFile(0)
	if err != nil {
		t.Errorf("UFS.GetCallerFile failed: %v", err)
	}
	if !file.Valid() {
		t.Errorf("UFS.GetCallerFile returned invalid file")
	}
}

func TestUFS_MountRootDirectory_MountOutputDir(t *testing.T) {
	tmpDir := t.TempDir()
	root := UFS.Dir(tmpDir)
	if err := UFS.MountRootDirectory(root); err != nil {
		t.Errorf("UFS.MountRootDirectory failed: %v", err)
	}
	if UFS.Root.String() != root.String() {
		t.Errorf("UFS.Root: expected %v, got %v", root, UFS.Root)
	}
	output := UFS.Dir(filepath.Join(tmpDir, "out"))
	if err := UFS.MountOutputDir(output); err != nil {
		t.Errorf("UFS.MountOutputDir failed: %v", err)
	}
	if UFS.Output.String() != output.String() {
		t.Errorf("UFS.Output: expected %v, got %v", output, UFS.Output)
	}
}

func TestUFS_OpenFile_Open_OpenBuffered(t *testing.T) {
	tmpDir := t.TempDir()
	file := UFS.File(filepath.Join(tmpDir, "open.txt"))
	content := []byte("open content")
	UFS.Create(file, func(w io.Writer) error {
		_, err := w.Write(content)
		return err
	})
	// OpenFile
	var got []byte
	err := UFS.OpenFile(file, func(f *os.File) error {
		got, _ = io.ReadAll(f)
		return nil
	})
	if err != nil {
		t.Fatalf("UFS.OpenFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("UFS.OpenFile: content mismatch")
	}
	// Open
	got = nil
	err = UFS.Open(file, func(r io.Reader) error {
		got, _ = io.ReadAll(r)
		return nil
	})
	if err != nil {
		t.Fatalf("UFS.Open: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("UFS.Open: content mismatch")
	}
	// OpenBuffered
	got = nil
	err = UFS.OpenBuffered(context.Background(), file, func(r io.Reader) error {
		got, _ = io.ReadAll(r)
		return nil
	})
	if err != nil {
		t.Fatalf("UFS.OpenBuffered: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("UFS.OpenBuffered: content mismatch")
	}
}

func TestUFS_ReadLines_Scan(t *testing.T) {
	tmpDir := t.TempDir()
	file := UFS.File(filepath.Join(tmpDir, "scan.txt"))
	lines := []string{"foo123", "bar456", "baz789"}
	UFS.Create(file, func(w io.Writer) error {
		for _, l := range lines {
			w.Write([]byte(l + "\n"))
		}
		return nil
	})
	// ReadLines
	var gotLines []string
	err := UFS.ReadLines(file, func(line string) error {
		gotLines = append(gotLines, line)
		return nil
	})
	if err != nil {
		t.Fatalf("UFS.ReadLines: %v", err)
	}
	if len(gotLines) != len(lines) {
		t.Errorf("UFS.ReadLines: expected %d lines, got %d", len(lines), len(gotLines))
	}
	// Scan
	re := regexp.MustCompile(`([a-z]+)([0-9]+)`)
	var matches [][]string
	err = UFS.Scan(file, base.Regexp{Regexp: re}, func(groups []string) error {
		matches = append(matches, groups)
		return nil
	})
	if err != nil {
		t.Fatalf("UFS.Scan: %v", err)
	}
	if len(matches) != len(lines) {
		t.Errorf("UFS.Scan: expected %d matches, got %d", len(lines), len(matches))
	}
}

func TestUFS_OpenLogProgress(t *testing.T) {
	tmpDir := t.TempDir()
	file := UFS.File(filepath.Join(tmpDir, "progress.txt"))
	content := []byte("progress content")
	UFS.Create(file, func(w io.Writer) error {
		_, err := w.Write(content)
		return err
	})
	// This just checks that OpenLogProgress does not panic and calls the callback
	err := UFS.OpenLogProgress(file, func(fp FileProgress, size int64, scope base.ProgressScope) error {
		buf := make([]byte, size)
		_, err := fp.Read(buf)
		return err
	})
	if err != nil {
		t.Fatalf("UFS.OpenLogProgress: %v", err)
	}
}

func TestUFS_GetCallerFolder(t *testing.T) {
	dir, err := UFS.GetCallerFolder(0)
	if err != nil {
		t.Errorf("UFS.GetCallerFolder failed: %v", err)
	}
	if !dir.Valid() {
		t.Errorf("UFS.GetCallerFolder returned invalid directory")
	}
}

func TestUFS_MTime(t *testing.T) {
	tmpDir := t.TempDir()
	file := UFS.File(filepath.Join(tmpDir, "mtime.txt"))
	UFS.Create(file, func(w io.Writer) error { return nil })
	mt := UFS.MTime(file)
	if mt.IsZero() {
		t.Errorf("UFS.MTime: expected non-zero time")
	}
}

func TestUFS_Fingerprint(t *testing.T) {
	tmpDir := t.TempDir()
	file := UFS.File(filepath.Join(tmpDir, "finger.txt"))
	UFS.Create(file, func(w io.Writer) error {
		_, err := w.Write([]byte("fingerprint"))
		return err
	})
	fp, err := UFS.Fingerprint(context.Background(), file, base.Fingerprint{})
	if err != nil {
		t.Errorf("UFS.Fingerprint: %v", err)
	}
	if fp == (base.Fingerprint{}) {
		t.Errorf("UFS.Fingerprint: expected non-zero fingerprint")
	}
}
