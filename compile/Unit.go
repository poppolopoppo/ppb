package compile

import (
	"fmt"
	"strings"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/internal/io"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * Target Build Order
 ***************************************/

type TargetBuildOrder int32

func (x *TargetBuildOrder) Serialize(ar Archive) {
	ar.Int32((*int32)(x))
}

/***************************************
 * Target Alias
 ***************************************/

type TargetAlias struct {
	EnvironmentAlias
	ModuleAlias
}

type TargetAliases = SetT[TargetAlias]

func NewTargetAlias(module Module, platform Platform, config Configuration) TargetAlias {
	return TargetAlias{
		EnvironmentAlias: NewEnvironmentAlias(platform, config),
		ModuleAlias:      module.GetModule().ModuleAlias,
	}
}
func (x *TargetAlias) Valid() bool {
	return x.EnvironmentAlias.Valid() && x.ModuleAlias.Valid()
}
func (x TargetAlias) Alias() BuildAlias {
	return MakeBuildAlias("Unit", x.String())
}
func (x *TargetAlias) Serialize(ar Archive) {
	ar.Serializable(&x.EnvironmentAlias)
	ar.Serializable(&x.ModuleAlias)
}
func (x TargetAlias) Compare(o TargetAlias) int {
	if cmp := x.ModuleAlias.Compare(o.ModuleAlias); cmp == 0 {
		return x.EnvironmentAlias.Compare(o.EnvironmentAlias)
	} else {
		return cmp
	}
}
func (x *TargetAlias) Set(in string) error {
	parts := strings.Split(in, "-")
	if len(parts) < 3 {
		return fmt.Errorf("invalid target alias: %q", in)
	}
	if err := x.ModuleAlias.Set(strings.Join(parts[:len(parts)-2], "-")); err != nil {
		return err
	}
	if err := x.PlatformAlias.Set(parts[len(parts)-2]); err != nil {
		return err
	}
	if err := x.ConfigurationAlias.Set(parts[len(parts)-1]); err != nil {
		return err
	}
	return nil
}
func (x TargetAlias) String() string {
	return fmt.Sprintf("%v-%v-%v", x.ModuleAlias, x.PlatformName, x.ConfigName)
}
func (x *TargetAlias) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *TargetAlias) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}
func (x *TargetAlias) AutoComplete(in AutoComplete) {
	for _, a := range FindBuildAliases(CommandEnv.BuildGraph(), "Unit") {
		in.Add(strings.TrimPrefix(a.String(), "Unit://"))
	}
}

/***************************************
 * Unit Rules
 ***************************************/

type UnitDecorator interface {
	Decorate(env *CompileEnv, unit *Unit) error
	fmt.Stringer
}

type Units = SetT[*Unit]

type Unit struct {
	TargetAlias TargetAlias

	Ordinal     TargetBuildOrder
	Payload     PayloadType
	OutputFile  Filename
	SymbolsFile Filename
	ExportFile  Filename
	ExtraFiles  FileSet

	Source          ModuleSource
	ModuleDir       Directory
	GeneratedDir    Directory
	IntermediateDir Directory

	PrecompiledHeader Filename
	PrecompiledSource Filename
	PrecompiledObject Filename

	IncludeDependencies TargetAliases
	CompileDependencies TargetAliases
	LinkDependencies    TargetAliases
	RuntimeDependencies TargetAliases

	CompilerAlias     CompilerAlias
	PreprocessorAlias CompilerAlias

	Environment ProcessEnvironment

	TransitiveFacet Facet // append in case of public dependency
	GeneratedFiles  FileSet
	CustomUnits     CustomUnitList

	CppRules
	Facet
}

func (unit *Unit) String() string {
	return unit.Alias().String()
}

func (unit *Unit) GetEnvironment() (*CompileEnv, error) {
	return FindGlobalBuildable[*CompileEnv](unit.TargetAlias.EnvironmentAlias.Alias())
}
func (unit *Unit) GetBuildModule() (Module, error) {
	return FindBuildModule(unit.TargetAlias.ModuleAlias)
}
func (unit *Unit) GetBuildCompiler() (Compiler, error) {
	return FindGlobalBuildable[Compiler](unit.CompilerAlias.Alias())
}
func (unit *Unit) GetBuildPreprocessor() (Compiler, error) {
	return FindGlobalBuildable[Compiler](unit.PreprocessorAlias.Alias())
}

func (unit *Unit) GetModule() *ModuleRules {
	if module, err := unit.GetBuildModule(); err == nil {
		return module.GetModule()
	} else {
		LogPanicErr(LogCompile, err)
		return nil
	}
}
func (unit *Unit) GetCompiler() *CompilerRules {
	if compiler, err := unit.GetBuildCompiler(); err == nil {
		return compiler.GetCompiler()
	} else {
		LogPanicErr(LogCompile, err)
		return nil
	}
}
func (unit *Unit) GetPreprocessor() *CompilerRules {
	if compiler, err := unit.GetBuildPreprocessor(); err == nil {
		return compiler.GetCompiler()
	} else {
		LogPanicErr(LogCompile, err)
		return nil
	}
}

func (unit *Unit) GetFacet() *Facet {
	return &unit.Facet
}
func (unit *Unit) DebugString() string {
	return PrettyPrint(unit)
}
func (unit *Unit) Decorate(env *CompileEnv, decorator ...UnitDecorator) error {
	LogVeryVerbose(LogCompile, "unit %v: decorate with [%v]", unit.TargetAlias, MakeStringer(func() string {
		return Join(",", decorator...).String()
	}))
	for _, x := range decorator {
		if err := x.Decorate(env, unit); err != nil {
			return err
		}
	}
	return nil
}

func (unit *Unit) GetBinariesOutput(compiler Compiler, src Filename, payload PayloadType) Filename {
	AssertIn(payload, PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB)
	modulePath := src.Relative(UFS.Source)
	modulePath = SanitizePath(modulePath, '-')
	return UFS.Binaries.AbsoluteFile(modulePath).Normalize().ReplaceExt(
		fmt.Sprintf("-%s%s", unit.TargetAlias.EnvironmentAlias, compiler.Extname(payload)))
}
func (unit *Unit) GetGeneratedOutput(compiler Compiler, src Filename, payload PayloadType) Filename {
	modulePath := src.Relative(unit.ModuleDir)
	return unit.GeneratedDir.AbsoluteFile(modulePath).Normalize().ReplaceExt(compiler.Extname(payload))
}
func (unit *Unit) GetIntermediateOutput(compiler Compiler, src Filename, payload PayloadType) Filename {
	AssertIn(payload, PAYLOAD_OBJECTLIST, PAYLOAD_PRECOMPILEDHEADER, PAYLOAD_STATICLIB)
	var modulePath string
	if src.Dirname.IsIn(unit.GeneratedDir) {
		modulePath = src.Relative(unit.GeneratedDir)
	} else {
		modulePath = src.Relative(unit.ModuleDir)
	}
	return unit.IntermediateDir.AbsoluteFile(modulePath).Normalize().ReplaceExt(compiler.Extname(payload))
}
func (unit *Unit) GetPayloadOutput(compiler Compiler, src Filename, payload PayloadType) (result Filename) {
	switch payload {
	case PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB:
		result = unit.GetBinariesOutput(compiler, src, payload)
	case PAYLOAD_OBJECTLIST, PAYLOAD_PRECOMPILEDHEADER, PAYLOAD_STATICLIB:
		result = unit.GetIntermediateOutput(compiler, src, payload)
	case PAYLOAD_HEADERS:
		result = src
	default:
		UnexpectedValue(payload)
	}
	return
}

func (unit *Unit) Serialize(ar Archive) {
	ar.Serializable(&unit.TargetAlias)

	ar.Serializable(&unit.Ordinal)
	ar.Serializable(&unit.Payload)
	ar.Serializable(&unit.OutputFile)
	ar.Serializable(&unit.SymbolsFile)
	ar.Serializable(&unit.ExportFile)
	ar.Serializable(&unit.ExtraFiles)

	ar.Serializable(&unit.Source)
	ar.Serializable(&unit.ModuleDir)
	ar.Serializable(&unit.GeneratedDir)
	ar.Serializable(&unit.IntermediateDir)

	ar.Serializable(&unit.PrecompiledHeader)
	ar.Serializable(&unit.PrecompiledSource)
	ar.Serializable(&unit.PrecompiledObject)

	SerializeSlice(ar, unit.IncludeDependencies.Ref())
	SerializeSlice(ar, unit.CompileDependencies.Ref())
	SerializeSlice(ar, unit.LinkDependencies.Ref())
	SerializeSlice(ar, unit.RuntimeDependencies.Ref())

	ar.Serializable(&unit.CompilerAlias)
	ar.Serializable(&unit.PreprocessorAlias)

	ar.Serializable(&unit.Environment)

	ar.Serializable(&unit.TransitiveFacet)
	ar.Serializable(&unit.GeneratedFiles)
	ar.Serializable(&unit.CustomUnits)

	ar.Serializable(&unit.CppRules)
	ar.Serializable(&unit.Facet)
}

/***************************************
 * Unit Factory
 ***************************************/

func (unit *Unit) Alias() BuildAlias {
	return unit.TargetAlias.Alias()
}
func (unit *Unit) Build(bc BuildContext) error {
	*unit = Unit{ // reset to default value before building
		TargetAlias: unit.TargetAlias,
	}

	compileEnv, err := unit.GetEnvironment()
	if err != nil {
		return err
	}

	if err := bc.NeedBuildable(&compileEnv.CompilerAlias); err != nil {
		return err
	}

	compiler, err := compileEnv.GetBuildCompiler()
	if err != nil {
		return err
	}

	bc.BuildGraph()
	moduleRules, err := FindGlobalBuildable[*ModuleRules](unit.TargetAlias.ModuleAlias.Alias())
	if err != nil {
		return err
	}

	expandedModule, err := compileModuleForEnv(bc, compileEnv, moduleRules)
	if err != nil {
		return err
	}

	relativePath := expandedModule.RelativePath()

	unit.Ordinal = 0
	unit.Source = expandedModule.Source
	unit.ModuleDir = expandedModule.ModuleDir
	unit.GeneratedDir = compileEnv.GeneratedDir().AbsoluteFolder(relativePath)
	unit.IntermediateDir = compileEnv.IntermediateDir().AbsoluteFolder(relativePath)
	unit.CompilerAlias = compileEnv.CompilerAlias
	unit.CppRules = compileEnv.GetCpp(&expandedModule)
	unit.Environment = compiler.GetCompiler().Environment
	unit.Payload = compileEnv.GetPayloadType(&expandedModule, unit.Link)
	unit.OutputFile = unit.GetPayloadOutput(compiler,
		unit.ModuleDir.Parent().File(unit.TargetAlias.ModuleAlias.ModuleName),
		unit.Payload)

	switch unit.Payload {
	case PAYLOAD_SHAREDLIB:
		// when linking against a shared lib we must provide the export .lib/.a, not the produced .dll/.so
		unit.ExportFile = unit.OutputFile.ReplaceExt(compiler.Extname(PAYLOAD_STATICLIB))
	default:
		if unit.Payload.HasOutput() {
			unit.ExportFile = unit.OutputFile
		}
	}

	switch unit.PCH {
	case PCH_DISABLED:
		break
	case PCH_MONOLITHIC, PCH_SHARED:
		if expandedModule.PrecompiledHeader == nil || expandedModule.PrecompiledSource == nil {
			if expandedModule.PrecompiledHeader != nil {
				LogPanic(LogCompile, "unit is using PCH_%s, but precompiled header is nil (source: %v)", unit.PCH, expandedModule.PrecompiledSource)
			}
			if expandedModule.PrecompiledSource != nil {
				LogPanic(LogCompile, "unit is using PCH_%s, but precompiled source is nil (header: %v)", unit.PCH, expandedModule.PrecompiledHeader)
			}
			unit.PCH = PCH_DISABLED
		} else {
			unit.PrecompiledHeader = *expandedModule.PrecompiledHeader
			unit.PrecompiledSource = unit.PrecompiledHeader

			IfWindows(func() {
				// CPP is only used on Windows platform
				unit.PrecompiledSource = *expandedModule.PrecompiledSource
			})

			Assert(func() bool { return expandedModule.PrecompiledHeader.Exists() })
			Assert(func() bool { return expandedModule.PrecompiledSource.Exists() })
			unit.PrecompiledObject = unit.GetPayloadOutput(compiler, unit.PrecompiledSource, PAYLOAD_PRECOMPILEDHEADER)
		}
	default:
		UnexpectedValuePanic(unit.PCH, unit.PCH)
	}

	unit.Facet = NewFacet()
	unit.Facet.Append(compileEnv, &expandedModule)

	if err := unit.Decorate(compileEnv, &expandedModule, compileEnv.GetPlatform(), compileEnv.GetConfig()); err != nil {
		return err
	}

	if err := unit.linkModuleDependencies(bc, compileEnv, PRIVATE, expandedModule.PrivateDependencies...); err != nil {
		return err
	}
	if err := unit.linkModuleDependencies(bc, compileEnv, PUBLIC, expandedModule.PublicDependencies...); err != nil {
		return err
	}
	if err := unit.linkModuleDependencies(bc, compileEnv, RUNTIME, expandedModule.RuntimeDependencies...); err != nil {
		return err
	}

	unit.Defines.Append(
		"BUILD_TARGET_NAME="+unit.TargetAlias.ModuleAlias.String(),
		fmt.Sprintf("BUILD_TARGET_ORDINAL=%d", unit.Ordinal))

	unit.Facet.PerformSubstitutions()

	if err := unit.Decorate(compileEnv, compiler.GetCompiler()); err != nil {
		return err
	}

	if err := CreateDirectory(bc, unit.OutputFile.Dirname); err != nil {
		return err
	}
	if err := CreateDirectory(bc, unit.IntermediateDir); err != nil {
		return err
	}

	staticDeps := BuildAliases{}

	for _, generator := range expandedModule.Generators {
		generated, err := generator.CreateGenerated(bc, &expandedModule, unit)
		if err != nil {
			return err
		}

		staticDeps.Append(generated.Alias())
		unit.GeneratedFiles.Append(generated.OutputFile)
	}

	_, err = bc.OutputFactory(WrapBuildFactory[*TargetActions](func(bi BuildInitializer) (*TargetActions, error) {
		return &TargetActions{
			TargetAlias: unit.TargetAlias,
		}, bi.DependsOn(staticDeps...)
	}))
	return err
}

func FindBuildUnit(target TargetAlias) (*Unit, error) {
	return FindGlobalBuildable[*Unit](target.Alias())
}

func NeedAllBuildUnits(bc BuildContext) (units []*Unit, err error) {
	modules, err := NeedAllBuildModules(bc)
	if err != nil {
		return
	}

	if err = ForeachEnvironmentAlias(func(ea EnvironmentAlias) error {
		for _, module := range modules {
			unit, err := FindBuildUnit(TargetAlias{
				ModuleAlias:      module.GetModule().ModuleAlias,
				EnvironmentAlias: ea,
			})
			if err != nil {
				return err
			}

			units = append(units, unit)
		}

		return nil
	}); err != nil {
		return
	}

	if err = bc.DependsOn(MakeBuildAliases(units...)...); err != nil {
		return
	}

	return
}

func (unit *Unit) addIncludeDependency(other *Unit) {
	if unit.IncludeDependencies.AppendUniq(other.TargetAlias) {
		LogDebug(LogCompile, "[%v] include dep -> %v", unit.TargetAlias, other.TargetAlias)
		unit.Facet.AppendUniq(&other.TransitiveFacet)
	}
}
func (unit *Unit) addCompileDependency(other *Unit) {
	if unit.CompileDependencies.AppendUniq(other.TargetAlias) {
		LogDebug(LogCompile, "[%v] compile dep -> %v", unit.TargetAlias, other.TargetAlias)
		unit.Facet.AppendUniq(&other.TransitiveFacet)
	}
}
func (unit *Unit) addLinkDependency(other *Unit) {
	if unit.LinkDependencies.AppendUniq(other.TargetAlias) {
		LogDebug(LogCompile, "[%v] link dep -> %v", unit.TargetAlias, other.TargetAlias)
		unit.Facet.AppendUniq(&other.TransitiveFacet)
	}
}
func (unit *Unit) addRuntimeDependency(other *Unit) {
	if unit.RuntimeDependencies.AppendUniq(other.TargetAlias) {
		LogDebug(LogCompile, "[%v] runtime dep -> %v", unit.TargetAlias, other.TargetAlias)
		unit.IncludePaths.AppendUniq(other.TransitiveFacet.IncludePaths...)
		unit.ForceIncludes.AppendUniq(other.TransitiveFacet.ForceIncludes...)
	}
}

func (unit *Unit) linkModuleDependencies(bc BuildContext, compileEnv *CompileEnv, vis VisibilityType, moduleAliases ...ModuleAlias) error {
	for _, moduleAlias := range moduleAliases {
		targetAlias := TargetAlias{
			ModuleAlias:      moduleAlias,
			EnvironmentAlias: compileEnv.EnvironmentAlias,
		}
		if err := bc.DependsOn(targetAlias.Alias()); err != nil {
			return err
		}

		other, err := FindBuildUnit(targetAlias)
		if err != nil {
			return err
		}

		if other.Ordinal >= unit.Ordinal {
			unit.Ordinal = other.Ordinal + 1
		}

		switch other.Payload {
		case PAYLOAD_HEADERS:
			unit.addIncludeDependency(other)

		case PAYLOAD_OBJECTLIST, PAYLOAD_PRECOMPILEDHEADER:
			unit.addCompileDependency(other)

		case PAYLOAD_STATICLIB, PAYLOAD_SHAREDLIB:
			switch vis {
			case PUBLIC, PRIVATE:
				if other.GetModule().ModuleType == MODULE_LIBRARY {
					unit.addLinkDependency(other)
				} else {
					unit.addCompileDependency(other)
				}

			case RUNTIME:
				if other.Payload == PAYLOAD_SHAREDLIB {
					unit.addRuntimeDependency(other)
				} else {
					LogPanic(LogCompile, "%v <%v> is linking against %v <%v> with %v visibility, which is not allowed",
						unit.Payload, unit, unit.Payload, other, vis)
				}

			default:
				UnexpectedValue(vis)
			}
		case PAYLOAD_EXECUTABLE:
			fallthrough // can't depend on an executable
		default:
			return MakeUnexpectedValueError(unit.Payload, other.Payload)
		}
	}

	return nil
}

func foreachModule(bc BuildContext, compileEnv *CompileEnv, each func(*ModuleRules) error, moduleAliases ...ModuleAlias) error {
	for _, moduleAlias := range moduleAliases {
		if err := bc.NeedBuildable(moduleAlias); err != nil {
			return err
		}

		moduleDependency, err := FindBuildModule(moduleAlias)
		if err != nil {
			return err
		}

		moduleRules, err := compileModuleForEnv(bc, compileEnv, moduleDependency.GetModule())
		if err != nil {
			return err
		}

		if err := each(&moduleRules); err != nil {
			return err
		}
	}

	return nil
}

func compileModuleForEnv(bc BuildContext, compileEnv *CompileEnv, moduleRules *ModuleRules) (ModuleRules, error) {
	module := moduleRules.ExpandModule(compileEnv)

	// public and runtime dependencies are viral

	foreachModule(bc, compileEnv, func(mr *ModuleRules) error {
		for _, moduleAlias := range mr.GetModule().PublicDependencies {
			module.PrivateDependencies.AppendUniq(moduleAlias)
		}
		for _, moduleAlias := range mr.GetModule().RuntimeDependencies {
			module.RuntimeDependencies.AppendUniq(moduleAlias)
		}
		return nil
	}, module.PrivateDependencies...)

	foreachModule(bc, compileEnv, func(mr *ModuleRules) error {
		for _, moduleAlias := range mr.GetModule().PublicDependencies {
			module.PublicDependencies.AppendUniq(moduleAlias)
		}
		for _, moduleAlias := range mr.GetModule().RuntimeDependencies {
			module.RuntimeDependencies.AppendUniq(moduleAlias)
		}
		return nil
	}, module.PublicDependencies...)

	foreachModule(bc, compileEnv, func(mr *ModuleRules) error {
		for _, moduleAlias := range mr.GetModule().RuntimeDependencies {
			module.RuntimeDependencies.AppendUniq(moduleAlias)
		}
		return nil
	}, module.RuntimeDependencies...)

	return module, nil
}
