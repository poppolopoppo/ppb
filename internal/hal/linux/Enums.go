//go:build linux

package linux

import (
	"fmt"
	"strings"

	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
)

/***************************************
 * Compiler type
 ***************************************/

type CompilerType byte

const (
	COMPILER_CLANG CompilerType = iota
	COMPILER_GCC
)

func GetCompilerTypes() []CompilerType {
	return []CompilerType{
		COMPILER_CLANG,
		COMPILER_GCC,
	}
}
func (x CompilerType) Description() string {
	switch x {
	case COMPILER_CLANG:
		return "LLVM Clang C++ compiler"
	case COMPILER_GCC:
		return "GNU Compiler Collection"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x CompilerType) String() string {
	switch x {
	case COMPILER_CLANG:
		return "CLANG"
	case COMPILER_GCC:
		return "GCC"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *CompilerType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case COMPILER_CLANG.String():
		*x = COMPILER_CLANG
	case COMPILER_GCC.String():
		*x = COMPILER_GCC
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *CompilerType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x CompilerType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *CompilerType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *CompilerType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetCompilerTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * LLVM Version
 ***************************************/

type LlvmVersion int32

const (
	llvm_any    LlvmVersion = -1
	LLVM_LATEST LlvmVersion = 0
	LLVM_16     LlvmVersion = 16
	LLVM_15     LlvmVersion = 15
	LLVM_14     LlvmVersion = 14
	LLVM_13     LlvmVersion = 13
	LLVM_12     LlvmVersion = 12
	LLVM_11     LlvmVersion = 11
	LLVM_10     LlvmVersion = 10
	LLVM_9      LlvmVersion = 9
)

func GetLlvmVersions() []LlvmVersion {
	return []LlvmVersion{
		LLVM_16,
		LLVM_15,
		LLVM_14,
		LLVM_13,
		LLVM_12,
		LLVM_11,
		LLVM_10,
		LLVM_9,
	}
}
func (v LlvmVersion) Equals(o LlvmVersion) bool {
	return (v == o)
}
func (v LlvmVersion) String() string {
	switch v {
	case llvm_any:
		return "ANY"
	case LLVM_LATEST:
		return "LATEST"
	case LLVM_16:
		return "16"
	case LLVM_15:
		return "15"
	case LLVM_14:
		return "14"
	case LLVM_13:
		return "13"
	case LLVM_12:
		return "12"
	case LLVM_11:
		return "11"
	case LLVM_10:
		return "10"
	case LLVM_9:
		return "9"
	default:
		base.UnreachableCode()
		return ""
	}
}
func (v *LlvmVersion) Set(in string) (err error) {
	switch in {
	case LLVM_LATEST.String():
		*v = LLVM_LATEST
	case LLVM_16.String():
		*v = LLVM_16
	case LLVM_15.String():
		*v = LLVM_15
	case LLVM_14.String():
		*v = LLVM_14
	case LLVM_13.String():
		*v = LLVM_13
	case LLVM_12.String():
		*v = LLVM_12
	case LLVM_11.String():
		*v = LLVM_11
	case LLVM_10.String():
		*v = LLVM_10
	case LLVM_9.String():
		*v = LLVM_9
	default:
		err = base.MakeUnexpectedValueError(v, in)
	}
	return err
}
func (x *LlvmVersion) Serialize(ar base.Archive) {
	ar.Int32((*int32)(x))
}
func (x *LlvmVersion) AutoComplete(in base.AutoComplete) {
	for _, it := range GetLlvmVersions() {
		in.Add(it.String(), fmt.Sprintf("LLVM compiler version %v", it))
	}
}

func getCppStdFromLlvm(ver LlvmVersion) compile.CppStdType {
	switch ver {
	case LLVM_16:
		return compile.CPPSTD_20
	case LLVM_15, LLVM_14, LLVM_13, LLVM_12, LLVM_11:
		return compile.CPPSTD_17
	case LLVM_10:
		return compile.CPPSTD_14
	case LLVM_9:
		return compile.CPPSTD_11
	default:
		base.UnexpectedValue(ver)
		return compile.CPPSTD_11
	}
}

/***************************************
 * Dump record layouts type
 ***************************************/

type DumpRecordLayoutsType byte

const (
	DUMPRECORDLAYOUTS_NONE DumpRecordLayoutsType = iota
	DUMPRECORDLAYOUTS_SIMPLE
	DUMPRECORDLAYOUTS_FULL
)

func DumpRecordLayouts() []DumpRecordLayoutsType {
	return []DumpRecordLayoutsType{
		DUMPRECORDLAYOUTS_NONE,
		DUMPRECORDLAYOUTS_SIMPLE,
		DUMPRECORDLAYOUTS_FULL,
	}
}
func (x DumpRecordLayoutsType) Description() string {
	switch x {
	case DUMPRECORDLAYOUTS_NONE:
		return "do not dump record layouts"
	case DUMPRECORDLAYOUTS_SIMPLE:
		return "dump simple record layouts"
	case DUMPRECORDLAYOUTS_FULL:
		return "dump full record layouts"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x DumpRecordLayoutsType) String() string {
	switch x {
	case DUMPRECORDLAYOUTS_NONE:
		return "NONE"
	case DUMPRECORDLAYOUTS_SIMPLE:
		return "SIMPLE"
	case DUMPRECORDLAYOUTS_FULL:
		return "FULL"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *DumpRecordLayoutsType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case DUMPRECORDLAYOUTS_NONE.String():
		*x = DUMPRECORDLAYOUTS_NONE
	case DUMPRECORDLAYOUTS_SIMPLE.String():
		*x = DUMPRECORDLAYOUTS_SIMPLE
	case DUMPRECORDLAYOUTS_FULL.String():
		*x = DUMPRECORDLAYOUTS_FULL
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *DumpRecordLayoutsType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x DumpRecordLayoutsType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *DumpRecordLayoutsType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *DumpRecordLayoutsType) AutoComplete(in base.AutoComplete) {
	for _, it := range DumpRecordLayouts() {
		in.Add(it.String(), it.Description())
	}
}
