package cmd

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

var CommandCheckBuild = newCompletionCommand[utils.BuildAlias, *utils.BuildAlias](
	"Debug",
	"check-build",
	"build graph aliases passed as input parameters",
	func(cc utils.CommandContext, ca *CompletionArgs[utils.BuildAlias, *utils.BuildAlias]) error {
		bg := utils.CommandEnv.BuildGraph()

		// build all nodes found
		result := bg.BuildMany(ca.Inputs, utils.OptionBuildForceIf(utils.GetCommandFlags().Force.Get()))
		return result.Join().Failure()
	})

var CommandCheckCache = utils.NewCommand(
	"Debug",
	"check-cache",
	"inspect action cache content validity and clean invalid/outdated entries",
	utils.OptionCommandRun(func(cc utils.CommandContext) error {
		cache := action.GetActionCache()
		cachePath := cache.GetCachePath()

		tempPath := utils.UFS.Transient.Folder("check-cache")
		utils.UFS.Mkdir(tempPath)
		defer os.RemoveAll(tempPath.String())

		cacheGlob := utils.MakeGlobRegexp("*" + cache.GetEntryExtname())
		expireTime := time.Now().AddDate(0, -1, 0) // remove all cached entries older than 1 month

		return cachePath.MatchFilesRec(func(f utils.Filename) error {
			base.GetGlobalThreadPool().Queue(func(base.ThreadContext) {
				var entry action.ActionCacheEntry

				removeCacheEntry := false
				defer func() {
					if !removeCacheEntry {
						return
					}
					base.LogWarning(utils.LogCommand, "remove %q cache entry")
					for _, bulk := range entry.Bulks {
						utils.UFS.Remove(bulk.Path)
					}

					utils.UFS.Remove(f)
				}()

				base.LogDebug(utils.LogCommand, "found cache entry %q", f)
				if err := entry.Load(f); err != nil {
					base.LogError(utils.LogCommand, "%v", err)
					removeCacheEntry = true
				}

				base.LogVerbose(utils.LogCommand, "read cache entry %q with key %s and %d bulks", f, entry.Key.GetFingerprint(), len(entry.Bulks))
				for i := 0; i < len(entry.Bulks); {
					removeBulk := false
					bulk := &entry.Bulks[i]

					dst := tempPath.Folder(bulk.Path.ReplaceExt("").Basename)
					defer os.RemoveAll(dst.String())

					if artifacts, err := bulk.Inflate(dst); err == nil {
						for _, it := range artifacts {
							utils.UFS.Remove(it)
						}
					} else {
						base.LogError(utils.LogCommand, "%v", err)
						removeBulk = true
					}

					if !removeBulk { // expire cache entries
						removeBulk = removeBulk || utils.UFS.MTime(bulk.Path).Before(expireTime)
					}

					if removeBulk {
						base.LogVerbose(utils.LogCommand, "remove cache bulk %q", bulk)
						utils.UFS.Remove(bulk.Path)

						if i+1 < len(entry.Bulks) {
							entry.Bulks[i] = entry.Bulks[len(entry.Bulks)-1]
						}
						entry.Bulks = entry.Bulks[:len(entry.Bulks)-1]
					} else {
						i++
					}
				}

				if len(entry.Bulks) == 0 {
					removeCacheEntry = true
				}
			})
			return nil
		}, cacheGlob)
	}))

var CheckFingerprint = newCompletionCommand[utils.BuildAlias, *utils.BuildAlias](
	"Debug",
	"check-fingerprint",
	"recompute nodes fingerprint and see if they match with the stamp stored in build graph",
	func(cc utils.CommandContext, ca *CompletionArgs[utils.BuildAlias, *utils.BuildAlias]) error {
		bg := utils.CommandEnv.BuildGraph()

		for _, a := range ca.Inputs {
			base.LogVerbose(utils.LogCommand, "find build graph node named %q", a)

			// find the node associated with this alias
			node := bg.Find(a)
			if node == nil {
				base.LogPanic(utils.LogCommand, "node could not be found %q", a)
			}

			// compute buildable fingerprint and check wether it matches save build stamp or not
			buildable := node.GetBuildable()
			checksum := utils.MakeBuildFingerprint(buildable)
			original := node.GetBuildStamp().Content

			// if save build stamp do not match, we will try to find which property is not stable by rebuilding it
			if original == checksum {
				base.LogInfo(utils.LogCommand, "%q -> OK\n\told: %v\n\tnew: %v", a, original, checksum)
			} else {
				base.LogWarning(utils.LogCommand, "%q -> KO\n\told: %v\n\tnew: %v", a, original, checksum)
			}

			// duplicate original buildable, so we can make a diff after the build
			copy := base.DuplicateObjectForDebug(buildable)

			// build the node and check for errors
			_, future := bg.Build(node, utils.OptionBuildForce)
			result := future.Join()
			if err := result.Failure(); err != nil {
				return err
			}

			base.LogInfo(utils.LogCommand, "%q ->\n\tbuild: %v", a, result.Success().BuildStamp)

			// finally make a diff between the original backup and the updated node after the build
			// -> the diff should issue an error on the property causing the desynchronization
			if err := base.SerializableDiff(copy, result.Success().Buildable); err != nil {
				return err
			}
		}

		return nil
	})

var CheckSerialize = utils.NewCommand(
	"Debug",
	"check-serialize",
	"write and load every node, then check for differences",
	utils.OptionCommandRun(func(cc utils.CommandContext) error {
		bg := utils.CommandEnv.BuildGraph()
		aliases := bg.Aliases()

		pbar := base.LogProgress(0, len(aliases), "check-serialize")
		defer pbar.Close()

		ar := base.NewArchiveDiff()
		defer ar.Close()

		type Stats struct {
			Num      int32
			Size     uint64
			Duration time.Duration
		}
		perClass := map[reflect.Type]Stats{}

		for _, a := range aliases {
			node := bg.Find(a)
			buildable := node.GetBuildable()

			bench := base.LogBenchmark(utils.LogCommand, "%10s bytes   %T -> %q", base.MakeStringer(func() string {
				return fmt.Sprint(ar.Len())
			}), buildable, a)

			// compare buildable with self: if an error happens then Serialize() has a bug somewhere
			// err should contain a stack of serialized objects, and hopefuly ease debugging
			if err := ar.Diff(buildable, buildable); err != nil {
				return err
			}

			duration := bench.Close()
			pbar.Inc()

			stats := perClass[reflect.TypeOf(buildable)]
			stats.Num++
			stats.Size += uint64(ar.Len())
			stats.Duration += duration

			perClass[reflect.TypeOf(buildable)] = stats
		}

		classBySize := base.Keys(perClass)
		sort.Slice(classBySize, func(i, j int) bool {
			return perClass[classBySize[i]].Size > perClass[classBySize[j]].Size
		})

		if len(classBySize) > 30 {
			classBySize = classBySize[:30]
		}

		for _, class := range classBySize {
			stats := perClass[class]
			base.LogInfo(utils.LogCommand, "%6d elts - avg: %10.3f b - total: %10.3f KiB -> %8.3f MiB/s  -  %v",
				stats.Num,
				float64(stats.Size)/float64(stats.Num),
				base.Kibibytes(stats.Size),
				base.Mebibytes(stats.Size)/(float64(stats.Duration.Seconds())),
				class)
		}

		return nil
	}))

var DependencyChain = newCompletionCommand[utils.BuildAlias, *utils.BuildAlias](
	"Debug",
	"dependency-chain",
	"find shortest dependency chain between 2 nodes",
	func(cc utils.CommandContext, ca *CompletionArgs[utils.BuildAlias, *utils.BuildAlias]) error {
		if len(ca.Inputs) < 2 {
			return fmt.Errorf("dependency-chain: must pass at least 2 targets")
		}

		// print the dependency chain found
		return openCompletion(ca, func(w io.Writer) error {

			for i := 1; i < len(ca.Inputs); i++ {
				// build graph will use Dijkstra to find the shortest path between the 2 nodes
				buildGraph := utils.CommandEnv.BuildGraph()
				chain, err := buildGraph.GetDependencyChain(
					utils.BuildAlias(ca.Inputs[0]),
					utils.BuildAlias(ca.Inputs[i]))
				if err != nil {
					return err
				}

				indent := ""
				for i, link := range chain {
					base.WithoutLog(func() {
						fmt.Fprintf(w, "%s[%d] %s: %s", indent, i, link.Type, link.Alias)
						if base.IsLogLevelActive(base.LOG_VERBOSE) {
							node := buildGraph.Find(link.Alias)
							fmt.Fprintf(w, " -> %v", node.GetBuildStamp())
						}
						fmt.Fprintln(w)
					})
					indent += "  "
				}
			}

			return nil
		})
	})

var DependencyFiles = newCompletionCommand[utils.BuildAlias, *utils.BuildAlias](
	"Debug",
	"dependency-files",
	"list all file dependencies for input nodes",
	func(cc utils.CommandContext, ca *CompletionArgs[utils.BuildAlias, *utils.BuildAlias]) error {
		// print the dependency chain found
		return openCompletion(ca, func(w io.Writer) error {
			bg := utils.CommandEnv.BuildGraph()
			var all utils.FileSet

			for _, a := range ca.Inputs {
				files, err := bg.GetDependencyInputFiles(a)
				if err != nil {
					return err
				}

				all.AppendUniq(files...)
			}

			all.Sort()

			for _, filename := range all {
				fmt.Fprintln(w, filename)
			}
			return nil
		})
	})

var ProgressBar = utils.NewCommand(
	"Debug",
	"progress-bar",
	"print several progress bars for debugging",
	utils.OptionCommandRun(func(cc utils.CommandContext) error {
		spinner := base.LogSpinner("spinner")
		defer spinner.Close()

		basecol := base.NewColor3f(.5, .5, .5)

		gradients := [6]strings.Builder{}

		for i := 0; i < 100; i++ {
			f := float64(i) / 100.0
			c := basecol.Brightness(f)

			gradients[0].WriteString(c.Quantize(true).Ansi(true))
			gradients[0].WriteRune('▇')

			gradients[1].WriteString(c.LinearToSrgb().Quantize(true).Ansi(true))
			gradients[1].WriteRune('▇')

			c.Broadcast(f)

			gradients[2].WriteString(c.Quantize(true).Ansi(true))
			gradients[2].WriteRune('▇')

			c.Broadcast(f)

			gradients[3].WriteString(c.LinearToSrgb().Quantize(true).Ansi(true))
			gradients[3].WriteRune('▇')

			c = base.NewPastelizerColor(f)

			gradients[4].WriteString(c.Quantize(true).Ansi(true))
			gradients[4].WriteRune('▇')

			c = base.NewHeatmapColor(f)

			gradients[5].WriteString(c.Quantize(true).Ansi(true))
			gradients[5].WriteRune('▇')
		}
		for _, gradient := range gradients {
			gradient.WriteRune('\n')
			gradient.WriteString(base.ANSI_RESET.String())

			base.LogForward(gradient.String())
		}

		white := base.NewColor3f(1, 1, 1).Quantize(true)
		black := base.NewColor3f(0, 0, 0).Quantize(true)

		base.LogForwardln(
			white.Ansi(true),
			black.Ansi(false),
			"White On Black", base.ANSI_RESET.String())
		base.LogForwardln(
			white.Ansi(false),
			black.Ansi(true),
			"Black On White", base.ANSI_RESET.String())
		base.LogForwardln(
			base.NewColor3f(1, 0, 0).Quantize(true).Ansi(true),
			base.NewColor3f(0, 0, 1).Quantize(true).Ansi(false),
			"Red On Blue", base.ANSI_RESET.String())
		base.LogForwardln(
			base.NewColor3f(1, 1, 0).Quantize(true).Ansi(true),
			base.NewColor3f(0, 1, 0).Quantize(true).Ansi(false),
			"Yellow On Green", base.ANSI_RESET.String())
		base.LogForwardln(
			base.NewColor3f(1, 0.1, 0.1).Brightness(0.8).Quantize(true).Ansi(true),
			base.NewColor3f(0.1, 0.1, 1).Brightness(0.3).Quantize(true).Ansi(false),
			"Light Red On Dark Blue", base.ANSI_RESET.String())

		go func() {
			tick := time.NewTicker(10 * time.Millisecond)
			for {
				pbar := base.LogProgress(0, 1000, "linear")
				for i := 0; i < 1000; i++ {
					<-tick.C
					pbar.Inc()
				}
				pbar.Close()
			}
		}()

		go func() {
			tick := time.NewTicker(10 * time.Millisecond)
			for {
				pbar := base.LogProgress(0, 333, "linear2")
				for i := 0; i < 333; i++ {
					<-tick.C
					pbar.Inc()
				}
				pbar.Close()
			}
		}()

		for {
			n := int(10 + rand.Uint32()%50)
			pbar := base.LogProgress(0, n, "progress")

			for i := 0; i < n; i++ {
				n2 := int(10 + rand.Uint32()%100)
				pbar2 := base.LogProgress(0, n2, "progress2")

				spinner2 := base.LogSpinner("spinner")

				for j := 0; j < n2; j++ {
					pbar2.Inc()
					spinner.Inc()
					spinner2.Inc()
					time.Sleep(time.Millisecond * 10)
				}

				pbar.Inc()
				time.Sleep(time.Millisecond * 50)
				pbar2.Close()
				spinner2.Close()
			}

			pbar.Close()
		}
	}))

var TestClient = utils.NewCommand(
	"Debug",
	"test_client",
	"internal command for debugging distribution'",
	utils.OptionCommandRun(func(cc utils.CommandContext) error {
		dist := action.GetActionDist()
		tick := time.NewTicker(3 * time.Second)
		defer tick.Stop()
		for {
			<-tick.C
			dist.GetDistStats()
			// stats.Print()
		}
	}))

var ShowVersion = utils.NewCommand(
	"Debug",
	"version",
	"print build version",
	utils.OptionCommandRun(func(cc utils.CommandContext) error {
		base.LogForwardln(utils.PROCESS_INFO.String())
		return nil
	}))

var ShowSeed = utils.NewCommand(
	"Debug",
	"seed",
	"print build seed",
	utils.OptionCommandRun(func(cc utils.CommandContext) error {
		base.LogForwardln(utils.GetProcessSeed().String())
		return nil
	}))
