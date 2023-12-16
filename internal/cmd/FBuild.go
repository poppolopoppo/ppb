package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * FBuild command
 ***************************************/

type FBuildCommand struct {
	Targets []compile.TargetAlias
	Any     utils.BoolVar
	Args    internal_io.FBuildArgs
}

var CommandFBuild = utils.NewCommandable(
	"Compilation",
	"fbuild",
	"launch FASTBuild compilation process",
	&FBuildCommand{})

func (x *FBuildCommand) Flags(cfv utils.CommandFlagsVisitor) {
	cfv.Variable("Any", "will build any unit matching the given args", &x.Any)
	x.Args.Flags(cfv)
}
func (x *FBuildCommand) Init(cc utils.CommandContext) error {
	cc.Options(
		utils.OptionCommandParsableFlags("CommandFBuild", "optional flags to pass to FASTBuild when compiling", x),
		compile.OptionCommandAllCompilationFlags(),
		utils.OptionCommandConsumeMany("TargetAlias", "build all targets specified as argument", &x.Targets),
	)
	return nil
}
func (x *FBuildCommand) Prepare(cc utils.CommandContext) error {
	// prepare source control early on, without blocking
	utils.BuildSourceControlModifiedFiles(utils.UFS.Source).Prepare(utils.CommandEnv.BuildGraph())
	return nil
}
func (x *FBuildCommand) Run(cc utils.CommandContext) error {
	if x.Any.Get() {
		targetGlobs := base.StringSet{}

		units, err := compile.NeedAllBuildUnits(utils.CommandEnv.BuildGraph().GlobalContext())
		if err != nil {
			return err
		}

		for _, target := range x.Targets {
			input := strings.ToUpper(target.String())
			for _, unit := range units {
				targetName := unit.TargetAlias.String()
				base.LogDebug(internal_io.LogFBuild, "check target <%v> against input %q", targetName, input)
				if strings.Contains(strings.ToUpper(targetName), input) {
					targetGlobs = append(targetGlobs, targetName)
				}
			}
		}

		if len(targetGlobs) == 0 {
			base.LogFatal("fbuild: no target matching [ %v ]", strings.Join(targetGlobs, ", "))
		}
	}

	sourceControlModifiedFiles := utils.BuildSourceControlModifiedFiles(utils.UFS.Source).Build(utils.CommandEnv.BuildGraph())
	if err := sourceControlModifiedFiles.Failure(); err != nil {
		return err
	}

	if err := utils.UFS.CreateBuffered(utils.UFS.Saved.File(".modified_files_list.txt"), func(w io.Writer) error {
		for _, file := range sourceControlModifiedFiles.Success().ModifiedFiles {
			if _, err := fmt.Fprintln(w, file.String()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	fbuild := internal_io.MakeFBuildExecutor(&x.Args, base.Stringize(x.Targets...)...)
	return fbuild.Run()
}
