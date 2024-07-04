package compile

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"
)

/***************************************
 * ArchType
 ***************************************/

type ArchType byte

const (
	ARCH_X86 ArchType = iota
	ARCH_X64
	ARCH_ARM
	ARCH_ARM64
)

var CurrentArch = base.Memoize(func() ArchType {
	switch runtime.GOARCH {
	case "386":
		return ARCH_X86
	case "amd64":
		return ARCH_X64
	case "arm":
		return ARCH_ARM
	case "arm64":
		return ARCH_ARM64
	default:
		base.UnexpectedValue(runtime.GOARCH)
		return ARCH_ARM
	}
})

func GetArchTypes() []ArchType {
	return []ArchType{
		ARCH_X86,
		ARCH_X64,
		ARCH_ARM,
		ARCH_ARM64,
	}
}
func (x ArchType) Equals(o ArchType) bool {
	return (x == o)
}
func (x ArchType) Description() string {
	switch x {
	case ARCH_X86:
		return "Intel x86 32 bits"
	case ARCH_X64:
		return "AMD x64 64 bits"
	case ARCH_ARM:
		return "unknown ARM 32 bits"
	case ARCH_ARM64:
		return "unknown ARM64 64 bits"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x ArchType) String() string {
	switch x {
	case ARCH_X86:
		return "x86"
	case ARCH_X64:
		return "x64"
	case ARCH_ARM:
		return "ARM"
	case ARCH_ARM64:
		return "ARM64"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *ArchType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case ARCH_X86.String():
		*x = ARCH_X86
	case ARCH_X64.String():
		*x = ARCH_X64
	case ARCH_ARM.String():
		*x = ARCH_ARM
	case ARCH_ARM64.String():
		*x = ARCH_ARM64
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *ArchType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x ArchType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *ArchType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *ArchType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetArchTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * CompilerFeatures
 ***************************************/

type CompilerFeature byte

type CompilerFeatureFlags = base.EnumSet[CompilerFeature, *CompilerFeature]

const (
	COMPILER_ALLOW_CACHING CompilerFeature = iota
	COMPILER_ALLOW_DISTRIBUTION
	COMPILER_ALLOW_RESPONSEFILE
	COMPILER_ALLOW_SOURCEMAPPING
	COMPILER_ALLOW_EDITANDCONTINUE
)

func GetCompilerFeatures() []CompilerFeature {
	return []CompilerFeature{
		COMPILER_ALLOW_CACHING,
		COMPILER_ALLOW_DISTRIBUTION,
		COMPILER_ALLOW_RESPONSEFILE,
		COMPILER_ALLOW_SOURCEMAPPING,
		COMPILER_ALLOW_EDITANDCONTINUE,
	}
}
func (x CompilerFeature) Ord() int32       { return int32(x) }
func (x *CompilerFeature) FromOrd(v int32) { *x = CompilerFeature(v) }
func (x CompilerFeature) Description() string {
	switch x {
	case COMPILER_ALLOW_CACHING:
		return "compiler payload can be stored in cache"
	case COMPILER_ALLOW_DISTRIBUTION:
		return "compiler can be executed on a remote worker"
	case COMPILER_ALLOW_RESPONSEFILE:
		return "compiler allows usage of response files for long command-lines"
	case COMPILER_ALLOW_SOURCEMAPPING:
		return "compiler can remap source for debug files"
	case COMPILER_ALLOW_EDITANDCONTINUE:
		return "compiler can generate a hot-reloadable payload"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x CompilerFeature) String() string {
	switch x {
	case COMPILER_ALLOW_CACHING:
		return "ALLOW_CACHING"
	case COMPILER_ALLOW_DISTRIBUTION:
		return "ALLOW_DISTRIBUTION"
	case COMPILER_ALLOW_RESPONSEFILE:
		return "ALLOW_RESPONSEFILE"
	case COMPILER_ALLOW_SOURCEMAPPING:
		return "ALLOW_SOURCEMAPPING"
	case COMPILER_ALLOW_EDITANDCONTINUE:
		return "ALLOW_EDITANDCONTINUE"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *CompilerFeature) Set(in string) error {
	switch strings.ToUpper(in) {
	case COMPILER_ALLOW_CACHING.String():
		*x = COMPILER_ALLOW_CACHING
	case COMPILER_ALLOW_DISTRIBUTION.String():
		*x = COMPILER_ALLOW_DISTRIBUTION
	case COMPILER_ALLOW_RESPONSEFILE.String():
		*x = COMPILER_ALLOW_RESPONSEFILE
	case COMPILER_ALLOW_SOURCEMAPPING.String():
		*x = COMPILER_ALLOW_SOURCEMAPPING
	case COMPILER_ALLOW_EDITANDCONTINUE.String():
		*x = COMPILER_ALLOW_EDITANDCONTINUE
	default:
		return base.MakeUnexpectedValueError(x, in)
	}
	return nil
}
func (x *CompilerFeature) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x CompilerFeature) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *CompilerFeature) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *CompilerFeature) AutoComplete(in base.AutoComplete) {
	for _, it := range GetCompilerFeatures() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * CppRttiType
 ***************************************/

type CppRttiType byte

const (
	CPPRTTI_INHERIT CppRttiType = iota
	CPPRTTI_ENABLED
	CPPRTTI_DISABLED
)

func GetCppRttiTypes() []CppRttiType {
	return []CppRttiType{
		CPPRTTI_INHERIT,
		CPPRTTI_ENABLED,
		CPPRTTI_DISABLED,
	}
}
func (x CppRttiType) Description() string {
	switch x {
	case CPPRTTI_INHERIT:
		return "inherit default value from configuration"
	case CPPRTTI_ENABLED:
		return "enable C++ RunTime Type Information generation (dynamic_cast<> will be available)"
	case CPPRTTI_DISABLED:
		return "disable C++ RunTime Type Information generation (dynamic_cast<> won't be available)"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x CppRttiType) String() string {
	switch x {
	case CPPRTTI_INHERIT:
		return "INHERIT"
	case CPPRTTI_ENABLED:
		return "ENABLED"
	case CPPRTTI_DISABLED:
		return "DISABLED"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x CppRttiType) IsInheritable() bool {
	return x == CPPRTTI_INHERIT
}
func (x *CppRttiType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case CPPRTTI_INHERIT.String():
		*x = CPPRTTI_INHERIT
	case CPPRTTI_ENABLED.String():
		*x = CPPRTTI_ENABLED
	case CPPRTTI_DISABLED.String():
		*x = CPPRTTI_DISABLED
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *CppRttiType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x CppRttiType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *CppRttiType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *CppRttiType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetCppRttiTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * CppStd
 ***************************************/

type CppStdType byte

const (
	CPPSTD_INHERIT CppStdType = iota
	CPPSTD_LATEST
	CPPSTD_11
	CPPSTD_14
	CPPSTD_17
	CPPSTD_20
	CPPSTD_23
)

func GetCppStdTypes() []CppStdType {
	return []CppStdType{
		CPPSTD_INHERIT,
		CPPSTD_LATEST,
		CPPSTD_23,
		CPPSTD_20,
		CPPSTD_17,
		CPPSTD_14,
		CPPSTD_11,
	}
}
func (x CppStdType) Description() string {
	switch x {
	case CPPSTD_INHERIT:
		return "inherit default value from configuration"
	case CPPSTD_LATEST:
		return "use latest C++ standard supported by the compiler"
	case CPPSTD_23:
		return "use C++23 standard"
	case CPPSTD_20:
		return "use C++20 standard"
	case CPPSTD_17:
		return "use C++17 standard"
	case CPPSTD_14:
		return "use C++14 standard"
	case CPPSTD_11:
		return "use C++11 standard"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x CppStdType) String() string {
	switch x {
	case CPPSTD_INHERIT:
		return "INHERIT"
	case CPPSTD_LATEST:
		return "LATEST"
	case CPPSTD_23:
		return "C++23"
	case CPPSTD_20:
		return "C++20"
	case CPPSTD_17:
		return "C++17"
	case CPPSTD_14:
		return "C++14"
	case CPPSTD_11:
		return "C++11"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *CppStdType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case CPPSTD_INHERIT.String():
		*x = CPPSTD_INHERIT
	case CPPSTD_LATEST.String():
		*x = CPPSTD_LATEST
	case CPPSTD_23.String():
		*x = CPPSTD_23
	case CPPSTD_20.String():
		*x = CPPSTD_20
	case CPPSTD_17.String():
		*x = CPPSTD_17
	case CPPSTD_14.String():
		*x = CPPSTD_14
	case CPPSTD_11.String():
		*x = CPPSTD_11
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x CppStdType) IsInheritable() bool {
	return x == CPPSTD_INHERIT
}
func (x *CppStdType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x CppStdType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *CppStdType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *CppStdType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetCppStdTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * DebugInfoType
 ***************************************/

type DebugInfoType byte

const (
	DEBUGINFO_INHERIT DebugInfoType = iota
	DEBUGINFO_DISABLED
	DEBUGINFO_EMBEDDED
	DEBUGINFO_SYMBOLS
	DEBUGINFO_HOTRELOAD
)

func GetDebugInfoTypes() []DebugInfoType {
	return []DebugInfoType{
		DEBUGINFO_INHERIT,
		DEBUGINFO_DISABLED,
		DEBUGINFO_EMBEDDED,
		DEBUGINFO_SYMBOLS,
		DEBUGINFO_HOTRELOAD,
	}
}
func (x DebugInfoType) Description() string {
	switch x {
	case DEBUGINFO_INHERIT:
		return "inherit default value from configuration"
	case DEBUGINFO_DISABLED:
		return "disable debugging symbols generation"
	case DEBUGINFO_EMBEDDED:
		return "debugging symbols are embedded inside each compilation unit"
	case DEBUGINFO_SYMBOLS:
		return "debugging symbols are stored in a Program Debugging Batabase (PDB)"
	case DEBUGINFO_HOTRELOAD:
		return "debugging symbols are stored in a Program Debugging Batabase (PDB) with hot-reload support"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x DebugInfoType) String() string {
	switch x {
	case DEBUGINFO_INHERIT:
		return "INHERIT"
	case DEBUGINFO_DISABLED:
		return "DISABLED"
	case DEBUGINFO_EMBEDDED:
		return "EMBEDDED"
	case DEBUGINFO_SYMBOLS:
		return "SYMBOLS"
	case DEBUGINFO_HOTRELOAD:
		return "HOTRELOAD"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x DebugInfoType) IsInheritable() bool {
	return x == DEBUGINFO_INHERIT
}
func (x *DebugInfoType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case DEBUGINFO_INHERIT.String():
		*x = DEBUGINFO_INHERIT
	case DEBUGINFO_DISABLED.String():
		*x = DEBUGINFO_DISABLED
	case DEBUGINFO_EMBEDDED.String():
		*x = DEBUGINFO_EMBEDDED
	case DEBUGINFO_SYMBOLS.String():
		*x = DEBUGINFO_SYMBOLS
	case DEBUGINFO_HOTRELOAD.String():
		*x = DEBUGINFO_HOTRELOAD
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *DebugInfoType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x DebugInfoType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *DebugInfoType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *DebugInfoType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetDebugInfoTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * Exceptions
 ***************************************/

type ExceptionType byte

const (
	EXCEPTION_INHERIT ExceptionType = iota
	EXCEPTION_DISABLED
	EXCEPTION_ENABLED
)

func GetExceptionTypes() []ExceptionType {
	return []ExceptionType{
		EXCEPTION_INHERIT,
		EXCEPTION_DISABLED,
		EXCEPTION_ENABLED,
	}
}
func (x ExceptionType) Description() string {
	switch x {
	case EXCEPTION_INHERIT:
		return "inherit default value from configuration"
	case EXCEPTION_DISABLED:
		return "disable C++ exception support"
	case EXCEPTION_ENABLED:
		return "enable C++ exception support"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x ExceptionType) String() string {
	switch x {
	case EXCEPTION_INHERIT:
		return "INHERIT"
	case EXCEPTION_DISABLED:
		return "DISABLED"
	case EXCEPTION_ENABLED:
		return "ENABLED"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x ExceptionType) IsInheritable() bool {
	return x == EXCEPTION_INHERIT
}
func (x *ExceptionType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case EXCEPTION_INHERIT.String():
		*x = EXCEPTION_INHERIT
	case EXCEPTION_DISABLED.String():
		*x = EXCEPTION_DISABLED
	case EXCEPTION_ENABLED.String():
		*x = EXCEPTION_ENABLED
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *ExceptionType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x ExceptionType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *ExceptionType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *ExceptionType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetExceptionTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * InstructionSet
 ***************************************/

type InstructionSet byte

type InstructionSets = base.EnumSet[InstructionSet, *InstructionSet]

const (
	INSTRUCTIONSET_INHERIT InstructionSet = iota
	INSTRUCTIONSET_AES
	INSTRUCTIONSET_AVX
	INSTRUCTIONSET_AVX2
	INSTRUCTIONSET_AVX512
	INSTRUCTIONSET_SSE2
	INSTRUCTIONSET_SSE3
	INSTRUCTIONSET_SSE4_1
	INSTRUCTIONSET_SSE4_2
	INSTRUCTIONSET_SSE4_a
)

func AllInstructionSets() []InstructionSet {
	return []InstructionSet{
		INSTRUCTIONSET_INHERIT,
		INSTRUCTIONSET_AES,
		INSTRUCTIONSET_AVX,
		INSTRUCTIONSET_AVX2,
		INSTRUCTIONSET_AVX512,
		INSTRUCTIONSET_SSE2,
		INSTRUCTIONSET_SSE3,
		INSTRUCTIONSET_SSE4_1,
		INSTRUCTIONSET_SSE4_2,
		INSTRUCTIONSET_SSE4_a,
	}
}
func (x InstructionSet) Ord() int32 {
	return (int32)(x)
}
func (x *InstructionSet) FromOrd(i int32) {
	*(*byte)(x) = byte(i)
}
func (x InstructionSet) IsInheritable() bool {
	return x == INSTRUCTIONSET_INHERIT
}
func (x InstructionSet) Description() string {
	switch x {
	case INSTRUCTIONSET_INHERIT:
		return "inherit from parent's value"
	case INSTRUCTIONSET_AES:
		return "Advanced Encryption Standard"
	case INSTRUCTIONSET_AVX:
		return "Advanced Vector Extensions"
	case INSTRUCTIONSET_AVX2:
		return "Advanced Vector Extensions 2"
	case INSTRUCTIONSET_AVX512:
		return "Advanced Vector Extensions 512"
	case INSTRUCTIONSET_SSE2:
		return "Streaming SIMD Extensions 2"
	case INSTRUCTIONSET_SSE3:
		return "Streaming SIMD Extensions 3"
	case INSTRUCTIONSET_SSE4_1:
		return "Streaming SIMD Extensions 4.1"
	case INSTRUCTIONSET_SSE4_2:
		return "Streaming SIMD Extensions 4.2"
	case INSTRUCTIONSET_SSE4_a:
		return "Streaming SIMD Extensions 4a"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x InstructionSet) String() string {
	switch x {
	case INSTRUCTIONSET_INHERIT:
		return "INHERIT"
	case INSTRUCTIONSET_AES:
		return "AES"
	case INSTRUCTIONSET_AVX:
		return "AVX"
	case INSTRUCTIONSET_AVX2:
		return "AVX2"
	case INSTRUCTIONSET_AVX512:
		return "AVX512"
	case INSTRUCTIONSET_SSE2:
		return "SSE2"
	case INSTRUCTIONSET_SSE3:
		return "SSE3"
	case INSTRUCTIONSET_SSE4_1:
		return "SSE4_1"
	case INSTRUCTIONSET_SSE4_2:
		return "SSE4_2"
	case INSTRUCTIONSET_SSE4_a:
		return "SSE4_a"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *InstructionSet) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case INSTRUCTIONSET_INHERIT.String():
		*x = INSTRUCTIONSET_INHERIT
	case INSTRUCTIONSET_AES.String():
		*x = INSTRUCTIONSET_AES
	case INSTRUCTIONSET_AVX.String():
		*x = INSTRUCTIONSET_AVX
	case INSTRUCTIONSET_AVX2.String():
		*x = INSTRUCTIONSET_AVX2
	case INSTRUCTIONSET_AVX512.String():
		*x = INSTRUCTIONSET_AVX512
	case INSTRUCTIONSET_SSE2.String():
		*x = INSTRUCTIONSET_SSE2
	case INSTRUCTIONSET_SSE3.String():
		*x = INSTRUCTIONSET_SSE3
	case INSTRUCTIONSET_SSE4_1.String():
		*x = INSTRUCTIONSET_SSE4_1
	case INSTRUCTIONSET_SSE4_2.String():
		*x = INSTRUCTIONSET_SSE4_2
	case INSTRUCTIONSET_SSE4_a.String():
		*x = INSTRUCTIONSET_SSE4_a
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *InstructionSet) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x InstructionSet) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *InstructionSet) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *InstructionSet) AutoComplete(in base.AutoComplete) {
	for _, it := range AllInstructionSets() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * LinkType
 ***************************************/

type LinkType byte

const (
	LINK_INHERIT LinkType = iota
	LINK_STATIC
	LINK_DYNAMIC
)

func GetLinkTypes() []LinkType {
	return []LinkType{
		LINK_INHERIT,
		LINK_STATIC,
		LINK_DYNAMIC,
	}
}
func (x LinkType) Description() string {
	switch x {
	case LINK_INHERIT:
		return "inherit default value from configuration"
	case LINK_STATIC:
		return "compiler will produce a static library"
	case LINK_DYNAMIC:
		return "compiler will produce a shared library"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x LinkType) String() string {
	switch x {
	case LINK_INHERIT:
		return "INHERIT"
	case LINK_STATIC:
		return "STATIC"
	case LINK_DYNAMIC:
		return "DYNAMIC"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x LinkType) IsInheritable() bool {
	return x == LINK_INHERIT
}
func (x *LinkType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case LINK_INHERIT.String():
		*x = LINK_INHERIT
	case LINK_STATIC.String():
		*x = LINK_STATIC
	case LINK_DYNAMIC.String():
		*x = LINK_DYNAMIC
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *LinkType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x LinkType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *LinkType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *LinkType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetLinkTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * ModuleType
 ***************************************/

type ModuleType byte

const (
	MODULE_INHERIT ModuleType = iota
	MODULE_PROGRAM
	MODULE_LIBRARY
	MODULE_EXTERNAL
	MODULE_HEADERS
)

func GetModuleTypes() []ModuleType {
	return []ModuleType{
		MODULE_PROGRAM,
		MODULE_LIBRARY,
		MODULE_EXTERNAL,
		MODULE_HEADERS,
	}
}
func (x ModuleType) Description() string {
	switch x {
	case MODULE_INHERIT:
		return "inherit default value from configuration"
	case MODULE_PROGRAM:
		return "module will produce an executable program"
	case MODULE_LIBRARY:
		return "module will produce a library (static or dynamic)"
	case MODULE_EXTERNAL:
		return "module will produce a list of compiled objects"
	case MODULE_HEADERS:
		return "module will produce nothing, since it does not contain source files"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x ModuleType) String() string {
	switch x {
	case MODULE_INHERIT:
		return "INHERIT"
	case MODULE_PROGRAM:
		return "PROGRAM"
	case MODULE_LIBRARY:
		return "LIBRARY"
	case MODULE_EXTERNAL:
		return "EXTERNAL"
	case MODULE_HEADERS:
		return "HEADERS"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *ModuleType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case MODULE_INHERIT.String():
		*x = MODULE_INHERIT
	case MODULE_PROGRAM.String():
		*x = MODULE_PROGRAM
	case MODULE_LIBRARY.String():
		*x = MODULE_LIBRARY
	case MODULE_EXTERNAL.String():
		*x = MODULE_EXTERNAL
	case MODULE_HEADERS.String():
		*x = MODULE_HEADERS
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x ModuleType) IsInheritable() bool {
	return x == MODULE_INHERIT
}
func (x *ModuleType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x ModuleType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *ModuleType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *ModuleType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetModuleTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * PrecompiledHeaderType
 ***************************************/

type PrecompiledHeaderType byte

const (
	PCH_INHERIT PrecompiledHeaderType = iota
	PCH_DISABLED
	PCH_MONOLITHIC
	PCH_SHARED
	PCH_HEADERUNIT
)

func GetPrecompiledHeaderTypes() []PrecompiledHeaderType {
	return []PrecompiledHeaderType{
		PCH_INHERIT,
		PCH_DISABLED,
		PCH_MONOLITHIC,
		PCH_SHARED,
		PCH_HEADERUNIT,
	}
}
func (x PrecompiledHeaderType) Description() string {
	switch x {
	case PCH_INHERIT:
		return "inherit default value from configuration"
	case PCH_DISABLED:
		return "disable precompiled header usage"
	case PCH_MONOLITHIC:
		return "generate a dedicated precompiled header for this module"
	case PCH_SHARED:
		return "reuse a shared precompiled header for this module"
	case PCH_HEADERUNIT:
		return "generate a C++20 header unit, which is faster/smaller than PCH and machine indepedent"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x PrecompiledHeaderType) String() string {
	switch x {
	case PCH_INHERIT:
		return "INHERIT"
	case PCH_DISABLED:
		return "DISABLED"
	case PCH_MONOLITHIC:
		return "MONOLITHIC"
	case PCH_SHARED:
		return "SHARED"
	case PCH_HEADERUNIT:
		return "HEADERUNIT"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x PrecompiledHeaderType) IsInheritable() bool {
	return x == PCH_INHERIT
}
func (x *PrecompiledHeaderType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case PCH_INHERIT.String():
		*x = PCH_INHERIT
	case PCH_DISABLED.String():
		*x = PCH_DISABLED
	case PCH_MONOLITHIC.String():
		*x = PCH_MONOLITHIC
	case PCH_SHARED.String():
		*x = PCH_SHARED
	case PCH_HEADERUNIT.String():
		*x = PCH_HEADERUNIT
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *PrecompiledHeaderType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x PrecompiledHeaderType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *PrecompiledHeaderType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *PrecompiledHeaderType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetPrecompiledHeaderTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * PayloadType
 ***************************************/

type PayloadType byte

const (
	PAYLOAD_EXECUTABLE PayloadType = iota
	PAYLOAD_OBJECTLIST
	PAYLOAD_STATICLIB
	PAYLOAD_SHAREDLIB
	PAYLOAD_HEADERUNIT
	PAYLOAD_PRECOMPILEDHEADER
	PAYLOAD_HEADERS
	PAYLOAD_SOURCES
	PAYLOAD_DEBUGSYMBOLS
	PAYLOAD_DEPENDENCIES

	NumPayloadTypes int32 = (int32(PAYLOAD_DEPENDENCIES) + 1)
)

func GetPayloadTypes() []PayloadType {
	return []PayloadType{
		PAYLOAD_EXECUTABLE,
		PAYLOAD_OBJECTLIST,
		PAYLOAD_STATICLIB,
		PAYLOAD_SHAREDLIB,
		PAYLOAD_HEADERUNIT,
		PAYLOAD_PRECOMPILEDHEADER,
		PAYLOAD_HEADERS,
		PAYLOAD_SOURCES,
		PAYLOAD_DEBUGSYMBOLS,
		PAYLOAD_DEPENDENCIES,
	}
}
func (x PayloadType) Ord() int32 {
	return (int32)(x)
}
func (x *PayloadType) FromOrd(i int32) {
	*(*byte)(x) = byte(i)
}
func (x PayloadType) Description() string {
	switch x {
	case PAYLOAD_EXECUTABLE:
		return "executable program"
	case PAYLOAD_OBJECTLIST:
		return "collection of compiled objects"
	case PAYLOAD_STATICLIB:
		return "static library"
	case PAYLOAD_SHAREDLIB:
		return "shared dynamic library"
	case PAYLOAD_HEADERUNIT:
		return "header unit"
	case PAYLOAD_PRECOMPILEDHEADER:
		return "shared precompiled header"
	case PAYLOAD_HEADERS:
		return "header files"
	case PAYLOAD_SOURCES:
		return "source files"
	case PAYLOAD_DEBUGSYMBOLS:
		return "program debugging database"
	case PAYLOAD_DEPENDENCIES:
		return "source file dependency list"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x PayloadType) String() string {
	switch x {
	case PAYLOAD_EXECUTABLE:
		return "EXECUTABLE"
	case PAYLOAD_OBJECTLIST:
		return "OBJECTLIST"
	case PAYLOAD_STATICLIB:
		return "STATICLIB"
	case PAYLOAD_SHAREDLIB:
		return "SHAREDLIB"
	case PAYLOAD_HEADERUNIT:
		return "HEADERUNIT"
	case PAYLOAD_PRECOMPILEDHEADER:
		return "PRECOMPILEDHEADER"
	case PAYLOAD_HEADERS:
		return "HEADERS"
	case PAYLOAD_SOURCES:
		return "SOURCES"
	case PAYLOAD_DEBUGSYMBOLS:
		return "DEBUGSYMBOLS"
	case PAYLOAD_DEPENDENCIES:
		return "DEPENDENCIES"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *PayloadType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case PAYLOAD_EXECUTABLE.String():
		*x = PAYLOAD_EXECUTABLE
	case PAYLOAD_OBJECTLIST.String():
		*x = PAYLOAD_OBJECTLIST
	case PAYLOAD_STATICLIB.String():
		*x = PAYLOAD_STATICLIB
	case PAYLOAD_SHAREDLIB.String():
		*x = PAYLOAD_SHAREDLIB
	case PAYLOAD_HEADERUNIT.String():
		*x = PAYLOAD_HEADERUNIT
	case PAYLOAD_PRECOMPILEDHEADER.String():
		*x = PAYLOAD_PRECOMPILEDHEADER
	case PAYLOAD_HEADERS.String():
		*x = PAYLOAD_HEADERS
	case PAYLOAD_SOURCES.String():
		*x = PAYLOAD_SOURCES
	case PAYLOAD_DEBUGSYMBOLS.String():
		*x = PAYLOAD_DEBUGSYMBOLS
	case PAYLOAD_DEPENDENCIES.String():
		*x = PAYLOAD_DEPENDENCIES
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x PayloadType) Compare(o PayloadType) int {
	if x < o {
		return -1
	} else if x == o {
		return 0
	} else {
		return 1
	}
}
func (x *PayloadType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x PayloadType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *PayloadType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *PayloadType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetPayloadTypes() {
		in.Add(it.String(), it.Description())
	}
}

func (x PayloadType) HasLinker() bool {
	switch x {
	case PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB:
		return true
	case PAYLOAD_OBJECTLIST, PAYLOAD_STATICLIB:
	case PAYLOAD_HEADERUNIT, PAYLOAD_PRECOMPILEDHEADER:
	case PAYLOAD_HEADERS, PAYLOAD_SOURCES, PAYLOAD_DEBUGSYMBOLS, PAYLOAD_DEPENDENCIES:
	default:
		base.UnexpectedValue(x)
	}
	return false
}
func (x PayloadType) HasOutput() bool {
	switch x {
	case PAYLOAD_EXECUTABLE, PAYLOAD_OBJECTLIST, PAYLOAD_STATICLIB, PAYLOAD_SHAREDLIB, PAYLOAD_HEADERUNIT:
		return true
	case PAYLOAD_HEADERS, PAYLOAD_SOURCES, PAYLOAD_PRECOMPILEDHEADER, PAYLOAD_DEBUGSYMBOLS, PAYLOAD_DEPENDENCIES:
	default:
		base.UnexpectedValue(x)
	}
	return false
}
func (x PayloadType) HasMultipleInput() bool {
	switch x {
	case PAYLOAD_EXECUTABLE, PAYLOAD_STATICLIB, PAYLOAD_SHAREDLIB:
		return true
	case PAYLOAD_OBJECTLIST, PAYLOAD_HEADERS, PAYLOAD_SOURCES, PAYLOAD_HEADERUNIT, PAYLOAD_PRECOMPILEDHEADER, PAYLOAD_DEBUGSYMBOLS, PAYLOAD_DEPENDENCIES:
	default:
		base.UnexpectedValue(x)
	}
	return false
}

/***************************************
 * OptimizationLevel
 ***************************************/

type OptimizationLevel byte

const (
	OPTIMIZE_INHERIT OptimizationLevel = iota
	OPTIMIZE_NONE
	OPTIMIZE_FOR_DEBUG
	OPTIMIZE_FOR_SIZE
	OPTIMIZE_FOR_SPEED
	OPTIMIZE_FOR_SHIPPING
)

func OptimizationTypes() []OptimizationLevel {
	return []OptimizationLevel{
		OPTIMIZE_INHERIT,
		OPTIMIZE_NONE,
		OPTIMIZE_FOR_DEBUG,
		OPTIMIZE_FOR_SIZE,
		OPTIMIZE_FOR_SPEED,
		OPTIMIZE_FOR_SHIPPING,
	}
}
func (x OptimizationLevel) Description() string {
	switch x {
	case OPTIMIZE_INHERIT:
		return "inherit default value from configuration"
	case OPTIMIZE_NONE:
		return "disable all compiler optimizations"
	case OPTIMIZE_FOR_DEBUG:
		return "sets a combination of optimizations that balance between speed and ease of debugging"
	case OPTIMIZE_FOR_SIZE:
		return "sets a combination of optimizations that generate minimum size code"
	case OPTIMIZE_FOR_SPEED:
		return "sets a combination of optimizations that optimizes code for maximum speed"
	case OPTIMIZE_FOR_SHIPPING:
		return "sets a combination of optimizations that generate best possible code, regardless of compilation times"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x OptimizationLevel) String() string {
	switch x {
	case OPTIMIZE_INHERIT:
		return "INHERIT"
	case OPTIMIZE_NONE:
		return "NONE"
	case OPTIMIZE_FOR_DEBUG:
		return "FOR_DEBUG"
	case OPTIMIZE_FOR_SIZE:
		return "FOR_SIZE"
	case OPTIMIZE_FOR_SPEED:
		return "FOR_SPEED"
	case OPTIMIZE_FOR_SHIPPING:
		return "FOR_SHIPPING"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x OptimizationLevel) IsEnabled() bool {
	return x != OPTIMIZE_INHERIT && x != OPTIMIZE_NONE
}
func (x OptimizationLevel) IsInheritable() bool {
	return x == OPTIMIZE_INHERIT
}
func (x *OptimizationLevel) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case OPTIMIZE_INHERIT.String():
		*x = OPTIMIZE_INHERIT
	case OPTIMIZE_NONE.String():
		*x = OPTIMIZE_NONE
	case OPTIMIZE_FOR_DEBUG.String():
		*x = OPTIMIZE_FOR_DEBUG
	case OPTIMIZE_FOR_SIZE.String():
		*x = OPTIMIZE_FOR_SIZE
	case OPTIMIZE_FOR_SPEED.String():
		*x = OPTIMIZE_FOR_SPEED
	case OPTIMIZE_FOR_SHIPPING.String():
		*x = OPTIMIZE_FOR_SHIPPING
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *OptimizationLevel) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x OptimizationLevel) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *OptimizationLevel) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *OptimizationLevel) AutoComplete(in base.AutoComplete) {
	for _, it := range OptimizationTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * CompilerSupportType
 ***************************************/

type SupportType byte

const (
	SUPPORT_INHERIT SupportType = iota
	SUPPORT_ALLOWED
	SUPPORT_UNAVAILABLE
)

func GetSupportTypes() []SupportType {
	return []SupportType{
		SUPPORT_INHERIT,
		SUPPORT_ALLOWED,
		SUPPORT_UNAVAILABLE,
	}
}
func (x SupportType) Description() string {
	switch x {
	case SUPPORT_INHERIT:
		return "inherit default value from configuration"
	case SUPPORT_ALLOWED:
		return "feature is supported"
	case SUPPORT_UNAVAILABLE:
		return "feature has no support"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x SupportType) String() string {
	switch x {
	case SUPPORT_INHERIT:
		return "INHERIT"
	case SUPPORT_ALLOWED:
		return "ALLOWED"
	case SUPPORT_UNAVAILABLE:
		return "UNSUPPORTED"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x SupportType) IsInheritable() bool {
	return x == SUPPORT_INHERIT
}
func (x *SupportType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case SUPPORT_INHERIT.String():
		*x = SUPPORT_INHERIT
	case SUPPORT_ALLOWED.String():
		*x = SUPPORT_ALLOWED
	case SUPPORT_UNAVAILABLE.String():
		*x = SUPPORT_UNAVAILABLE
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *SupportType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x SupportType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *SupportType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *SupportType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetSupportTypes() {
		in.Add(it.String(), it.Description())
	}
}

func (x SupportType) Enabled() bool {
	switch x {
	case SUPPORT_ALLOWED:
		return true
	case SUPPORT_UNAVAILABLE, SUPPORT_INHERIT:
	default:
		base.UnexpectedValuePanic(x, x)
	}
	return false
}

/***************************************
 * RuntimeType
 ***************************************/

type RuntimeLibType byte

const (
	RUNTIMELIB_INHERIT RuntimeLibType = iota
	RUNTIMELIB_DYNAMIC
	RUNTIMELIB_DYNAMIC_DEBUG
	RUNTIMELIB_STATIC
	RUNTIMELIB_STATIC_DEBUG
)

func GetRuntimeLibTypes() []RuntimeLibType {
	return []RuntimeLibType{
		RUNTIMELIB_INHERIT,
		RUNTIMELIB_DYNAMIC,
		RUNTIMELIB_DYNAMIC_DEBUG,
		RUNTIMELIB_STATIC,
		RUNTIMELIB_STATIC_DEBUG,
	}
}
func (x RuntimeLibType) Description() string {
	switch x {
	case RUNTIMELIB_INHERIT:
		return "inherit default value from configuration"
	case RUNTIMELIB_DYNAMIC:
		return "use dynamic runtime provided by host system"
	case RUNTIMELIB_DYNAMIC_DEBUG:
		return "use dynamic debug runtime provided by host system"
	case RUNTIMELIB_STATIC:
		return "embed static runtime inside binary"
	case RUNTIMELIB_STATIC_DEBUG:
		return "embed static debug runtime inside binary"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x RuntimeLibType) String() string {
	switch x {
	case RUNTIMELIB_INHERIT:
		return "INHERIT"
	case RUNTIMELIB_DYNAMIC:
		return "DYNAMIC"
	case RUNTIMELIB_DYNAMIC_DEBUG:
		return "DYNAMIC_DEBUG"
	case RUNTIMELIB_STATIC:
		return "STATIC"
	case RUNTIMELIB_STATIC_DEBUG:
		return "STATIC_DEBUG"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x RuntimeLibType) IsDebug() bool {
	switch x {
	case RUNTIMELIB_DYNAMIC_DEBUG, RUNTIMELIB_STATIC_DEBUG:
		return true
	default:
		return false
	}
}
func (x RuntimeLibType) IsInheritable() bool {
	return x == RUNTIMELIB_INHERIT
}
func (x *RuntimeLibType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case RUNTIMELIB_INHERIT.String():
		*x = RUNTIMELIB_INHERIT
	case RUNTIMELIB_DYNAMIC.String():
		*x = RUNTIMELIB_DYNAMIC
	case RUNTIMELIB_DYNAMIC_DEBUG.String():
		*x = RUNTIMELIB_DYNAMIC_DEBUG
	case RUNTIMELIB_STATIC.String():
		*x = RUNTIMELIB_STATIC
	case RUNTIMELIB_STATIC_DEBUG.String():
		*x = RUNTIMELIB_STATIC_DEBUG
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *RuntimeLibType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x RuntimeLibType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *RuntimeLibType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *RuntimeLibType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetRuntimeLibTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * SanitizerType
 ***************************************/

type SanitizerType byte

const (
	SANITIZER_INHERIT SanitizerType = iota
	SANITIZER_NONE
	SANITIZER_ADDRESS
	SANITIZER_THREAD
	SANITIZER_UNDEFINED_BEHAVIOR
)

func GetSanitizerTypes() []SanitizerType {
	return []SanitizerType{
		SANITIZER_INHERIT,
		SANITIZER_NONE,
		SANITIZER_ADDRESS,
		SANITIZER_THREAD,
		SANITIZER_UNDEFINED_BEHAVIOR,
	}
}
func (x SanitizerType) Description() string {
	switch x {
	case SANITIZER_INHERIT:
		return "inherit default value from configuration"
	case SANITIZER_NONE:
		return "don't use a sanitizer"
	case SANITIZER_ADDRESS:
		return "use address sanitizer for memory issues"
	case SANITIZER_THREAD:
		return "use thread sanitizer for race conditions"
	case SANITIZER_UNDEFINED_BEHAVIOR:
		return "use undefined behavior sanitizer to bad practices"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x SanitizerType) String() string {
	switch x {
	case SANITIZER_INHERIT:
		return "INHERIT"
	case SANITIZER_NONE:
		return "NONE"
	case SANITIZER_ADDRESS:
		return "ADDRESS"
	case SANITIZER_THREAD:
		return "THREAD"
	case SANITIZER_UNDEFINED_BEHAVIOR:
		return "UNDEFINED_BEHAVIOR"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x SanitizerType) IsEnabled() bool {
	return !x.IsInheritable() && x != SANITIZER_NONE
}
func (x SanitizerType) IsInheritable() bool {
	return x == SANITIZER_INHERIT
}
func (x *SanitizerType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case SANITIZER_INHERIT.String():
		*x = SANITIZER_INHERIT
	case SANITIZER_NONE.String():
		*x = SANITIZER_NONE
	case SANITIZER_ADDRESS.String():
		*x = SANITIZER_ADDRESS
	case SANITIZER_THREAD.String():
		*x = SANITIZER_THREAD
	case SANITIZER_UNDEFINED_BEHAVIOR.String():
		*x = SANITIZER_UNDEFINED_BEHAVIOR
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *SanitizerType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x SanitizerType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *SanitizerType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *SanitizerType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetSanitizerTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * TagType
 ***************************************/

type TagType byte

type TagFlags = base.EnumSet[TagType, *TagType]

const (
	TAG_DEBUG TagType = iota
	TAG_NDEBUG
	TAG_PROFILING
	TAG_SHIPPING
	TAG_DEVEL
	TAG_TEST
	TAG_FASTDEBUG
)

func GetTagTypes() []TagType {
	return []TagType{
		TAG_DEBUG,
		TAG_NDEBUG,
		TAG_PROFILING,
		TAG_SHIPPING,
		TAG_DEVEL,
		TAG_TEST,
		TAG_FASTDEBUG,
	}
}
func (x TagType) Ord() int32           { return int32(x) }
func (x *TagType) FromOrd(value int32) { *x = TagType(value) }
func (x TagType) Description() string {
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
		base.UnexpectedValue(x)
		return ""
	}
}
func (x TagType) String() string {
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
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *TagType) Set(in string) (err error) {
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
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *TagType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x TagType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *TagType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *TagType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetTagTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * Unity
 ***************************************/

type UnityType int32

const (
	UNITY_INHERIT   UnityType = 0
	UNITY_AUTOMATIC UnityType = -1
	UNITY_DISABLED  UnityType = -2
)

func (x UnityType) Ord() int32 {
	return int32(x)
}
func GetUnityTypes() []UnityType {
	return []UnityType{
		UNITY_INHERIT,
		UNITY_AUTOMATIC,
		UNITY_DISABLED,
	}
}
func (x UnityType) Description() string {
	switch x {
	case UNITY_INHERIT:
		return "inherit default value from configuration"
	case UNITY_AUTOMATIC:
		return "will bundle source files in large auto-generated unity files for faster compilation times"
	case UNITY_DISABLED:
		return "do not generate unity files, each file will be compiled individually"
	default:
		if x <= 0 {
			base.LogPanic(LogCompile, "invalid unity type: %d", x.Ord())
		}
		return fmt.Sprint(int32(x))
	}
}
func (x UnityType) String() string {
	switch x {
	case UNITY_INHERIT:
		return "INHERIT"
	case UNITY_AUTOMATIC:
		return "AUTOMATIC"
	case UNITY_DISABLED:
		return "DISABLED"
	default:
		if x <= 0 {
			base.LogPanic(LogCompile, "invalid unity type: %d", x.Ord())
		}
		return fmt.Sprint(int32(x))
	}
}
func (x UnityType) IsInheritable() bool {
	return x == UNITY_INHERIT
}
func (x *UnityType) Set(in string) error {
	switch strings.ToUpper(in) {
	case UNITY_INHERIT.String():
		*x = UNITY_INHERIT
	case UNITY_AUTOMATIC.String():
		*x = UNITY_AUTOMATIC
	case UNITY_DISABLED.String():
		*x = UNITY_DISABLED
	default:
		if i64, err := strconv.ParseInt(in, 10, 32); err == nil {
			*x = UnityType(int32(i64)) // explicit number
		} else {
			return err
		}
	}
	return nil
}
func (x *UnityType) Serialize(ar base.Archive) {
	ar.Int32((*int32)(x))
}
func (x UnityType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *UnityType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *UnityType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetUnityTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * VisibilityType
 ***************************************/

type VisibilityType byte

const (
	PRIVATE VisibilityType = iota
	PUBLIC
	RUNTIME
)

func GetVisibilityTypes() []VisibilityType {
	return []VisibilityType{
		PRIVATE,
		PUBLIC,
		RUNTIME,
	}
}
func (x VisibilityType) Ord() int32       { return int32(x) }
func (x *VisibilityType) FromOrd(v int32) { *(*byte)(x) = byte(v) }
func (x VisibilityType) Description() string {
	switch x {
	case PRIVATE:
		return "private dependency can be referenced from private source files, but not from public"
	case PUBLIC:
		return "public dependency can be referenced from both private and public source files, and are transitive"
	case RUNTIME:
		return "runtime dependency can only be loaded at runtime from a shared-library"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x VisibilityType) String() string {
	switch x {
	case PRIVATE:
		return "PRIVATE"
	case PUBLIC:
		return "PUBLIC"
	case RUNTIME:
		return "RUNTIME"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *VisibilityType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case PRIVATE.String():
		*x = PRIVATE
	case PUBLIC.String():
		*x = PUBLIC
	case RUNTIME.String():
		*x = RUNTIME
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *VisibilityType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x VisibilityType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *VisibilityType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *VisibilityType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetVisibilityTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * WarningLevel
 ***************************************/

type WarningLevel byte

const (
	WARNING_INHERIT WarningLevel = iota
	WARNING_DISABLED
	WARNING_WARN
	WARNING_ERROR
)

func GetWarningLevels() []WarningLevel {
	return []WarningLevel{
		WARNING_INHERIT,
		WARNING_DISABLED,
		WARNING_WARN,
		WARNING_ERROR,
	}
}
func (x WarningLevel) IsEnabled() bool {
	return !x.IsInheritable() && x != WARNING_DISABLED
}
func (x WarningLevel) IsInheritable() bool {
	return x == WARNING_INHERIT
}
func (x WarningLevel) Description() string {
	switch x {
	case WARNING_INHERIT:
		return "inherit from parent's value"
	case WARNING_DISABLED:
		return "warning will be disabled"
	case WARNING_WARN:
		return "warning will be visible but the build will continue"
	case WARNING_ERROR:
		return "warning will be visible and will be considered as an error"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x WarningLevel) String() string {
	switch x {
	case WARNING_INHERIT:
		return "INHERIT"
	case WARNING_DISABLED:
		return "DISABLED"
	case WARNING_WARN:
		return "WARN"
	case WARNING_ERROR:
		return "ERROR"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *WarningLevel) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case WARNING_INHERIT.String():
		*x = WARNING_INHERIT
	case WARNING_DISABLED.String():
		*x = WARNING_DISABLED
	case WARNING_WARN.String():
		*x = WARNING_WARN
	case WARNING_ERROR.String():
		*x = WARNING_ERROR
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *WarningLevel) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x WarningLevel) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *WarningLevel) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *WarningLevel) AutoComplete(in base.AutoComplete) {
	for _, it := range GetWarningLevels() {
		in.Add(it.String(), it.Description())
	}
}
