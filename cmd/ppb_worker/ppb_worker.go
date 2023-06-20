package main

import (
	"github.com/poppolopoppo/ppb/app"
	"github.com/poppolopoppo/ppb/cluster"
	"github.com/poppolopoppo/ppb/utils"
)

var CommandWork = utils.NewCommand(
	"Worker", "work",
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

func main() {
	source, _ := utils.UFS.GetCallerFile(0)

	app.WithCommandEnv("worker", source, func(env *utils.CommandEnvT) error {
		env.LoadConfig()

		err := env.Run()

		env.SaveConfig()
		return err
	})
}
