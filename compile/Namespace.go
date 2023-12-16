package compile

import (
	"fmt"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

type Namespace interface {
	GetNamespace() *NamespaceRules
	utils.Buildable
	base.Serializable
	fmt.Stringer
}

/***************************************
 * Namespace Alias
 ***************************************/

type NamespaceAlias struct {
	NamespaceName string
}

type NamespaceAliases = base.SetT[NamespaceAlias]

func (x NamespaceAlias) Valid() bool {
	return len(x.NamespaceName) > 0
}
func (x NamespaceAlias) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("Rules", "Namespace", x.String())
}
func (x *NamespaceAlias) Serialize(ar base.Archive) {
	ar.String(&x.NamespaceName)
}
func (x NamespaceAlias) String() string {
	return x.NamespaceName
}
func (x NamespaceAlias) Compare(o NamespaceAlias) int {
	return strings.Compare(x.NamespaceName, o.NamespaceName)
}
func (x *NamespaceAlias) Set(in string) (err error) {
	x.NamespaceName = in
	return nil
}
func (x NamespaceAlias) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *NamespaceAlias) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *NamespaceAlias) AutoComplete(in base.AutoComplete) {
	namespaces, err := NeedAllBuildNamespaceAliases(utils.CommandEnv.BuildGraph().GlobalContext())
	if err == nil {
		for _, it := range namespaces {
			in.Add(it.String(), it.Alias().String())
		}
	} else {
		utils.CommandPanic(err)
	}
}

/***************************************
 * Namespace Rules
 ***************************************/

type NamespaceRules struct {
	NamespaceAlias NamespaceAlias

	NamespaceParent   NamespaceAlias
	NamespaceChildren NamespaceAliases
	NamespaceDir      utils.Directory
	NamespaceModules  ModuleAliases

	Facet
}

func (rules *NamespaceRules) String() string {
	return rules.NamespaceAlias.String()
}

func (rules *NamespaceRules) GetFacet() *Facet {
	return rules.Facet.GetFacet()
}
func (rules *NamespaceRules) GetNamespace() *NamespaceRules {
	return rules
}
func (rules *NamespaceRules) GetParentNamespace() Namespace {
	if namespace, err := FindBuildNamespace(rules.NamespaceParent); err == nil {
		return namespace
	} else {
		base.LogPanicErr(LogCompile, err)
		return nil
	}
}
func (rules *NamespaceRules) Decorate(env *CompileEnv, unit *Unit) error {
	if rules.NamespaceParent.Valid() {
		parent := rules.GetParentNamespace()
		if err := parent.GetNamespace().Decorate(env, unit); err != nil {
			return err
		}
	}

	unit.Facet.Append(rules)
	return nil
}

func (rules *NamespaceRules) Serialize(ar base.Archive) {
	ar.Serializable(&rules.NamespaceAlias)

	ar.Serializable(&rules.NamespaceParent)
	base.SerializeSlice(ar, rules.NamespaceChildren.Ref())

	ar.Serializable(&rules.NamespaceDir)
	base.SerializeSlice(ar, rules.NamespaceModules.Ref())

	ar.Serializable(&rules.Facet)
}

/***************************************
 * Build Namespace
 ***************************************/

func (x *NamespaceRules) Alias() utils.BuildAlias {
	return x.GetNamespace().NamespaceAlias.Alias()
}
func (x *NamespaceRules) Build(bc utils.BuildContext) error {
	return nil
}

func FindBuildNamespace(namespace NamespaceAlias) (Namespace, error) {
	return utils.FindGlobalBuildable[Namespace](namespace.Alias())
}

func NeedAllBuildNamespaceAliases(bc utils.BuildContext) (namespaceAliases []NamespaceAlias, err error) {
	rootModel, err := BuildRootNamespaceModel().Need(bc)
	if err != nil {
		return []NamespaceAlias{}, err
	}

	err = ForeachNamespaceChildrenAlias(bc, rootModel.GetNamespaceAlias(), func(na NamespaceAlias) error {
		namespaceAliases = append(namespaceAliases, na)
		return nil
	})
	return
}

func ForeachNamespaceChildrenAlias(bc utils.BuildContext, namespaceAlias NamespaceAlias, each func(NamespaceAlias) error) error {
	namespace, err := utils.FindGlobalBuildable[Namespace](namespaceAlias.Alias())
	if err != nil {
		return err
	}

	if err := bc.NeedBuildables(namespace); err != nil {
		return err
	}

	if err := each(namespaceAlias); err != nil {
		return err
	}

	for _, namespaceAlias := range namespace.GetNamespace().NamespaceChildren {
		if err := ForeachNamespaceChildrenAlias(bc, namespaceAlias, each); err != nil {
			return err
		}
	}

	return nil
}
