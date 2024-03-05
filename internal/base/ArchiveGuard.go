package base

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

/***************************************
 * ArchiveGuard
 ***************************************/

const (
	ARCHIVEGUARD_ENABLED        = false
	ARCHIVEGUARD_CANARY  uint32 = 0xDEADBEEF
)

var (
	ArchiveGuardTag = MakeArchiveTagIf(MakeFourCC('G', 'A', 'R', 'D'), ARCHIVEGUARD_ENABLED)

	ARCHIVEGUARD_TAG_RAW          FourCC = MakeFourCC('R', 'A', 'W', 'B')
	ARCHIVEGUARD_TAG_BYTE         FourCC = MakeFourCC('B', 'Y', 'T', 'E')
	ARCHIVEGUARD_TAG_BOOL         FourCC = MakeFourCC('B', 'O', 'O', 'L')
	ARCHIVEGUARD_TAG_INT32        FourCC = MakeFourCC('S', 'I', '3', '2')
	ARCHIVEGUARD_TAG_INT64        FourCC = MakeFourCC('S', 'I', '6', '4')
	ARCHIVEGUARD_TAG_UINT32       FourCC = MakeFourCC('U', 'I', '3', '2')
	ARCHIVEGUARD_TAG_UINT64       FourCC = MakeFourCC('U', 'I', '6', '4')
	ARCHIVEGUARD_TAG_FLOAT32      FourCC = MakeFourCC('F', 'T', '3', '2')
	ARCHIVEGUARD_TAG_FLOAT64      FourCC = MakeFourCC('F', 'T', '6', '4')
	ARCHIVEGUARD_TAG_STRING       FourCC = MakeFourCC('S', 'T', 'R', 'G')
	ARCHIVEGUARD_TAG_TIME         FourCC = MakeFourCC('T', 'I', 'M', 'E')
	ARCHIVEGUARD_TAG_SERIALIZABLE FourCC = MakeFourCC('S', 'R', 'L', 'Z')
)

type ArchiveGuard struct {
	inner Archive
	level int
}

func NewArchiveGuard(ar Archive) Archive {
	if ar.HasTags(ArchiveGuardTag) {
		return ArchiveGuard{inner: ar}
	} else {
		return ar
	}
}

func (ar ArchiveGuard) serializeLog(format string, args ...interface{}) {
	if IsLogLevelActive(LOG_DEBUG) {
		indent := strings.Repeat("  ", ar.level)
		LogDebug(LogSerialize, "%s%s", indent, fmt.Sprintf(format, args...))
	}
}
func (ar ArchiveGuard) checkTag(tag FourCC) {
	canary := ARCHIVEGUARD_CANARY
	if ar.inner.UInt32(&canary); canary != ARCHIVEGUARD_CANARY {
		LogPanic(LogSerialize, "invalid canary guard : 0x%08X vs 0x%08X", canary, ARCHIVEGUARD_CANARY)
	}

	check := tag
	if check.Serialize(ar.inner); tag != check {
		LogPanic(LogSerialize, "invalid tag guard : %s (0x%08X) vs %s (0x%08X)", check, check, tag, tag)
	}
}
func (ar *ArchiveGuard) typeGuard(value any, tag FourCC, format string, args ...interface{}) func() {
	ar.serializeLog(format, args...)
	ar.checkTag(tag)

	if tag == ARCHIVEGUARD_TAG_SERIALIZABLE {
		ar.level++
	}

	return func() {
		LogPanicIfFailed(LogSerialize, ar.inner.Error())
		ar.checkTag(tag)

		vo := reflect.ValueOf(value)
		if vo.Kind() == reflect.Pointer {
			vo = vo.Elem()
		}
		ar.serializeLog("\t-> %v: %v", vo.Type(), vo)

		if tag == ARCHIVEGUARD_TAG_SERIALIZABLE {
			ar.level--
		}
	}
}

func (ar ArchiveGuard) Factory() SerializableFactory     { return ar.inner.Factory() }
func (ar ArchiveGuard) Error() error                     { return ar.inner.Error() }
func (ar ArchiveGuard) OnError(err error)                { ar.inner.OnError(err) }
func (ar ArchiveGuard) OnErrorf(msg string, args ...any) { ar.inner.OnErrorf(msg, args...) }
func (ar ArchiveGuard) HasTags(tags ...FourCC) bool      { return ar.inner.HasTags(tags...) }
func (ar ArchiveGuard) SetTags(tags ...FourCC)           { ar.inner.SetTags(tags...) }

func (ar ArchiveGuard) Flags() ArchiveFlags {
	return ar.inner.Flags()
}

func (ar ArchiveGuard) Raw(value []byte) {
	Assert(func() bool { return len(value) > 0 })
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_RAW, "ar.Raw(%d)", len(value))()
	ar.inner.Raw(value)
}
func (ar ArchiveGuard) Byte(value *byte) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_BYTE, "ar.Byte()")()
	ar.inner.Byte(value)
}
func (ar ArchiveGuard) Bool(value *bool) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_BOOL, "ar.Bool()")()
	ar.inner.Bool(value)
}
func (ar ArchiveGuard) Int32(value *int32) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_INT32, "ar.Int32()")()
	ar.inner.Int32(value)
}
func (ar ArchiveGuard) Int64(value *int64) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_INT64, "ar.Int64()")()
	ar.inner.Int64(value)
}
func (ar ArchiveGuard) UInt32(value *uint32) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_UINT32, "ar.UInt32()")()
	ar.inner.UInt32(value)
}
func (ar ArchiveGuard) UInt64(value *uint64) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_UINT64, "ar.UInt64()")()
	ar.inner.UInt64(value)
}
func (ar ArchiveGuard) Float32(value *float32) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_FLOAT32, "ar.Float32()")()
	ar.inner.Float32(value)
}
func (ar ArchiveGuard) Float64(value *float64) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_FLOAT64, "ar.Float64()")()
	ar.inner.Float64(value)
}
func (ar ArchiveGuard) String(value *string) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_STRING, "ar.String()")()
	ar.inner.String(value)
}
func (ar ArchiveGuard) Time(value *time.Time) {
	AssertNotIn(value, nil)
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_TIME, "ar.Time()")()
	ar.inner.Time(value)
}
func (ar ArchiveGuard) Serializable(value Serializable) {
	Assert(func() bool { return !IsNil(value) })
	defer ar.typeGuard(value, ARCHIVEGUARD_TAG_SERIALIZABLE, "ar.Serializable(%T)", value)()
	value.Serialize(ar) // don't use inner here, or recursive descent won't use guards
}
