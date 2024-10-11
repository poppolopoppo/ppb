package compile

import (
	"fmt"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"

	internal_io "github.com/poppolopoppo/ppb/internal/io"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * Target Alias
 ***************************************/

type TargetAlias struct {
	EnvironmentAlias
	ModuleAlias
}

type TargetAliases = base.SetT[TargetAlias]

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
	return MakeBuildAlias("Unit", x.PlatformName, x.ConfigName, x.ModuleAlias.NamespaceName, x.ModuleAlias.ModuleName)
}
func (x TargetAlias) String() string {
	return fmt.Sprintf("%v-%v-%v", x.ModuleAlias, x.PlatformName, x.ConfigName)
}
func (x *TargetAlias) Serialize(ar base.Archive) {
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
func (x *TargetAlias) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *TargetAlias) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x TargetAlias) AutoComplete(in base.AutoComplete) {
	if bg, ok := in.GetUserParam().(BuildGraphReadPort); ok {
		var modules base.SetT[Module]
		ForeachBuildable(bg, func(_ BuildAlias, m Module) error {
			modules.Append(m)
			return nil
		})

		ForeachEnvironmentAlias(func(ea EnvironmentAlias) error {
			for _, module := range modules {
				targetAlias := TargetAlias{
					EnvironmentAlias: ea,
					ModuleAlias:      module.GetModule().ModuleAlias,
				}
				in.Add(targetAlias.String(), module.GetModule().ModuleType.String())
			}
			return nil
		})
	} else {
		base.UnreachableCode()
	}
}

/***************************************
 * Compilation Environment injection
 ***************************************/

type UnitDecorator interface {
	Decorate(bg BuildGraphReadPort, env *CompileEnv, unit *Unit) error
	fmt.Stringer
}

type UnitCompileEvent struct {
	Environment *CompileEnv
	Unit        *Unit
}

var onUnitCompileEvent base.ConcurrentEvent[UnitCompileEvent]

func OnUnitCompile(e base.EventDelegate[UnitCompileEvent]) base.DelegateHandle {
	return onUnitCompileEvent.Add(e)
}
func RemoveOnUnitCompile(h base.DelegateHandle) bool {
	return onUnitCompileEvent.Remove(h)
}

/***************************************
 * Unit Rules
 ***************************************/

type Units = base.SetT[*Unit]

type Unit struct {
	TargetAlias TargetAlias

	Ordinal     int32
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

	Environment internal_io.ProcessEnvironment

	TransitiveFacet Facet // append in case of public dependency
	GeneratedFiles  FileSet
	CustomUnits     CustomUnitList

	CppRules
	Facet
}

func (unit *Unit) String() string {
	return unit.Alias().String()
}

func (unit *Unit) GetEnvironment(bg BuildGraphReadPort) (*CompileEnv, error) {
	return FindBuildable[*CompileEnv](bg, unit.TargetAlias.EnvironmentAlias.Alias())
}
func (unit *Unit) GetBuildModule(bg BuildGraphReadPort) (Module, error) {
	return FindBuildModule(bg, unit.TargetAlias.ModuleAlias)
}
func (unit *Unit) GetBuildCompiler(bg BuildGraphReadPort) (Compiler, error) {
	return FindBuildable[Compiler](bg, unit.CompilerAlias.Alias())
}
func (unit *Unit) GetBuildPreprocessor(bg BuildGraphReadPort) (Compiler, error) {
	return FindBuildable[Compiler](bg, unit.PreprocessorAlias.Alias())
}

func (unit *Unit) GetModule(bg BuildGraphReadPort) *ModuleRules {
	if module, err := unit.GetBuildModule(bg); err == nil {
		return module.GetModule()
	} else {
		base.LogPanicErr(LogCompile, err)
		return nil
	}
}
func (unit *Unit) GetCompiler(bg BuildGraphReadPort) *CompilerRules {
	if compiler, err := unit.GetBuildCompiler(bg); err == nil {
		return compiler.GetCompiler()
	} else {
		base.LogPanicErr(LogCompile, err)
		return nil
	}
}
func (unit *Unit) GetPreprocessor(bg BuildGraphReadPort) *CompilerRules {
	if compiler, err := unit.GetBuildPreprocessor(bg); err == nil {
		return compiler.GetCompiler()
	} else {
		base.LogPanicErr(LogCompile, err)
		return nil
	}
}

func (unit *Unit) GetFacet() *Facet {
	return &unit.Facet
}
func (unit *Unit) DebugString() string {
	return base.PrettyPrint(unit)
}
func (unit *Unit) Decorate(bg BuildGraphReadPort, env *CompileEnv, decorator ...UnitDecorator) error {
	base.LogVeryVerbose(LogCompile, "unit %v: decorate with [%v]", unit.TargetAlias, base.MakeStringer(func() string {
		return base.Join(",", decorator...).String()
	}))
	for _, x := range decorator {
		if err := x.Decorate(bg, env, unit); err != nil {
			return err
		}
	}
	return nil
}

func (unit *Unit) GetBinariesOutput(compiler Compiler, src Filename, payload PayloadType) Filename {
	base.AssertIn(payload, PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB)
	modulePath := src.Relative(UFS.Source)
	modulePath = SanitizePath(modulePath, '-')
	modulePath = fmt.Sprintf("%s-%s", modulePath, unit.TargetAlias.EnvironmentAlias)
	return compiler.GetPayloadOutput(unit, payload, UFS.Binaries.AbsoluteFile(modulePath))
}
func (unit *Unit) GetIntermediateOutput(compiler Compiler, src Filename, payload PayloadType) Filename {
	base.AssertIn(payload, PAYLOAD_OBJECTLIST, PAYLOAD_HEADERUNIT, PAYLOAD_PRECOMPILEDHEADER, PAYLOAD_PRECOMPILEDOBJECT, PAYLOAD_STATICLIB)
	var modulePath string
	if src.Dirname.IsIn(unit.GeneratedDir) {
		modulePath = src.Relative(unit.GeneratedDir)
	} else {
		modulePath = src.Relative(unit.ModuleDir)
	}
	return compiler.GetPayloadOutput(unit, payload, unit.IntermediateDir.AbsoluteFile(modulePath))
}
func (unit *Unit) GetPayloadOutput(compiler Compiler, src Filename, payload PayloadType) (result Filename) {
	switch payload {
	case PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB:
		result = unit.GetBinariesOutput(compiler, src, payload)
	case PAYLOAD_OBJECTLIST, PAYLOAD_HEADERUNIT, PAYLOAD_PRECOMPILEDHEADER, PAYLOAD_PRECOMPILEDOBJECT, PAYLOAD_STATICLIB:
		result = unit.GetIntermediateOutput(compiler, src, payload)
	case PAYLOAD_HEADERS:
		result = src
	default:
		base.UnexpectedValue(payload)
	}
	return
}

func (unit *Unit) Serialize(ar base.Archive) {
	ar.Serializable(&unit.TargetAlias)

	ar.Int32(&unit.Ordinal)
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

	base.SerializeSlice(ar, unit.IncludeDependencies.Ref())
	base.SerializeSlice(ar, unit.CompileDependencies.Ref())
	base.SerializeSlice(ar, unit.LinkDependencies.Ref())
	base.SerializeSlice(ar, unit.RuntimeDependencies.Ref())

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

	compileEnv, err := unit.GetEnvironment(bc)
	if err != nil {
		return err
	}

	compilerBuildable, err := bc.NeedBuildable(compileEnv.CompilerAlias)
	if err != nil {
		return err
	}
	compiler := compilerBuildable.(Compiler)

	moduleRules, err := FindBuildable[*ModuleRules](bc, unit.TargetAlias.ModuleAlias.Alias())
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
	unit.CppRules = compileEnv.GetCpp(bc, &expandedModule)
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
	case PCH_MONOLITHIC, PCH_SHARED, PCH_HEADERUNIT:
		if !expandedModule.PrecompiledHeader.Valid() || !expandedModule.PrecompiledSource.Valid() {
			if expandedModule.PrecompiledHeader.Valid() {
				base.LogPanic(LogCompile, "unit is using PCH_%s, but precompiled source is nil (header: %v)", unit.PCH, expandedModule.PrecompiledHeader)
			}
			if expandedModule.PrecompiledSource.Valid() {
				base.LogPanic(LogCompile, "unit is using PCH_%s, but precompiled header is nil (source: %v)", unit.PCH, expandedModule.PrecompiledSource)
			}
			unit.PCH = PCH_DISABLED
		} else {
			base.Assert(func() bool { return expandedModule.PrecompiledHeader.Exists() })
			base.Assert(func() bool { return expandedModule.PrecompiledSource.Exists() })

			unit.PrecompiledHeader = expandedModule.PrecompiledHeader
			unit.PrecompiledSource = expandedModule.PrecompiledSource
		}
	default:
		base.UnexpectedValuePanic(unit.PCH, unit.PCH)
	}

	unit.Facet = NewFacet()
	unit.Facet.Append(compileEnv, &expandedModule)

	if err := unit.Decorate(bc, compileEnv, &expandedModule, compileEnv.GetPlatform(bc), compileEnv.GetConfig(bc)); err != nil {
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

	if err := unit.Decorate(bc, compileEnv, compiler.GetCompiler()); err != nil {
		return err
	}

	if err := internal_io.CreateDirectory(bc, unit.OutputFile.Dirname); err != nil {
		return err
	}
	if err := internal_io.CreateDirectory(bc, unit.IntermediateDir); err != nil {
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

	onUnitCompileEvent.Invoke(UnitCompileEvent{
		Environment: compileEnv,
		Unit:        unit,
	})

	_, err = bc.OutputFactory(WrapBuildFactory[*TargetActions](func(bi BuildInitializer) (*TargetActions, error) {
		return &TargetActions{
			TargetAlias: unit.TargetAlias,
		}, bi.DependsOn(staticDeps...)
	}))
	return err
}

func FindBuildUnit(bg BuildGraphReadPort, target TargetAlias) (*Unit, error) {
	return FindBuildable[*Unit](bg, target.Alias())
}

func ForeachBuildUnits(bg BuildGraphReadPort, ea EnvironmentAlias, each func(*Unit) error, ma ...ModuleAlias) error {
	for _, it := range ma {
		unit, err := FindBuildUnit(bg, TargetAlias{EnvironmentAlias: ea, ModuleAlias: it})
		if err != nil {
			return err
		}
		if err = each(unit); err != nil {
			return err
		}
	}
	return nil
}

func NeedAllBuildUnits(bc BuildContext) (units []*Unit, err error) {
	modules, err := NeedAllBuildModules(bc)
	if err != nil {
		return
	}

	if err = ForeachEnvironmentAlias(func(ea EnvironmentAlias) error {
		for _, module := range modules {
			buildable, err := bc.NeedBuildable(TargetAlias{
				ModuleAlias:      module.GetModule().ModuleAlias,
				EnvironmentAlias: ea,
			})
			if err != nil {
				return err
			}

			units = append(units, buildable.(*Unit))
		}

		return nil
	}); err != nil {
		return
	}

	return
}

func NeedAllUnitAliases(bc BuildContext) (aliases []TargetAlias, err error) {
	moduleAliases, err := NeedAllModuleAliases(bc)
	if err != nil {
		return
	}

	err = ForeachEnvironmentAlias(func(ea EnvironmentAlias) error {
		for _, ma := range moduleAliases {
			aliases = append(aliases, TargetAlias{
				ModuleAlias:      ma,
				EnvironmentAlias: ea,
			})
		}
		return nil
	})
	return
}

func (unit *Unit) addIncludeDependency(other *Unit) {
	if unit.IncludeDependencies.AppendUniq(other.TargetAlias) {
		base.LogDebug(LogCompile, "[%v] include dep -> %v", unit.TargetAlias, other.TargetAlias)
		unit.Facet.AppendUniq(&other.TransitiveFacet)
	}
}
func (unit *Unit) addCompileDependency(other *Unit) {
	if unit.CompileDependencies.AppendUniq(other.TargetAlias) {
		base.LogDebug(LogCompile, "[%v] compile dep -> %v", unit.TargetAlias, other.TargetAlias)
		unit.Facet.AppendUniq(&other.TransitiveFacet)
	}
}
func (unit *Unit) addLinkDependency(other *Unit) {
	if unit.LinkDependencies.AppendUniq(other.TargetAlias) {
		base.LogDebug(LogCompile, "[%v] link dep -> %v", unit.TargetAlias, other.TargetAlias)
		unit.Facet.AppendUniq(&other.TransitiveFacet)
	}
}
func (unit *Unit) addRuntimeDependency(other *Unit) {
	if unit.RuntimeDependencies.AppendUniq(other.TargetAlias) {
		base.LogDebug(LogCompile, "[%v] runtime dep -> %v", unit.TargetAlias, other.TargetAlias)
		unit.IncludePaths.AppendUniq(other.TransitiveFacet.IncludePaths...)
		unit.ForceIncludes.AppendUniq(other.TransitiveFacet.ForceIncludes...)
	}
}

func (unit *Unit) linkModuleDependencies(bc BuildContext, compileEnv *CompileEnv, vis VisibilityType, moduleAliases ...ModuleAlias) error {
	for _, moduleAlias := range moduleAliases {
		buildable, err := bc.NeedBuildable(TargetAlias{
			ModuleAlias:      moduleAlias,
			EnvironmentAlias: compileEnv.EnvironmentAlias,
		}.Alias())
		if err != nil {
			return err
		}
		other := buildable.(*Unit)

		if other.Ordinal >= unit.Ordinal {
			unit.Ordinal = other.Ordinal + 1
		}

		switch other.Payload {
		case PAYLOAD_HEADERS:
			unit.addIncludeDependency(other)

		case PAYLOAD_OBJECTLIST, PAYLOAD_HEADERUNIT, PAYLOAD_PRECOMPILEDHEADER:
			unit.addCompileDependency(other)

		case PAYLOAD_STATICLIB, PAYLOAD_SHAREDLIB:
			switch vis {
			case PUBLIC, PRIVATE:
				if other.GetModule(bc).ModuleType == MODULE_LIBRARY {
					unit.addLinkDependency(other)
				} else {
					unit.addCompileDependency(other)
				}

			case RUNTIME:
				if other.Payload == PAYLOAD_SHAREDLIB {
					unit.addRuntimeDependency(other)
				} else {
					base.LogPanic(LogCompile, "%v <%v> is linking against %v <%v> with %v visibility, which is not allowed",
						unit.Payload, unit, unit.Payload, other, vis)
				}

			default:
				base.UnexpectedValue(vis)
			}
		case PAYLOAD_EXECUTABLE, PAYLOAD_PRECOMPILEDOBJECT:
			fallthrough // can't depend on an executable or precompiled object
		default:
			return base.MakeUnexpectedValueError(unit.Payload, other.Payload)
		}
	}

	return nil
}

func foreachModule(bc BuildContext, compileEnv *CompileEnv, each func(*ModuleRules) error, moduleAliases ...ModuleAlias) error {
	for _, moduleAlias := range moduleAliases {
		buildable, err := bc.NeedBuildable(moduleAlias)
		if err != nil {
			return err
		}

		moduleRules, err := compileModuleForEnv(bc, compileEnv, buildable.(Module).GetModule())
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
