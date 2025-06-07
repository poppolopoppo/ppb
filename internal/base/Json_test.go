package base

import (
	"reflect"
	"strings"
	"testing"
)

type TestTagType byte

type TestTagFlags = EnumSet[TestTagType, *TestTagType]

const (
	TAG_DEBUG TestTagType = iota
	TAG_NDEBUG
	TAG_PROFILING
	TAG_SHIPPING
	TAG_DEVEL
	TAG_TEST
	TAG_FASTDEBUG
)

func GetTestTagTypes() []TestTagType {
	return []TestTagType{
		TAG_DEBUG,
		TAG_NDEBUG,
		TAG_PROFILING,
		TAG_SHIPPING,
		TAG_DEVEL,
		TAG_TEST,
		TAG_FASTDEBUG,
	}
}
func (x TestTagType) Ord() int32 { return int32(x) }
func (x TestTagType) Mask() int32 {
	return EnumBitMask(GetTestTagTypes()...)
}
func (x *TestTagType) FromOrd(value int32) { *x = TestTagType(value) }
func (x TestTagType) Description() string {
	switch x {
	case TAG_DEBUG:
		return "unit with use debug"
	case TAG_NDEBUG:
		return "unit won't use debug"
	case TAG_PROFILING:
		return "unit will support profiling tools"
	case TAG_SHIPPING:
		return "unit targets shipping for final release"
	case TAG_DEVEL:
		return "unit is aimed at developers for debugging"
	case TAG_TEST:
		return "unit is aimed at tester for AQ"
	case TAG_FASTDEBUG:
		return "unit will be compiled as a shared library for faster link times and hot-reload support"
	default:
		UnexpectedValue(x)
		return ""
	}
}
func (x TestTagType) String() string {
	switch x {
	case TAG_DEBUG:
		return "DEBUG"
	case TAG_NDEBUG:
		return "NDEBUG"
	case TAG_PROFILING:
		return "PROFILING"
	case TAG_SHIPPING:
		return "SHIPPING"
	case TAG_DEVEL:
		return "DEVEL"
	case TAG_TEST:
		return "TEST"
	case TAG_FASTDEBUG:
		return "FASTDEBUG"
	default:
		UnexpectedValue(x)
		return ""
	}
}
func (x *TestTagType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case TAG_DEBUG.String():
		*x = TAG_DEBUG
	case TAG_NDEBUG.String():
		*x = TAG_NDEBUG
	case TAG_PROFILING.String():
		*x = TAG_PROFILING
	case TAG_SHIPPING.String():
		*x = TAG_SHIPPING
	case TAG_DEVEL.String():
		*x = TAG_DEVEL
	case TAG_TEST.String():
		*x = TAG_TEST
	case TAG_FASTDEBUG.String():
		*x = TAG_FASTDEBUG
	default:
		err = MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *TestTagType) Serialize(ar Archive) {
	ar.Byte((*byte)(x))
}
func (x TestTagType) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *TestTagType) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}
func (x TestTagType) AutoComplete(in AutoComplete) {
	for _, it := range GetTestTagTypes() {
		in.Add(it.String(), it.Description())
	}
}

var (
	_ = EnumGettable(TestTagFlags(0))
)

func TestJsonSchemaEnumSet(t *testing.T) {
	// This test is a placeholder for testing JSON schema generation for EnumSet types.
	// The actual implementation would depend on the specific EnumSet type and its methods.
	// Here we can just check if the schema generation does not panic or return an error.
	customType := makeJsonSchemaReflector(nil)
	schema := customType.createSchemaForType(reflect.TypeFor[TestTagFlags]())

	if schema.ID.String() != "#/$defs/TestTagTypeFlags" {
		t.Errorf("Expected schema ID to be '#/$defs/TestTagTypeFlags', got '%s'", schema.ID.String())
	}
	if schema.Type != "string" {
		t.Errorf("Expected schema type to be 'string', got '%s'", schema.Type)
	}
	if len(schema.Pattern) == 0 {
		t.Error("Expected schema pattern to be non-empty")
	}
	var defaultValue TestTagType
	if len(schema.Enum) != int(defaultValue.Mask()) {
		t.Errorf("Expected schema enum to be <%d>, got <%d>", int(defaultValue.Mask()-1), len(schema.Enum))
	}
}

func TestJsonSchemaEnumSetMap(t *testing.T) {
	// This test is a placeholder for testing JSON schema generation for EnumSet types.
	// The actual implementation would depend on the specific EnumSet type and its methods.
	// Here we can just check if the schema generation does not panic or return an error.
	customType := makeJsonSchemaReflector(nil)
	schema := customType.createSchemaForType(reflect.TypeFor[map[TestTagFlags]TestTagFlags]())

	if schema.ID.String() != "#/$defs/Map_TestTagTypeFlags_TestTagTypeFlags" {
		t.Errorf("Expected schema ID to be '#/$defs/Map_TestTagTypeFlags_TestTagTypeFlags', got '%s'", schema.ID.String())
	}
	if schema.Type != "object" {
		t.Errorf("Expected schema type to be 'object', got '%s'", schema.Type)
	}
	var defaultValue TestTagType
	if schema.Properties.Len() != int(defaultValue.Mask()) {
		t.Errorf("Expected properties enum to be <%d>, got <%d>", int(defaultValue.Mask()-1), schema.Properties.Len())
	}
}
