package compile

import (
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var LogCompile = NewLogCategory("Compile")

var AllCompilationFlags []struct {
	Name, Description string
	CommandParsableFlags
}

func NewCompilationFlags[T any, P interface {
	*T
	CommandParsableFlags
}](name, description string, flags *T) func() P {
	parsable := P(flags)
	AllCompilationFlags = append(AllCompilationFlags, struct {
		Name        string
		Description string
		CommandParsableFlags
	}{
		Name:                 name,
		Description:          description,
		CommandParsableFlags: parsable,
	})
	return NewCommandParsableFlags[T, P](flags)
}

func OptionCommandAllCompilationFlags() CommandOptionFunc {
	return OptionCommandItem(func(ci CommandItem) {
		for _, it := range AllCompilationFlags {
			ci.Options(OptionCommandParsableFlags(it.Name, it.Description, it.CommandParsableFlags))
		}
	})
}

func InitCompile() {
	LogTrace(LogCompile, "build/compile.Init()")

	// register type for serialization
	RegisterSerializable(&NamespaceModel{})
	RegisterSerializable(&ModuleModel{})

	RegisterSerializable(&ActionRules{})
	RegisterSerializable(&actionCache{})
	RegisterSerializable(&BuildConfig{})
	RegisterSerializable(&BuildGenerated{})
	RegisterSerializable(&CompilationDatabaseBuilder{})
	RegisterSerializable(&CompileEnv{})
	RegisterSerializable(&CompilerAlias{})
	RegisterSerializable(&CompilerRules{})
	RegisterSerializable(&ConfigRules{})
	RegisterSerializable(&ConfigurationAlias{})
	RegisterSerializable(&CustomUnit{})
	RegisterSerializable(&EnvironmentAlias{})
	RegisterSerializable(&Facet{})
	RegisterSerializable(&GeneratorRules{})
	RegisterSerializable(&ModuleAlias{})
	RegisterSerializable(&ModuleRules{})
	RegisterSerializable(&NamespaceRules{})
	RegisterSerializable(&PlatformAlias{})
	RegisterSerializable(&PlatformRules{})
	RegisterSerializable(&TargetActions{})
	RegisterSerializable(&TargetAlias{})
	RegisterSerializable(&TargetPayload{})
	RegisterSerializable(&Unit{})
	RegisterSerializable(&UnityFile{})

	AllConfigurations.Add("Debug", Configuration_Debug)
	AllConfigurations.Add("FastDebug", Configuration_FastDebug)
	AllConfigurations.Add("Devel", Configuration_Devel)
	AllConfigurations.Add("Test", Configuration_Test)
	AllConfigurations.Add("Shipping", Configuration_Shipping)
}

/***************************************
 * Compile Flags
 ***************************************/

type CompileFlags CppRules

var GetCompileFlags = NewCompilationFlags("compile_flags", "cross-platform compilation flags", &CompileFlags{
	AdaptiveUnity:   INHERITABLE_TRUE,
	Avx2:            INHERITABLE_TRUE,
	Benchmark:       INHERITABLE_FALSE,
	CompilerVerbose: INHERITABLE_FALSE,
	CppRtti:         CPPRTTI_INHERIT,
	CppStd:          CPPSTD_INHERIT,
	DebugSymbols:    DEBUG_INHERIT,
	Deterministic:   INHERITABLE_TRUE,
	Exceptions:      EXCEPTION_INHERIT,
	Incremental:     INHERITABLE_INHERIT,
	Link:            LINK_INHERIT,
	LinkerVerbose:   INHERITABLE_FALSE,
	LTO:             INHERITABLE_INHERIT,
	PCH:             PCH_INHERIT,
	RuntimeChecks:   INHERITABLE_INHERIT,
	Sanitizer:       SANITIZER_NONE,
	SizePerUnity:    300 * 1024.0, // 300 KiB
	Unity:           UNITY_INHERIT,
})

func (flags *CompileFlags) GetCpp() *CppRules { return (*CppRules)(flags) }
func (flags *CompileFlags) Flags(cfv CommandFlagsVisitor) {
	cfv.Persistent("AdaptiveUnity", "enable/disable adaptive unity using source control", &flags.AdaptiveUnity)
	cfv.Persistent("Avx2", "enable/disable Advanced Vector Extensions 2 (AVX2)", &flags.Avx2)
	cfv.Persistent("Benchmark", "enable/disable compilation benchmarks", &flags.Benchmark)
	cfv.Persistent("CompilerVerbose", "enable/disable compiler verbose output", &flags.CompilerVerbose)
	cfv.Persistent("CppRtti", "override C++ rtti support ["+JoinString(",", CppRttiTypes()...)+"]", &flags.CppRtti)
	cfv.Persistent("CppStd", "override C++ standard ["+JoinString(",", CppStdTypes()...)+"]", &flags.CppStd)
	cfv.Persistent("DebugFastLink", "override debug symbols fastlink mode ["+JoinString(",", DebugTypes()...)+"]", &flags.DebugFastLink)
	cfv.Persistent("DebugSymbols", "override debug symbols mode ["+JoinString(",", DebugTypes()...)+"]", &flags.DebugSymbols)
	cfv.Persistent("Deterministic", "enable/disable deterministic compilation output", &flags.Deterministic)
	cfv.Persistent("Exceptions", "override exceptions mode ["+JoinString(",", ExceptionTypes()...)+"]", &flags.Exceptions)
	cfv.Persistent("Incremental", "enable/disable incremental linker", &flags.Incremental)
	cfv.Persistent("Link", "override link type ["+JoinString(",", LinkTypes()...)+"]", &flags.Link)
	cfv.Persistent("LinkerVerbose", "enable/disable linker verbose output", &flags.LinkerVerbose)
	cfv.Persistent("LTO", "enable/disable link time optimization", &flags.LTO)
	cfv.Persistent("PCH", "override size limit for splitting unity files ["+JoinString(",", PrecompiledHeaderTypes()...)+"]", &flags.PCH)
	cfv.Persistent("RuntimeChecks", "enable/disable runtime security checks", &flags.RuntimeChecks)
	cfv.Persistent("Sanitizer", "override sanitizer mode ["+JoinString(",", SanitizerTypes()...)+"]", &flags.Sanitizer)
	cfv.Persistent("SizePerUnity", "size limit for splitting unity files", &flags.SizePerUnity)
	cfv.Persistent("Unity", "override unity build mode ["+JoinString(",", UnityTypes()...)+"]", &flags.Unity)
}
