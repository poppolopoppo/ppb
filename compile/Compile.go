package compile

import (
	//lint:ignore ST1001 ignore dot imports warning
	"github.com/poppolopoppo/ppb/internal/base"
	. "github.com/poppolopoppo/ppb/utils"
)

var LogCompile = base.NewLogCategory("Compile")

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
	base.LogTrace(LogCompile, "build/compile.Init()")

	// register type for serialization
	base.RegisterSerializable(&NamespaceModel{})
	base.RegisterSerializable(&ModuleModel{})

	base.RegisterSerializable(&BuildConfig{})
	base.RegisterSerializable(&BuildGenerated{})
	base.RegisterSerializable(&CompilationDatabaseBuilder{})
	base.RegisterSerializable(&CompileEnv{})
	base.RegisterSerializable(&CompilerAlias{})
	base.RegisterSerializable(&CompilerRules{})
	base.RegisterSerializable(&ConfigRules{})
	base.RegisterSerializable(&ConfigurationAlias{})
	base.RegisterSerializable(&CustomUnit{})
	base.RegisterSerializable(&EnvironmentAlias{})
	base.RegisterSerializable(&Facet{})
	base.RegisterSerializable(&GeneratorRules{})
	base.RegisterSerializable(&ModuleAlias{})
	base.RegisterSerializable(&ModuleRules{})
	base.RegisterSerializable(&NamespaceRules{})
	base.RegisterSerializable(&PlatformAlias{})
	base.RegisterSerializable(&PlatformRules{})
	base.RegisterSerializable(&TargetActions{})
	base.RegisterSerializable(&TargetAlias{})
	base.RegisterSerializable(&TargetPayload{})
	base.RegisterSerializable(&Unit{})
	base.RegisterSerializable(&UnityFile{})

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

var GetCompileFlags = NewCompilationFlags("GenericCompilation", "cross-platform compilation flags", &CompileFlags{
	AdaptiveUnity:   base.INHERITABLE_TRUE,
	Avx2:            base.INHERITABLE_TRUE,
	Benchmark:       base.INHERITABLE_FALSE,
	CompilerVerbose: base.INHERITABLE_FALSE,
	CppRtti:         CPPRTTI_INHERIT,
	CppStd:          CPPSTD_INHERIT,
	DebugSymbols:    DEBUG_INHERIT,
	Deterministic:   base.INHERITABLE_TRUE,
	Exceptions:      EXCEPTION_INHERIT,
	Incremental:     base.INHERITABLE_INHERIT,
	Link:            LINK_INHERIT,
	LinkerVerbose:   base.INHERITABLE_FALSE,
	LTO:             base.INHERITABLE_INHERIT,
	PCH:             PCH_INHERIT,
	RuntimeChecks:   base.INHERITABLE_INHERIT,
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
	cfv.Persistent("CppRtti", "override C++ rtti support", &flags.CppRtti)
	cfv.Persistent("CppStd", "override C++ standard", &flags.CppStd)
	cfv.Persistent("DebugFastLink", "override debug symbols fastlink mode", &flags.DebugFastLink)
	cfv.Persistent("DebugSymbols", "override debug symbols mode", &flags.DebugSymbols)
	cfv.Persistent("Deterministic", "enable/disable deterministic compilation output", &flags.Deterministic)
	cfv.Persistent("Exceptions", "override exceptions mode", &flags.Exceptions)
	cfv.Persistent("Incremental", "enable/disable incremental linker", &flags.Incremental)
	cfv.Persistent("Link", "override link type", &flags.Link)
	cfv.Persistent("LinkerVerbose", "enable/disable linker verbose output", &flags.LinkerVerbose)
	cfv.Persistent("LTO", "enable/disable link time optimization", &flags.LTO)
	cfv.Persistent("PCH", "override size limit for splitting unity files", &flags.PCH)
	cfv.Persistent("RuntimeChecks", "enable/disable runtime security checks", &flags.RuntimeChecks)
	cfv.Persistent("Sanitizer", "override sanitizer mode", &flags.Sanitizer)
	cfv.Persistent("SizePerUnity", "size limit for splitting unity files", &flags.SizePerUnity)
	cfv.Persistent("Unity", "override unity build mode", &flags.Unity)
}
