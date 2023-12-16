package compile

import (
	"fmt"
	"strings"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

type Namespace interface {
	GetNamespace() *NamespaceRules
	Buildable
	Serializable
	fmt.Stringer
}

/***************************************
 * Namespace Alias
 ***************************************/

type NamespaceAlias struct {
	NamespaceName string
}

type NamespaceAliases = SetT[NamespaceAlias]

func (x NamespaceAlias) Valid() bool {
	return len(x.NamespaceName) > 0
}
func (x NamespaceAlias) Alias() BuildAlias {
	return MakeBuildAlias("Rules", "Namespace", x.String())
}
func (x *NamespaceAlias) Serialize(ar Archive) {
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
	return UnsafeBytesFromString(x.String()), nil
}
func (x *NamespaceAlias) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}

/***************************************
 * Namespace Rules
 ***************************************/

type NamespaceRules struct {
	NamespaceAlias NamespaceAlias

	NamespaceParent   NamespaceAlias
	NamespaceChildren NamespaceAliases
	NamespaceDir      Directory
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
		LogPanicErr(LogCompile, err)
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

func (rules *NamespaceRules) Serialize(ar Archive) {
	ar.Serializable(&rules.NamespaceAlias)

	ar.Serializable(&rules.NamespaceParent)
	SerializeSlice(ar, rules.NamespaceChildren.Ref())

	ar.Serializable(&rules.NamespaceDir)
	SerializeSlice(ar, rules.NamespaceModules.Ref())

	ar.Serializable(&rules.Facet)
}

/***************************************
 * Build Namespace
 ***************************************/

func (x *NamespaceRules) Alias() BuildAlias {
	return x.GetNamespace().NamespaceAlias.Alias()
}
func (x *NamespaceRules) Build(bc BuildContext) error {
	return nil
}

func FindBuildNamespace(namespace NamespaceAlias) (Namespace, error) {
	return FindGlobalBuildable[Namespace](namespace.Alias())
}
