package compile

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	//lint:ignore ST1001 ignore dot imports warning
	"github.com/poppolopoppo/ppb/internal/base"
)

/***************************************
 * ArchType
 ***************************************/

type ArchType int32

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

func ArchTypes() []ArchType {
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
	ar.Int32((*int32)(x))
}
func (x ArchType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *ArchType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *ArchType) AutoComplete(in base.AutoComplete) {
	for _, it := range ArchTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * CompilerFeatures
 ***************************************/

type CompilerFeature int32

type CompilerFeatureFlags = base.EnumSet[CompilerFeature, *CompilerFeature]

const (
	COMPILER_ALLOW_CACHING CompilerFeature = iota
	COMPILER_ALLOW_DISTRIBUTION
	COMPILER_ALLOW_RESPONSEFILE
	COMPILER_ALLOW_SOURCEMAPPING
	COMPILER_ALLOW_EDITANDCONTINUE
)

func CompilerFeatures() []CompilerFeature {
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
	ar.Int32((*int32)(x))
}
func (x CompilerFeature) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *CompilerFeature) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *CompilerFeature) AutoComplete(in base.AutoComplete) {
	for _, it := range CompilerFeatures() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * ConfigType
 ***************************************/

type ConfigType int32

const (
	CONFIG_DEBUG ConfigType = iota
	CONFIG_FASTDEBUG
	CONFIG_DEVEL
	CONFIG_TEST
	CONFIG_SHIPPING
)

func ConfigTypes() []ConfigType {
	return []ConfigType{
		CONFIG_DEBUG,
		CONFIG_FASTDEBUG,
		CONFIG_DEVEL,
		CONFIG_TEST,
		CONFIG_SHIPPING,
	}
}
func (x ConfigType) String() string {
	switch x {
	case CONFIG_DEBUG:
		return "DEBUG"
	case CONFIG_FASTDEBUG:
		return "FASTDEBUG"
	case CONFIG_DEVEL:
		return "DEVEL"
	case CONFIG_TEST:
		return "TEST"
	case CONFIG_SHIPPING:
		return "SHIPPING"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *ConfigType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case CONFIG_DEBUG.String():
		*x = CONFIG_DEBUG
	case CONFIG_FASTDEBUG.String():
		*x = CONFIG_FASTDEBUG
	case CONFIG_DEVEL.String():
		*x = CONFIG_DEVEL
	case CONFIG_TEST.String():
		*x = CONFIG_TEST
	case CONFIG_SHIPPING.String():
		*x = CONFIG_SHIPPING
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *ConfigType) Serialize(ar base.Archive) {
	ar.Int32((*int32)(x))
}
func (x ConfigType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *ConfigType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *ConfigType) AutoComplete(in base.AutoComplete) {
	for _, it := range ConfigTypes() {
		in.Add(it.String(), "")
	}
}

/***************************************
 * CppRttiType
 ***************************************/

type CppRttiType int32

const (
	CPPRTTI_INHERIT CppRttiType = iota
	CPPRTTI_ENABLED
	CPPRTTI_DISABLED
)

func CppRttiTypes() []CppRttiType {
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
	ar.Int32((*int32)(x))
}
func (x CppRttiType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *CppRttiType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *CppRttiType) AutoComplete(in base.AutoComplete) {
	for _, it := range CppRttiTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * CppStd
 ***************************************/

type CppStdType int32

const (
	CPPSTD_INHERIT CppStdType = iota
	CPPSTD_LATEST
	CPPSTD_11
	CPPSTD_14
	CPPSTD_17
	CPPSTD_20
)

func CppStdTypes() []CppStdType {
	return []CppStdType{
		CPPSTD_INHERIT,
		CPPSTD_LATEST,
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
	ar.Int32((*int32)(x))
}
func (x CppStdType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *CppStdType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *CppStdType) AutoComplete(in base.AutoComplete) {
	for _, it := range CppStdTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * DebugType
 ***************************************/

type DebugType int32

const (
	DEBUG_INHERIT DebugType = iota
	DEBUG_DISABLED
	DEBUG_EMBEDDED
	DEBUG_SYMBOLS
	DEBUG_HOTRELOAD
)

func DebugTypes() []DebugType {
	return []DebugType{
		DEBUG_INHERIT,
		DEBUG_DISABLED,
		DEBUG_EMBEDDED,
		DEBUG_SYMBOLS,
		DEBUG_HOTRELOAD,
	}
}
func (x DebugType) Description() string {
	switch x {
	case DEBUG_INHERIT:
		return "inherit default value from configuration"
	case DEBUG_DISABLED:
		return "disable debugging symbols generation"
	case DEBUG_EMBEDDED:
		return "debugging symbols are embedded inside each compilation unit"
	case DEBUG_SYMBOLS:
		return "debugging symbols are stored in a Program Debugging Batabase (PDB)"
	case DEBUG_HOTRELOAD:
		return "debugging symbols are stored in a Program Debugging Batabase (PDB) with hot-reload support"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x DebugType) String() string {
	switch x {
	case DEBUG_INHERIT:
		return "INHERIT"
	case DEBUG_DISABLED:
		return "DISABLED"
	case DEBUG_EMBEDDED:
		return "EMBEDDED"
	case DEBUG_SYMBOLS:
		return "SYMBOLS"
	case DEBUG_HOTRELOAD:
		return "HOTRELOAD"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x DebugType) IsInheritable() bool {
	return x == DEBUG_INHERIT
}
func (x *DebugType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case DEBUG_INHERIT.String():
		*x = DEBUG_INHERIT
	case DEBUG_DISABLED.String():
		*x = DEBUG_DISABLED
	case DEBUG_EMBEDDED.String():
		*x = DEBUG_EMBEDDED
	case DEBUG_SYMBOLS.String():
		*x = DEBUG_SYMBOLS
	case DEBUG_HOTRELOAD.String():
		*x = DEBUG_HOTRELOAD
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *DebugType) Serialize(ar base.Archive) {
	ar.Int32((*int32)(x))
}
func (x DebugType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *DebugType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *DebugType) AutoComplete(in base.AutoComplete) {
	for _, it := range DebugTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * Exceptions
 ***************************************/

type ExceptionType int32

const (
	EXCEPTION_INHERIT ExceptionType = iota
	EXCEPTION_DISABLED
	EXCEPTION_ENABLED
)

func ExceptionTypes() []ExceptionType {
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
	ar.Int32((*int32)(x))
}
func (x ExceptionType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *ExceptionType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *ExceptionType) AutoComplete(in base.AutoComplete) {
	for _, it := range ExceptionTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * LinkType
 ***************************************/

type LinkType int32

const (
	LINK_INHERIT LinkType = iota
	LINK_STATIC
	LINK_DYNAMIC
)

func LinkTypes() []LinkType {
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
	ar.Int32((*int32)(x))
}
func (x LinkType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *LinkType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *LinkType) AutoComplete(in base.AutoComplete) {
	for _, it := range LinkTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * ModuleType
 ***************************************/

type ModuleType int32

const (
	MODULE_INHERIT ModuleType = iota
	MODULE_PROGRAM
	MODULE_LIBRARY
	MODULE_EXTERNAL
	MODULE_HEADERS
)

func ModuleTypes() []ModuleType {
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
	ar.Int32((*int32)(x))
}
func (x ModuleType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *ModuleType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *ModuleType) AutoComplete(in base.AutoComplete) {
	for _, it := range ModuleTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * PrecompiledHeaderType
 ***************************************/

type PrecompiledHeaderType int32

const (
	PCH_INHERIT PrecompiledHeaderType = iota
	PCH_DISABLED
	PCH_MONOLITHIC
	PCH_SHARED
)

func PrecompiledHeaderTypes() []PrecompiledHeaderType {
	return []PrecompiledHeaderType{
		PCH_INHERIT,
		PCH_DISABLED,
		PCH_MONOLITHIC,
		PCH_SHARED,
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
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *PrecompiledHeaderType) Serialize(ar base.Archive) {
	ar.Int32((*int32)(x))
}
func (x PrecompiledHeaderType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *PrecompiledHeaderType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *PrecompiledHeaderType) AutoComplete(in base.AutoComplete) {
	for _, it := range PrecompiledHeaderTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * PayloadType
 ***************************************/

type PayloadType int32

const (
	PAYLOAD_EXECUTABLE PayloadType = iota
	PAYLOAD_OBJECTLIST
	PAYLOAD_STATICLIB
	PAYLOAD_SHAREDLIB
	PAYLOAD_PRECOMPILEDHEADER
	PAYLOAD_HEADERS
	PAYLOAD_SOURCES
	PAYLOAD_DEBUGSYMBOLS
	PAYLOAD_DEPENDENCIES
)

const NumPayloadTypes = int32(PAYLOAD_HEADERS) + 1

func PayloadTypes() []PayloadType {
	return []PayloadType{
		PAYLOAD_EXECUTABLE,
		PAYLOAD_OBJECTLIST,
		PAYLOAD_STATICLIB,
		PAYLOAD_SHAREDLIB,
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
	*(*int32)(x) = i
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
	ar.Int32((*int32)(x))
}
func (x PayloadType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *PayloadType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *PayloadType) AutoComplete(in base.AutoComplete) {
	for _, it := range PayloadTypes() {
		in.Add(it.String(), it.Description())
	}
}

func (x PayloadType) HasLinker() bool {
	switch x {
	case PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB:
		return true
	case PAYLOAD_OBJECTLIST, PAYLOAD_STATICLIB:
	case PAYLOAD_HEADERS, PAYLOAD_SOURCES, PAYLOAD_PRECOMPILEDHEADER, PAYLOAD_DEBUGSYMBOLS, PAYLOAD_DEPENDENCIES:
	default:
		base.UnexpectedValue(x)
	}
	return false
}
func (x PayloadType) HasOutput() bool {
	switch x {
	case PAYLOAD_EXECUTABLE, PAYLOAD_OBJECTLIST, PAYLOAD_STATICLIB, PAYLOAD_SHAREDLIB:
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
	case PAYLOAD_OBJECTLIST, PAYLOAD_HEADERS, PAYLOAD_SOURCES, PAYLOAD_PRECOMPILEDHEADER, PAYLOAD_DEBUGSYMBOLS, PAYLOAD_DEPENDENCIES:
	default:
		base.UnexpectedValue(x)
	}
	return false
}

/***************************************
 * CompilerSupportType
 ***************************************/

type CompilerSupportType int32

const (
	COMPILERSUPPORT_INHERIT CompilerSupportType = iota
	COMPILERSUPPORT_ALLOWED
	COMPILERSUPPORT_UNSUPPORTED
)

func CompilerSupportTypes() []CompilerSupportType {
	return []CompilerSupportType{
		COMPILERSUPPORT_INHERIT,
		COMPILERSUPPORT_ALLOWED,
		COMPILERSUPPORT_UNSUPPORTED,
	}
}
func (x CompilerSupportType) Description() string {
	switch x {
	case COMPILERSUPPORT_INHERIT:
		return "inherit default value from configuration"
	case COMPILERSUPPORT_ALLOWED:
		return "compiler supports this feature"
	case COMPILERSUPPORT_UNSUPPORTED:
		return "compiler do not support this feature"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x CompilerSupportType) String() string {
	switch x {
	case COMPILERSUPPORT_INHERIT:
		return "INHERIT"
	case COMPILERSUPPORT_ALLOWED:
		return "ALLOWED"
	case COMPILERSUPPORT_UNSUPPORTED:
		return "UNSUPPORTED"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x CompilerSupportType) IsInheritable() bool {
	return x == COMPILERSUPPORT_INHERIT
}
func (x *CompilerSupportType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case COMPILERSUPPORT_INHERIT.String():
		*x = COMPILERSUPPORT_INHERIT
	case COMPILERSUPPORT_ALLOWED.String():
		*x = COMPILERSUPPORT_ALLOWED
	case COMPILERSUPPORT_UNSUPPORTED.String():
		*x = COMPILERSUPPORT_UNSUPPORTED
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *CompilerSupportType) Serialize(ar base.Archive) {
	ar.Int32((*int32)(x))
}
func (x CompilerSupportType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *CompilerSupportType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *CompilerSupportType) AutoComplete(in base.AutoComplete) {
	for _, it := range CompilerSupportTypes() {
		in.Add(it.String(), it.Description())
	}
}

func (x CompilerSupportType) Enabled() bool {
	switch x {
	case COMPILERSUPPORT_ALLOWED:
		return true
	case COMPILERSUPPORT_UNSUPPORTED, COMPILERSUPPORT_INHERIT:
	default:
		base.UnexpectedValuePanic(x, x)
	}
	return false
}

/***************************************
 * SanitizerType
 ***************************************/

type SanitizerType int32

const (
	SANITIZER_INHERIT SanitizerType = iota
	SANITIZER_NONE
	SANITIZER_ADDRESS
	SANITIZER_THREAD
	SANITIZER_UNDEFINED_BEHAVIOR
)

func SanitizerTypes() []SanitizerType {
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
	ar.Int32((*int32)(x))
}
func (x SanitizerType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *SanitizerType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *SanitizerType) AutoComplete(in base.AutoComplete) {
	for _, it := range SanitizerTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * TagType
 ***************************************/

type TagType int32

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

func TagTypes() []TagType {
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
	ar.Int32((*int32)(x))
}
func (x TagType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *TagType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *TagType) AutoComplete(in base.AutoComplete) {
	for _, it := range TagTypes() {
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
func UnityTypes() []UnityType {
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
		if i, err := strconv.Atoi(in); err == nil {
			*x = UnityType(i) // explicit number
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
	for _, it := range UnityTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * VisibilityType
 ***************************************/

type VisibilityType int32

const (
	PRIVATE VisibilityType = iota
	PUBLIC
	RUNTIME
)

func VisibilityTypes() []VisibilityType {
	return []VisibilityType{
		PRIVATE,
		PUBLIC,
		RUNTIME,
	}
}
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
	ar.Int32((*int32)(x))
}
func (x VisibilityType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *VisibilityType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *VisibilityType) AutoComplete(in base.AutoComplete) {
	for _, it := range VisibilityTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * VisibilityMask
 ***************************************/

type VisibilityMask uint32

const (
	VIS_EVERYTHING VisibilityMask = 0xFFFFFFFF
	VIS_NOTHING    VisibilityMask = 0
)

func MakeVisibilityMask(it ...VisibilityType) (result VisibilityMask) {
	return *result.Append(it...)
}
func (m *VisibilityMask) Append(it ...VisibilityType) *VisibilityMask {
	result := int32(*m)
	for _, x := range it {
		result = result | (int32(1) << int32(x))
	}
	*m = VisibilityMask(result)
	return m
}
func (m *VisibilityMask) Clear() {
	*m = VisibilityMask(0)
}
func (m VisibilityMask) All(o VisibilityMask) bool {
	return (m & o) == o
}
func (m VisibilityMask) Any(o VisibilityMask) bool {
	return (m & o) != 0
}
func (m VisibilityMask) Has(flag VisibilityType) bool {
	return m.All(MakeVisibilityMask(flag))
}
func (m VisibilityMask) Public() bool  { return m.Has(PUBLIC) }
func (m VisibilityMask) Private() bool { return m.Has(PRIVATE) }
func (m VisibilityMask) Runtime() bool { return m.Has(RUNTIME) }
func (m *VisibilityMask) Set(in string) error {
	m.Clear()
	for _, x := range strings.Split(in, "|") {
		var vis VisibilityType
		if err := vis.Set(x); err == nil {
			m.Append(vis)
		} else {
			return err
		}
	}
	return nil
}
func (m VisibilityMask) String() (result string) {
	var notFirst bool
	for i := 0; i < 0xFFFFFFFF; i++ {
		flag := int32(1) << int32(i)
		if (int32(m) & flag) == flag {
			if notFirst {
				result += "|"
			}
			result += VisibilityType(i).String()
			notFirst = true
		}
	}
	return result
}
func (x *VisibilityMask) Serialize(ar base.Archive) {
	ar.UInt32((*uint32)(x))
}
func (x VisibilityMask) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *VisibilityMask) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
