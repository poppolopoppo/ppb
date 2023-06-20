package compile

import (
	"fmt"
	"regexp"
	"strings"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/internal/io"
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

func FindTargetPayload(targetAlias TargetAlias, payloadType PayloadType) (*TargetPayload, error) {
	return FindGlobalBuildable[*TargetPayload](MakeBuildAlias("Payloads", payloadType.String(), targetAlias.String()))
}

func (x *TargetPayload) Alias() BuildAlias {
	return MakeBuildAlias("Payloads", x.PayloadType.String(), x.TargetAlias.String())
}
func (x *TargetPayload) Build(bc BuildContext) error {
	bc.Annotate(fmt.Sprintf("%d outputs", len(x.ActionAliases)))
	return nil
}
func (x *TargetPayload) Serialize(ar Archive) {
	ar.Serializable(&x.TargetAlias)
	ar.Serializable(&x.PayloadType)
	SerializeSlice(ar, x.ActionAliases.Ref())
}

func (x *TargetPayload) GetActions() (ActionSet, error) {
	return GetBuildActions(x.ActionAliases)
}

/***************************************
 * Target Actions
 ***************************************/

type PayloadBuildAliases = [NumPayloadTypes]BuildAliases

type TargetActions struct {
	TargetAlias     TargetAlias
	OutputType      PayloadType
	PresentPayloads EnumSet[PayloadType, *PayloadType]
}

func FindTargetActions(targetAlias TargetAlias) (*TargetActions, error) {
	return FindGlobalBuildable[*TargetActions](MakeBuildAlias("Targets", targetAlias.String()))
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

	return NeedTargetActions(bc, Map(func(u *Unit) TargetAlias { return u.TargetAlias }, units...)...)
}

func (x *TargetActions) Alias() BuildAlias {
	return MakeBuildAlias("Targets", x.TargetAlias.String())
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
	Assert(func() bool { return nil != unit })

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
func (x *TargetActions) Serialize(ar Archive) {
	ar.Serializable(&x.TargetAlias)
	ar.Serializable(&x.OutputType)
	ar.Serializable(&x.PresentPayloads)
}

func (x *TargetActions) GetOutputPayload() (*TargetPayload, error) {
	return x.GetPayload(x.OutputType)
}
func (x *TargetActions) GetOutputActions() (ActionSet, error) {
	if targetPayload, err := x.GetOutputPayload(); err == nil {
		return targetPayload.GetActions()
	} else {
		return ActionSet{}, err
	}
}

func GetBuildActions(aliases BuildAliases) (ActionSet, error) {
	Assert(aliases.IsUniq)
	result := make(ActionSet, len(aliases))
	for i, alias := range aliases {
		if action, err := FindGlobalBuildable[Action](alias); err == nil {
			Assert(func() bool { return nil != action })
			result[i] = action
		} else {
			return ActionSet{}, err
		}
	}
	return result, nil
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

	scm *SourceControlModifiedFiles

	*TargetActions
	TargetPayloads [NumPayloadTypes]*TargetPayload
	BuildContext
}

func (x *buildActionGenerator) CreateActions() error {
	var targetOutputs ActionSet
	x.OutputType = x.Unit.Payload

	customs, err := x.CustomActions(ActionSet{})
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
			UnexpectedValuePanic(x.Unit.Payload, x.Unit.Payload)
		}

	} else {
		if err := x.CreatePayload(PAYLOAD_HEADERS, customs.Aliases()); err != nil {
			return err
		}

		targetOutputs = customs
	}

	AssertMessage(func() bool { return x.OutputType == PAYLOAD_HEADERS || len(targetOutputs) > 0 },
		"target %q has no output but should have since it's a %v module", x.TargetAlias, x.OutputType)

	if IsLogLevelActive(LOG_VERYVERBOSE) {
		allActions := ActionSet{}
		LogPanicIfFailed(LogAction, targetOutputs.ExpandDependencies(&allActions))
		LogVeryVerbose(LogAction, "%q outputs %v payload with %d artifacts (%d total actions)", x.Unit, x.Unit.Payload, len(targetOutputs), len(allActions))
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

func (x *buildActionGenerator) makeActionFactory(action Action) BuildFactoryTyped[Action] {
	return WrapBuildFactory(func(bi BuildInitializer) (Action, error) {
		rules := action.GetAction()

		// track executable file
		if err := bi.NeedFile(rules.Executable); err != nil {
			return nil, err
		}

		// track dependent actions as build dependency and as a member alias list
		if err := bi.DependsOn(rules.Dependencies...); err != nil {
			return nil, err
		}

		// only insert generated unity files as static inputs
		// do *NOT* insert all inputs as static dependencies: they are already recorded as dynamic dependencies by ActionRules.Build()
		for _, filename := range rules.Inputs {
			if unity, err := FindUnityFile(filename); err == nil {
				if err = bi.NeedBuildable(unity); err != nil {
					return nil, err
				}
			}
		}

		// create output directories
		outputDirs := DirSet{}
		for _, filename := range rules.Outputs {
			outputDirs.AppendUniq(filename.Dirname)
		}
		for _, filename := range rules.Extras {
			outputDirs.AppendUniq(filename.Dirname)
		}

		for _, directory := range outputDirs {
			if _, err := BuildDirectoryCreator(directory).Need(bi); err != nil {
				return nil, err
			}
		}

		return action, nil
	})
}
func (x *buildActionGenerator) NewActionRules(
	payload PayloadType,
	executable Filename,
	workingDir Directory,
	environment ProcessEnvironment,
	inputs, outputs, exports, extras FileSet,
	dependentActions ActionSet,
	arguments ...string) *ActionRules {
	Assert(func() bool { return len(inputs) > 0 })
	Assert(func() bool { return len(outputs) > 0 })

	cacheMode := x.Compiler.AllowCaching(x.Unit, payload)
	distMode := x.Compiler.AllowDistribution(x.Unit, payload)
	responseFile := x.Compiler.AllowResponseFile(x.Unit, payload)
	editAndContinue := x.Compiler.AllowEditAndContinue(x.Unit, payload)

	if editAndContinue.Enabled() {
		// #TODO: find another workaround for MSVC hot-reload, which doesn't respect file case... should make a post first for support I guess
		LogVeryVerbose(LogAction, "force lower case for output because MSVC hotreload:\n\torig: %q\n\thack: %q")
		toLower := func(files FileSet) FileSet {
			for i, fname := range files {
				files[i] = Filename{
					Dirname:  fname.Dirname,
					Basename: strings.ToLower(fname.Basename),
				}
			}
			return files
		}
		outputs = toLower(outputs)
		exports = toLower(exports)
		extras = toLower(extras)
	}

	// perform argument expansion when build graph is created
	arguments = performArgumentSubstitution(x.Unit, payload, inputs, outputs, arguments...)

	action := &ActionRules{
		TargetAlias:  x.Unit.TargetAlias,
		Payload:      payload,
		Executable:   executable,
		WorkingDir:   workingDir,
		Environment:  environment,
		Inputs:       inputs,
		Outputs:      outputs,
		Exports:      exports,
		Extras:       extras,
		Arguments:    arguments,
		CacheMode:    cacheMode,
		DistMode:     distMode,
		ResponseFile: responseFile,
		Dependencies: dependentActions.Aliases(),
	}
	AssertMessage(func() bool { return len(inputs) > 0 }, "%v: no action input present", action.Alias())
	AssertMessage(func() bool { return len(outputs) > 0 }, "%v: no action output present", action.Alias())
	AssertNotIn(action.CacheMode, CACHE_INHERIT)

	if action.CacheMode.HasRead() || action.CacheMode.HasWrite() {
		// exclude locally modified files from caching
		if x.scm == nil {
			if scm, err := BuildSourceControlModifiedFiles(x.Unit.ModuleDir).Need(x.BuildContext); err == nil {
				x.scm = scm
			} else {
				LogPanicErr(LogAction, err)
			}
		}

		for _, file := range inputs {
			if x.scm.HasUnversionedModifications(file) {
				LogVerbose(LogAction, "%v: excluded from cache since %q is locally modified", action.Alias(), file)
				action.CacheMode = CACHE_NONE
				break
			}
		}
	}

	return action
}

func (x *buildActionGenerator) NewAction(
	payload PayloadType,
	executable Filename,
	workingDir Directory,
	environment ProcessEnvironment,
	inputs, outputs, exports, extras FileSet,
	dependentActions ActionSet,
	arguments ...string) (Action, error) {
	rules := x.NewActionRules(
		payload,
		executable, workingDir, environment,
		inputs, outputs, exports, extras,
		dependentActions, arguments...)
	buildable, err := x.BuildContext.OutputFactory(x.makeActionFactory(rules), OptionBuildForce)
	if err == nil {
		return buildable.(Action), nil
	} else {
		return nil, err
	}
}
func (x *buildActionGenerator) NewSourceDependencyAction(
	payload PayloadType,
	executable Filename,
	workingDir Directory,
	environment ProcessEnvironment,
	inputs, outputs, exports, extras FileSet,
	dependentActions ActionSet,
	arguments ...string) (Action, error) {
	if OnRunCommandWithDetours != nil {
		// platform supports running process with IO detouring
		return x.NewAction(
			payload,
			executable, workingDir, environment,
			inputs, outputs, exports, extras,
			dependentActions, arguments...)
	} else {
		// no support: must rely on compiler support for source dependencies
		rules := x.NewActionRules(
			payload,
			executable, workingDir, environment,
			inputs, outputs, exports, extras,
			dependentActions, arguments...,
		)
		action := x.Compiler.SourceDependencies(rules)
		buildable, err := x.BuildContext.OutputFactory(x.makeActionFactory(action), OptionBuildForce)
		if err == nil {
			action = buildable.(Action)
		}
		return action, err
	}
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
func (x *buildActionGenerator) GetOutputActions(targets ...TargetAlias) (ActionSet, error) {
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
		return ActionSet{}, err
	}
	return GetBuildActions(result)
}

func (x *buildActionGenerator) PrecompilerHeaderActions(dependencies ActionSet) (ActionSet, error) {
	actions := ActionSet{}
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

		action, err := x.NewSourceDependencyAction(
			PAYLOAD_PRECOMPILEDHEADER,
			compilerRules.Executable,
			UFS.Root,
			compilerRules.Environment,
			inputs, outputs, exports, extras,
			dependencies,
			x.Unit.PrecompiledHeaderOptions...)
		if err != nil {
			return ActionSet{}, err
		}

		actions.Append(action)
	case PCH_SHARED:
		return ActionSet{}, fmt.Errorf("PCH_SHARED is not supported at the monent")
	default:
		UnexpectedValuePanic(x.Unit.PCH, x.Unit.PCH)
	}
	return actions, nil
}
func (x *buildActionGenerator) CustomActions(dependencies ActionSet) (ActionSet, error) {
	result := ActionSet{}
	for _, custom := range x.Unit.CustomUnits {
		compiler, err := custom.GetBuildCompiler()
		if err != nil {
			return ActionSet{}, err
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
			return ActionSet{}, err
		}
	}
	return result, nil
}
func (x *buildActionGenerator) ObjectAction(
	dependencies ActionSet,
	input, output Filename) (Action, error) {
	compilerRules := x.Compiler.GetCompiler()
	return x.NewSourceDependencyAction(
		PAYLOAD_OBJECTLIST,
		compilerRules.Executable,
		UFS.Root,
		compilerRules.Environment,
		FileSet{input}, FileSet{output}, FileSet{output}, FileSet{},
		dependencies,
		x.Unit.CompilerOptions...)
}
func (x *buildActionGenerator) ObjectListActions(dependencies ActionSet) (ActionSet, error) {
	includeDeps, err := x.GetOutputActions(x.Unit.IncludeDependencies...)
	if err != nil {
		return ActionSet{}, err
	}

	dependencies.Append(includeDeps...)

	sourceFiles, err := x.Unit.GetSourceFiles(x.BuildContext)
	if err != nil {
		return ActionSet{}, err
	}

	Assert(sourceFiles.IsUniq)
	objs := make(ActionSet, len(sourceFiles))

	for i, input := range sourceFiles {
		output := x.Unit.GetPayloadOutput(x.Compiler, input, PAYLOAD_OBJECTLIST)
		action, err := x.ObjectAction(dependencies, input, output)
		if err != nil {
			return ActionSet{}, err
		}

		objs[i] = action
	}

	return objs, nil
}
func (x *buildActionGenerator) LibrarianActions(pchs ActionSet, objs ActionSet) (ActionSet, error) {
	AssertIn(x.Unit.Payload, PAYLOAD_STATICLIB, PAYLOAD_SHAREDLIB)

	compileDeps, err := x.GetOutputActions(x.Unit.CompileDependencies...)
	if err != nil {
		return ActionSet{}, err
	}

	compilerRules := x.Compiler.GetCompiler()
	dependencies := ActionSet{}
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

	return ActionSet{lib}, err
}
func (x *buildActionGenerator) LinkActions(pchs ActionSet, objs ActionSet) (ActionSet, error) {
	AssertIn(x.Unit.Payload, PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB)

	compileDeps, err := x.GetOutputActions(x.Unit.CompileDependencies...)
	if err != nil {
		return ActionSet{}, err
	}

	linkDeps, err := x.GetOutputActions(x.Unit.LinkDependencies...)
	if err != nil {
		return ActionSet{}, err
	}

	runtimeDeps, err := x.GetOutputActions(x.Unit.RuntimeDependencies...)
	if err != nil {
		return ActionSet{}, err
	}

	dependencies := ActionSet{}
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

	return ActionSet{link}, err
}

/***************************************
 * Command-line quoting and parameter expansion
 ***************************************/

var getArgumentSubstitutionRE = Memoize(func() *regexp.Regexp {
	return regexp.MustCompile(`%(\d)`)
})

func performArgumentSubstitution(unit *Unit, payload PayloadType, inputs FileSet, outputs FileSet, arguments ...string) StringSet {
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
