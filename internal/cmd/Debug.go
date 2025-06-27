package cmd

import (
	"context"
	"fmt"
	"io"
	"math"
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

var CommandCheckCache = utils.NewCommand(
	"Debug",
	"check-cache",
	"inspect action cache content validity and clean invalid/outdated entries",
	utils.OptionCommandRun(func(cc utils.CommandContext) error {
		utils.GetCommandFlags().Summary.Enable()

		bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "CheckCache"})
		defer bg.Close()

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

					base.LogWarning(utils.LogCommand, "remove %q cache entry", f)
					for _, bulk := range entry.Bulks {
						utils.UFS.Remove(bulk.Path)
					}

					utils.UFS.Remove(f)
				}()

				base.LogDebug(utils.LogCommand, "found cache entry %q", f)
				if err := entry.OpenEntry(context.TODO(), f); err != nil {
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

					artifacts, err := bulk.Inflate(context.TODO(), dst)
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

						base.LogWarning(utils.LogCommand, "remove cache bulk %q: %v", bulk.Path, err)

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
			}, base.TASKPRIORITY_NORMAL, base.ThreadPoolDebugId{Category: "CheckCache", Arg: f})
			return nil
		}, cacheGlob)
	}))

var CheckSerialize = utils.NewCommand(
	"Debug",
	"check-serialize",
	"write and load every node, then check for differences",
	utils.OptionCommandRun(func(cc utils.CommandContext) error {
		bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "CheckSerialize"})
		defer bg.Close()

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
			for i := 0; i < n; i++ {
				c := color(float64(i+1) / float64(n))

				sb.WriteString(c.Quantize().Ansi(true))
				sb.WriteRune('▇')
			}
			sb.WriteString(base.ANSI_RESET.String())
			sb.WriteRune(' ')
			sb.WriteString(title)
			base.LogForwardln(sb.String())
		}

		printGradient("brightness", 100, func(f float64) base.Color3f { return basecol.Brightness(f) })
		printGradient("broadcast", 100, func(f float64) (c base.Color3f) { c.Broadcast(f); return })
		printGradient("pastelizer", 100, func(f float64) base.Color3f { return base.NewPastelizerColor(f) })
		printGradient("heatmap", 100, func(f float64) base.Color3f { return base.NewHeatmapColor(f) })
		printGradient("coldhot", 100, func(f float64) base.Color3f { return base.NewColdHotColor(f) })
		printGradient("coldhot sqrt", 100, func(f float64) base.Color3f { return base.NewColdHotColor(math.Sqrt(f)) })

		white := base.NewColor3f(1, 1, 1).Quantize()
		black := base.NewColor3f(0, 0, 0).Quantize()

		base.LogForwardln(
			white.Ansi(true),
			black.Ansi(false),
			"White On Black", base.ANSI_RESET.String())
		base.LogForwardln(
			white.Ansi(false),
			black.Ansi(true),
			"Black On White", base.ANSI_RESET.String())
		base.LogForwardln(
			base.NewColor3f(1, 0, 0).Quantize().Ansi(true),
			base.NewColor3f(0, 0, 1).Quantize().Ansi(false),
			"Red On Blue", base.ANSI_RESET.String())
		base.LogForwardln(
			base.NewColor3f(1, 1, 0).Quantize().Ansi(true),
			base.NewColor3f(0, 1, 0).Quantize().Ansi(false),
			"Yellow On Green", base.ANSI_RESET.String())
		base.LogForwardln(
			base.NewColor3f(1, 0.1, 0.1).Brightness(0.8).Quantize().Ansi(true),
			base.NewColor3f(0.1, 0.1, 1).Brightness(0.3).Quantize().Ansi(false),
			"Light Red On Dark Blue", base.ANSI_RESET.String())

		base.LogForwardln("Unicode emojis:")
		base.LogForwardln(string(base.UnicodeEmojis))
		base.LogForwardln(string(base.UnicodeEmojisShuffled))

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

type BuildAliasesArgs struct {
	Aliases  utils.BuildAliases
	Detailed utils.BoolVar
}

func (x *BuildAliasesArgs) Flags(cfv utils.CommandFlagsVisitor) {
	cfv.Variable("l", "add more details to completion output", &x.Detailed)
}

func newBuildAliasesCommand(category, name, description string, run func(utils.CommandContext, *BuildAliasesArgs) error) func() utils.CommandItem {
	args := new(BuildAliasesArgs)
	return utils.NewCommand(category, name, description,
		utils.OptionCommandConsumeMany("Aliases", "select build nodes by their build aliases", args.Aliases.Ref()),
		utils.OptionCommandParsableAccessor("BuildAliasesArgs", "options for commands operating on build graph", func() *BuildAliasesArgs { return args }),
		utils.OptionCommandRun(func(cc utils.CommandContext) error {
			return run(cc, args)
		}))
}

var CommandCheckBuild = newBuildAliasesCommand(
	"Debug",
	"check-build",
	"build graph aliases passed as input parameters",
	func(cc utils.CommandContext, args *BuildAliasesArgs) error {
		bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "CheckBuild"})
		defer bg.Close()

		_, err := bg.BuildMany(args.Aliases,
			utils.OptionBuildForceIf(utils.GetCommandFlags().Force.Get()))
		return err
	})

var CheckFingerprint = newBuildAliasesCommand(
	"Debug",
	"check-fingerprint",
	"recompute nodes fingerprint and see if they match with the stamp stored in build graph",
	func(cc utils.CommandContext, args *BuildAliasesArgs) error {
		bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "CheckFingerprint"})
		defer bg.Close()

		for _, a := range args.Aliases {
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

var DependencyChain = newBuildAliasesCommand(
	"Debug",
	"dependency-chain",
	"find shortest dependency chain between 2 nodes",
	func(cc utils.CommandContext, args *BuildAliasesArgs) error {
		if len(args.Aliases) < 2 {
			return fmt.Errorf("dependency-chain: must pass at least 2 targets")
		}

		bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "DependencyChain"})
		defer bg.Close()

		// print the dependency chain found

		for i := 1; i < len(args.Aliases); i++ {
			// build graph will use Dijkstra to find the shortest path between the 2 nodes
			chain, err := bg.GetDependencyChain(args.Aliases[0], args.Aliases[i],
				// weight by link type, favor output > static > dynamic
				func(bdl utils.BuildDependencyLink) float32 { return float32(bdl.Type) })
			if err != nil {
				return err
			}

			indent := ""
			for i, link := range chain {
				base.LogForwardf("%s[%d] %s: %s", indent, i, link.Type, link.Alias)

				if args.Detailed.Get() {
					node, err := bg.Expect(link.Alias)
					if err != nil {
						return err
					}
					base.LogForwardf(" -> %v", node.GetBuildStamp())
				}

				base.LogForwardln()
				indent += "  "
			}
		}

		return nil
	})

var DependencyFiles = newBuildAliasesCommand(
	"Debug",
	"dependency-files",
	"list all file dependencies for input nodes",
	func(cc utils.CommandContext, args *BuildAliasesArgs) error {
		bg := utils.CommandEnv.BuildGraph().OpenReadPort(base.ThreadPoolDebugId{Category: "DependencyFiles"})
		defer bg.Close()

		files, err := bg.GetDependencyInputFiles(true, args.Aliases...)
		if err != nil {
			return err
		}

		files.Sort()

		var w io.Writer = base.GetLogger()
		for _, filename := range files {
			if err := printFileCompletion(w, filename, args.Detailed.Get()); err != nil {
				return err
			}
		}
		return nil
	})

var OutputFiles = newBuildAliasesCommand(
	"Debug",
	"output-files",
	"list all output files for input nodes",
	func(cc utils.CommandContext, args *BuildAliasesArgs) error {
		bg := utils.CommandEnv.BuildGraph().OpenReadPort(base.ThreadPoolDebugId{Category: "OutputFiles"})
		defer bg.Close()

		files, err := bg.GetDependencyOutputFiles(args.Aliases...)
		if err != nil {
			return err
		}

		files.Sort()

		var w io.Writer = base.GetLogger()
		for _, filename := range files {
			if err := printFileCompletion(w, filename, args.Detailed.Get()); err != nil {
				return err
			}
		}
		return nil
	})
