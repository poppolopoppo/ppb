package compile

import (
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

type CppWarnings struct {
	Default        WarningLevel `json:",omitempty" jsonschema:"description=Default default warning level for compiler"`
	Deprecation    WarningLevel `json:",omitempty" jsonschema:"description=Warning level for deprecated features"`
	Pedantic       WarningLevel `json:",omitempty" jsonschema:"description=Warning level for pedantic checks"`
	ShadowVariable WarningLevel `json:",omitempty" jsonschema:"description=Warning level for shadowed variables"`
	UndefinedMacro WarningLevel `json:",omitempty" jsonschema:"description=Warning level for undefined macros"`
	UnsafeTypeCast WarningLevel `json:",omitempty" jsonschema:"description=Warning level for unsafe type casts"`
}

type CppRules struct {
	SizePerUnity base.SizeInBytes `json:",omitempty" jsonschema:"description=Size of each unity file, used for adaptive unity builds"`
	Instructions InstructionSets  `json:",omitempty" jsonschema:"description=Instruction sets to use for the build, e.g. AVX2, AVX512"`

	Warnings CppWarnings `json:",omitempty" jsonschema:"description=Warning levels for the compiler, can be set to OFF, DEFAULT, or HIGH"`

	CppStd        CppStdType            `json:",omitempty" jsonschema:"description=The C++ standard to use for the build, e.g. C++11, C++14, C++17, C++20"`
	CppRtti       CppRttiType           `json:",omitempty" jsonschema:"description=Whether to enable RTTI (Run-Time Type Information) in the build"`
	DebugInfo     DebugInfoType         `json:",omitempty" jsonschema:"description=Debug information level for the build, can be set to OFF, DEFAULT, or FULL"`
	Exceptions    ExceptionType         `json:",omitempty" jsonschema:"description=Whether to enable C++ exceptions in the build"`
	FloatingPoint FloatingPointType     `json:",omitempty" jsonschema:"description=Floating point instruction mode"`
	Link          LinkType              `json:",omitempty" jsonschema:"description=Linking options for the build"`
	Optimize      OptimizationLevel     `json:",omitempty" jsonschema:"description=Optimization level for the build"`
	PCH           PrecompiledHeaderType `json:",omitempty" jsonschema:"description=Precompiled header options for the build"`
	RuntimeLib    RuntimeLibType        `json:",omitempty" jsonschema:"description=Runtime library options for the build"`
	Sanitizer     SanitizerType         `json:",omitempty" jsonschema:"description=Sanitizer options for the build"`
	Unity         UnityType             `json:",omitempty" jsonschema:"description=Unity build options for the build"`

	AdaptiveUnity  utils.BoolVar `json:",omitempty" jsonschema:"description=Enable adaptive unity builds"`
	Benchmark      utils.BoolVar `json:",omitempty" jsonschema:"description=Enable benchmarking"`
	Deterministic  utils.BoolVar `json:",omitempty" jsonschema:"description=Enable deterministic builds"`
	DebugFastLink  utils.BoolVar `json:",omitempty" jsonschema:"description=Enable fast linking for debugging"`
	Incremental    utils.BoolVar `json:",omitempty" jsonschema:"description=Enable incremental builds"`
	LTO            utils.BoolVar `json:",omitempty" jsonschema:"description=Enable link-time optimization"`
	RuntimeChecks  utils.BoolVar `json:",omitempty" jsonschema:"description=Enable runtime checks"`
	StaticAnalysis utils.BoolVar `json:",omitempty" jsonschema:"description=Enable compiler static analysis"`

	CompilerVerbose utils.BoolVar `json:",omitempty" jsonschema:"description=Enable verbose compiler output"`
	LinkerVerbose   utils.BoolVar `json:",omitempty" jsonschema:"description=Enable verbose linker output"`
}

type Cpp interface {
	GetCpp() *CppRules
	base.Serializable
}

func (rules *CppRules) GetCpp() *CppRules {
	return rules
}
func (rules *CppRules) DeepCopy(src *CppRules) {
	*rules = *src
}
func (rules *CppRules) Serialize(ar base.Archive) {
	ar.Serializable(&rules.SizePerUnity)
	ar.Serializable(&rules.Instructions)

	ar.Serializable(&rules.Warnings.Default)
	ar.Serializable(&rules.Warnings.Deprecation)
	ar.Serializable(&rules.Warnings.Pedantic)
	ar.Serializable(&rules.Warnings.ShadowVariable)
	ar.Serializable(&rules.Warnings.UndefinedMacro)
	ar.Serializable(&rules.Warnings.UnsafeTypeCast)

	ar.Serializable(&rules.CppStd)
	ar.Serializable(&rules.CppRtti)
	ar.Serializable(&rules.DebugInfo)
	ar.Serializable(&rules.Exceptions)
	ar.Serializable(&rules.FloatingPoint)
	ar.Serializable(&rules.Link)
	ar.Serializable(&rules.Optimize)
	ar.Serializable(&rules.PCH)
	ar.Serializable(&rules.RuntimeLib)
	ar.Serializable(&rules.Sanitizer)
	ar.Serializable(&rules.Unity)

	ar.Serializable(&rules.AdaptiveUnity)
	ar.Serializable(&rules.Benchmark)
	ar.Serializable(&rules.Deterministic)
	ar.Serializable(&rules.DebugFastLink)
	ar.Serializable(&rules.Incremental)
	ar.Serializable(&rules.LTO)
	ar.Serializable(&rules.RuntimeChecks)
	ar.Serializable(&rules.StaticAnalysis)

	ar.Serializable(&rules.CompilerVerbose)
	ar.Serializable(&rules.LinkerVerbose)
}
func (rules *CppRules) Inherit(other *CppRules) {
	base.Inherit(&rules.CppStd, other.CppStd)
	base.Inherit(&rules.CppRtti, other.CppRtti)
	base.Inherit(&rules.DebugInfo, other.DebugInfo)
	base.Inherit(&rules.Exceptions, other.Exceptions)
	base.Inherit(&rules.FloatingPoint, other.FloatingPoint)
	base.Inherit(&rules.Instructions, other.Instructions)
	base.Inherit(&rules.PCH, other.PCH)
	base.Inherit(&rules.Link, other.Link)
	base.Inherit(&rules.Optimize, other.Optimize)
	base.Inherit(&rules.RuntimeLib, other.RuntimeLib)
	base.Inherit(&rules.Sanitizer, other.Sanitizer)
	base.Inherit(&rules.Unity, other.Unity)

	base.Inherit(&rules.Warnings.Default, other.Warnings.Default)
	base.Inherit(&rules.Warnings.Deprecation, other.Warnings.Deprecation)
	base.Inherit(&rules.Warnings.Pedantic, other.Warnings.Pedantic)
	base.Inherit(&rules.Warnings.ShadowVariable, other.Warnings.ShadowVariable)
	base.Inherit(&rules.Warnings.UndefinedMacro, other.Warnings.UndefinedMacro)
	base.Inherit(&rules.Warnings.UnsafeTypeCast, other.Warnings.UnsafeTypeCast)

	base.Inherit(&rules.AdaptiveUnity, other.AdaptiveUnity)
	base.Inherit(&rules.Benchmark, other.Benchmark)
	base.Inherit(&rules.Deterministic, other.Deterministic)
	base.Inherit(&rules.DebugFastLink, other.DebugFastLink)
	base.Inherit(&rules.Incremental, other.Incremental)
	base.Inherit(&rules.LTO, other.LTO)
	base.Inherit(&rules.RuntimeChecks, other.RuntimeChecks)
	base.Inherit(&rules.StaticAnalysis, other.StaticAnalysis)
	base.Inherit(&rules.SizePerUnity, other.SizePerUnity)

	base.Inherit(&rules.CompilerVerbose, other.CompilerVerbose)
	base.Inherit(&rules.LinkerVerbose, other.LinkerVerbose)
}
func (rules *CppRules) Overwrite(other *CppRules) {
	base.Overwrite(&rules.CppStd, other.CppStd)
	base.Overwrite(&rules.CppRtti, other.CppRtti)
	base.Overwrite(&rules.DebugInfo, other.DebugInfo)
	base.Overwrite(&rules.Exceptions, other.Exceptions)
	base.Overwrite(&rules.FloatingPoint, other.FloatingPoint)
	base.Overwrite(&rules.Instructions, other.Instructions)
	base.Overwrite(&rules.PCH, other.PCH)
	base.Overwrite(&rules.Link, other.Link)
	base.Overwrite(&rules.Optimize, other.Optimize)
	base.Overwrite(&rules.RuntimeLib, other.RuntimeLib)
	base.Overwrite(&rules.Sanitizer, other.Sanitizer)
	base.Overwrite(&rules.Unity, other.Unity)

	base.Overwrite(&rules.Warnings.Default, other.Warnings.Default)
	base.Overwrite(&rules.Warnings.Deprecation, other.Warnings.Deprecation)
	base.Overwrite(&rules.Warnings.Pedantic, other.Warnings.Pedantic)
	base.Overwrite(&rules.Warnings.ShadowVariable, other.Warnings.ShadowVariable)
	base.Overwrite(&rules.Warnings.UndefinedMacro, other.Warnings.UndefinedMacro)
	base.Overwrite(&rules.Warnings.UnsafeTypeCast, other.Warnings.UnsafeTypeCast)

	base.Overwrite(&rules.AdaptiveUnity, other.AdaptiveUnity)
	base.Overwrite(&rules.Benchmark, other.Benchmark)
	base.Overwrite(&rules.Deterministic, other.Deterministic)
	base.Overwrite(&rules.DebugFastLink, other.DebugFastLink)
	base.Overwrite(&rules.Incremental, other.Incremental)
	base.Overwrite(&rules.LTO, other.LTO)
	base.Overwrite(&rules.RuntimeChecks, other.RuntimeChecks)
	base.Overwrite(&rules.StaticAnalysis, other.StaticAnalysis)
	base.Overwrite(&rules.SizePerUnity, other.SizePerUnity)

	base.Overwrite(&rules.CompilerVerbose, other.CompilerVerbose)
	base.Overwrite(&rules.LinkerVerbose, other.LinkerVerbose)
}
