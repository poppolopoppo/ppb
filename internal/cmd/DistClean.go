package cmd

import (
	"os"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/compile"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var CommandDistClean = NewCommand(
	"Compilation",
	"distclean",
	"erase generated artifacts",
	OptionCommandCompletionArgs(),
	OptionCommandRun(func(cc CommandContext) error {
		args := GetCompletionArgs()
		if len(args.Inputs) == 0 {
			LogClaim(LogCommand, "dist-clean all output folders and database")

			distCleanDir(UFS.Binaries)
			distCleanDir(UFS.Cache)
			distCleanDir(UFS.Generated)
			distCleanDir(UFS.Intermediate)
			distCleanDir(UFS.Projects)
			distCleanDir(UFS.Transient)

			// clean the database, not the config
			distCleanFile(CommandEnv.DatabasePath())

		} else {
			re := MakeGlobRegexp(Stringize(args.Inputs...)...)
			LogClaim(LogCommand, "dist-clean all targets matching /%v/", re)

			units, err := NeedAllBuildUnits(CommandEnv.BuildGraph().GlobalContext())
			if err != nil {
				return err
			}

			for _, unit := range units {
				if re.MatchString(unit.TargetAlias.String()) {
					LogInfo(LogCommand, "dist-clean %q build unit", unit.String())

					distCleanDir(unit.GeneratedDir)
					distCleanDir(unit.IntermediateDir)

					unit.OutputFile.Dirname.MatchFiles(func(f Filename) error {
						distCleanFile(f)
						return nil
					}, MakeGlobRegexp(unit.OutputFile.ReplaceExt(".*").Basename))
				}
			}

			return nil
		}

		return nil
	}))

func distCleanFile(f Filename) {
	if f.Exists() {
		LogVerbose(LogCommand, "remove file '%v'", f)
		err := os.Remove(f.String())
		if err != nil {
			LogWarning(LogCommand, "distclean: %v", err)
		}
		f.Invalidate()
	}
}
func distCleanDir(d Directory) {
	if d.Exists() {
		LogVerbose(LogCommand, "remove directory '%v'", d)
		err := os.RemoveAll(d.String())
		if err != nil {
			LogWarning(LogCommand, "distclean: %v", err)
		}
		d.Invalidate()
	}
}
