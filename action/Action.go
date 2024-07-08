package action

import (
	"fmt"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"

	"github.com/poppolopoppo/ppb/utils"
)

var LogAction = base.NewLogCategory("Action")

func InitAction() {
	base.LogTrace(LogAction, "build/action.Init()")

	base.RegisterSerializable[ActionRules]()
	base.RegisterSerializable[actionCache]()
}

type Action interface {
	GetAction() *ActionRules
	utils.Buildable
	fmt.Stringer
}

/***************************************
 * Action Rules
 ***************************************/

type ActionAlias struct {
	ExportFile utils.Filename
}

type ActionAliases = base.SetT[ActionAlias]

func NewActionAlias(exportFile utils.Filename) ActionAlias {
	base.Assert(exportFile.Valid)
	return ActionAlias{ExportFile: exportFile}
}
func (x ActionAlias) Valid() bool {
	return x.ExportFile.Valid()
}
func (x ActionAlias) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("Action", x.ExportFile.Dirname.Path, x.ExportFile.Basename)
}
func (x ActionAlias) String() string {
	base.Assert(func() bool { return x.Valid() })
	return x.ExportFile.String()
}
func (x ActionAlias) Compare(o ActionAlias) int {
	return x.ExportFile.Compare(o.ExportFile)
}
func (x ActionAlias) AutoComplete(in base.AutoComplete) {
	utils.CommandEnv.BuildGraph().Range(func(ba utils.BuildAlias, bn utils.BuildNode) error {
		switch buildable := bn.GetBuildable().(type) {
		case Action:
			in.Add(buildable.GetAction().GetGeneratedFile().String(), "")
		}
		return nil
	})
}
func (x *ActionAlias) Serialize(ar base.Archive) {
	ar.Serializable(&x.ExportFile)
}
func (x *ActionAlias) Set(in string) (err error) {
	return x.ExportFile.Set(in)
}
func (x *ActionAlias) MarshalText() ([]byte, error) {
	return x.ExportFile.MarshalText()
}
func (x *ActionAlias) UnmarshalText(data []byte) error {
	return x.ExportFile.UnmarshalText(data)
}

/***************************************
 * Action Rules
 ***************************************/

type ActionRules struct {
	CommandRules

	OutputFiles   utils.FileSet // all output files that should be tracked
	ExportIndex   int32         // index of export file in outputs files
	Prerequisites ActionAliases // actions to run dynamically only if cache missed (PCH)

	Options OptionFlags
}

func (x *ActionRules) Alias() utils.BuildAlias {
	return x.GetActionAlias().Alias()
}

func (x *ActionRules) GetAction() *ActionRules { return x }
func (x *ActionRules) GetActionAlias() ActionAlias {
	return NewActionAlias(x.OutputFiles[x.ExportIndex])
}
func (x *ActionRules) GetGeneratedFile() utils.Filename { return x.OutputFiles[x.ExportIndex] }

func (x *ActionRules) AppendDependentActions(bg utils.BuildGraph, result *ActionSet) error {
	buildNode, err := bg.Expect(x.Alias())
	if err != nil {
		return err
	}

	for _, it := range bg.GetStaticDependencies(buildNode) {
		switch buildable := it.GetBuildable().(type) {
		case Action:
			result.AppendUniq(buildable)
		}
	}

	if prerequisites, err := GetBuildActions(x.Prerequisites...); err == nil {
		result.AppendUniq(prerequisites...)
	} else {
		return err
	}

	return nil
}

func (x *ActionRules) GetStaticInputFiles(bg utils.BuildGraph) (results utils.FileSet) {
	node, err := bg.Expect(x.Alias())
	base.LogPanicIfFailed(LogAction, err)

	for _, it := range bg.GetStaticDependencies(node) {
		if buildable, ok := (it.GetBuildable()).(utils.BuildableSourceFile); ok {
			if inputFile := buildable.GetSourceFile(); inputFile != x.Executable {
				results.Append(inputFile)
			}
		}
	}
	return
}

func (x *ActionRules) Serialize(ar base.Archive) {
	ar.Serializable(&x.CommandRules)
	ar.Serializable(&x.OutputFiles)
	base.SerializeSlice(ar, x.Prerequisites.Ref())
	ar.Int32(&x.ExportIndex)
	ar.Serializable(&x.Options)

}
func (x *ActionRules) String() string {
	return x.CommandRules.String()
}

func (x *ActionRules) Build(bc utils.BuildContext) error {
	// consolidate static input files
	var staticInputFiles, excludedInputFiles utils.FileSet
	for _, it := range bc.GetStaticDependencies() {
		if err := harvestActionInputFiles(bc, it, &staticInputFiles, &excludedInputFiles); err != nil {
			return err
		}
	}

	// check if we can read action cache
	var cacheKey ActionCacheKey
	var cacheArtifact CacheArtifact

	hasValidCacheArtifact := false
	wasRetrievedFromCache := false

	flags := GetActionFlags()
	if x.Options.Has(OPT_ALLOW_CACHEREAD) && flags.CacheMode.HasRead() {
		var err error
		cacheArtifact, cacheKey, err = createActionCacheArtifact(&x.CommandRules, staticInputFiles, x.OutputFiles)
		if err != nil {
			return err
		}
		hasValidCacheArtifact = true

		if err = GetActionCache().CacheRead(cacheKey, &cacheArtifact); err == nil {
			wasRetrievedFromCache = true // cache-hit
			bc.Annotate(utils.AnnocateBuildComment(`CACHE`))

			// restore dynamic dependencies
			if err = bc.NeedFiles(cacheArtifact.DependencyFiles...); err != nil {
				return err
			}
		} else {
			base.LogWarningVerbose(LogAction, "%v: %v", x.Alias(), err)
		}
	}

	// run process if the cache missed
	if !wasRetrievedFromCache {
		// need prerequisites before building if cache missed
		var prerequisiteFiles utils.FileSet
		if err := bc.NeedBuildAliasables(len(x.Prerequisites),
			func(i int) utils.BuildAliasable { return x.Prerequisites[i].Alias() },
			func(i int, br utils.BuildResult) error {
				return harvestActionInputFiles(bc, br, &prerequisiteFiles, &excludedInputFiles)
			}); err != nil {
			return err
		}

		// remove possibly duplicate files from prerequisites (like compiler executable)
		prerequisiteFiles.Remove(staticInputFiles...)

		// either run locally or distribute to a remote worker
		readFiles, err := executeOrDistributeAction(bc, x, flags, staticInputFiles, prerequisiteFiles)
		if err != nil {
			return err
		}

		// whole input files set = static + dynamic
		if x.Options.Has(OPT_ALLOW_CACHEWRITE) && flags.CacheMode.HasWrite() {
			if !hasValidCacheArtifact {
				if cacheArtifact, cacheKey, err = createActionCacheArtifact(&x.CommandRules, staticInputFiles, x.OutputFiles); err != nil {
					return err
				}
			}

			cacheArtifact.DependencyFiles = readFiles.ConcatUniq(prerequisiteFiles...)
			cacheArtifact.DependencyFiles.Remove(excludedInputFiles...)

			bc.OnBuilt(func(node utils.BuildNode) error {
				base.AssertErr(func() error {
					if actionAlias := x.Alias(); node.Alias() != actionAlias {
						return fmt.Errorf("action cache mismatching alias: %q vs %q", node.Alias(), actionAlias)
					}
					return nil
				})
				return asyncCacheWriteAction(cacheKey, &cacheArtifact)
			})
		}
	}

	// check that process did write expected files and track them as outputs
	return bc.OutputFile(x.OutputFiles...)
}

func harvestActionInputFiles(bc utils.BuildContext, br utils.BuildResult, results, excludeds *utils.FileSet) error {
	switch buildable := br.Buildable.(type) {
	case Action:
		rules := buildable.GetAction()

		if err := bc.NeedFiles(rules.GetGeneratedFile()); err != nil {
			return err
		}

		if rules.Options.Has(OPT_PROPAGATE_INPUTS) {
			inputs, err := bc.BuildGraph().GetDependencyInputFiles(false, br.BuildAlias)
			if err != nil {
				return err
			}
			results.AppendUniq(inputs...)
			excludeds.Append(rules.OutputFiles...)
		} else {
			results.Append(rules.GetGeneratedFile())
		}

	case utils.BuildableGeneratedFile:
		file := buildable.GetGeneratedFile()
		if err := bc.NeedFiles(file); err != nil {
			return err
		}

		results.Append(file)

	case utils.BuildableSourceFile:
		results.Append(buildable.GetSourceFile())
	}
	return nil
}

func asyncCacheWriteAction(cacheKey ActionCacheKey, cacheArtifact *CacheArtifact) error {
	// queue a task with all heavy work to avoid slowing hot path of actions exection
	base.GetGlobalThreadPool().Queue(func(base.ThreadContext) {
		bg := utils.CommandEnv.BuildGraph()

		// disable caching when inputs have unversioned modifications
		writeToCache := true
		if _, err := utils.ForeachLocalSourceControlModifications(bg.GlobalContext(), func(modified utils.Filename, state utils.SourceControlState) error {
			writeToCache = false
			base.LogWarningVerbose(LogAction, "%v: excluded from cache since %q is seen as %v by source control", utils.ForceLocalFilename(cacheArtifact.OutputFiles[0]), modified, state)
			return nil
		}, cacheArtifact.InputFiles.Concat(cacheArtifact.DependencyFiles...)...); err != nil {
			base.LogPanicIfFailed(LogActionCache, err)
		}

		// finally write compiled artifacts to the cache
		if writeToCache {
			cacheArtifact.DependencyFiles.Sort()
			err := GetActionCache().CacheWrite(cacheKey, cacheArtifact)
			base.LogPanicIfFailed(LogActionCache, err)
		}
	}, base.TASKPRIORITY_LOW) // executing tasks has more priority than caching results

	return nil
}

func createActionCacheArtifact(command *CommandRules, inputFiles, outputFiles utils.FileSet) (CacheArtifact, ActionCacheKey, error) {
	var cacheArtifact CacheArtifact
	cacheArtifact.Command = *command
	cacheArtifact.InputFiles = inputFiles
	cacheArtifact.InputFiles.Sort()
	cacheArtifact.OutputFiles = outputFiles
	cacheArtifact.OutputFiles.Sort()

	cacheKey, err := GetActionCache().CacheKey(&cacheArtifact)
	return cacheArtifact, cacheKey, err
}

func executeOrDistributeAction(bc utils.BuildContext, action *ActionRules, flags *ActionFlags, staticInputFiles, prerequisiteFiles utils.FileSet) (utils.FileSet, error) {
	var processOptions internal_io.ProcessOptions
	var readFiles utils.FileSet

	// create a temporary map with all static inputs: we want mutual exclusion between static and dynamic dependencies
	staticFiles := make(map[utils.Filename]bool, len(staticInputFiles)+len(prerequisiteFiles)+len(action.OutputFiles))
	for _, it := range staticInputFiles {
		staticFiles[it] = true
	}
	for _, it := range prerequisiteFiles {
		staticFiles[it] = true
	}
	for _, it := range action.OutputFiles {
		staticFiles[it] = true
	}

	// run the external process with action command-line and file access hooking
	processOptions.Init(
		// internal_io.OptionProcessNewProcessGroup, // do not catch parent's signals
		internal_io.OptionProcessEnvironment(action.Environment),
		internal_io.OptionProcessWorkingDir(action.WorkingDir),
		internal_io.OptionProcessCaptureOutputIf(flags.ShowOutput.Get()),
		internal_io.OptionProcessUseResponseFileIf(action.Options.Has(OPT_ALLOW_RESPONSEFILE) && flags.ResponseFile.Get()),
		internal_io.OptionProcessFileAccess(func(far internal_io.FileAccessRecord) error {
			ignoreFile := true

			// only file access read/execute: output files could mess with writable mapped system Dll on Windows :'(
			if far.Access.HasRead() && !(far.Access.HasWrite() || far.Access.HasExecute()) {
				_, ignoreFile = staticFiles[far.Path]
			}

			if !ignoreFile {
				readFiles.Append(far.Path)
			}

			if flags.ShowFiles.Get() || base.IsLogLevelActive(base.LOG_VERYVERBOSE) {
				base.LogForwardf("%v: [%s]  %s%s", base.MakeStringer(func() string {
					return action.Alias().String()
				}), far.Access, far.Path, base.Blend("", " (IGNORED)", ignoreFile))
			}

			return nil
		}))

	// check action and environment parameters allow for distribution
	wasDistributed := false
	if action.Options.Has(OPT_ALLOW_DISTRIBUTION) && flags.DistMode.Enabled() {
		// check if process can be distributed in remote worker cluster
		if actionDist := GetActionDist(); actionDist.CanDistribute(flags.DistMode.Forced()) {
			peer, err := actionDist.DistributeAction(action.Alias(), action.Executable, action.Arguments, &processOptions)

			if wasDistributed = (peer != nil); wasDistributed {
				bc.Annotate(utils.AnnocateBuildComment(peer.GetAddress()))
				if err != nil {
					return readFiles, err
				}
			}
		}
	}

	// run process locally if it was not distributed
	if !wasDistributed {
		// limit number of concurrent external processes with MakeGlobalWorkerFuture()
		future := base.MakeGlobalWorkerFuture(func(tc base.ThreadContext) (int, error) {
			bc.Annotate(utils.AnnocateBuildCommentf("Thread:%d/%d", tc.GetThreadId()+1, tc.GetThreadPool().GetArity()))

			internal_io.OptionProcessOnSpinnerMessage(func(executable utils.Filename, arguments base.StringSet, options *internal_io.ProcessOptions) base.ProgressScope {
				spinner := base.LogSpinnerEx(
					base.ProgressOptionFormat("[W:%02d/%2d] %v",
						tc.GetThreadId()+1,
						tc.GetThreadPool().GetArity(),
						utils.ForceLocalFilename(action.GetGeneratedFile())),
					base.ProgressOptionColor(base.NewPastelizerColor(float64(tc.GetThreadId())/float64(tc.GetThreadPool().GetArity())).Quantize(true)))
				return spinner
			})(&processOptions)

			// check if we should log executed command-line
			if flags.ShowCmds.Get() {
				base.LogForwardln("\"", action.Executable.String(), "\" \"", strings.Join(action.Arguments, "\" \""), "\"")
			}

			return 0, internal_io.RunProcess(action.Executable, action.Arguments, internal_io.OptionProcessStruct(&processOptions))
		}, base.TASKPRIORITY_HIGH)

		if err := future.Join().Failure(); err != nil {
			return readFiles, err
		}
	}

	return readFiles, bc.NeedFiles(readFiles...)
}

/***************************************
 * Action Set
 ***************************************/

type ActionSet []Action

func (x ActionSet) Slice() []Action { return x }
func (x ActionSet) Aliases() ActionAliases {
	aliases := make([]ActionAlias, len(x))
	for i, it := range x {
		aliases[i] = it.GetAction().GetActionAlias()
	}
	return aliases
}
func (x ActionSet) Contains(action Action) bool {
	for _, it := range x {
		if it == action {
			return true
		}
	}
	return false
}
func (x *ActionSet) Append(actions ...Action) {
	*x = base.AppendComparable_CheckUniq(*x, actions...)
}
func (x *ActionSet) AppendUniq(actions ...Action) {
	*x = base.AppendUniq(*x, actions...)
}
func (x ActionSet) Concat(actions ...Action) ActionSet {
	return base.AppendComparable_CheckUniq(x, actions...)
}
func (x ActionSet) ExpandDependencies(bg utils.BuildGraph) (ActionSet, error) {
	var result ActionSet = base.CopySlice(x...)

	for i := 0; i < len(result); i++ {
		if err := result[i].GetAction().AppendDependentActions(bg, &result); err != nil {
			return nil, err
		}
	}

	return base.ReverseSlice(result...), nil
}
func (x ActionSet) AppendDependencies(bg utils.BuildGraph, result *ActionSet) error {
	before := len(*result)
	result.AppendUniq(x...)

	for i := before; i < len(*result); i++ {
		if err := (*result)[i].GetAction().AppendDependentActions(bg, result); err != nil {
			return err
		}
	}

	base.ReverseSlice((*result)[before:]...)
	return nil
}
func (x ActionSet) GetOutputFiles() (result utils.FileSet) {
	for _, action := range x {
		result.Append(action.GetAction().OutputFiles...)
	}
	return result
}
func (x ActionSet) GetExportFiles() (results utils.FileSet) {
	results = make(utils.FileSet, len(x))
	for i, action := range x {
		results[i] = action.GetAction().GetGeneratedFile()
	}
	return
}

func FindBuildAction(alias ActionAlias) (Action, error) {
	return utils.FindGlobalBuildable[Action](alias.Alias())
}

func ForeachBuildAction(bg utils.BuildGraph, each func(utils.BuildNode, Action) error) error {
	return bg.Range(func(ba utils.BuildAlias, bn utils.BuildNode) error {
		switch buildable := bn.GetBuildable().(type) {
		case Action:
			return each(bn, buildable)
		}
		return nil
	})
}

func GetBuildActions(aliases ...ActionAlias) (ActionSet, error) {
	result := make(ActionSet, len(aliases))
	for i, alias := range aliases {
		if action, err := FindBuildAction(alias); err == nil {
			base.Assert(func() bool { return nil != action })
			result[i] = action
		} else {
			return ActionSet{}, err
		}
	}
	return result, nil
}
