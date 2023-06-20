package compile

import (
	"fmt"
	"strings"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var LogAction = NewLogCategory("Action")

/***************************************
 * ActionFlags
 ***************************************/

type ActionFlags struct {
	CacheMode    CacheModeType
	CachePath    Directory
	DistMode     DistModeType
	ResponseFile CompilerSupportType
	ShowCmds     BoolVar
	ShowFiles    BoolVar
	ShowOutput   BoolVar
}

func (x *ActionFlags) Flags(cfv CommandFlagsVisitor) {
	cfv.Persistent("CacheMode", "use input hashing to store/retrieve action outputs ["+JoinString(",", CacheModeTypes()...)+"]", &x.CacheMode)
	cfv.Persistent("CachePath", "set path used to store cached actions", &x.CachePath)
	cfv.Persistent("DistMode", "distribute actions to a cluster of remote workers ["+JoinString(",", DistModeTypes()...)+"]", &x.DistMode)
	cfv.Persistent("ResponseFile", "control response files usage ["+JoinString(",", CompilerSupportTypes()...)+"]", &x.ResponseFile)
	cfv.Variable("ShowCmds", "print executed compilation commands", &x.ShowCmds)
	cfv.Variable("ShowFiles", "print file accesses for external commands", &x.ShowFiles)
	cfv.Variable("ShowOutput", "always show compilation commands output", &x.ShowOutput)
}

var GetActionFlags = NewCommandParsableFlags(&ActionFlags{
	CacheMode:    CACHE_NONE,
	CachePath:    UFS.Cache,
	DistMode:     DIST_NONE,
	ResponseFile: COMPILERSUPPORT_ALLOWED,
	ShowCmds:     INHERITABLE_FALSE,
	ShowFiles:    INHERITABLE_FALSE,
	ShowOutput:   INHERITABLE_FALSE,
})

/***************************************
 * Action
 ***************************************/

type Action interface {
	GetAction() *ActionRules
	DependsOn(actions ...Action)
	Buildable
	fmt.Stringer
}

type ActionRules struct {
	TargetAlias  TargetAlias
	Payload      PayloadType
	Executable   Filename
	WorkingDir   Directory
	Environment  ProcessEnvironment
	CacheMode    CacheModeType
	DistMode     DistModeType
	ResponseFile CompilerSupportType
	Inputs       FileSet
	Outputs      FileSet
	Exports      FileSet
	Extras       FileSet
	Arguments    StringSet
	Dependencies BuildAliases
}

func (x *ActionRules) Alias() BuildAlias {
	return MakeBuildAlias("Action", x.Outputs.Join(";"))
}
func (x *ActionRules) Build(bc BuildContext) error {
	// track inputs that are generated by another action
	if err := bc.NeedFile(x.Inputs...); err != nil {
		return err
	}

	// consolidate output files
	outputFiles := FileSet{}
	outputFiles.AppendUniq(x.Outputs...) // some entries could be shared between the 3 sets
	outputFiles.AppendUniq(x.Exports...)
	outputFiles.AppendUniq(x.Extras...)
	outputFiles.Sort()

	// check if we can read action cache
	var cacheKey ActionCacheKey
	wasRetrievedFromCache := false

	flags := GetActionFlags()
	if flags.CacheMode.HasRead() && x.CacheMode.HasRead() {
		var err error
		if cacheKey, err = GetActionCache().CacheRead(x, outputFiles, OptionBuildStruct(bc.Options())); err == nil {
			wasRetrievedFromCache = true // cache-hit
			bc.Annotate(`CACHE`)
		}
	}

	// run process if the cache missed
	if !wasRetrievedFromCache {
		var processOptions ProcessOptions
		var readFiles FileSet

		// run the external process with action command-line and file access hooking
		processOptions.Init(
			OptionProcessNoSpinner,
			OptionProcessEnvironment(x.Environment),
			OptionProcessWorkingDir(x.WorkingDir),
			OptionProcessCaptureOutputIf(flags.ShowOutput.Get()),
			OptionProcessUseResponseFileIf(flags.ResponseFile.Enabled() && x.ResponseFile.Enabled()),
			OptionProcessFileAccess(func(far FileAccessRecord) error {
				if flags.ShowFiles.Get() || IsLogLevelActive(LOG_VERYVERBOSE) {
					LogForwardf("%v: [%s]  %s", MakeStringer(func() string {
						return x.Alias().String()
					}), far.Access, far.Path)
				}

				// only file access read/execute: output files could messed with writable mapped system Dll on Windows :'(
				if far.Access.HasRead() && !(far.Access.HasWrite() || far.Access.HasExecute()) {
					if !x.Inputs.Contains(far.Path) &&
						!x.Outputs.Contains(far.Path) &&
						!x.Exports.Contains(far.Path) &&
						!x.Extras.Contains(far.Path) {
						readFiles.AppendUniq(far.Path)
					}
				}

				return nil
			}))

		// check action and environment parameters allow for caching
		wasDistributed := false
		if flags.DistMode.Enabled() && x.DistMode.Enabled() {

			// check if process can be distributed in remote worker cluster
			if actionDist := GetActionDist(); actionDist.CanDistribute(flags.DistMode.Forced() || x.DistMode.Forced()) {
				peer, err := actionDist.DistributeAction(x.Alias(), x.Executable, x.Arguments, &processOptions)

				if wasDistributed = (peer != nil); wasDistributed {
					bc.Annotate(peer.GetAddress())
					if err != nil {
						return err
					}
				}
			}
		}

		// run process locally if it was not distributed
		if !wasDistributed {

			// limit number of concurrent external processes with MakeGlobalWorkerFuture()
			future := MakeGlobalWorkerFuture(func(tc ThreadContext) (int, error) {
				// check if we should log executed command-line
				if flags.ShowCmds.Get() {
					LogForwardln("\"", x.Executable.String(), "\" \"", strings.Join(x.Arguments, "\" \""), "\"")
				}

				bc.Annotate(fmt.Sprintf("Thread:%d/%d", tc.GetThreadId()+1, tc.GetThreadPool().GetArity()))
				return 0, RunProcess(x.Executable, x.Arguments, OptionProcessStruct(&processOptions))
			})

			if err := future.Join().Failure(); err != nil {
				return err
			}
		}

		if err := bc.NeedFile(readFiles...); err != nil {
			return err
		}
	}

	// check if this action should be cached-in
	if flags.CacheMode.HasWrite() && x.CacheMode.HasWrite() && !wasRetrievedFromCache /* if we compiled this action */ {
		bc.OnBuilt(func(node BuildNode) error {
			// queue a new asynchronous task to avoid blocking the buildgraph
			Assert(func() bool { return GetActionCache().(*actionCache).makeActionKey(x) == cacheKey })
			return GetActionCache().AsyncCacheWrite(node, cacheKey, outputFiles, OptionBuildStruct(bc.Options()))
		})
	} else if flags.CacheMode.HasWrite() && !wasRetrievedFromCache {
		LogVeryVerbose(LogActionCache, "skipped cache write for %q (cache-mode=%v)", x.Alias(), x.CacheMode)
	}

	// check that process did write expected files and track them as outputs
	return bc.OutputFile(outputFiles...)
}

func (x *ActionRules) GetAction() *ActionRules { return x }
func (x *ActionRules) DependsOn(actions ...Action) {
	for _, other := range actions {
		x.Dependencies.AppendUniq(other.Alias())
	}
}
func (x *ActionRules) Serialize(ar Archive) {
	ar.Serializable(&x.TargetAlias)
	ar.Serializable(&x.Payload)
	ar.Serializable(&x.Executable)
	ar.Serializable(&x.WorkingDir)
	ar.Serializable(&x.Environment)

	ar.Serializable(&x.CacheMode)
	ar.Serializable(&x.DistMode)
	ar.Serializable(&x.ResponseFile)

	ar.Serializable(&x.Inputs)
	ar.Serializable(&x.Arguments)

	ar.Serializable(&x.Outputs)
	ar.Serializable(&x.Exports)
	ar.Serializable(&x.Extras)

	SerializeSlice(ar, x.Dependencies.Ref())
}
func (x *ActionRules) String() string {
	oss := strings.Builder{}
	fmt.Fprintf(&oss, "%q", x.Executable)
	for _, arg := range x.Arguments {
		fmt.Fprintf(&oss, " %q", arg)
	}
	return oss.String()
}

/***************************************
 * Action Set
 ***************************************/

type ActionSet []Action

func (x ActionSet) Aliases() BuildAliases {
	return MakeBuildAliases(x...)
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
	Assert(func() bool {
		for _, it := range actions {
			action := it.GetAction()
			AssertMessage(func() bool { return len(action.Inputs) > 0 }, "%v: action without input", action.Alias())
			AssertMessage(func() bool { return len(action.Outputs) > 0 }, "%v: action without output", action.Alias())
		}
		return true
	})

	*x = AppendComparable_CheckUniq(*x, actions...)
}
func (x *ActionSet) DependsOn(actions ...Action) {
	if len(actions) == 0 {
		return
	}
	for _, action := range *x {
		action.DependsOn(actions...)
	}
}
func (x ActionSet) ExpandDependencies(result *ActionSet) error {
	for _, action := range x {
		if !result.Contains(action) {
			if actions, err := GetBuildActions(action.GetAction().Dependencies); err == nil {
				if err := actions.ExpandDependencies(result); err == nil {
					result.Append(action)
				} else {
					return err
				}
			} else {
				return err
			}
		}
	}
	return nil
}
func (x ActionSet) GetOutputFiles() (result FileSet) {
	for _, action := range x {
		result.Append(action.GetAction().Outputs...)
	}
	return result
}
func (x ActionSet) GetExportFiles() (result FileSet) {
	for _, action := range x {
		result.Append(action.GetAction().Exports...)
	}
	return result
}
