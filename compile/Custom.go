package compile

import (
	"fmt"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

type CustomRules struct {
	CustomName string

	CompilerAlias CompilerAlias
	Source        ModuleSource
	Facet
}

type Custom interface {
	GetCustom() *CustomRules
	base.Serializable
	fmt.Stringer
}

func (rules *CustomRules) String() string {
	return rules.CustomName
}
func (rules *CustomRules) GetConfig() *CustomRules {
	return rules
}
func (rules *CustomRules) GetCompiler(bg utils.BuildGraphReadPort) Compiler {
	compiler, err := utils.FindBuildable[Compiler](bg, rules.CompilerAlias.Alias())
	base.LogPanicIfFailed(LogCompile, err)
	return compiler
}
func (rules *CustomRules) GetFacet() *Facet {
	return rules.Facet.GetFacet()
}
func (rules *CustomRules) Serialize(ar base.Archive) {
	ar.String(&rules.CustomName)

	ar.Serializable(&rules.CompilerAlias)
	ar.Serializable(&rules.Source)
	ar.Serializable(&rules.Facet)
}

type CustomList []Custom

func (list *CustomList) Append(it ...Custom) {
	*list = append(*list, it...)
}
func (list *CustomList) Prepend(it ...Custom) {
	*list = append(it, *list...)
}
func (list *CustomList) Serialize(ar base.Archive) {
	base.SerializeMany(ar, func(it *Custom) {
		base.SerializeExternal(ar, it)
	}, (*[]Custom)(list))
}

type CustomUnit struct {
	Unit
}

type CustomUnitList []CustomUnit

func (list *CustomUnitList) Append(it ...CustomUnit) {
	*list = append(*list, it...)
}
func (list *CustomUnitList) Serialize(ar base.Archive) {
	base.SerializeSlice(ar, (*[]CustomUnit)(list))
}
