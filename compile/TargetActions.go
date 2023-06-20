package compile

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * Target Payload
 ***************************************/

// TargetPayload is separated from TargetActions to avoid
// invalidation of *ALL* actions when any source file was changed.
// They also serve as a build alias for all actions associated to a payload.

type TargetPayload struct {
	TargetAlias   TargetAlias
	PayloadType   PayloadType
	ActionAliases BuildAliases
}

func MakeTargetPayloadAlias(ta TargetAlias, payload PayloadType) BuildAlias {
	return MakeBuildAlias("Payloads", ta.PlatformName, ta.ConfigName, ta.NamespaceName, ta.ModuleName, payload.String())
}

func FindTargetPayload(targetAlias TargetAlias, payloadType PayloadType) (*TargetPayload, error) {
	return FindGlobalBuildable[*TargetPayload](
		MakeTargetPayloadAlias(targetAlias, payloadType))
}

func (x *TargetPayload) Alias() BuildAlias {
	return MakeTargetPayloadAlias(x.TargetAlias, x.PayloadType)
}
func (x *TargetPayload) Build(bc BuildContext) error {
	bc.Annotate(fmt.Sprintf("%d outputs", len(x.ActionAliases)))
	return nil
}
func (x *TargetPayload) Serialize(ar base.Archive) {
	ar.Serializable(&x.TargetAlias)
	ar.Serializable(&x.PayloadType)
	base.SerializeSlice(ar, x.ActionAliases.Ref())
}

func (x *TargetPayload) GetActions() (action.ActionSet, error) {
	return action.GetBuildActions(x.ActionAliases)
}

/***************************************
 * Target Actions
 ***************************************/

type PayloadBuildAliases = [NumPayloadTypes]BuildAliases

type TargetActions struct {
	TargetAlias     TargetAlias
	OutputType      PayloadType
	PresentPayloads base.EnumSet[PayloadType, *PayloadType]
}

func MakeTargetActionsAlias(ta TargetAlias) BuildAlias {
	return MakeBuildAlias("Targets", ta.PlatformName, ta.ConfigName, ta.NamespaceName, ta.ModuleName)
}

func FindTargetActions(targetAlias TargetAlias) (*TargetActions, error) {
	return FindGlobalBuildable[*TargetActions](MakeTargetActionsAlias(targetAlias))
}

func NeedTargetActions(bc BuildContext, targetAliases ...TargetAlias) (targets []*TargetActions, err error) {
	targets = make([]*TargetActions, len(targetAliases))

	for i, targetAlias := range targetAliases {
		var target *TargetActions
		if target, err = FindTargetActions(targetAlias); err == nil {
			targets[i] = target
		} else {
			return
		}
	}

	if err = bc.DependsOn(MakeBuildAliases(targets...)...); err != nil {
		return
	}

	return
}

func NeedAllTargetActions(bc BuildContext) (targets []*TargetActions, err error) {
	units, err := NeedAllBuildUnits(bc)
	if err != nil {
		return
	}

	return NeedTargetActions(bc, base.Map(func(u *Unit) TargetAlias { return u.TargetAlias }, units...)...)
}

func (x *TargetActions) Alias() BuildAlias {
	return MakeTargetActionsAlias(x.TargetAlias)
}
func (x *TargetActions) GetPayload(payloadType PayloadType) (*TargetPayload, error) {
	targetPayload, err := FindTargetPayload(x.TargetAlias, payloadType)
	if err == nil {
		return targetPayload, nil
	} else {
		return nil, err
	}
}
func (x *TargetActions) Build(bc BuildContext) error {
	x.OutputType = PAYLOAD_HEADERS
	x.PresentPayloads.Clear()

	unit, err := FindBuildUnit(x.TargetAlias)
	if err != nil {
		return err
	}
	base.Assert(func() bool { return nil != unit })

	compiler, err := unit.GetBuildCompiler()
	if err != nil {
		return err
	}

	generator := buildActionGenerator{
		Environment:   x.TargetAlias.EnvironmentAlias,
		Unit:          unit,
		Compiler:      compiler,
		TargetActions: x,
		BuildContext:  bc,
	}

	if err := generator.CreateActions(); err != nil {
		return err
	}

	numActions := generator.NumActions()
	bc.Annotate(fmt.Sprintf("%d payloads, %d actions", x.PresentPayloads.Len(), numActions))
	return nil
}
func (x *TargetActions) Serialize(ar base.Archive) {
	ar.Serializable(&x.TargetAlias)
	ar.Serializable(&x.OutputType)
	ar.Serializable(&x.PresentPayloads)
}

func (x *TargetActions) GetOutputPayload() (*TargetPayload, error) {
	return x.GetPayload(x.OutputType)
}
func (x *TargetActions) GetOutputActions() (action.ActionSet, error) {
	if targetPayload, err := x.GetOutputPayload(); err == nil {
		return targetPayload.GetActions()
	} else {
		return action.ActionSet{}, err
	}
}

func ForeachTargetActions(ea EnvironmentAlias, each func(*TargetActions) error, ma ...ModuleAlias) error {
	for _, it := range ma {
		target, err := FindTargetActions(TargetAlias{EnvironmentAlias: ea, ModuleAlias: it})
		if err != nil {
			return err
		}
		if err := each(target); err != nil {
			return err
		}
	}
	return nil
}

/***************************************
 * Build Action Generator
 ***************************************/

type buildActionGenerator struct {
	Environment EnvironmentAlias
	Unit        *Unit
	Compiler    Compiler

	*TargetActions
	TargetPayloads [NumPayloadTypes]*TargetPayload
	BuildContext
}

func (x *buildActionGenerator) CreateActions() error {
	var targetOutputs action.ActionSet
	x.OutputType = x.Unit.Payload

	customs, err := x.CustomActions(action.ActionSet{})
	if err != nil {
		return err
	}

	if x.Unit.Payload != PAYLOAD_HEADERS {
		pchs, err := x.PrecompilerHeaderActions(customs)
		if err != nil {
			return err
		}

		if err := x.CreatePayload(PAYLOAD_PRECOMPILEDHEADER, pchs.Aliases()); err != nil {
			return err
		}

		objs, err := x.ObjectListActions(pchs)
		if err != nil {
			return err
		}

		if err := x.CreatePayload(PAYLOAD_OBJECTLIST, objs.Aliases()); err != nil {
			return err
		}

		switch x.Unit.Payload {
		case PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB:
			link, err := x.LinkActions(pchs, append(objs, customs...))
			if err != nil {
				return err
			}

			runtimes, err := x.GetOutputActions(x.Unit.RuntimeDependencies...)
			if err != nil {
				return err
			}

			link.DependsOn(runtimes...)

			if err := x.CreatePayload(x.Unit.Payload, link.Aliases()); err != nil {
				return err
			}

			targetOutputs = link

		case PAYLOAD_STATICLIB:
			lib, err := x.LibrarianActions(pchs, append(objs, customs...))
			if err != nil {
				return err
			}

			if err := x.CreatePayload(PAYLOAD_STATICLIB, lib.Aliases()); err != nil {
				return err
			}

			targetOutputs = lib

		case PAYLOAD_OBJECTLIST:
			targetOutputs = objs

		default:
			base.UnexpectedValuePanic(x.Unit.Payload, x.Unit.Payload)
		}

	} else {
		if err := x.CreatePayload(PAYLOAD_HEADERS, customs.Aliases()); err != nil {
			return err
		}

		targetOutputs = customs
	}

	base.AssertErr(func() error {
		if x.OutputType == PAYLOAD_HEADERS || len(targetOutputs) > 0 {
			return nil
		}
		return fmt.Errorf("target %q has no output but should have since it's a %v module", x.TargetAlias, x.OutputType)
	})

	if base.IsLogLevelActive(base.LOG_VERYVERBOSE) {
		allActions := action.ActionSet{}
		base.LogPanicIfFailed(LogCompile, targetOutputs.ExpandDependencies(&allActions))
		base.LogVeryVerbose(LogCompile, "%q outputs %v payload with %d artifacts (%d total actions)", x.Unit, x.Unit.Payload, len(targetOutputs), len(allActions))
	}

	return nil
}
func (x *buildActionGenerator) NumActions() (total int) {
	for _, targetPayload := range x.TargetPayloads {
		if targetPayload != nil {
			total += targetPayload.ActionAliases.Len()
		}
	}
	return
}
func (x *buildActionGenerator) CreatePayload(payloadType PayloadType, actionAliases BuildAliases) error {
	targetPayload := &TargetPayload{
		TargetAlias:   x.TargetAlias,
		PayloadType:   payloadType,
		ActionAliases: actionAliases,
	}

	x.PresentPayloads.Add(payloadType)
	x.TargetPayloads[payloadType] = targetPayload

	return x.BuildContext.OutputNode(WrapBuildFactory(func(bi BuildInitializer) (*TargetPayload, error) {
		return targetPayload, bi.DependsOn(targetPayload.ActionAliases...)
	}))
}

func (x *buildActionGenerator) createBaseAction(
	payload PayloadType,
	executable Filename,
	workingDir Directory,
	environment internal_io.ProcessEnvironment,
	inputs, outputs, exports, extras FileSet,
	dependentActions action.ActionSet,
	sourceControlPath Directory,
	arguments ...string) (action.Action, error) {
	base.Assert(func() bool { return len(inputs) > 0 })
	base.Assert(func() bool { return len(outputs) > 0 })

	// perform argument expansion when build graph is created
	arguments = performArgumentSubstitution(x.Unit, payload, inputs, outputs, arguments...)

	rules := &action.ActionRules{
		Executable:        executable,
		WorkingDir:        workingDir,
		Environment:       environment,
		Inputs:            inputs,
		Outputs:           outputs,
		Exports:           exports,
		Extras:            extras,
		Arguments:         arguments,
		Dependencies:      dependentActions.Aliases(),
		SourceControlPath: sourceControlPath,
	}
	base.AssertErr(func() error {
		if len(inputs) > 0 {
			return nil
		}
		return fmt.Errorf("%v: no action input present", rules.Alias())
	})
	base.AssertErr(func() error {
		if len(outputs) > 0 {
			return nil
		}
		return fmt.Errorf("%v: no action output present", rules.Alias())
	})

	cacheMode := x.Compiler.AllowCaching(x.Unit, payload)
	base.AssertNotIn(cacheMode, action.CACHE_INHERIT)

	if cacheMode.HasRead() {
		rules.Options.Add(action.OPT_ALLOW_CACHEREAD)
	}
	if cacheMode.HasWrite() {
		rules.Options.Add(action.OPT_ALLOW_CACHEWRITE)
	}

	distMode := x.Compiler.AllowDistribution(x.Unit, payload)
	base.AssertNotIn(distMode, action.DIST_INHERIT)

	if distMode.Enabled() {
		rules.Options.Add(action.OPT_ALLOW_DISTRIBUTION)
	}

	responseFile := x.Compiler.AllowResponseFile(x.Unit, payload)
	base.AssertNotIn(responseFile, COMPILERSUPPORT_INHERIT)

	if responseFile.Enabled() {
		rules.Options.Add(action.OPT_ALLOW_RESPONSEFILE)
	}

	buildAction := x.Compiler.CreateAction(x.Unit, payload, rules)

	// only insert generated unity files as static inputs
	// do *NOT* insert all inputs as static dependencies: they are already recorded as dynamic dependencies by ActionRules.Build()
	var staticDeps BuildAliases
	for _, filename := range rules.Inputs {
		if unity, err := FindUnityFile(filename); err == nil {
			staticDeps.Append(unity.Alias())
		}
	}

	actionFactory := action.BuildAction(buildAction, staticDeps...)
	if buildable, err := x.BuildContext.OutputFactory(actionFactory, OptionBuildForce); err == nil {
		return buildable.(action.Action), nil
	} else {
		return nil, err
	}
}

func (x *buildActionGenerator) NewAction(
	payload PayloadType,
	executable Filename,
	workingDir Directory,
	environment internal_io.ProcessEnvironment,
	inputs, outputs, exports, extras FileSet,
	dependentActions action.ActionSet,
	arguments ...string) (action.Action, error) {
	var sourceControlPath Directory
	return x.createBaseAction(payload, executable, workingDir, environment, inputs, outputs, exports, extras, dependentActions, sourceControlPath, arguments...)
}
func (x *buildActionGenerator) NewActionWithSourceControl(
	payload PayloadType,
	executable Filename,
	workingDir Directory,
	environment internal_io.ProcessEnvironment,
	inputs, outputs, exports, extras FileSet,
	dependentActions action.ActionSet,
	arguments ...string) (action.Action, error) {
	var sourceControlPath = x.Unit.ModuleDir
	return x.createBaseAction(payload, executable, workingDir, environment, inputs, outputs, exports, extras, dependentActions, sourceControlPath, arguments...)
}

func (x *buildActionGenerator) NeedTargetActions(each func(*TargetActions) error, targets ...TargetAlias) error {
	for _, targetAlias := range targets {
		if targetActions, err := FindTargetActions(targetAlias); err == nil {
			if err := x.BuildContext.DependsOn(targetActions.Alias()); err != nil {
				return err
			}
			if err := each(targetActions); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}
func (x *buildActionGenerator) GetOutputActions(targets ...TargetAlias) (action.ActionSet, error) {
	result := BuildAliases{}
	err := x.NeedTargetActions(func(ta *TargetActions) error {
		if payload, err := ta.GetOutputPayload(); err == nil {
			result.AppendUniq(payload.ActionAliases...)
		} else {
			return err
		}
		return nil
	}, targets...)
	if err != nil {
		return action.ActionSet{}, err
	}
	return action.GetBuildActions(result)
}

func (x *buildActionGenerator) PrecompilerHeaderActions(dependencies action.ActionSet) (action.ActionSet, error) {
	actions := action.ActionSet{}
	switch x.Unit.PCH {
	case PCH_DISABLED:
		// nothing to do
	case PCH_MONOLITHIC:
		compilerRules := x.Compiler.GetCompiler()
		pchObject := Filename{
			Dirname:  x.Unit.PrecompiledObject.Dirname,
			Basename: x.Unit.PrecompiledObject.Basename + x.Compiler.Extname(PAYLOAD_OBJECTLIST)}

		inputs := FileSet{x.Unit.PrecompiledSource, x.Unit.PrecompiledHeader}
		outputs := FileSet{pchObject}
		exports := FileSet{pchObject}
		extras := FileSet{x.Unit.PrecompiledObject}

		buildAction, err := x.NewActionWithSourceControl(
			PAYLOAD_PRECOMPILEDHEADER,
			compilerRules.Executable,
			UFS.Root,
			compilerRules.Environment,
			inputs, outputs, exports, extras,
			dependencies,
			x.Unit.PrecompiledHeaderOptions...)
		if err != nil {
			return action.ActionSet{}, err
		}

		actions.Append(buildAction)
	case PCH_SHARED:
		return action.ActionSet{}, fmt.Errorf("PCH_SHARED is not supported at the monent")
	default:
		base.UnexpectedValuePanic(x.Unit.PCH, x.Unit.PCH)
	}
	return actions, nil
}
func (x *buildActionGenerator) CustomActions(dependencies action.ActionSet) (action.ActionSet, error) {
	result := action.ActionSet{}
	for _, custom := range x.Unit.CustomUnits {
		compiler, err := custom.GetBuildCompiler()
		if err != nil {
			return action.ActionSet{}, err
		}
		generator := buildActionGenerator{
			Environment:   x.TargetAlias.EnvironmentAlias,
			Unit:          &custom.Unit,
			Compiler:      compiler,
			TargetActions: x.TargetActions,
			BuildContext:  x.BuildContext,
		}
		if actions, err := generator.ObjectListActions(dependencies); err == nil {
			result.Append(actions...)
		} else {
			return action.ActionSet{}, err
		}
	}
	return result, nil
}
func (x *buildActionGenerator) ObjectAction(
	dependencies action.ActionSet,
	input, output Filename) (action.Action, error) {
	compilerRules := x.Compiler.GetCompiler()
	return x.NewActionWithSourceControl(
		PAYLOAD_OBJECTLIST,
		compilerRules.Executable,
		UFS.Root,
		compilerRules.Environment,
		FileSet{input}, FileSet{output}, FileSet{output}, FileSet{},
		dependencies,
		x.Unit.CompilerOptions...)
}
func (x *buildActionGenerator) ObjectListActions(dependencies action.ActionSet) (action.ActionSet, error) {
	includeDeps, err := x.GetOutputActions(x.Unit.IncludeDependencies...)
	if err != nil {
		return action.ActionSet{}, err
	}

	dependencies.Append(includeDeps...)

	sourceFiles, err := x.Unit.GetSourceFiles(x.BuildContext)
	if err != nil {
		return action.ActionSet{}, err
	}

	base.Assert(sourceFiles.IsUniq)
	objs := make(action.ActionSet, len(sourceFiles))

	for i, input := range sourceFiles {
		output := x.Unit.GetPayloadOutput(x.Compiler, input, PAYLOAD_OBJECTLIST)
		buildAction, err := x.ObjectAction(dependencies, input, output)
		if err != nil {
			return action.ActionSet{}, err
		}

		objs[i] = buildAction
	}

	return objs, nil
}
func (x *buildActionGenerator) LibrarianActions(pchs action.ActionSet, objs action.ActionSet) (action.ActionSet, error) {
	base.AssertIn(x.Unit.Payload, PAYLOAD_STATICLIB, PAYLOAD_SHAREDLIB)

	compileDeps, err := x.GetOutputActions(x.Unit.CompileDependencies...)
	if err != nil {
		return action.ActionSet{}, err
	}

	compilerRules := x.Compiler.GetCompiler()
	dependencies := action.ActionSet{}
	dependencies.Append(pchs...)
	dependencies.Append(objs...)
	dependencies.Append(compileDeps...)

	inputs := dependencies.GetExportFiles()
	outputs := FileSet{x.Unit.OutputFile}
	exports := FileSet{x.Unit.ExportFile}
	extras := NewFileSet(x.Unit.ExtraFiles...)
	if x.Unit.SymbolsFile.Valid() {
		extras.Append(x.Unit.SymbolsFile)
	}

	lib, err := x.NewAction(
		PAYLOAD_STATICLIB,
		compilerRules.Librarian,
		UFS.Root,
		compilerRules.Environment,
		inputs, outputs, exports, extras,
		dependencies,
		x.Unit.LibrarianOptions...)

	return action.ActionSet{lib}, err
}
func (x *buildActionGenerator) LinkActions(pchs action.ActionSet, objs action.ActionSet) (action.ActionSet, error) {
	base.AssertIn(x.Unit.Payload, PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB)

	compileDeps, err := x.GetOutputActions(x.Unit.CompileDependencies...)
	if err != nil {
		return action.ActionSet{}, err
	}

	linkDeps, err := x.GetOutputActions(x.Unit.LinkDependencies...)
	if err != nil {
		return action.ActionSet{}, err
	}

	runtimeDeps, err := x.GetOutputActions(x.Unit.RuntimeDependencies...)
	if err != nil {
		return action.ActionSet{}, err
	}

	dependencies := action.ActionSet{}
	dependencies.Append(pchs...)
	dependencies.Append(objs...)
	dependencies.Append(compileDeps...)
	dependencies.Append(linkDeps...)

	inputs := dependencies.GetExportFiles()
	outputs := FileSet{x.Unit.OutputFile}
	exports := FileSet{x.Unit.ExportFile}
	extras := NewFileSet(x.Unit.ExtraFiles...)
	if x.Unit.SymbolsFile.Valid() {
		extras.Append(x.Unit.SymbolsFile)
	}

	compilerRules := x.Compiler.GetCompiler()
	link, err := x.NewAction(
		x.Unit.Payload,
		compilerRules.Linker,
		UFS.Root,
		compilerRules.Environment,
		inputs, outputs, exports, extras,
		append(dependencies, runtimeDeps...),
		x.Unit.LinkerOptions...)

	return action.ActionSet{link}, err
}

/***************************************
 * Command-line quoting and parameter expansion
 ***************************************/

var getArgumentSubstitutionRE = base.Memoize(func() *regexp.Regexp {
	return regexp.MustCompile(`%(\d)`)
})

func performArgumentSubstitution(unit *Unit, payload PayloadType, inputs FileSet, outputs FileSet, arguments ...string) base.StringSet {
	result := make([]string, 0, len(arguments))

	for _, arg := range arguments {
		// substitution rules are inherited from FASTBuild, see https://fastbuild.org/docs/functions/objectlist.html
		if strings.Contains(arg, "%") {
			if payload.HasMultipleInput() {
				if strings.Contains(arg, "%1") {
					for _, input := range inputs {
						relativePath := MakeLocalFilename(input)
						result = append(result, strings.ReplaceAll(arg, "%1", relativePath))
					}
					continue
				}
			} else {
				for _, input := range inputs {
					relativePath := MakeLocalFilename(input)
					arg = strings.Replace(arg, "%1", relativePath, 1)
				}
			}

			if payload != PAYLOAD_PRECOMPILEDHEADER {
				for _, output := range outputs {
					relativePath := MakeLocalFilename(output)
					arg = strings.Replace(arg, "%2", relativePath, 1)
				}
			} else { // special for PCH generation
				arg = strings.ReplaceAll(arg, "%2", MakeLocalFilename(unit.PrecompiledObject)) // stdafx.pch
				arg = strings.ReplaceAll(arg, "%3", MakeLocalFilename(outputs[0]))             // stdafx.pch.obj
			}
		}

		result = append(result, arg)
	}
	return result
}
