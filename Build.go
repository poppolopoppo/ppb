package ppb

import (
	"os"

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

	// exit process with non-zero code to notify of failure outside of this program
	defer func() {
		if err != nil {
			os.Exit(2)
		}
	}()

	err = app.WithCommandEnv(prefix, source, func(env *utils.CommandEnvT) error {
		io.InitIO()
		hal.InitCompile()
		action.InitAction()
		compile.InitCompile()
		cmd.InitCmd()

		return env.Run()
	})
	return err
}
