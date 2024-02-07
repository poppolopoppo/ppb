package cmd

import (
	"fmt"
	"io"
	"regexp"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/cluster"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * Imported Action
 ***************************************/

type ImportedAction struct {
	Command action.CommandRules

	OutputFile utils.Filename `json:",omitempty"`
	ExportFile utils.Filename `json:",omitempty"`
	ExtraFiles utils.FileSet  `json:",omitempty"`

	DynamicInputFiles utils.FileSet `json:",omitempty"`
	StaticInputFiles  utils.FileSet `json:",omitempty"`

	DynamicDependencies action.ActionAliases `json:",omitempty"`
	StaticDependencies  action.ActionAliases `json:",omitempty"`

	DependencyOutputFile     utils.Filename `json:",omitempty"`
	DependencySourcePatterns base.StringSet `json:",omitempty"`

	Options action.OptionFlags `json:",omitempty"`
}

func (x *ImportedAction) GetActionAlias() action.ActionAlias {
	return action.NewActionAlias(x.ExportFile)
}

/***************************************
 * Import Actions
 ***************************************/

type ImportActionsCommand struct {
	InputFiles []utils.Filename
	Build      utils.BoolVar
	Clean      utils.BoolVar
}

var CommandImportActions = utils.NewCommandable(
	"Interop",
	"import-actions",
	"imports actions from external json file(s), ignoring project configuration",
	&ImportActionsCommand{
		Build: base.INHERITABLE_FALSE,
		Clean: base.INHERITABLE_FALSE,
	})

func (x *ImportActionsCommand) Flags(cfv utils.CommandFlagsVisitor) {
	cfv.Variable("Build", "build imported actions immediately", &x.Build)
	cfv.Variable("Clean", "erase all by files outputted by selected actions", &x.Clean)
	action.GetActionFlags().Flags(cfv)
}
func (x *ImportActionsCommand) Init(ci utils.CommandContext) error {
	ci.Options(
		utils.OptionCommandParsableFlags("ImportCommand", "control actions importing", x),
		utils.OptionCommandParsableAccessor("ClusterFlags", "action distribution in network cluster", cluster.GetClusterFlags),
		utils.OptionCommandParsableAccessor("WorkerFlags", "set hardware limits for local action compilation", cluster.GetWorkerFlags),
		utils.OptionCommandConsumeMany("InputFiles", "build all targets specified as argument", &x.InputFiles),
	)
	return nil
}
func (x *ImportActionsCommand) Run(cc utils.CommandContext) error {
	base.LogClaim(utils.LogCommand, "import <%v>...", base.JoinString(">, <", x.InputFiles...))

	bg := utils.CommandEnv.BuildGraph()
	buildAliases := utils.BuildAliases{}
	exportFileToAction := make(map[utils.Filename]*ImportedAction)

	for _, src := range x.InputFiles {
		var importedActions []ImportedAction

		if err := utils.UFS.OpenBuffered(src, func(r io.Reader) error {
			return base.JsonDeserialize(&importedActions, r)
		}); err != nil {
			return err
		}

		// first-pass: register actions and sanitize inputs, while ignoring dynamic files/inputs
		for i, it := range importedActions {
			if !it.OutputFile.Valid() {
				it.OutputFile = it.ExportFile
			}
			if !it.ExportFile.Valid() {
				it.ExportFile = it.OutputFile
			}
			if !it.ExportFile.Valid() {
				return fmt.Errorf("import empty export file: %q", &it.Command)
			}

			if _, ok := exportFileToAction[it.ExportFile]; ok {
				return fmt.Errorf("export file already imported: %q", it.ExportFile)
			}

			base.LogVerbose(utils.LogBuildGraph, "imported %q action from %q", action.NewActionAlias(it.ExportFile), src)

			exportFileToAction[it.ExportFile] = &it
			importedActions[i] = it
		}

		// second-pass: resolve all dynamic dependencies and create action models
		for _, it := range importedActions {
			model := action.ActionModel{
				Command:          it.Command,
				OutputFile:       it.OutputFile,
				ExportFile:       it.ExportFile,
				ExtraFiles:       it.ExtraFiles,
				StaticInputFiles: it.StaticInputFiles,
				StaticDeps:       utils.MakeBuildAliases(it.StaticDependencies...),
				Options:          it.Options,
			}

			var err error
			if model.DynamicInputs, err = action.GetBuildActions(base.Map(action.NewActionAlias, it.DynamicInputFiles...)...); err != nil {
				return err
			}
			if model.Prerequisites, err = action.GetBuildActions(it.DynamicDependencies...); err != nil {
				return err
			}

			if action, err := action.BuildAction(&model, func(am *action.ActionModel) (action.Action, error) {
				actionRules := am.CreateActionRules()

				if it.DependencyOutputFile.Valid() {
					return NewDependencyOutputAction(&actionRules, it.DependencyOutputFile, it.DependencySourcePatterns), nil
				}

				return &actionRules, nil
			}).Init(bg, utils.OptionBuildDirtyIf(x.Clean.Get()), utils.OptionBuildForce); err == nil {
				if x.Build.Get() {
					buildAliases.Append(action.Alias())
				}
			} else {
				return err
			}
		}
	}

	base.LogInfo(utils.LogBuildGraph, "succesfuly imported %d actions", len(exportFileToAction))

	// check if imported actions need to be built
	if len(buildAliases) > 0 {
		if _, err := bg.BuildMany(buildAliases, utils.OptionBuildForceIf(x.Clean.Get())); err != nil {
			return err
		}
	}

	return nil
}

/***************************************
 * Export Actions
 ***************************************/

var ExportActions = newCompletionCommand[action.ActionAlias, *action.ActionAlias](
	"Interop",
	"export-actions",
	"export selection compilation actions to Json",
	func(cc utils.CommandContext, ca *CompletionArgs[action.ActionAlias, *action.ActionAlias]) error {
		bg := utils.CommandEnv.BuildGraph()
		actions := make(map[action.ActionAlias]utils.BuildNode)
		if err := action.ForeachBuildAction(bg, func(bn utils.BuildNode, a action.Action) error {
			actions[a.GetAction().GetActionAlias()] = bn
			return nil
		}); err != nil {
			return err
		}

		return openCompletion(ca, func(w io.Writer) error {
			var filteredActions action.ActionSet

			if err := filterCompletion(ca, func(alias action.ActionAlias) error {
				node := actions[alias]
				filteredActions.Append(node.GetBuildable().(action.Action))
				return nil
			}, base.Keys(actions)...); err != nil {
				return err
			}

			if ca.Detailed.Get() {
				var err error
				if filteredActions, err = filteredActions.ExpandDependencies(bg); err != nil {
					return err
				}
			}

			importedActions := make([]ImportedAction, len(filteredActions))
			for i, it := range filteredActions {
				node := actions[it.GetAction().GetActionAlias()]
				rules := node.GetBuildable().(action.Action).GetAction()

				imported := ImportedAction{
					Command:    rules.CommandRules,
					ExportFile: rules.GetGeneratedFile(),
					ExtraFiles: utils.NewFileSet(rules.OutputFiles...),
					Options:    rules.Options,
				}
				imported.ExtraFiles.Delete(int(rules.ExportIndex))

				outputFileBasename := imported.ExportFile.TrimExt()
				for i, it := range imported.ExtraFiles {
					if it.TrimExt() == outputFileBasename {
						imported.OutputFile = it
						imported.ExtraFiles.Delete(i)
						break
					}
				}

				if !imported.OutputFile.Valid() {
					imported.OutputFile = imported.ExportFile
				}

				bg := utils.CommandEnv.BuildGraph()
				for _, it := range bg.GetStaticDependencies(node) {
					switch buildable := it.GetBuildable().(type) {
					case *utils.FileDependency:
						if buildable.Filename != rules.Executable {
							imported.StaticInputFiles.Append(buildable.Filename)
						}
					case action.Action:
						imported.StaticDependencies.Append(buildable.GetAction().GetActionAlias())
						imported.DynamicInputFiles.Append(buildable.GetAction().GetGeneratedFile())
						imported.DynamicDependencies.Append(buildable.GetAction().Prerequisites...)
					}
				}

				importedActions[i] = imported
			}

			return base.JsonSerialize(importedActions, w, base.OptionJsonPrettyPrint(true))
		})
	})

/***************************************
 * Dependency Build Context: intercept dynamic file dependencies
 ***************************************/

type dependencyBuildContext struct {
	inner utils.BuildContext

	OnFileNeeded base.PublicEvent[[]utils.Filename]
	OnFileOutput base.PublicEvent[[]utils.Filename]
}

func (x *dependencyBuildContext) GetBuildContext() utils.BuildContext {
	return x
}

func (x *dependencyBuildContext) BuildGraph() utils.BuildGraph { return x.inner.BuildGraph() }
func (x *dependencyBuildContext) GetStaticDependencies() []utils.BuildResult {
	return x.inner.GetStaticDependencies()
}
func (x *dependencyBuildContext) Annotate(annotations ...utils.BuildAnnotateFunc) {
	x.inner.Annotate(annotations...)
}
func (x *dependencyBuildContext) OnBuilt(it func(utils.BuildNode) error) {
	x.inner.OnBuilt(it)
}

func (x *dependencyBuildContext) OutputFile(files ...utils.Filename) error {
	if err := x.OnFileOutput.Invoke(files); err != nil {
		return err
	}
	return x.inner.OutputFile(files...)
}
func (x *dependencyBuildContext) NeedFiles(files ...utils.Filename) error {
	if err := x.OnFileNeeded.Invoke(files); err != nil {
		return err
	}
	return x.inner.NeedFiles(files...)
}

func (x *dependencyBuildContext) DependsOn(aliases ...utils.BuildAlias) error {

	return x.inner.DependsOn(aliases...)
}
func (x *dependencyBuildContext) NeedBuildable(buildables utils.BuildAliasable, opts ...utils.BuildOptionFunc) (utils.Buildable, error) {
	return x.inner.NeedBuildable(buildables, opts...)
}
func (x *dependencyBuildContext) NeedFactory(factory utils.BuildFactory, opts ...utils.BuildOptionFunc) (utils.Buildable, error) {
	return x.inner.NeedFactory(factory, opts...)
}
func (x *dependencyBuildContext) NeedFactories(factories ...utils.BuildFactory) error {
	return x.inner.NeedFactories(factories...)
}
func (x *dependencyBuildContext) NeedDirectories(directories ...utils.Directory) error {
	return x.inner.NeedDirectories(directories...)
}
func (x *dependencyBuildContext) NeedBuildAliasables(n int, buildAliasables func(int) utils.BuildAliasable, onBuildResult func(int, utils.BuildResult) error) error {
	return x.inner.NeedBuildAliasables(n, buildAliasables, onBuildResult)
}
func (x *dependencyBuildContext) NeedBuildResult(results ...utils.BuildResult) {
	x.inner.NeedBuildResult(results...)
}
func (x *dependencyBuildContext) OutputNode(factories ...utils.BuildFactory) error {
	return x.inner.OutputNode(factories...)
}
func (x *dependencyBuildContext) OutputFactory(factory utils.BuildFactory, opts ...utils.BuildOptionFunc) (utils.Buildable, error) {
	return x.inner.OutputFactory(factory, opts...)
}

/***************************************
 * DependencyOutputAction (mainly for Unreal Build Tool interop)
 ***************************************/

type DependencyOutputAction struct {
	action.ActionRules

	DependencyOutputFile   utils.Filename
	DependencySourceRegexp string
}

func NewDependencyOutputAction(rules *action.ActionRules, dependencyOutputFile utils.Filename, dependencySourcePatterns base.StringSet) *DependencyOutputAction {
	return &DependencyOutputAction{
		ActionRules:            *rules,
		DependencyOutputFile:   dependencyOutputFile,
		DependencySourceRegexp: utils.MakeGlobRegexpExpr(dependencySourcePatterns...),
	}
}
func (x *DependencyOutputAction) Build(bc utils.BuildContext) error {
	// export dependencies matching the source regexp
	var dependencyFiles utils.FileSet
	dependencySourceRE := regexp.MustCompile(x.DependencySourceRegexp)

	// check static inputs files
	for _, it := range bc.GetStaticDependencies() {
		switch buildable := it.Buildable.(type) {
		case utils.BuildableSourceFile:
			if dependencySourceRE.MatchString(buildable.GetSourceFile().Basename) {
				dependencyFiles.Append(buildable.GetSourceFile())
			}
		case utils.BuildableGeneratedFile:
			if dependencySourceRE.MatchString(buildable.GetGeneratedFile().Basename) {
				dependencyFiles.Append(buildable.GetGeneratedFile())
			}
		}
	}

	// wraps build context to observe dynamic file dependencies
	observerContext := dependencyBuildContext{inner: bc}
	observerContext.OnFileNeeded.Add(func(files []utils.Filename) error {
		dependencyFiles.Append(base.RemoveUnless(func(it utils.Filename) bool {
			return dependencySourceRE.MatchString(it.Basename)
		}, files...)...)
		return nil
	})

	// execute regular action compilation
	if err := x.ActionRules.Build(&observerContext); err != nil {
		return err
	}

	// make output deterministic by sorting files
	dependencyFiles.Sort()

	// finally output the dependency file
	if err := utils.UFS.CreateBuffered(x.DependencyOutputFile, func(w io.Writer) error {
		for _, it := range dependencyFiles {
			if _, err := fmt.Fprintln(w, it); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return bc.OutputFile(x.DependencyOutputFile)
}
func (x *DependencyOutputAction) Serialize(ar base.Archive) {
	ar.Serializable(&x.ActionRules)
	ar.Serializable(&x.DependencyOutputFile)
	ar.String(&x.DependencySourceRegexp)
}
