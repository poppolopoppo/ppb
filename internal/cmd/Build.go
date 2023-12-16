package cmd

import (
	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/cluster"
	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"

	//lint:ignore ST1001 ignore dot imports warning
	"github.com/poppolopoppo/ppb/utils"
)

type BuildCommand struct {
	Targets []compile.TargetAlias
	Clean   utils.BoolVar
	Glob    utils.BoolVar
	Rebuild utils.BoolVar
}

var CommandBuild = utils.NewCommandable(
	"Compilation",
	"build",
	"launch action compilation process",
	&BuildCommand{
		Clean:   base.INHERITABLE_FALSE,
		Glob:    base.INHERITABLE_FALSE,
		Rebuild: base.INHERITABLE_FALSE,
	})

func (x *BuildCommand) Flags(cfv utils.CommandFlagsVisitor) {
	cfv.Variable("Clean", "erase all by files outputted by selected actions", &x.Clean)
	cfv.Variable("Glob", "treat provided targets as glob expressions", &x.Glob)
	cfv.Variable("Rebuild", "rebuild selected actions, same as building after a clean", &x.Rebuild)
	action.GetActionFlags().Flags(cfv)
}
func (x *BuildCommand) Init(ci utils.CommandContext) error {
	ci.Options(
		compile.OptionCommandAllCompilationFlags(),
		utils.OptionCommandParsableFlags("BuildCommand", "control compilation actions execution", x),
		utils.OptionCommandParsableAccessor("ClusterFlags", "action distribution in network cluster", cluster.GetClusterFlags),
		utils.OptionCommandParsableAccessor("WorkerFlags", "set hardware limits for local action compilation", cluster.GetWorkerFlags),
		utils.OptionCommandConsumeMany("TargetAlias", "build all targets specified as argument", &x.Targets),
	)
	return nil
}
func (x *BuildCommand) Prepare(cc utils.CommandContext) error {
	actionFlags := action.GetActionFlags()

	// async prepare action cache early if cache is enabled
	if actionFlags.CacheMode.HasRead() || actionFlags.CacheMode.HasWrite() {
		go action.GetActionCache()
	}

	// async prepare action distribution early if distrubution is enabled
	if actionFlags.DistMode.Enabled() {
		go action.GetActionDist()
	}

	return nil
}
func (x *BuildCommand) Run(cc utils.CommandContext) error {
	base.LogClaim(utils.LogCommand, "build <%v>...", base.JoinString(">, <", x.Targets...))

	bg := utils.CommandEnv.BuildGraph()

	// select target that match input by globbing
	if x.Glob.Get() {
		units, err := compile.NeedAllBuildUnits(bg.GlobalContext())
		if err != nil {
			return err
		}

		re := utils.MakeGlobRegexp(base.Stringize(x.Targets...)...)

		// overwrite user input with matching targets found
		for _, unit := range units {
			if re.MatchString(unit.TargetAlias.String()) {
				x.Targets = append(x.Targets, unit.TargetAlias)
			}
		}
	} else {
		// correct case errors by default

		for i, target := range x.Targets {
			// verify module path and correct case if necessary
			if module, err := compile.FindBuildModule(target.ModuleAlias); err == nil {
				target.ModuleAlias = module.GetModule().ModuleAlias
			} else {
				return err
			}

			// verify configuration name and correct case if necessary
			if cfg, err := compile.FindConfiguration(target.ConfigName); err == nil {
				target.ConfigurationAlias = cfg.GetConfig().ConfigurationAlias
			} else {
				return err
			}

			// verify platform name and correct case if necessary
			if plf, err := compile.FindPlatform(target.PlatformName); err == nil {
				target.PlatformAlias = plf.GetPlatform().PlatformAlias
			} else {
				return err
			}

			x.Targets[i] = target
		}
	}

	// select target that exactly match input,
	targetActions, err := compile.NeedTargetActions(bg.GlobalContext(), x.Targets...)
	if err != nil {
		return err
	}

	if x.Clean.Get() || x.Rebuild.Get() {
		if err := x.cleanBuild(bg, targetActions); err != nil {
			return err
		}
	}

	if !x.Clean.Get() || x.Rebuild.Get() {
		if err := x.doBuild(bg, targetActions); err != nil {
			return err
		}
	}

	return nil
}
func (x *BuildCommand) doBuild(bg utils.BuildGraph, targets []*compile.TargetActions) error {
	aliases := utils.BuildAliases{}
	for _, ta := range targets {
		if tp, err := ta.GetOutputPayload(); err == nil {
			aliases.Append(tp.Alias())
			base.LogVerbose(utils.LogCommand, "selected <%v> actions: %v", tp.Alias(), tp.ActionAliases)
		} else {
			return err
		}
	}

	_, err := bg.BuildMany(aliases,
		utils.OptionBuildForceIf(x.Rebuild.Get()),
		utils.OptionWarningOnMissingOutputIf(!x.Rebuild.Get()))
	return err
}
func (x *BuildCommand) cleanBuild(bg utils.BuildGraph, targets []*compile.TargetActions) error {
	aliases := utils.BuildAliases{}
	for _, ta := range targets {
		if err := ta.ForeachPayload(func(tp *compile.TargetPayload) error {
			aliases.Append(tp.ActionAliases...)
			return nil
		}); err != nil {
			return err
		}
	}

	actions, err := action.GetBuildActions(aliases)
	if err != nil {
		return err
	}

	expandeds := action.ActionSet{}
	actions.ExpandDependencies(bg, &expandeds)

	filesToDelete, err := bg.GetDependencyOutputFiles(expandeds.Aliases()...)
	if err != nil {
		return err
	}

	pbar := base.LogProgress(0, 0, "clean %d files from %d actions", len(filesToDelete), len(expandeds))
	defer pbar.Close()

	return base.ParallelRange(func(file utils.Filename) error {
		distCleanFile(file)
		pbar.Inc()
		return nil
	}, filesToDelete...)
}
