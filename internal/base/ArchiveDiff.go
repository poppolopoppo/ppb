package base

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

/***************************************
 * ArchiveDiff
 ***************************************/

type ArchiveDiff struct {
	buffer  *bytes.Buffer
	compare *ArchiveBinaryReader
	stack   []Serializable
	level   int
	len     int
	verbose bool
	basicArchive

	OnSerializableBegin PublicEvent[Serializable]
	OnSerializableEnd   PublicEvent[Serializable]
}

func SerializableDiff(a, b Serializable) error {
	ar := NewArchiveDiff()
	defer ar.Close()
	return ar.Diff(a, b)
}

func NewArchiveDiff() ArchiveDiff {
	return ArchiveDiff{
		basicArchive: newBasicArchive(),
		buffer:       TransientBuffer.Allocate(),
		verbose:      IsLogLevelActive(LOG_TRACE),
	}
}
func (x *ArchiveDiff) Close() error {
	TransientBuffer.Release(x.buffer)
	x.buffer = nil
	return nil
}

func (x *ArchiveDiff) Len() int {
	return x.len
}
func (x *ArchiveDiff) Tell() int {
	return x.len - x.buffer.Len()
}

func (x *ArchiveDiff) Diff(a, b Serializable) error {
	x.buffer.Reset()
	x.err = nil
	x.level = 0
	x.stack = []Serializable{}

	return Recover(func() error {
		// write a in memory
		if err := WithArchiveBinaryWriter(x.buffer, func(ar Archive) error {
			ar.Serializable(a)
			return nil
		}, AR_DETERMINISM); err != nil {
			return err
		}

		// record serialized size
		x.len = x.buffer.Len()

		// read b from memory, but do not actually update b
		return WithArchiveBinaryReader(x.buffer, func(ar Archive) error {
			x.compare = ar.(*ArchiveBinaryReader)
			x.compare.flags.Remove(AR_LOADING) // don't want to overwrite b, so we tweak AR_LOADING flag
			x.Serializable(b)
			x.compare = nil
			return nil
		}, AR_DETERMINISM)
	})
}

func (x *ArchiveDiff) Flags() ArchiveFlags {
	return x.compare.flags
}
func (x *ArchiveDiff) Error() error {
	if err := x.basicArchive.Error(); err != nil {
		return x.basicArchive.err
	}
	if err := x.compare.Error(); err != nil {
		return err
	}
	return nil
}

func (ar ArchiveDiff) serializeLog(format string, args ...interface{}) {
	if ar.verbose && IsLogLevelActive(LOG_DEBUG) {
		indent := strings.Repeat("  ", ar.level)
		LogDebug(LogSerialize, "%s%s", indent, fmt.Sprintf(format, args...))
	}
}
func (x *ArchiveDiff) onDiff(old, new any) {
	details := strings.Builder{}
	indent := "  "
	for i, it := range x.stack {
		fmt.Fprintf(&details, "%s[%d] %T: %q\n", indent, i, it, it)
		indent += "  "
	}
	x.OnErrorf("serializable: difference found -> %q != %q\n%s", old, new, details.String())
}

func checkArchiveDiffForScalar[T comparable](ar *ArchiveDiff, serialize func(*T), value *T) {
	ar.compare.flags.Add(AR_LOADING)          // need to restore AR_LOADING while reading scalar values,
	defer ar.compare.flags.Remove(AR_LOADING) // for this scope only

	cmp := *value
	serialize(&cmp)
	ar.serializeLog("%T: '%v' == '%v'", *value, *value, cmp)

	if cmp != *value {
		ar.onDiff(*value, cmp)
	}
}

func (x *ArchiveDiff) Raw(value []byte) {
	cmp := (*x.compare.bytes)[:len(value)]
	x.compare.Raw(cmp)
	x.serializeLog("%T: '%v' == '%v'", value, value, cmp)
	if !bytes.Equal(cmp, value) {
		x.onDiff(value, cmp)
	}
}
func (x *ArchiveDiff) Byte(value *byte) {
	checkArchiveDiffForScalar(x, x.compare.Byte, value)
}
func (x *ArchiveDiff) Bool(value *bool) {
	checkArchiveDiffForScalar(x, x.compare.Bool, value)
}
func (x *ArchiveDiff) Int32(value *int32) {
	checkArchiveDiffForScalar(x, x.compare.Int32, value)
}
func (x *ArchiveDiff) Int64(value *int64) {
	checkArchiveDiffForScalar(x, x.compare.Int64, value)
}
func (x *ArchiveDiff) UInt32(value *uint32) {
	checkArchiveDiffForScalar(x, x.compare.UInt32, value)
}
func (x *ArchiveDiff) UInt64(value *uint64) {
	checkArchiveDiffForScalar(x, x.compare.UInt64, value)
}
func (x *ArchiveDiff) Float32(value *float32) {
	checkArchiveDiffForScalar(x, x.compare.Float32, value)
}
func (x *ArchiveDiff) Float64(value *float64) {
	checkArchiveDiffForScalar(x, x.compare.Float64, value)
}
func (x *ArchiveDiff) String(value *string) {
	checkArchiveDiffForScalar(x, x.compare.String, value)
}
func (x *ArchiveDiff) Time(value *time.Time) {
	checkArchiveDiffForScalar(x, x.compare.Time, value)
}
func (x *ArchiveDiff) Serializable(value Serializable) {
	x.serializeLog(" --> %T", value)

	x.level++
	x.stack = append(x.stack, value)

	x.OnSerializableBegin.Invoke(value)

	// don't use compare here, or recursive descent won't use diffs functions above
	value.Serialize(x)

	x.OnSerializableEnd.Invoke(value)

	AssertIn(x.stack[len(x.stack)-1], value)
	x.stack = x.stack[:len(x.stack)-1]
	x.level--
}
