package compile

import (
	"fmt"
	"strings"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/internal/base"

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
	ActionAliases action.ActionAliases
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
	bc.Annotate(
		AnnocateBuildCommentf("%d outputs", len(x.ActionAliases)),
		AnnocateBuildMute)
	return nil
}
func (x *TargetPayload) Serialize(ar base.Archive) {
	ar.Serializable(&x.TargetAlias)
	ar.Serializable(&x.PayloadType)
	base.SerializeSlice(ar, x.ActionAliases.Ref())
}

func (x *TargetPayload) GetActions() (action.ActionSet, error) {
	return action.GetBuildActions(x.ActionAliases...)
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

	err = bc.DependsOn(MakeBuildAliases(targets...)...)
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
func (x *TargetActions) ForeachPayload(each func(*TargetPayload) error) error {
	return x.PresentPayloads.Range(func(i int, pt PayloadType) error {
		payload, err := x.GetPayload(pt)
		if err == nil {
			err = each(payload)
		}
		return err
	})
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
	bc.Annotate(AnnocateBuildCommentf("%d payloads, %d actions", x.PresentPayloads.Len(), numActions))
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

	customs, err := x.CustomActions()
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

		headerUnits, err := x.HeaderUnitActions(customs)
		if err != nil {
			return err
		}

		if err := x.CreatePayload(PAYLOAD_HEADERUNIT, headerUnits.Aliases()); err != nil {
			return err
		}

		objs, err := x.ObjectListActions(headerUnits, pchs)
		if err != nil {
			return err
		}

		if err := x.CreatePayload(PAYLOAD_OBJECTLIST, objs.Aliases()); err != nil {
			return err
		}

		switch x.Unit.Payload {
		case PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB:
			link, err := x.LinkActions(headerUnits, pchs, objs.Concat(customs...))
			if err != nil {
				return err
			}
			base.AssertNotIn(len(link), 0)

			if err := x.CreatePayload(x.Unit.Payload, link.Aliases()); err != nil {
				return err
			}

			targetOutputs = link

		case PAYLOAD_STATICLIB:
			lib, err := x.LibrarianActions(headerUnits, pchs, objs.Concat(customs...))
			if err != nil {
				return err
			}
			base.AssertNotIn(len(lib), 0)

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
		if err := x.ForceCreatePayload(PAYLOAD_HEADERS, customs.Aliases()); err != nil {
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
		x.BuildContext.OnBuilt(func(bn BuildNode) error {
			allActions, err := targetOutputs.ExpandDependencies(CommandEnv.BuildGraph())
			if err == nil {
				base.LogVeryVerbose(LogCompile, "%q outputs %v payload with %d artifacts (%d total actions)", x.Unit, x.Unit.Payload, len(targetOutputs), len(allActions))
			}
			return err
		})
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
func (x *buildActionGenerator) ForceCreatePayload(payloadType PayloadType, actionAliases action.ActionAliases) error {
	targetPayload := &TargetPayload{
		TargetAlias:   x.TargetAlias,
		PayloadType:   payloadType,
		ActionAliases: actionAliases,
	}

	x.PresentPayloads.Add(payloadType)
	x.TargetPayloads[payloadType] = targetPayload

	return x.BuildContext.OutputNode(WrapBuildFactory(func(bi BuildInitializer) (*TargetPayload, error) {
		return targetPayload, bi.DependsOn(MakeBuildAliases(targetPayload.ActionAliases...)...)
	}))
}
func (x *buildActionGenerator) CreatePayload(payloadType PayloadType, actionAliases action.ActionAliases) error {
	if !actionAliases.Empty() {
		return x.ForceCreatePayload(payloadType, actionAliases)
	}
	return nil
}

func (x *buildActionGenerator) CreateAction(
	payload PayloadType,
	model action.ActionModel,
) (action.Action, error) {
	// check if caching is allowed by compiler for this payload
	cacheMode := x.Compiler.AllowCaching(x.Unit, payload)
	base.AssertNotIn(cacheMode, action.CACHE_INHERIT)
	if cacheMode.HasRead() {
		model.Options.Add(action.OPT_ALLOW_CACHEREAD)
	}
	if cacheMode.HasWrite() {
		model.Options.Add(action.OPT_ALLOW_CACHEWRITE)
	}

	// check if distribution is allowed by compiler for this payload
	distMode := x.Compiler.AllowDistribution(x.Unit, payload)
	base.AssertNotIn(distMode, action.DIST_INHERIT)
	if distMode.Enabled() {
		model.Options.Add(action.OPT_ALLOW_DISTRIBUTION)
	}

	// check if compiler supports response files
	responseFile := x.Compiler.AllowResponseFile(x.Unit, payload)
	base.AssertNotIn(responseFile, SUPPORT_INHERIT)
	if responseFile.Enabled() {
		model.Options.Add(action.OPT_ALLOW_RESPONSEFILE)
	}

	// expand %1, %2 and %3: this is the final step, after every other side-effect has been applied
	model.Command.Arguments = performArgumentSubstitution(payload, &model)

	// finally, outputs generated action in build graph
	actionFactory := action.BuildAction(&model,
		func(model *action.ActionModel) (action.Action, error) {
			a := x.Compiler.CreateAction(x.Unit, payload, model)
			return a, nil
		})

	if buildable, err := x.BuildContext.OutputFactory(actionFactory, OptionBuildForce); err == nil {
		return buildable.(action.Action), nil
	} else {
		return nil, err
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
func (x *buildActionGenerator) GetOutputActions(targets ...TargetAlias) (action.ActionSet, error) {
	aliases := action.ActionAliases{}
	err := x.NeedTargetActions(func(ta *TargetActions) error {
		if payload, err := ta.GetOutputPayload(); err == nil {
			aliases.Append(payload.ActionAliases...)
		} else {
			return err
		}
		return nil
	}, targets...)
	if err == nil {
		return action.GetBuildActions(aliases...)
	}
	return action.ActionSet{}, err
}

func (x *buildActionGenerator) HeaderUnitActions(dependencies action.ActionSet) (action.ActionSet, error) {
	actions := action.ActionSet{}
	switch x.Unit.PCH {
	case PCH_HEADERUNIT:
		compilerRules := x.Compiler.GetCompiler()

		headerUnitObject := Filename{
			Dirname:  x.Unit.PrecompiledObject.Dirname,
			Basename: x.Unit.PrecompiledObject.Basename + x.Compiler.Extname(PAYLOAD_OBJECTLIST)}

		buildAction, err := x.CreateAction(
			PAYLOAD_HEADERUNIT,
			action.ActionModel{
				Command: action.CommandRules{
					Arguments:   x.Unit.HeaderUnitOptions,
					Environment: compilerRules.Environment,
					Executable:  compilerRules.Executable,
					WorkingDir:  UFS.Root,
				},
				StaticInputFiles: FileSet{x.Unit.PrecompiledHeader},
				ExportFile:       headerUnitObject,
				OutputFile:       x.Unit.PrecompiledObject,
				StaticDeps:       MakeBuildAliases(dependencies...),
				Options:          action.MakeOptionFlags(action.OPT_ALLOW_SOURCEDEPENDENCIES),
			})
		if err != nil {
			return action.ActionSet{}, err
		}

		actions.Append(buildAction)
	}
	return actions, nil
}

func (x *buildActionGenerator) PrecompilerHeaderActions(dependencies action.ActionSet) (action.ActionSet, error) {
	actions := action.ActionSet{}
	switch x.Unit.PCH {
	case PCH_DISABLED:
		// nothing to do
	case PCH_MONOLITHIC:
		compilerRules := x.Compiler.GetCompiler()

		pchObject := x.Compiler.GetPayloadOutput(x.Unit, PAYLOAD_PRECOMPILEDOBJECT, x.Unit.PrecompiledObject)

		buildAction, err := x.CreateAction(
			PAYLOAD_PRECOMPILEDHEADER,
			action.ActionModel{
				Command: action.CommandRules{
					Arguments:   x.Unit.PrecompiledHeaderOptions,
					Environment: compilerRules.Environment,
					Executable:  compilerRules.Executable,
					WorkingDir:  UFS.Root,
				},
				StaticInputFiles: FileSet{x.Unit.PrecompiledSource, x.Unit.PrecompiledHeader},
				ExportFile:       pchObject,
				OutputFile:       x.Unit.PrecompiledObject,
				StaticDeps:       MakeBuildAliases(dependencies...),
				// PCH object should not be stored in cache, but objects compiled with it can still be stored if we track PCH inputs instead of PCH outputs
				Options: action.MakeOptionFlags(action.OPT_PROPAGATE_INPUTS, action.OPT_ALLOW_SOURCEDEPENDENCIES),
			})
		if err != nil {
			return action.ActionSet{}, err
		}

		actions.Append(buildAction)
	case PCH_HEADERUNIT:
		// handled in HeaderUnitActions()
	case PCH_SHARED:
		return action.ActionSet{}, fmt.Errorf("PCH_SHARED is not supported at the monent")
	default:
		base.UnexpectedValuePanic(x.Unit.PCH, x.Unit.PCH)
	}
	return actions, nil
}
func (x *buildActionGenerator) CustomActions() (action.ActionSet, error) {
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
		if actions, err := generator.ObjectListActions(action.ActionSet{}, action.ActionSet{}); err == nil {
			result.Append(actions...)
		} else {
			return action.ActionSet{}, err
		}
	}
	return result, nil
}
func (x *buildActionGenerator) ObjectListActions(headerUnits, pchs action.ActionSet) (action.ActionSet, error) {
	includeDeps, err := x.GetOutputActions(x.Unit.IncludeDependencies...)
	if err != nil {
		return action.ActionSet{}, err
	}

	sourceFiles, err := x.Unit.GetSourceFiles(x.BuildContext)
	if err != nil {
		return action.ActionSet{}, err
	}

	compilerRules := x.Compiler.GetCompiler()

	includeAliases := make(BuildAliases, 0, len(includeDeps)+len(headerUnits)+1)
	for _, it := range includeDeps {
		includeAliases.Append(it.Alias())
	}
	for _, it := range headerUnits {
		includeAliases.Append(it.Alias())
	}

	base.Assert(sourceFiles.IsUniq)
	objs := make(action.ActionSet, len(sourceFiles))
	for i, input := range sourceFiles {
		output := x.Unit.GetPayloadOutput(x.Compiler, input, PAYLOAD_OBJECTLIST)

		staticDeps := includeAliases
		staticInputFiles := FileSet{input}
		dynamicInputFiles := FileSet{}

		objs[i], err = x.CreateAction(
			PAYLOAD_OBJECTLIST,
			action.ActionModel{
				Command: action.CommandRules{
					Arguments:   x.Unit.CompilerOptions,
					Environment: compilerRules.Environment,
					Executable:  compilerRules.Executable,
					WorkingDir:  UFS.Root,
				},
				StaticInputFiles:  staticInputFiles,
				DynamicInputFiles: dynamicInputFiles,
				ExportFile:        output,
				OutputFile:        output,
				Prerequisites:     pchs,
				StaticDeps:        staticDeps,
				// allow compiler support for dependency list generation
				Options: action.MakeOptionFlags(action.OPT_ALLOW_SOURCEDEPENDENCIES),
			})

		if err != nil {
			return nil, err
		}
	}

	return objs, nil
}
func (x *buildActionGenerator) LibrarianActions(headerUnits, pchs, objs action.ActionSet) (action.ActionSet, error) {
	base.AssertIn(x.Unit.Payload, PAYLOAD_STATICLIB, PAYLOAD_SHAREDLIB)
	base.AssertNotIn(len(objs), 0)

	compileDeps, err := x.GetOutputActions(x.Unit.CompileDependencies...)
	if err != nil {
		return action.ActionSet{}, err
	}

	extraFiles := NewFileSet(x.Unit.ExtraFiles...)
	if x.Unit.SymbolsFile.Valid() {
		extraFiles.Append(x.Unit.SymbolsFile)
	}

	compilerRules := x.Compiler.GetCompiler()

	lib, err := x.CreateAction(
		PAYLOAD_STATICLIB,
		action.ActionModel{
			Command: action.CommandRules{
				Arguments:   x.Unit.LibrarianOptions,
				Environment: compilerRules.Environment,
				Executable:  compilerRules.Librarian,
				WorkingDir:  UFS.Root,
			},
			DynamicInputs: objs.Concat(compileDeps...).Concat(headerUnits...),
			ExportFile:    x.Unit.ExportFile,
			OutputFile:    x.Unit.OutputFile,
			ExtraFiles:    extraFiles,
			Prerequisites: pchs,
		})

	return action.ActionSet{lib}, err
}
func (x *buildActionGenerator) LinkActions(headerUnits, pchs, objs action.ActionSet) (action.ActionSet, error) {
	base.AssertIn(x.Unit.Payload, PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB)
	base.AssertNotIn(len(objs), 0)

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

	extraFiles := NewFileSet(x.Unit.ExtraFiles...)
	if x.Unit.SymbolsFile.Valid() {
		extraFiles.Append(x.Unit.SymbolsFile)
	}

	compilerRules := x.Compiler.GetCompiler()

	link, err := x.CreateAction(
		x.Unit.Payload,
		action.ActionModel{
			Command: action.CommandRules{
				Arguments:   x.Unit.LinkerOptions,
				Environment: compilerRules.Environment,
				Executable:  compilerRules.Linker,
				WorkingDir:  UFS.Root,
			},
			DynamicInputs: objs.Concat(compileDeps...).Concat(linkDeps...).Concat(headerUnits...),
			ExportFile:    x.Unit.ExportFile,
			OutputFile:    x.Unit.OutputFile,
			ExtraFiles:    extraFiles,
			Prerequisites: pchs,
			StaticDeps:    MakeBuildAliases(runtimeDeps...),
		})

	return action.ActionSet{link}, err
}

/***************************************
 * Command-line quoting and parameter expansion
 ***************************************/

func performArgumentSubstitution(payload PayloadType, model *action.ActionModel) base.StringSet {
	var (
		allowRelativePath = model.Options.Any(action.OPT_ALLOW_RELATIVEPATH)
		inputFiles        = model.GetCommandInputFiles()
		result            = make([]string, 0, len(model.Command.Arguments)+len(inputFiles)+len(model.ExtraFiles)+1)
		nextInput         = 0
	)
	base.Assert(model.ExportFile.Valid)
	base.Assert(model.OutputFile.Valid)
	base.AssertNotIn(len(inputFiles), 0)

	switch payload {
	case PAYLOAD_STATICLIB, PAYLOAD_SHAREDLIB, PAYLOAD_EXECUTABLE:
		inputFiles.Append(model.Prerequisites.GetExportFiles()...) // PCHs
	}

	for _, arg := range model.Command.Arguments {
		// substitution rules are inherited from FASTBuild, see https://fastbuild.org/docs/functions/objectlist.html
		if strings.ContainsRune(arg, '%') {
			if strings.Contains(arg, "%1") {
				if payload.HasMultipleInput() {
					for _, input := range inputFiles {
						relativePath := MakeLocalFilenameIFP(input, allowRelativePath)
						result = append(result, strings.ReplaceAll(arg, "%1", relativePath))
					}
					continue
				} else {
					input := inputFiles[nextInput]
					relativePath := MakeLocalFilenameIFP(input, allowRelativePath)
					arg = strings.ReplaceAll(arg, "%1", relativePath)
					nextInput++
				}
			}

			switch payload {
			case PAYLOAD_PRECOMPILEDHEADER, PAYLOAD_HEADERUNIT:
				arg = strings.ReplaceAll(arg, "%2", MakeLocalFilenameIFP(model.OutputFile, allowRelativePath)) // stdafx.pch
				// special for PCH generation
				arg = strings.ReplaceAll(arg, "%3", MakeLocalFilenameIFP(model.ExportFile, allowRelativePath)) // stdafx.pch.obj
			default:
				relativePath := MakeLocalFilenameIFP(model.OutputFile, allowRelativePath)
				arg = strings.ReplaceAll(arg, "%2", relativePath)
			}
		}

		result = append(result, arg)
	}
	return result
}
