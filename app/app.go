package app

import (
	"os"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/internal/hal"
	"github.com/poppolopoppo/ppb/utils"
)

func WithCommandEnv(prefix string, caller utils.Filename, scope func(*utils.CommandEnvT) error) error {
	startedAt := time.Now()

	defer base.StartTrace()()
	defer base.PurgePinnedLogs()

	utils.UFS.Caller = caller

	env, err := utils.InitCommandEnv(prefix, os.Args[1:], startedAt)
	if err == nil {
		defer utils.StartProfiling()()

		hal.InitHAL()
		utils.InitUtils()

		defer func() {
			if er := env.Close(); er != nil && err == nil {
				err = er
			}
		}()
		err = scope(env)
	}

	if err != nil {
		base.LogForwardln("")
		base.LogError(utils.LogCommand, "%v", err)
	}
	return err
}
