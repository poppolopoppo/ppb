package compile

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

type Facetable interface {
	GetFacet() *Facet
}

type FacetDecorator interface {
	Decorate(*CompileEnv, *Unit) error
}

type VariableDefinition struct {
	Name, Value string
}
type VariableDefinitions []VariableDefinition

/***************************************
 * Facet
 ***************************************/

type Facet struct {
	Defines base.StringSet

	ForceIncludes      utils.FileSet
	IncludePaths       utils.DirSet
	ExternIncludePaths utils.DirSet
	SystemIncludePaths utils.DirSet

	AnalysisOptions          base.StringSet
	PreprocessorOptions      base.StringSet
	CompilerOptions          base.StringSet
	PrecompiledHeaderOptions base.StringSet

	Libraries    utils.FileSet
	LibraryPaths utils.DirSet

	LibrarianOptions base.StringSet
	LinkerOptions    base.StringSet

	Tags    TagFlags
	Exports VariableDefinitions
}

func (facet *Facet) GetFacet() *Facet {
	return facet
}
func (facet *Facet) Serialize(ar base.Archive) {
	ar.Serializable(&facet.Defines)

	ar.Serializable(&facet.ForceIncludes)
	ar.Serializable(&facet.IncludePaths)
	ar.Serializable(&facet.ExternIncludePaths)
	ar.Serializable(&facet.SystemIncludePaths)

	ar.Serializable(&facet.AnalysisOptions)
	ar.Serializable(&facet.PreprocessorOptions)
	ar.Serializable(&facet.CompilerOptions)
	ar.Serializable(&facet.PrecompiledHeaderOptions)

	ar.Serializable(&facet.Libraries)
	ar.Serializable(&facet.LibraryPaths)

	ar.Serializable(&facet.LibrarianOptions)
	ar.Serializable(&facet.LinkerOptions)

	ar.Serializable(&facet.Tags)
	ar.Serializable(&facet.Exports)
}

func NewFacet() Facet {
	return Facet{
		Defines:                  base.StringSet{},
		ForceIncludes:            utils.FileSet{},
		IncludePaths:             utils.DirSet{},
		ExternIncludePaths:       utils.DirSet{},
		SystemIncludePaths:       utils.DirSet{},
		AnalysisOptions:          base.StringSet{},
		PreprocessorOptions:      base.StringSet{},
		CompilerOptions:          base.StringSet{},
		PrecompiledHeaderOptions: base.StringSet{},
		Libraries:                utils.FileSet{},
		LibraryPaths:             utils.DirSet{},
		LibrarianOptions:         base.StringSet{},
		LinkerOptions:            base.StringSet{},
		Tags:                     TagFlags(0),
		Exports:                  VariableDefinitions{},
	}
}
func (x *Facet) DeepCopy(src *Facet) {
	x.Defines = base.NewStringSet(src.Defines...)
	x.ForceIncludes = utils.NewFileSet(src.ForceIncludes...)
	x.IncludePaths = utils.NewDirSet(src.IncludePaths...)
	x.ExternIncludePaths = utils.NewDirSet(src.ExternIncludePaths...)
	x.SystemIncludePaths = utils.NewDirSet(src.SystemIncludePaths...)
	x.AnalysisOptions = base.NewStringSet(src.AnalysisOptions...)
	x.PreprocessorOptions = base.NewStringSet(src.PreprocessorOptions...)
	x.CompilerOptions = base.NewStringSet(src.CompilerOptions...)
	x.PrecompiledHeaderOptions = base.NewStringSet(src.PrecompiledHeaderOptions...)
	x.Libraries = utils.NewFileSet(src.Libraries...)
	x.LibraryPaths = utils.NewDirSet(src.LibraryPaths...)
	x.LibrarianOptions = base.NewStringSet(src.LibrarianOptions...)
	x.LinkerOptions = base.NewStringSet(src.LinkerOptions...)
	x.Tags = src.Tags
	x.Exports = base.CopySlice(src.Exports...)
}

func (facet *Facet) Tagged(tag TagType) bool {
	return facet.Tags.Has(tag)
}
func (facet *Facet) Append(others ...Facetable) {
	for _, o := range others {
		x := o.GetFacet()
		facet.Defines.Append(x.Defines...)
		facet.ForceIncludes.Append(x.ForceIncludes...)
		facet.IncludePaths.Append(x.IncludePaths...)
		facet.ExternIncludePaths.Append(x.ExternIncludePaths...)
		facet.SystemIncludePaths.Append(x.SystemIncludePaths...)
		facet.AnalysisOptions.Append(x.AnalysisOptions...)
		facet.PreprocessorOptions.Append(x.PreprocessorOptions...)
		facet.CompilerOptions.Append(x.CompilerOptions...)
		facet.PrecompiledHeaderOptions.Append(x.PrecompiledHeaderOptions...)
		facet.Libraries.Append(x.Libraries...)
		facet.LibraryPaths.Append(x.LibraryPaths...)
		facet.LibrarianOptions.Append(x.LibrarianOptions...)
		facet.LinkerOptions.Append(x.LinkerOptions...)
		facet.Tags.Append(x.Tags)
		facet.Exports.Append(x.Exports)
	}
}
func (facet *Facet) AppendUniq(others ...Facetable) {
	for _, o := range others {
		x := o.GetFacet()
		facet.Defines.AppendUniq(x.Defines...)
		facet.ForceIncludes.AppendUniq(x.ForceIncludes...)
		facet.IncludePaths.AppendUniq(x.IncludePaths...)
		facet.ExternIncludePaths.AppendUniq(x.ExternIncludePaths...)
		facet.SystemIncludePaths.AppendUniq(x.SystemIncludePaths...)
		facet.AnalysisOptions.AppendUniq(x.AnalysisOptions...)
		facet.PreprocessorOptions.AppendUniq(x.PreprocessorOptions...)
		facet.CompilerOptions.AppendUniq(x.CompilerOptions...)
		facet.PrecompiledHeaderOptions.AppendUniq(x.PrecompiledHeaderOptions...)
		facet.Libraries.AppendUniq(x.Libraries...)
		facet.LibraryPaths.AppendUniq(x.LibraryPaths...)
		facet.LibrarianOptions.AppendUniq(x.LibrarianOptions...)
		facet.LinkerOptions.AppendUniq(x.LinkerOptions...)
		facet.Tags.Append(x.Tags)
		facet.Exports.Append(x.Exports)
	}
}
func (facet *Facet) Prepend(others ...Facetable) {
	for _, o := range others {
		x := o.GetFacet()
		facet.Defines.Prepend(x.Defines...)
		facet.ForceIncludes.Prepend(x.ForceIncludes...)
		facet.IncludePaths.Prepend(x.IncludePaths...)
		facet.ExternIncludePaths.Prepend(x.ExternIncludePaths...)
		facet.SystemIncludePaths.Prepend(x.SystemIncludePaths...)
		facet.AnalysisOptions.Prepend(x.AnalysisOptions...)
		facet.PreprocessorOptions.Prepend(x.PreprocessorOptions...)
		facet.CompilerOptions.Prepend(x.CompilerOptions...)
		facet.PrecompiledHeaderOptions.Prepend(x.PrecompiledHeaderOptions...)
		facet.Libraries.Prepend(x.Libraries...)
		facet.LibraryPaths.Prepend(x.LibraryPaths...)
		facet.LibrarianOptions.Prepend(x.LibrarianOptions...)
		facet.LinkerOptions.Prepend(x.LinkerOptions...)
		facet.Tags.Append(facet.Tags)
		facet.Exports.Prepend(x.Exports)
	}
}

func (facet *Facet) AddCompilationFlag(flags ...string) {
	facet.AnalysisOptions.Append(flags...)
	facet.AddCompilationFlag_NoAnalysis(flags...)
}
func (facet *Facet) AddCompilationFlag_NoAnalysis(flags ...string) {
	facet.PrecompiledHeaderOptions.Append(flags...)
	facet.PreprocessorOptions.Append(flags...)
	facet.CompilerOptions.Append(flags...)
}
func (facet *Facet) AddCompilationFlag_NoPreprocessor(flags ...string) {
	facet.PrecompiledHeaderOptions.Append(flags...)
	facet.CompilerOptions.Append(flags...)
}
func (facet *Facet) RemoveCompilationFlag(flags ...string) {
	facet.AnalysisOptions.Remove(flags...)
	facet.PrecompiledHeaderOptions.Remove(flags...)
	facet.PreprocessorOptions.Remove(flags...)
	facet.CompilerOptions.Remove(flags...)
}

func (facet *Facet) PerformSubstitutions() {
	if subst := facet.Exports.Prepare(); len(subst) > 0 {
		facet.Defines = base.Map(subst.ExpandString, facet.Defines...)
		facet.ForceIncludes = base.Map(subst.ExpandFilename, facet.ForceIncludes...)
		facet.IncludePaths = base.Map(subst.ExpandDirectory, facet.IncludePaths...)
		facet.ExternIncludePaths = base.Map(subst.ExpandDirectory, facet.ExternIncludePaths...)
		facet.SystemIncludePaths = base.Map(subst.ExpandDirectory, facet.SystemIncludePaths...)
		facet.AnalysisOptions = base.Map(subst.ExpandString, facet.AnalysisOptions...)
		facet.PreprocessorOptions = base.Map(subst.ExpandString, facet.PreprocessorOptions...)
		facet.CompilerOptions = base.Map(subst.ExpandString, facet.CompilerOptions...)
		facet.PrecompiledHeaderOptions = base.Map(subst.ExpandString, facet.PrecompiledHeaderOptions...)
		facet.Libraries = base.Map(subst.ExpandFilename, facet.Libraries...)
		facet.LibraryPaths = base.Map(subst.ExpandDirectory, facet.LibraryPaths...)
		facet.LibrarianOptions = base.Map(subst.ExpandString, facet.LibrarianOptions...)
		facet.LinkerOptions = base.Map(subst.ExpandString, facet.LinkerOptions...)
	}
}

func (facet *Facet) String() string {
	return base.PrettyPrint(facet)
}

/***************************************
 * Variable Substitutions
 ***************************************/

var getVariableSubstitutionsRegexp = base.Memoize(func() *regexp.Regexp {
	return regexp.MustCompile(`\{\{\.([^}]+)\}\}`)
})

type VariableSubstitutions map[string]string

func NewVariableSubstituions(definitions ...VariableDefinition) (result VariableSubstitutions) {
	result = make(VariableSubstitutions, len(definitions))
	for _, def := range definitions {
		result[fmt.Sprint(`{{.`, def.Name, `}}`)] = def.Value
	}
	return result
}

func (vars VariableSubstitutions) Valid() bool {
	return len(vars) > 0
}
func (vars VariableSubstitutions) ExpandString(str string) string {
	if strings.ContainsAny(str, "{{.") {
		str = getVariableSubstitutionsRegexp().ReplaceAllStringFunc(str, func(varname string) string {
			if value, ok := vars[varname]; ok {
				return value
			}
			return varname // Return the original match if variable not found
		})
	}
	return str
}
func (vars VariableSubstitutions) ExpandDirectory(dir utils.Directory) (result utils.Directory) {
	if strings.ContainsAny(dir.String(), "{{.") {
		dir = utils.MakeDirectory(getVariableSubstitutionsRegexp().ReplaceAllStringFunc(dir.String(), func(varname string) string {
			if value, ok := vars[varname]; ok {
				return value
			}
			return varname // Return the original match if variable not found
		}))
	}
	return dir
}
func (vars VariableSubstitutions) ExpandFilename(it utils.Filename) utils.Filename {
	return utils.Filename{
		Dirname:  vars.ExpandDirectory(it.Dirname),
		Basename: vars.ExpandString(it.Basename),
	}
}

/***************************************
 * Variable Definitions
 ***************************************/

func (vars VariableDefinitions) Prepare() VariableSubstitutions {
	return NewVariableSubstituions(vars...)
}
func (vars *VariableDefinitions) Add(name, value string) {
	if i, ok := vars.IndexOf(name); ok {
		(*vars)[i].Value = value
	} else {
		*vars = append(*vars, VariableDefinition{
			Name: name, Value: value,
		})
	}
}
func (vars VariableDefinitions) Get(from string) string {
	if i, ok := vars.IndexOf(from); ok {
		return vars[i].Value
	} else {
		base.LogPanic(LogCompile, "variable-substitutions: could not find [[:%s:]] in %v", from, vars)
		return ""
	}
}
func (vars VariableDefinitions) IndexOf(from string) (int, bool) {
	for i, it := range vars {
		if it.Name == from {
			return i, true
		}
	}
	return len(vars), false
}
func (vars *VariableDefinitions) Append(other VariableDefinitions) {
	for _, it := range other {
		if _, ok := vars.IndexOf(it.Name); !ok {
			*vars = append(*vars, it)
		}
	}
}
func (vars *VariableDefinitions) Prepend(other VariableDefinitions) {
	for _, it := range other {
		vars.Add(it.Name, it.Value)
	}
}
func (vars *VariableDefinitions) Serialize(ar base.Archive) {
	base.SerializeSlice(ar, (*[]VariableDefinition)(vars))
}

/***************************************
 * Variable Definition
 ***************************************/

func (def VariableDefinition) String() string {
	return fmt.Sprint(def.Name, `=`, def.Value)
}
func (def *VariableDefinition) Set(in string) error {
	parsed := strings.SplitN(in, `=`, 2)
	if len(parsed) != 2 {
		return fmt.Errorf("invalid variable definition %q", in)
	}

	def.Name = parsed[0]
	def.Value = parsed[1]
	return nil
}
func (x VariableDefinition) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *VariableDefinition) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (def *VariableDefinition) Serialize(ar base.Archive) {
	ar.String(&def.Name)
	ar.String(&def.Value)
}
