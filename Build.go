package ppb

import (
	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/app"
	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/internal/cmd"
	"github.com/poppolopoppo/ppb/internal/hal"
	"github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
)

var LogPPB = base.NewLogCategory("PPB")

/***************************************
 * Launch Command (program entry point)
 ***************************************/

func LaunchCommand(prefix string) error {
	source, err := utils.UFS.GetCallerFile(2)
	if err != nil {
		return err
	}

	return app.WithCommandEnv(prefix, source, func(env *utils.CommandEnvT) error {
		io.InitIO()
		hal.InitCompile()
		action.InitAction()
		compile.InitCompile()
		cmd.InitCmd()

		env.LoadConfig()

		err := env.Run()

		env.SaveConfig()
		if er := env.SaveBuildGraph(); er != nil && err == nil {
			err = er
		}

		return err
	})
}
