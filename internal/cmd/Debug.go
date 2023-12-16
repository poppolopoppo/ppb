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
		_, err := bg.BuildMany(ca.Inputs, utils.OptionBuildForceIf(utils.GetCommandFlags().Force.Get()))
		return err
	})

var CommandCheckCache = utils.NewCommand(
	"Debug",
	"check-cache",
	"inspect action cache content validity and clean invalid/outdated entries",
	utils.OptionCommandRun(func(cc utils.CommandContext) error {
		utils.GetCommandFlags().Summary.Enable()

		cache := action.GetActionCache()
		cachePath := cache.GetCachePath()

		tempPath := utils.UFS.Transient.Folder("check-cache")
		utils.UFS.Mkdir(tempPath)
		defer os.RemoveAll(tempPath.String())

		cacheGlob := utils.MakeGlobRegexp("*" + cache.GetEntryExtname())
		expireTime := time.Now().AddDate(0, -1, 0) // remove all cached entries older than 1 month

		numEntries := 0
		numBulks := 0
		numArtifacts := 0
		numDeletedBulks := 0
		numDeletedEntries := 0
		defer func() {
			base.GetGlobalThreadPool().Join()
			base.LogForwardf("\nFound %d cache entries with %d bulks (%d artifacts)",
				numEntries, numBulks, numArtifacts)
			if numDeletedBulks > 0 || numDeletedEntries > 0 {
				base.LogForwardf(
					"Deleted %d cache entries and %d bulks",
					numDeletedEntries, numDeletedBulks)
			}
		}()

		return cachePath.MatchFilesRec(func(f utils.Filename) error {
			base.GetGlobalThreadPool().Queue(func(base.ThreadContext) {
				numEntries++

				var entry action.ActionCacheEntry

				removeCacheEntry := false
				defer func() {
					if !removeCacheEntry {
						return
					}
					numDeletedEntries++

					base.LogWarning(utils.LogCommand, "remove %q cache entry")
					for _, bulk := range entry.Bulks {
						utils.UFS.Remove(bulk.Path)
					}

					utils.UFS.Remove(f)
				}()

				base.LogDebug(utils.LogCommand, "found cache entry %q", f)
				if err := entry.OpenEntry(f); err != nil {
					base.LogError(utils.LogCommand, "%v", err)
					removeCacheEntry = true
				}

				numBulks += len(entry.Bulks)

				base.LogVerbose(utils.LogCommand, "read cache entry %q with key %s and %d bulks", f, entry.Key.GetFingerprint(), len(entry.Bulks))
				for i := 0; i < len(entry.Bulks); {
					removeBulk := false
					bulk := &entry.Bulks[i]

					dst := tempPath.Folder(bulk.Path.ReplaceExt("").Basename)
					defer os.RemoveAll(dst.String())

					artifacts, err := bulk.Inflate(dst)
					if err == nil {
						numArtifacts += len(artifacts)
						for _, it := range artifacts {
							utils.UFS.Remove(it)
						}
					} else {
						base.LogError(utils.LogCommand, "%v", err)
						removeBulk = true
					}

					if !removeBulk { // expire cache entries
						if mtime := utils.UFS.MTime(bulk.Path); mtime.Before(expireTime) {
							removeBulk = true
							err = fmt.Errorf("cache expiration (%v < %v)", mtime, expireTime)
						}
					}

					if removeBulk {
						numDeletedBulks++

						base.LogWarning(utils.LogCommand, "remove cache bulk %q: %v", bulk, err)

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
			}, base.TASKPRIORITY_HIGH)
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
			node, err := bg.Expect(a)
			if err != nil {
				return err
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

		pbar := base.LogProgress(0, int64(len(aliases)), "check-serialize")
		defer pbar.Close()

		ar := base.NewArchiveDiff()
		defer ar.Close()

		type Stats struct {
			Num      int32
			Size     int64
			Duration time.Duration
		}
		perClass := make(map[reflect.Type]Stats)

		if base.IsLogLevelActive(base.LOG_VERBOSE) {
			perSerializable := make([]struct {
				offset    int
				startedAt time.Duration
			}, 0)
			ar.OnSerializableBegin.Add(func(s base.Serializable) error {
				perSerializable = append(perSerializable, struct {
					offset    int
					startedAt time.Duration
				}{
					offset:    ar.Tell(),
					startedAt: base.Elapsed(),
				})
				return nil
			})
			ar.OnSerializableEnd.Add(func(s base.Serializable) error {
				scope := perSerializable[len(perSerializable)-1]
				duration := base.Elapsed() - scope.startedAt
				size := ar.Tell() - scope.offset
				perSerializable = perSerializable[:len(perSerializable)-1]

				typeOf := reflect.TypeOf(s)
				stats := perClass[typeOf]
				stats.Num++
				stats.Size += int64(size)
				stats.Duration += duration
				perClass[typeOf] = stats

				return nil
			})
			for _, a := range aliases {
				node, err := bg.Expect(a)
				if err != nil {
					return err
				}
				buildable := node.GetBuildable()

				// compare buildable with self: if an error happens then Serialize() has a bug somewhere
				// err should contain a stack of serialized objects, and hopefuly ease debugging
				if err := ar.Diff(buildable, buildable); err != nil {
					return err
				}

				pbar.Inc()
			}
		} else {
			for _, a := range aliases {
				node, err := bg.Expect(a)
				if err != nil {
					return err
				}
				buildable := node.GetBuildable()

				bench := base.LogBenchmark(utils.LogCommand, "%T -> %q", buildable, a)

				// compare buildable with self: if an error happens then Serialize() has a bug somewhere
				// err should contain a stack of serialized objects, and hopefuly ease debugging
				if err := ar.Diff(buildable, buildable); err != nil {
					return err
				}

				duration := bench.Close()
				pbar.Inc()

				typeOf := reflect.TypeOf(buildable)
				stats := perClass[typeOf]
				stats.Num++
				stats.Size += int64(ar.Len())
				stats.Duration += duration
				perClass[typeOf] = stats
			}
		}

		classBySize := base.Keys(perClass)
		sort.Slice(classBySize, func(i, j int) bool {
			return perClass[classBySize[i]].Size > perClass[classBySize[j]].Size
		})

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
					fmt.Fprintf(w, "%s[%d] %s: %s", indent, i, link.Type, link.Alias)

					if base.IsLogLevelActive(base.LOG_VERBOSE) {
						node, err := buildGraph.Expect(link.Alias)
						if err != nil {
							return err
						}
						fmt.Fprintf(w, " -> %v", node.GetBuildStamp())
					}

					fmt.Fprintln(w)
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
		return openCompletion(ca, func(w io.Writer) error {
			bg := utils.CommandEnv.BuildGraph()

			files, err := bg.GetDependencyInputFiles(true, ca.Inputs...)
			if err != nil {
				return err
			}

			files.Sort()

			for _, filename := range files {
				fmt.Fprintln(w, filename)
			}
			return nil
		})
	})

var OutputFiles = newCompletionCommand[utils.BuildAlias, *utils.BuildAlias](
	"Debug",
	"output-files",
	"list all output files for input nodes",
	func(cc utils.CommandContext, ca *CompletionArgs[utils.BuildAlias, *utils.BuildAlias]) error {
		return openCompletion(ca, func(w io.Writer) error {
			bg := utils.CommandEnv.BuildGraph()

			files, err := bg.GetDependencyOutputFiles(ca.Inputs...)
			if err != nil {
				return err
			}

			files.Sort()

			for _, filename := range files {
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

		printGradient := func(title string, n int, color func(float64) base.Color3f) {
			sb := strings.Builder{}
			sb.WriteString(base.ANSI_RESET.String())
			for i := 0; i < 100; i++ {
				c := color(float64(i) / 100.0)

				sb.WriteString(c.Quantize(true).Ansi(true))
				sb.WriteRune('▇')
			}
			sb.WriteString(base.ANSI_RESET.String())
			sb.WriteRune(' ')
			sb.WriteString(title)
			base.LogForwardln(sb.String())
		}

		printGradient("brightness linear", 100, func(f float64) base.Color3f { return basecol.Brightness(f) })
		printGradient("brightness srgb", 100, func(f float64) base.Color3f { return basecol.Brightness(f).LinearToSrgb() })
		printGradient("broadcast linear", 100, func(f float64) (c base.Color3f) { c.Broadcast(f); return })
		printGradient("broadcast srgb", 100, func(f float64) (c base.Color3f) { c.Broadcast(f); return c.LinearToSrgb() })
		printGradient("pastelizer", 100, func(f float64) base.Color3f { return base.NewPastelizerColor(f) })
		printGradient("heatmap", 100, func(f float64) base.Color3f { return base.NewHeatmapColor(f) })

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

		if !base.EnableInteractiveShell() || !base.IsLogLevelActive(base.LOG_INFO) {
			return nil
		}

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

		base.LogProgress(0, 50, "empty")
		base.LogProgress(0, 50, "full").Set(50)

		for {
			n := int(10 + rand.Uint32()%50)
			pbar := base.LogProgress(0, int64(n), "progress")

			for i := 0; i < n; i++ {
				n2 := int(10 + rand.Uint32()%100)
				pbar2 := base.LogProgress(0, int64(n2), "progress2")

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
	"internal command for debugging distribution",
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
		base.LogForwardln(utils.GetProcessInfo().String())
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
