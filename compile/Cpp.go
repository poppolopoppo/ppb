package compile

import (
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

type CppWarnings struct {
	Default        WarningLevel
	Deprecation    WarningLevel
	Pedantic       WarningLevel
	ShadowVariable WarningLevel
	UndefinedMacro WarningLevel
	UnsafeTypeCast WarningLevel
}

type CppRules struct {
	CppStd       CppStdType
	CppRtti      CppRttiType
	DebugInfo    DebugInfoType
	Exceptions   ExceptionType
	Instructions InstructionSets
	Link         LinkType
	Optimize     OptimizationLevel
	PCH          PrecompiledHeaderType
	RuntimeLib   RuntimeLibType
	Sanitizer    SanitizerType
	Unity        UnityType

	Warnings CppWarnings

	AdaptiveUnity utils.BoolVar
	Benchmark     utils.BoolVar
	Deterministic utils.BoolVar
	DebugFastLink utils.BoolVar
	Incremental   utils.BoolVar
	LTO           utils.BoolVar
	RuntimeChecks utils.BoolVar
	SizePerUnity  base.SizeInBytes

	CompilerVerbose utils.BoolVar
	LinkerVerbose   utils.BoolVar
}

type Cpp interface {
	GetCpp() *CppRules
	base.Serializable
}

func (rules *CppRules) GetCpp() *CppRules {
	return rules
}
func (rules *CppRules) Serialize(ar base.Archive) {
	ar.Serializable(&rules.CppStd)
	ar.Serializable(&rules.CppRtti)
	ar.Serializable(&rules.DebugInfo)
	ar.Serializable(&rules.Exceptions)
	ar.Serializable(&rules.Instructions)
	ar.Serializable(&rules.Link)
	ar.Serializable(&rules.Optimize)
	ar.Serializable(&rules.PCH)
	ar.Serializable(&rules.RuntimeLib)
	ar.Serializable(&rules.Sanitizer)
	ar.Serializable(&rules.Unity)

	ar.Serializable(&rules.Warnings.Default)
	ar.Serializable(&rules.Warnings.Deprecation)
	ar.Serializable(&rules.Warnings.Pedantic)
	ar.Serializable(&rules.Warnings.ShadowVariable)
	ar.Serializable(&rules.Warnings.UndefinedMacro)
	ar.Serializable(&rules.Warnings.UnsafeTypeCast)

	ar.Serializable(&rules.AdaptiveUnity)
	ar.Serializable(&rules.Benchmark)
	ar.Serializable(&rules.Deterministic)
	ar.Serializable(&rules.DebugFastLink)
	ar.Serializable(&rules.Incremental)
	ar.Serializable(&rules.LTO)
	ar.Serializable(&rules.RuntimeChecks)
	ar.Serializable(&rules.SizePerUnity)

	ar.Serializable(&rules.CompilerVerbose)
	ar.Serializable(&rules.LinkerVerbose)
}
func (rules *CppRules) Inherit(other *CppRules) {
	base.Inherit(&rules.CppStd, other.CppStd)
	base.Inherit(&rules.CppRtti, other.CppRtti)
	base.Inherit(&rules.DebugInfo, other.DebugInfo)
	base.Inherit(&rules.Exceptions, other.Exceptions)
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
	base.Inherit(&rules.SizePerUnity, other.SizePerUnity)

	base.Inherit(&rules.CompilerVerbose, other.CompilerVerbose)
	base.Inherit(&rules.LinkerVerbose, other.LinkerVerbose)
}
func (rules *CppRules) Overwrite(other *CppRules) {
	base.Overwrite(&rules.CppStd, other.CppStd)
	base.Overwrite(&rules.CppRtti, other.CppRtti)
	base.Overwrite(&rules.DebugInfo, other.DebugInfo)
	base.Overwrite(&rules.Exceptions, other.Exceptions)
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
	base.Overwrite(&rules.SizePerUnity, other.SizePerUnity)

	base.Overwrite(&rules.CompilerVerbose, other.CompilerVerbose)
	base.Overwrite(&rules.LinkerVerbose, other.LinkerVerbose)
}
