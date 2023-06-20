package app

import (
	"os"
	"time"

	"github.com/poppolopoppo/ppb/internal/hal"
	"github.com/poppolopoppo/ppb/utils"
)

func WithCommandEnv(prefix string, caller utils.Filename, scope func(*utils.CommandEnvT) error) error {
	startedAt := time.Now()

	defer utils.StartTrace()()
	defer utils.PurgePinnedLogs()

	utils.UFS.Caller = caller

	env := utils.InitCommandEnv(prefix, os.Args[1:], startedAt)

	defer utils.StartProfiling()()

	hal.InitHAL()
	utils.InitUtils()

	err := scope(env)

	if err != nil {
		utils.LogForwardln("")
		utils.LogError(utils.LogCommand, "%v", err)
	}
	return err
}
