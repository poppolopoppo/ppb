package compile

import (
	"github.com/poppolopoppo/ppb/internal/base"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var LogCompile = base.NewLogCategory("Compile")

func InitCompile() {
	base.LogTrace(LogCompile, "build/compile.Init()")

	// register type for serialization
	base.RegisterSerializable[NamespaceModel]()
	base.RegisterSerializable[ModuleModel]()

	base.RegisterSerializable[BuildConfig]()
	base.RegisterSerializable[BuildGenerated]()
	base.RegisterSerializable[CompilationDatabaseBuilder]()
	base.RegisterSerializable[CompileEnv]()
	base.RegisterSerializable[CompilerAlias]()
	base.RegisterSerializable[CompilerRules]()
	base.RegisterSerializable[ConfigRules]()
	base.RegisterSerializable[ConfigurationAlias]()
	base.RegisterSerializable[CustomUnit]()
	base.RegisterSerializable[EnvironmentAlias]()
	base.RegisterSerializable[Facet]()
	base.RegisterSerializable[GeneratorRules]()
	base.RegisterSerializable[ModuleAlias]()
	base.RegisterSerializable[ModuleRules]()
	base.RegisterSerializable[NamespaceRules]()
	base.RegisterSerializable[PlatformAlias]()
	base.RegisterSerializable[PlatformRules]()
	base.RegisterSerializable[TargetActions]()
	base.RegisterSerializable[TargetAlias]()
	base.RegisterSerializable[TargetPayload]()
	base.RegisterSerializable[Unit]()
	base.RegisterSerializable[UnityFile]()

	AllConfigurations.Add("Debug", Configuration_Debug)
	AllConfigurations.Add("FastDebug", Configuration_FastDebug)
	AllConfigurations.Add("Devel", Configuration_Devel)
	AllConfigurations.Add("Test", Configuration_Test)
	AllConfigurations.Add("Shipping", Configuration_Shipping)
}

var AllCompilationFlags []struct {
	CommandOptionFunc
	BuildFactory
}

func NewCompilationFlags[T any, P interface {
	*T
	CommandParsableFlags
}](name, description string, flags T) func(BuildInitializer, ...BuildOptionFunc) (P, error) {
	factory := NewCommandParsableFactory[T, P](name, flags)
	AllCompilationFlags = append(AllCompilationFlags, struct {
		CommandOptionFunc
		BuildFactory
	}{
		CommandOptionFunc: OptionCommandParsableAccessor(name, description, func() P {
			if builder, err := factory.Need(CommandEnv.BuildGraph().GlobalContext()); err == nil {
				return &builder.Flags
			} else {
				CommandPanic(err)
				return nil
			}
		}),
		BuildFactory: factory,
	})

	return func(bi BuildInitializer, opts ...BuildOptionFunc) (P, error) {
		if builder, err := factory.Need(bi, opts...); err == nil {
			return &builder.Flags, nil
		} else {
			return nil, err
		}
	}
}

func OptionCommandAllCompilationFlags() CommandOptionFunc {
	return OptionCommandItem(func(ci CommandItem) {
		for _, it := range AllCompilationFlags {
			ci.Options(it.CommandOptionFunc)
		}
	})
}

/***************************************
 * Compile Flags
 ***************************************/

type CompileFlags CppRules

var GetCompileFlags = NewCompilationFlags("GenericCompilation", "cross-platform compilation flags", CompileFlags{
	AdaptiveUnity:   base.INHERITABLE_TRUE,
	Benchmark:       base.INHERITABLE_FALSE,
	CompilerVerbose: base.INHERITABLE_FALSE,
	CppRtti:         CPPRTTI_INHERIT,
	CppStd:          CPPSTD_INHERIT,
	DebugFastLink:   base.INHERITABLE_FALSE,
	DebugInfo:       DEBUGINFO_INHERIT,
	Deterministic:   base.INHERITABLE_TRUE,
	Exceptions:      EXCEPTION_INHERIT,
	Incremental:     base.INHERITABLE_INHERIT,
	Instructions:    base.MakeEnumSet(INSTRUCTIONSET_AVX2, INSTRUCTIONSET_SSE3),
	Link:            LINK_INHERIT,
	LinkerVerbose:   base.INHERITABLE_FALSE,
	LTO:             base.INHERITABLE_INHERIT,
	Optimize:        OPTIMIZE_INHERIT,
	PCH:             PCH_INHERIT,
	RuntimeChecks:   base.INHERITABLE_INHERIT,
	RuntimeLib:      RUNTIMELIB_INHERIT,
	Sanitizer:       SANITIZER_NONE,
	SizePerUnity:    150 * 1024.0, // 150 KiB
	Unity:           UNITY_INHERIT,
	Warnings: CppWarnings{
		Default:        WARNING_ERROR,
		Deprecation:    WARNING_ERROR,
		Pedantic:       WARNING_ERROR,
		ShadowVariable: WARNING_ERROR,
		UndefinedMacro: WARNING_ERROR,
		UnsafeTypeCast: WARNING_ERROR,
	},
})

func (flags *CompileFlags) GetCpp() *CppRules { return (*CppRules)(flags) }
func (flags *CompileFlags) Flags(cfv CommandFlagsVisitor) {
	cfv.Persistent("AdaptiveUnity", "enable/disable adaptive unity using source control", &flags.AdaptiveUnity)
	cfv.Persistent("Benchmark", "enable/disable compilation benchmarks", &flags.Benchmark)
	cfv.Persistent("CompilerVerbose", "enable/disable compiler verbose output", &flags.CompilerVerbose)
	cfv.Persistent("CppRtti", "override C++ rtti support", &flags.CppRtti)
	cfv.Persistent("CppStd", "override C++ standard", &flags.CppStd)
	cfv.Persistent("DebugFastLink", "override debug symbols fastlink mode", &flags.DebugFastLink)
	cfv.Persistent("DebugInfo", "override debug symbols mode", &flags.DebugInfo)
	cfv.Persistent("Deterministic", "enable/disable deterministic compilation output", &flags.Deterministic)
	cfv.Persistent("Exceptions", "override exceptions mode", &flags.Exceptions)
	cfv.Persistent("Instructions", "enable/disable CPU instruction sets", &flags.Instructions)
	cfv.Persistent("Incremental", "enable/disable incremental linker", &flags.Incremental)
	cfv.Persistent("Link", "override link type", &flags.Link)
	cfv.Persistent("LinkerVerbose", "enable/disable linker verbose output", &flags.LinkerVerbose)
	cfv.Persistent("LTO", "enable/disable link time optimization", &flags.LTO)
	cfv.Persistent("Optimize", "override compiler optimization level", &flags.Optimize)
	cfv.Persistent("PCH", "override size limit for splitting unity files", &flags.PCH)
	cfv.Persistent("RuntimeChecks", "enable/disable runtime security checks", &flags.RuntimeChecks)
	cfv.Persistent("RuntimeLib", "override runtime library selection", &flags.RuntimeLib)
	cfv.Persistent("Sanitizer", "override sanitizer mode", &flags.Sanitizer)
	cfv.Persistent("SizePerUnity", "size limit for splitting unity files", &flags.SizePerUnity)
	cfv.Persistent("Unity", "override unity build mode", &flags.Unity)
	cfv.Persistent("Warning", "override default warning level", &flags.Warnings.Default)
	cfv.Persistent("Warning:Deprecation", "override deprecation warning level", &flags.Warnings.Deprecation)
	cfv.Persistent("Warning:ShadowVariable", "override shadow variable warning level", &flags.Warnings.ShadowVariable)
	cfv.Persistent("Warning:UndefinedMacro", "override undefined macro identifier warning level", &flags.Warnings.UndefinedMacro)
	cfv.Persistent("Warning:UnsafeTypeCast", "override unsafe type cast warning level", &flags.Warnings.UnsafeTypeCast)
}
