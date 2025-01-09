package compile

import (
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

var LogModel = base.NewLogCategory("Model")

const PCH_DEFAULT_HEADER = "stdafx.h"
const PCH_DEFAULT_SOURCE = "stdafx.cpp"

func BuildRootNamespaceModel() utils.BuildFactoryTyped[*NamespaceModel] {
	return buildNamespaceModelEx(utils.CommandEnv.RootFile(), "", true)
}

func GetRootNamespaceName() string {
	return strings.TrimSuffix(utils.CommandEnv.RootFile().Basename, NAMESPACEMODEL_EXT)
}

/***************************************
 * Namespace Model
 ***************************************/

const NAMESPACEMODEL_EXT = "-namespace.json"

type NamespaceModel struct {
	Children base.StringSet
	Modules  base.StringSet

	RootNamespace bool

	ExtensionModel
}

func BuildNamespaceModel(source utils.Filename, namespace string) utils.BuildFactoryTyped[*NamespaceModel] {
	return buildNamespaceModelEx(source, namespace, false)
}
func buildNamespaceModelEx(source utils.Filename, namespace string, rootNamespace bool) utils.BuildFactoryTyped[*NamespaceModel] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (NamespaceModel, error) {
		extensionModel, err := buildExtensionModel(bi, source, namespace, NAMESPACEMODEL_EXT)
		return NamespaceModel{
			ExtensionModel: extensionModel,
			RootNamespace:  rootNamespace,
		}, err
	})
}

func (x *NamespaceModel) GetNamespaceAlias() NamespaceAlias {
	return NamespaceAlias{
		NamespaceName: x.GetAbsoluteName(),
	}
}
func (x *NamespaceModel) Build(bc utils.BuildContext) error {
	*x = NamespaceModel{
		ExtensionModel: ExtensionModel{
			Name:      x.Name,
			Source:    x.Source,
			Namespace: x.Namespace,
		},
		RootNamespace: x.RootNamespace,
	}

	if err := utils.UFS.OpenBuffered(x.Source, func(r io.Reader) error {
		return base.JsonDeserialize(x, r)
	}); err != nil {
		return err
	}

	rules := &NamespaceRules{
		NamespaceAlias:   NamespaceAlias{x.GetAbsoluteName()},
		NamespaceDir:     x.Source.Dirname,
		NamespaceModules: ModuleAliases{},
		Facet:            x.Facet,
	}

	if namespace, err := x.GetNamespaceModel(bc); err == nil && namespace != nil {
		rules.NamespaceParent = namespace.GetNamespaceAlias()
		x.applyModelExtensions(&namespace.ExtensionModel)
	} else if err != nil {
		return err
	}

	if !x.hasAllowedPlatforms(rules.NamespaceAlias) {
		return nil
	}

	absoluteName := x.GetAbsoluteName()
	if x.RootNamespace {
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

	_, err := bc.OutputFactory(utils.WrapBuildFactory(func(bi utils.BuildInitializer) (*NamespaceRules, error) {
		return rules, nil
	}), utils.OptionBuildForce)
	return err
}
func (x *NamespaceModel) Serialize(ar base.Archive) {
	ar.Serializable(&x.Children)
	ar.Serializable(&x.Modules)
	ar.Bool(&x.RootNamespace)
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

	SourceDirs    base.StringSet
	SourceGlobs   base.StringSet
	ExcludedGlobs base.StringSet
	SourceFiles   base.StringSet
	ExcludedFiles base.StringSet
	ForceIncludes base.StringSet
	IsolatedFiles base.StringSet
	ExtraFiles    base.StringSet
	ExtraDirs     base.StringSet

	PrecompiledHeader utils.StringVar
	PrecompiledSource utils.StringVar

	PrivateDependencies ModuleAliases
	PublicDependencies  ModuleAliases
	RuntimeDependencies ModuleAliases

	CppRules
	ExtensionModel
}

func BuildModuleModel(source utils.Filename, namespace string) utils.BuildFactoryTyped[*ModuleModel] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (ModuleModel, error) {
		extensionModel, err := buildExtensionModel(bi, source, namespace, MODULEMODEL_EXT)
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
func (x *ModuleModel) Build(bc utils.BuildContext) error {
	*x = ModuleModel{
		ExtensionModel: ExtensionModel{
			Name:      x.Name,
			Source:    x.Source,
			Namespace: x.Namespace,
		},
	}

	if err := utils.UFS.OpenBuffered(x.Source, func(r io.Reader) error {
		return base.JsonDeserialize(x, r)
	}); err != nil {
		return err
	}

	namespace, err := x.GetNamespaceModel(bc)
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

	if err := x.applyArchetypes(&rules, moduleAlias); err != nil {
		return err
	}

	rules.ForceIncludes.Append(utils.MakeFileSet(moduleDir, x.ForceIncludes...)...)
	rules.Source.ExtraFiles.AppendUniq(x.Source)

	if !x.PrecompiledHeader.IsInheritable() {
		f := moduleDir.AbsoluteFile(x.PrecompiledHeader.Get()).Normalize()
		rules.PrecompiledHeader = f
	} else if f := moduleDir.File(PCH_DEFAULT_HEADER); f.Exists() {
		rules.PrecompiledHeader = f
	}
	if !x.PrecompiledSource.IsInheritable() {
		f := moduleDir.AbsoluteFile(x.PrecompiledSource.Get()).Normalize()
		rules.PrecompiledSource = f
	} else if f := moduleDir.File(PCH_DEFAULT_SOURCE); f.Exists() {
		rules.PrecompiledSource = f
	}

	_, err = bc.OutputFactory(utils.WrapBuildFactory(func(bi utils.BuildInitializer) (*ModuleRules, error) {
		dependencyAliases := make(utils.BuildAliases, 0, len(x.PrivateDependencies)+len(x.PublicDependencies)+len(x.RuntimeDependencies))

		for _, moduleAlias := range x.PrivateDependencies {
			dependencyAliases.Append(utils.MakeBuildAlias("Model", moduleAlias.NamespaceName, moduleAlias.ModuleName))
		}

		for _, moduleAlias := range x.PublicDependencies {
			dependencyAliases.Append(utils.MakeBuildAlias("Model", moduleAlias.NamespaceName, moduleAlias.ModuleName))
		}

		for _, moduleAlias := range x.RuntimeDependencies {
			dependencyAliases.Append(utils.MakeBuildAlias("Model", moduleAlias.NamespaceName, moduleAlias.ModuleName))
		}

		return &rules, bi.DependsOn(dependencyAliases...)
	}), utils.OptionBuildForce)
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
			SourceDirs:    utils.MakeDirSet(moduleDir, x.SourceDirs...).Normalize(),
			SourceFiles:   utils.MakeFileSet(moduleDir, x.SourceFiles...).Normalize(),
			ExcludedFiles: utils.MakeFileSet(moduleDir, x.ExcludedFiles...).Normalize(),
			IsolatedFiles: utils.MakeFileSet(moduleDir, x.IsolatedFiles...).Normalize(),
			ExtraFiles:    utils.MakeFileSet(moduleDir, x.ExtraFiles...).Normalize(),
			ExtraDirs:     utils.MakeDirSet(moduleDir, x.ExtraDirs...).Normalize(),
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
func (x *ModuleModel) Serialize(ar base.Archive) {
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

	base.SerializeSlice(ar, x.PrivateDependencies.Ref())
	base.SerializeSlice(ar, x.PublicDependencies.Ref())
	base.SerializeSlice(ar, x.RuntimeDependencies.Ref())

	ar.Serializable(&x.CppRules)
	ar.Serializable(&x.ExtensionModel)
}
func (x *ModuleModel) Append(o *ModuleModel) {
	base.Inherit(&x.ModuleType, o.ModuleType)

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
	base.Overwrite(&x.ModuleType, o.ModuleType)

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
	Name      string         `json:"-"`
	Source    utils.Filename `json:"-"`
	Namespace string         `json:"-"`

	Archetypes       base.StringSet
	AllowedPlatforms base.SetT[PlatformAlias]
	HAL              map[base.HostId]ModuleModel
	TAG              map[TagFlags]ModuleModel

	Facet
}

func buildExtensionModel(bi utils.BuildInitializer, source utils.Filename, namespace string, extname string) (ExtensionModel, error) {
	source = source.Normalize()
	name := strings.TrimSuffix(source.Basename, extname)

	if err := bi.NeedFiles(source); err != nil {
		return ExtensionModel{}, err
	}

	return ExtensionModel{
		Name:      name,
		Source:    source,
		Namespace: namespace,
	}, nil
}

func (x *ExtensionModel) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("Model", x.Namespace, x.Name)
}
func (x *ExtensionModel) GetAbsoluteName() string {
	if len(x.Namespace) > 0 {
		return path.Join(x.Namespace, x.Name)
	} else {
		return x.Name
	}
}
func (x *ExtensionModel) GetNamespaceModel(bg utils.BuildGraphReadPort) (*NamespaceModel, error) {
	if len(x.Namespace) > 0 {
		return utils.FindBuildable[*NamespaceModel](bg, utils.MakeBuildAlias("Model", x.Namespace))
	} else {
		return nil, nil
	}
}
func (x *ExtensionModel) Append(o *ExtensionModel) {
	x.Archetypes.AppendUniq(o.Archetypes...)
	x.AllowedPlatforms.AppendUniq(o.AllowedPlatforms...)

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
	x.AllowedPlatforms.PrependUniq(o.AllowedPlatforms...)

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
func (x *ExtensionModel) Serialize(ar base.Archive) {
	ar.String(&x.Name)
	ar.Serializable(&x.Source)
	ar.String(&x.Namespace)
	ar.Serializable(&x.Archetypes)
	base.SerializeSlice(ar, x.AllowedPlatforms.Ref())
	base.SerializeMap(ar, &x.HAL)
	base.SerializeMap(ar, &x.TAG)
	ar.Serializable(&x.Facet)
}
func (x *ExtensionModel) DeepCopy(src *ExtensionModel) {
	x.Name = src.Name
	x.Source = src.Source
	x.Namespace = src.Namespace
	x.Archetypes = base.NewStringSet(src.Archetypes...)
	x.AllowedPlatforms = base.NewSet(src.AllowedPlatforms.Slice()...)
	x.HAL = base.CopyMap(src.HAL)
	x.TAG = base.CopyMap(src.TAG)
	x.Facet.DeepCopy(&src.Facet)
}

func (src *ExtensionModel) hasAllowedPlatforms(name fmt.Stringer) bool {
	if len(src.AllowedPlatforms) > 0 {
		localPlatform := GetLocalHostPlatformAlias()
		if !src.AllowedPlatforms.Contains(localPlatform) {
			base.LogTrace(LogModel, "%v: not allowed on <%v> platform", name, localPlatform)
			return false
		}
	}
	return true
}
func (src *ExtensionModel) applyArchetypes(rules *ModuleRules, name ModuleAlias) error {
	return src.Archetypes.Range(func(id string) error {
		id = strings.ToUpper(id)
		if decorator, ok := AllArchetypes.Get(id); ok {
			base.LogTrace(LogModel, "%v: inherit module archtype <%v>", name, id)
			if err := decorator(rules); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("%v: invalid module archtype <%v>", name, id)
		}
		return nil
	})
}
func (src *ExtensionModel) applyHAL(model *ModuleModel, name ModuleAlias) {
	hostId := base.GetCurrentHost().Id
	for id, other := range src.HAL {
		var hal base.HostId
		if err := hal.Set(id.String()); err == nil && hal == hostId {
			base.LogTrace(LogModel, "%v: inherit platform facet [%v]", name, id)
			model.Prepend(&other)
		} else if err != nil {
			base.LogError(LogModel, "%v: invalid platform id [%v], %v", name, id, err)
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
