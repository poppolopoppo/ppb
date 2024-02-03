package cmd

import (
	"time"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/cluster"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

var DistClient = utils.NewCommand(
	"Distribution",
	"client",
	"internal command for debugging distribution",
	utils.OptionCommandRun(func(cc utils.CommandContext) error {
		dist := action.GetActionDist()
		tick := time.NewTicker(3 * time.Second)
		defer tick.Stop()
		for {
			<-tick.C

			if base.IsLogLevelActive(base.LOG_VERBOSE) {
				dist.GetDistStats().Print()
			}
		}
	}))

var DistWorker = utils.NewCommand(
	"Distribution",
	"worker",
	"listen for incomming requests and execute distributed tasks",
	utils.OptionCommandParsableAccessor("ClusterFlags", "action distribution in network cluster", cluster.GetClusterFlags),
	utils.OptionCommandParsableAccessor("WorkerFlags", "local worker settings", cluster.GetWorkerFlags),
	utils.OptionCommandRun(func(cc utils.CommandContext) error {
		peers := cluster.NewCluster()
		worker, cancel, err := peers.StartWorker()
		if worker != nil {
			utils.CommandEnv.OnExit(func(*utils.CommandEnvT) (err error) {
				cancel()
				return worker.Close()
			})
			if er := worker.Close(); er != nil && err == nil {
				err = er
			}
		}
		return err
	}))
