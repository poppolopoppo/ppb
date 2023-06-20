package cmd

import (
	"fmt"
	"io"
	"strings"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/compile"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/internal/io"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * FBuild command
 ***************************************/

type FBuildCommand struct {
	Targets []TargetAlias
	Any     BoolVar
	Args    FBuildArgs
}

var CommandFBuild = NewCommandable(
	"Compilation",
	"fbuild",
	"launch FASTBuild compilation process",
	&FBuildCommand{})

func (x *FBuildCommand) Flags(cfv CommandFlagsVisitor) {
	cfv.Variable("Any", "will build any unit matching the given args", &x.Any)
	x.Args.Flags(cfv)
}
func (x *FBuildCommand) Init(cc CommandContext) error {
	cc.Options(
		OptionCommandParsableFlags("CommandFBuild", "optional flags to pass to FASTBuild when compiling", x),
		OptionCommandAllCompilationFlags(),
		OptionCommandConsumeMany("TargetAlias", "build all targets specified as argument", &x.Targets),
	)
	return nil
}
func (x *FBuildCommand) Prepare(cc CommandContext) error {
	// prepare source control early on, without blocking
	BuildSourceControlModifiedFiles(UFS.Source).Prepare(CommandEnv.BuildGraph())
	return nil
}
func (x *FBuildCommand) Run(cc CommandContext) error {
	if x.Any.Get() {
		targetGlobs := StringSet{}

		units, err := NeedAllBuildUnits(CommandEnv.BuildGraph().GlobalContext())
		if err != nil {
			return err
		}

		for _, target := range x.Targets {
			input := strings.ToUpper(target.String())
			for _, unit := range units {
				targetName := unit.TargetAlias.String()
				LogDebug(LogFBuild, "check target <%v> against input %q", targetName, input)
				if strings.Contains(strings.ToUpper(targetName), input) {
					targetGlobs = append(targetGlobs, targetName)
				}
			}
		}

		if len(targetGlobs) == 0 {
			LogFatal("fbuild: no target matching [ %v ]", strings.Join(targetGlobs, ", "))
		}
	}

	sourceControlModifiedFiles := BuildSourceControlModifiedFiles(UFS.Source).Build(CommandEnv.BuildGraph())
	if err := sourceControlModifiedFiles.Failure(); err != nil {
		return err
	}

	if err := UFS.CreateBuffered(UFS.Saved.File(".modified_files_list.txt"), func(w io.Writer) error {
		for _, file := range sourceControlModifiedFiles.Success().ModifiedFiles {
			if _, err := fmt.Fprintln(w, file.String()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	fbuild := MakeFBuildExecutor(&x.Args, Stringize(x.Targets...)...)
	return fbuild.Run()
}
