package cmd

import (
	"os"

	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

var CommandDistClean = newCompletionCommand(
	"Compilation",
	"distclean",
	"erase generated artifacts",
	func(cc utils.CommandContext, args *CompletionArgs) error {
		if len(args.GlobPatterns) == 0 {
			base.LogClaim(utils.LogCommand, "dist-clean all output folders and database")

			distCleanDir(utils.UFS.Binaries)
			distCleanDir(utils.UFS.Cache)
			distCleanDir(utils.UFS.Generated)
			distCleanDir(utils.UFS.Intermediate)
			distCleanDir(utils.UFS.Projects)
			distCleanDir(utils.UFS.Transient)

			// clean the database, not the config
			distCleanFile(utils.CommandEnv.DatabasePath())

		} else {
			re := utils.MakeGlobRegexp(base.MakeStringerSet(args.GlobPatterns...)...)
			base.LogClaim(utils.LogCommand, "dist-clean all targets matching /%v/", re)

			bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "DistClean"})
			defer bg.Close()

			units, err := compile.NeedAllBuildUnits(bg.GlobalContext())
			if err != nil {
				return err
			}

			for _, unit := range units {
				if re.MatchString(unit.TargetAlias.String()) {
					base.LogInfo(utils.LogCommand, "dist-clean %q build unit", unit.String())

					distCleanDir(unit.GeneratedDir)
					distCleanDir(unit.IntermediateDir)

					unit.OutputFile.Dirname.MatchFiles(func(f utils.Filename) error {
						distCleanFile(f)
						return nil
					}, utils.MakeGlobRegexp(unit.OutputFile.ReplaceExt(".*").Basename))
				}
			}

			return nil
		}

		return nil
	})

func distCleanFile(f utils.Filename) {
	if f.Exists() {
		base.LogVerbose(utils.LogCommand, "remove file '%v'", f)
		err := os.Remove(f.String())
		if err != nil {
			base.LogWarning(utils.LogCommand, "distclean: %v", err)
		}
		f.Invalidate()
	}
}
func distCleanDir(d utils.Directory) {
	if d.Exists() {
		base.LogVerbose(utils.LogCommand, "remove directory '%v'", d)
		err := os.RemoveAll(d.String())
		if err != nil {
			base.LogWarning(utils.LogCommand, "distclean: %v", err)
		}
		d.Invalidate()
	}
}
