package compile

import (
	"fmt"
	"io"
	"path"
	"strings"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var LogModel = NewLogCategory("Model")

const PCH_DEFAULT_HEADER = "stdafx.h"
const PCH_DEFAULT_SOURCE = "stdafx.cpp"

func BuildRootNamespaceModel() BuildFactoryTyped[*NamespaceModel] {
	return BuildNamespaceModel(CommandEnv.RootFile(), "")
}

/***************************************
 * Namespace Model
 ***************************************/

const NAMESPACEMODEL_EXT = "-namespace.json"

type NamespaceModel struct {
	Children StringSet
	Modules  StringSet

	ExtensionModel
}

func BuildNamespaceModel(source Filename, namespace string) BuildFactoryTyped[*NamespaceModel] {
	return MakeBuildFactory(func(bi BuildInitializer) (NamespaceModel, error) {
		extensionModel, err := buildExtensionModel(bi, source,
			strings.TrimSuffix(source.Basename, NAMESPACEMODEL_EXT),
			namespace)
		return NamespaceModel{
			ExtensionModel: extensionModel,
		}, err
	})
}

func (x *NamespaceModel) GetNamespaceAlias() NamespaceAlias {
	return NamespaceAlias{
		NamespaceName: x.GetAbsoluteName(),
	}
}
func (x *NamespaceModel) Build(bc BuildContext) error {
	*x = NamespaceModel{
		ExtensionModel: ExtensionModel{
			Name:      x.Name,
			Source:    x.Source,
			Namespace: x.Namespace,
		},
	}

	if err := UFS.OpenBuffered(x.Source, func(r io.Reader) error {
		return JsonDeserialize(x, r)
	}); err != nil {
		return err
	}

	rules := &NamespaceRules{
		NamespaceAlias:   NamespaceAlias{x.GetAbsoluteName()},
		NamespaceDir:     x.Source.Dirname,
		NamespaceModules: ModuleAliases{},
		Facet:            x.Facet,
	}

	if namespace, err := x.GetNamespaceModel(); err == nil && namespace != nil {
		rules.NamespaceParent = namespace.GetNamespaceAlias()
		x.applyModelExtensions(&namespace.ExtensionModel)
	} else if err != nil {
		return err
	}

	if !x.hasAllowedPlatforms(rules.NamespaceAlias) {
		return nil
	}

	absoluteName := x.GetAbsoluteName()
	if x.Source == CommandEnv.RootFile() {
		absoluteName = ""
	}

	for _, it := range x.Children {
		filename := rules.NamespaceDir.Folder(it).File(it + NAMESPACEMODEL_EXT)
		if namespace, err := BuildNamespaceModel(filename, absoluteName).Output(bc); err == nil {
			rules.NamespaceChildren.Append(namespace.GetNamespaceAlias())
		} else {
			return err
		}
	}

	for _, it := range x.Modules {
		filename := rules.NamespaceDir.Folder(it).File(it + MODULEMODEL_EXT)
		if module, err := BuildModuleModel(filename, absoluteName).Output(bc); err == nil {
			rules.NamespaceModules.Append(module.GetModuleAlias())
		} else {
			return err
		}
	}

	_, err := bc.OutputFactory(WrapBuildFactory(func(bi BuildInitializer) (*NamespaceRules, error) {
		return rules, nil
	}), OptionBuildForce)
	return err
}
func (x *NamespaceModel) Serialize(ar Archive) {
	ar.Serializable(&x.Children)
	ar.Serializable(&x.Modules)
	ar.Serializable(&x.ExtensionModel)
}
func (x *NamespaceModel) Append(o *NamespaceModel) {
	x.Children.Append(o.Children...)
	x.Modules.Append(o.Modules...)
	x.ExtensionModel.Append(&o.ExtensionModel)
}
func (x *NamespaceModel) Prepend(o *NamespaceModel) {
	x.Children.Prepend(o.Children...)
	x.Modules.Prepend(o.Modules...)
	x.ExtensionModel.Prepend(&o.ExtensionModel)
}

/***************************************
 * Module Model
 ***************************************/

const MODULEMODEL_EXT = "-module.json"

type ModuleModel struct {
	ModuleType ModuleType

	SourceDirs    StringSet
	SourceGlobs   StringSet
	ExcludedGlobs StringSet
	SourceFiles   StringSet
	ExcludedFiles StringSet
	ForceIncludes StringSet
	IsolatedFiles StringSet
	ExtraFiles    StringSet
	ExtraDirs     StringSet

	PrecompiledHeader StringVar
	PrecompiledSource StringVar

	PrivateDependencies ModuleAliases
	PublicDependencies  ModuleAliases
	RuntimeDependencies ModuleAliases

	CppRules
	ExtensionModel
}

func BuildModuleModel(source Filename, namespace string) BuildFactoryTyped[*ModuleModel] {
	return MakeBuildFactory(func(bi BuildInitializer) (ModuleModel, error) {
		extensionModel, err := buildExtensionModel(bi, source,
			strings.TrimSuffix(source.Basename, MODULEMODEL_EXT),
			namespace)
		return ModuleModel{
			ExtensionModel: extensionModel,
		}, err
	})
}

func (x *ModuleModel) GetModuleAlias() ModuleAlias {
	return ModuleAlias{
		NamespaceAlias: NamespaceAlias{x.Namespace},
		ModuleName:     x.Name,
	}
}
func (x *ModuleModel) Build(bc BuildContext) error {
	*x = ModuleModel{
		ExtensionModel: ExtensionModel{
			Name:      x.Name,
			Source:    x.Source,
			Namespace: x.Namespace,
		},
	}

	if err := UFS.OpenBuffered(x.Source, func(r io.Reader) error {
		return JsonDeserialize(x, r)
	}); err != nil {
		return err
	}

	namespace, err := x.GetNamespaceModel()
	if err != nil {
		return err
	}
	x.applyModelExtensions(&namespace.ExtensionModel)

	moduleAlias := x.GetModuleAlias()
	moduleDir := x.Source.Dirname

	if !x.hasAllowedPlatforms(moduleAlias) {
		return nil
	}

	rules, err := x.createModuleRules(moduleAlias)
	if err != nil {
		return err
	}

	x.applyArchetypes(&rules, moduleAlias)

	rules.ForceIncludes.Append(x.ForceIncludes.ToFileSet(moduleDir)...)
	rules.Source.ExtraFiles.AppendUniq(x.Source)

	if !x.PrecompiledHeader.IsInheritable() {
		f := moduleDir.AbsoluteFile(x.PrecompiledHeader.Get()).Normalize()
		rules.PrecompiledHeader = &f
	} else if f := moduleDir.File(PCH_DEFAULT_HEADER); f.Exists() {
		rules.PrecompiledHeader = &f
	}
	if !x.PrecompiledSource.IsInheritable() {
		f := moduleDir.AbsoluteFile(x.PrecompiledSource.Get()).Normalize()
		rules.PrecompiledSource = &f
	} else if f := moduleDir.File(PCH_DEFAULT_SOURCE); f.Exists() {
		rules.PrecompiledSource = &f
	}

	_, err = bc.OutputFactory(WrapBuildFactory(func(bi BuildInitializer) (*ModuleRules, error) {
		dependencyAliases := make([]BuildAlias, 0, len(x.PrivateDependencies)+len(x.PublicDependencies)+len(x.RuntimeDependencies))
		for _, moduleAlias := range x.PrivateDependencies {
			dependencyAliases = append(dependencyAliases, MakeBuildAlias("Model", moduleAlias.NamespaceName, moduleAlias.ModuleName))
		}
		for _, moduleAlias := range x.PublicDependencies {
			dependencyAliases = append(dependencyAliases, MakeBuildAlias("Model", moduleAlias.NamespaceName, moduleAlias.ModuleName))
		}
		for _, moduleAlias := range x.RuntimeDependencies {
			dependencyAliases = append(dependencyAliases, MakeBuildAlias("Model", moduleAlias.NamespaceName, moduleAlias.ModuleName))
		}
		return &rules, bi.DependsOn(dependencyAliases...)
	}), OptionBuildForce)
	return err
}
func (x *ModuleModel) createModuleRules(moduleAlias ModuleAlias) (ModuleRules, error) {
	x.applyHAL(x, moduleAlias)

	moduleDir := x.Source.Dirname
	rules := ModuleRules{
		ModuleAlias: moduleAlias,
		ModuleDir:   moduleDir,
		ModuleType:  x.ModuleType,
		CppRules:    x.CppRules,
		Source: ModuleSource{
			SourceGlobs:   x.SourceGlobs,
			ExcludedGlobs: x.ExcludedGlobs,
			SourceDirs:    x.SourceDirs.ToDirSet(moduleDir).Normalize(),
			SourceFiles:   x.SourceFiles.ToFileSet(moduleDir).Normalize(),
			ExcludedFiles: x.ExcludedFiles.ToFileSet(moduleDir).Normalize(),
			IsolatedFiles: x.IsolatedFiles.ToFileSet(moduleDir).Normalize(),
			ExtraFiles:    x.ExtraFiles.ToFileSet(moduleDir).Normalize(),
			ExtraDirs:     x.ExtraDirs.ToDirSet(moduleDir).Normalize(),
		},
		PrivateDependencies: x.PrivateDependencies,
		PublicDependencies:  x.PublicDependencies,
		RuntimeDependencies: x.RuntimeDependencies,
		Facet:               x.Facet,
		PerTags:             map[TagFlags]ModuleRules{},
	}

	for tags, model := range x.TAG {
		if model.hasAllowedPlatforms(moduleAlias) {
			var err error
			rules.PerTags[tags], err = model.createModuleRules(moduleAlias)
			if err != nil {
				return ModuleRules{}, err
			}
		}
	}

	return rules, nil
}
func (x *ModuleModel) Serialize(ar Archive) {
	ar.Serializable(&x.ModuleType)

	ar.Serializable(&x.SourceDirs)
	ar.Serializable(&x.SourceGlobs)
	ar.Serializable(&x.ExcludedGlobs)
	ar.Serializable(&x.SourceFiles)
	ar.Serializable(&x.ExcludedFiles)
	ar.Serializable(&x.ForceIncludes)
	ar.Serializable(&x.IsolatedFiles)
	ar.Serializable(&x.ExtraFiles)
	ar.Serializable(&x.ExtraDirs)

	ar.Serializable(&x.PrecompiledHeader)
	ar.Serializable(&x.PrecompiledSource)

	SerializeSlice(ar, x.PrivateDependencies.Ref())
	SerializeSlice(ar, x.PublicDependencies.Ref())
	SerializeSlice(ar, x.RuntimeDependencies.Ref())

	ar.Serializable(&x.CppRules)
	ar.Serializable(&x.ExtensionModel)
}
func (x *ModuleModel) Append(o *ModuleModel) {
	Inherit(&x.ModuleType, o.ModuleType)

	x.SourceDirs.Append(o.SourceDirs...)
	x.SourceGlobs.Append(o.SourceGlobs...)
	x.ExcludedGlobs.Append(o.ExcludedGlobs...)
	x.SourceFiles.Append(o.SourceFiles...)
	x.ExcludedFiles.Append(o.ExcludedFiles...)
	x.ForceIncludes.Append(o.ForceIncludes...)
	x.IsolatedFiles.Append(o.IsolatedFiles...)
	x.ExtraFiles.Append(o.ExtraFiles...)
	x.ExtraDirs.Append(o.ExtraDirs...)

	x.PrecompiledHeader.Inherit(o.PrecompiledHeader)
	x.PrecompiledSource.Inherit(o.PrecompiledSource)

	x.PrivateDependencies.Append(o.PrivateDependencies...)
	x.PublicDependencies.Append(o.PublicDependencies...)
	x.RuntimeDependencies.Append(o.RuntimeDependencies...)

	x.CppRules.Inherit(&o.CppRules)
	x.ExtensionModel.Append(&o.ExtensionModel)
}
func (x *ModuleModel) Prepend(o *ModuleModel) {
	Overwrite(&x.ModuleType, o.ModuleType)

	x.SourceDirs.Prepend(o.SourceDirs...)
	x.SourceGlobs.Prepend(o.SourceGlobs...)
	x.ExcludedGlobs.Prepend(o.ExcludedGlobs...)
	x.SourceFiles.Prepend(o.SourceFiles...)
	x.ExcludedFiles.Prepend(o.ExcludedFiles...)
	x.ForceIncludes.Prepend(o.ForceIncludes...)
	x.IsolatedFiles.Prepend(o.IsolatedFiles...)
	x.ExtraFiles.Prepend(o.ExtraFiles...)
	x.ExtraDirs.Prepend(o.ExtraDirs...)

	x.PrecompiledHeader.Overwrite(o.PrecompiledHeader)
	x.PrecompiledSource.Overwrite(o.PrecompiledSource)

	x.PrivateDependencies.Prepend(o.PrivateDependencies...)
	x.PublicDependencies.Prepend(o.PublicDependencies...)
	x.RuntimeDependencies.Prepend(o.RuntimeDependencies...)

	x.CppRules.Inherit(&o.CppRules)
	x.ExtensionModel.Prepend(&o.ExtensionModel)
}

/***************************************
 * Extension Model
 ***************************************/

type ExtensionModel struct {
	// following fields are deduced and not serialized
	Name      string   `json:"-"`
	Source    Filename `json:"-"`
	Namespace string   `json:"-"`

	Archetypes      StringSet
	AllowedPlaforms SetT[PlatformAlias]
	HAL             map[HostId]ModuleModel
	TAG             map[TagFlags]ModuleModel

	Facet
}

func buildExtensionModel(bi BuildInitializer, source Filename, name string, namespace string) (ExtensionModel, error) {
	source = source.Normalize()

	if err := bi.NeedFile(source); err != nil {
		return ExtensionModel{}, err
	}

	return ExtensionModel{
		Name:      name,
		Source:    source,
		Namespace: namespace,
	}, nil
}

func (x *ExtensionModel) Alias() BuildAlias {
	return MakeBuildAlias("Model", x.Namespace, x.Name)
}
func (x *ExtensionModel) GetAbsoluteName() string {
	if len(x.Namespace) > 0 {
		return path.Join(x.Namespace, x.Name)
	} else {
		return x.Name
	}
}
func (x *ExtensionModel) GetNamespaceModel() (*NamespaceModel, error) {
	if len(x.Namespace) > 0 {
		return FindGlobalBuildable[*NamespaceModel](MakeBuildAlias("Model", x.Namespace))
	} else {
		return nil, nil
	}
}
func (x *ExtensionModel) Append(o *ExtensionModel) {
	x.Archetypes.AppendUniq(o.Archetypes...)
	x.AllowedPlaforms.AppendUniq(o.AllowedPlaforms...)

	for k, v := range o.HAL {
		if w, ok := x.HAL[k]; ok {
			w.Append(&v)
			x.HAL[k] = w
		} else {
			x.HAL[k] = v
		}
	}
	for k, v := range o.TAG {
		if w, ok := x.TAG[k]; ok {
			w.Append(&v)
			x.TAG[k] = w
		} else {
			x.TAG[k] = v
		}
	}

	x.Facet.Append(&o.Facet)
}
func (x *ExtensionModel) Prepend(o *ExtensionModel) {
	x.Archetypes.PrependUniq(o.Archetypes...)
	x.AllowedPlaforms.PrependUniq(o.AllowedPlaforms...)

	for k, v := range o.HAL {
		if w, ok := x.HAL[k]; ok {
			w.Prepend(&v)
			x.HAL[k] = w
		} else {
			x.HAL[k] = v
		}
	}
	for k, v := range o.TAG {
		if w, ok := x.TAG[k]; ok {
			w.Prepend(&v)
			x.TAG[k] = w
		} else {
			x.TAG[k] = v
		}
	}

	x.Facet.Prepend(&o.Facet)
}
func (x *ExtensionModel) Serialize(ar Archive) {
	ar.String(&x.Name)
	ar.Serializable(&x.Source)
	ar.String(&x.Namespace)
	ar.Serializable(&x.Archetypes)
	SerializeSlice(ar, x.AllowedPlaforms.Ref())
	SerializeMap(ar, &x.HAL)
	SerializeMap(ar, &x.TAG)
	ar.Serializable(&x.Facet)
}
func (x *ExtensionModel) DeepCopy(src *ExtensionModel) {
	x.Name = src.Name
	x.Source = src.Source
	x.Namespace = src.Namespace
	x.Archetypes = NewStringSet(src.Archetypes...)
	x.AllowedPlaforms = NewSet(src.AllowedPlaforms...)
	x.HAL = CopyMap(src.HAL)
	x.TAG = CopyMap(src.TAG)
	x.Facet.DeepCopy(&src.Facet)
}

func (src *ExtensionModel) hasAllowedPlatforms(name fmt.Stringer) bool {
	if len(src.AllowedPlaforms) > 0 {
		localPlatform := GetLocalHostPlatformAlias()
		if src.AllowedPlaforms.Contains(localPlatform) {
			LogTrace(LogModel, "%v: not allowed on <%v> platform", name, localPlatform)
			return false
		}
	}
	return true
}
func (src *ExtensionModel) applyArchetypes(rules *ModuleRules, name ModuleAlias) {
	src.Archetypes.Range(func(id string) {
		id = strings.ToUpper(id)
		if decorator, ok := AllArchetypes.Get(id); ok {
			LogTrace(LogModel, "%v: inherit module archtype <%v>", name, id)
			decorator(rules)
		} else {
			LogFatal("%v: invalid module archtype <%v>", name, id)
		}
	})
}
func (src *ExtensionModel) applyHAL(model *ModuleModel, name ModuleAlias) {
	hostId := CurrentHost().Id
	for id, other := range src.HAL {
		var hal HostId
		if err := hal.Set(id.String()); err == nil && hal == hostId {
			LogTrace(LogModel, "%v: inherit platform facet [%v]", name, id)
			model.Prepend(&other)
		} else if err != nil {
			LogError(LogModel, "%v: invalid platform id [%v], %v", name, id, err)
		}
	}
}
func (model *ExtensionModel) applyModelExtensions(other *ExtensionModel) {
	model.Archetypes.PrependUniq(other.Archetypes...)

	for key, src := range other.HAL {
		if dst, ok := model.HAL[key]; ok {
			dst.Append(&src)
		} else {
			model.HAL[key] = src
		}
	}

	for key, src := range other.TAG {
		if dst, ok := model.TAG[key]; ok {
			dst.Append(&src)
		} else {
			model.TAG[key] = src
		}
	}
}
