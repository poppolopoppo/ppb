package compile

import (
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

type CppRules struct {
	CppStd       CppStdType
	CppRtti      CppRttiType
	DebugSymbols DebugType
	Exceptions   ExceptionType
	PCH          PrecompiledHeaderType
	Link         LinkType
	Sanitizer    SanitizerType
	Unity        UnityType

	SizePerUnity    utils.IntVar
	AdaptiveUnity   utils.BoolVar
	Avx2            utils.BoolVar
	Benchmark       utils.BoolVar
	Deterministic   utils.BoolVar
	DebugFastLink   utils.BoolVar
	LTO             utils.BoolVar
	Incremental     utils.BoolVar
	RuntimeChecks   utils.BoolVar
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
	ar.Serializable(&rules.DebugSymbols)
	ar.Serializable(&rules.Exceptions)
	ar.Serializable(&rules.PCH)
	ar.Serializable(&rules.Link)
	ar.Serializable(&rules.Sanitizer)
	ar.Serializable(&rules.Unity)

	ar.Serializable(&rules.SizePerUnity)
	ar.Serializable(&rules.AdaptiveUnity)
	ar.Serializable(&rules.Avx2)
	ar.Serializable(&rules.Benchmark)
	ar.Serializable(&rules.Deterministic)
	ar.Serializable(&rules.DebugFastLink)
	ar.Serializable(&rules.LTO)
	ar.Serializable(&rules.Incremental)
	ar.Serializable(&rules.RuntimeChecks)
	ar.Serializable(&rules.CompilerVerbose)
	ar.Serializable(&rules.LinkerVerbose)
}
func (rules *CppRules) Inherit(other *CppRules) {
	base.Inherit(&rules.CppStd, other.CppStd)
	base.Inherit(&rules.CppRtti, other.CppRtti)
	base.Inherit(&rules.DebugSymbols, other.DebugSymbols)
	base.Inherit(&rules.Exceptions, other.Exceptions)
	base.Inherit(&rules.PCH, other.PCH)
	base.Inherit(&rules.Link, other.Link)
	base.Inherit(&rules.Sanitizer, other.Sanitizer)
	base.Inherit(&rules.Unity, other.Unity)

	base.Inherit(&rules.SizePerUnity, other.SizePerUnity)
	base.Inherit(&rules.AdaptiveUnity, other.AdaptiveUnity)
	base.Inherit(&rules.Avx2, other.Avx2)
	base.Inherit(&rules.Benchmark, other.Benchmark)
	base.Inherit(&rules.Deterministic, other.Deterministic)
	base.Inherit(&rules.DebugFastLink, other.DebugFastLink)
	base.Inherit(&rules.LTO, other.LTO)
	base.Inherit(&rules.Incremental, other.Incremental)
	base.Inherit(&rules.RuntimeChecks, other.RuntimeChecks)
	base.Inherit(&rules.CompilerVerbose, other.CompilerVerbose)
	base.Inherit(&rules.LinkerVerbose, other.LinkerVerbose)
}
func (rules *CppRules) Overwrite(other *CppRules) {
	base.Overwrite(&rules.CppStd, other.CppStd)
	base.Overwrite(&rules.CppRtti, other.CppRtti)
	base.Overwrite(&rules.DebugSymbols, other.DebugSymbols)
	base.Overwrite(&rules.Exceptions, other.Exceptions)
	base.Overwrite(&rules.PCH, other.PCH)
	base.Overwrite(&rules.Link, other.Link)
	base.Overwrite(&rules.Sanitizer, other.Sanitizer)
	base.Overwrite(&rules.Unity, other.Unity)

	base.Overwrite(&rules.SizePerUnity, other.SizePerUnity)
	base.Overwrite(&rules.AdaptiveUnity, other.AdaptiveUnity)
	base.Overwrite(&rules.Avx2, other.Avx2)
	base.Overwrite(&rules.Benchmark, other.Benchmark)
	base.Overwrite(&rules.Deterministic, other.Deterministic)
	base.Overwrite(&rules.DebugFastLink, other.DebugFastLink)
	base.Overwrite(&rules.LTO, other.LTO)
	base.Overwrite(&rules.Incremental, other.Incremental)
	base.Overwrite(&rules.RuntimeChecks, other.RuntimeChecks)
	base.Overwrite(&rules.CompilerVerbose, other.CompilerVerbose)
	base.Overwrite(&rules.LinkerVerbose, other.LinkerVerbose)
}
