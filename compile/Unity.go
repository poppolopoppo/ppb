package compile

import (
	"fmt"
	"io"
	"math"
	"sort"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
)

func (unit *Unit) GetSourceFiles(bc utils.BuildContext) (sourceFiles utils.FileSet, err error) {
	sourceFiles, err = unit.Source.GetFileSet(bc)
	if err != nil {
		return
	}

	switch unit.Unity {
	// check if unity build is enabled
	case UNITY_DISABLED, UNITY_INHERIT:
		return
	}

	sourceFiles.Sort()

	isolatedFiles := utils.NewFileSet(unit.Source.IsolatedFiles...)

	var totalSize int64
	sourceFileInfos := make([]*utils.FileInfo, len(sourceFiles))
	for i, file := range sourceFiles {
		var info *utils.FileInfo
		if info, err = file.Info(); err != nil {
			return
		}

		if !isolatedFiles.Contains(file) {
			if info.Size() < int64(unit.SizePerUnity) {
				sourceFileInfos[i] = info
				totalSize += info.Size()
			} else {
				base.LogVerbose(LogCompile, "%v: isolated large file %q from unity (%v)", unit.TargetAlias, file, base.SizeInBytes(info.Size()))
				isolatedFiles.Append(file)
			}
		}
	}

	var numUnityFiles int
	switch unit.Unity {
	case UNITY_AUTOMATIC:
		numUnityFiles = int(math.Ceil(float64(totalSize) / float64(unit.SizePerUnity)))
		base.LogVeryVerbose(LogCompile, "%v: %d unity files (%.2f KiB)", unit.TargetAlias, numUnityFiles, float64(totalSize)/1024.0)
	case UNITY_DISABLED, UNITY_INHERIT:
		base.UnexpectedValuePanic(unit.Unity, UNITY_INHERIT)
	default:
		if unit.Unity.Ord() > 0 {
			numUnityFiles = int(unit.Unity.Ord())
		} else {
			base.UnexpectedValuePanic(unit.Unity, unit.Unity)
		}
	}

	if numUnityFiles >= len(sourceFiles) {
		base.LogWarning(LogCompile, "%v: %d unity files (%.2f KiB) is superior to source files count (%d files), disabling unity (was %v)",
			unit.TargetAlias, numUnityFiles, float64(totalSize)/1024.0, len(sourceFiles), unit.Unity)
		numUnityFiles = 0
	}

	if numUnityFiles == 0 {
		// keep original source fileset
		return
	}
	base.AssertErr(func() error {
		if numUnityFiles > 0 {
			return nil
		}
		return fmt.Errorf("unity: invalid count of unity files %d (was %v)", numUnityFiles, unit.Unity)
	})

	// generate unity files
	unityDir := unit.GeneratedDir.Folder("Unity")
	if err = internal_io.CreateDirectory(bc, unityDir); err != nil {
		return
	}

	// detect PCH parameters: shoud unity files include "stdafx.h"?
	unityIncludes := base.StringSet{}
	switch unit.PCH {
	case PCH_MONOLITHIC, PCH_SHARED:
		// add pch header
		unityIncludes.Append(unit.PrecompiledHeader.Basename)
	case PCH_DISABLED:
		// no includes
	default:
		base.UnexpectedValuePanic(unit.PCH, unit.PCH)
	}

	// prepare clusters from previously estimated count
	type unityFileWithSize struct {
		TotalSize int64
		UnityFile
	}
	unityFiles := make([]unityFileWithSize, numUnityFiles)
	for i := range unityFiles {
		unityFiles[i] = unityFileWithSize{
			UnityFile: UnityFile{
				Output:   unityDir.File(fmt.Sprintf("Unity_%d_of_%d.cpp", (i + 1), numUnityFiles)),
				Includes: unityIncludes,
			},
		}
	}

	sourceFilesSorted := make([]int, len(sourceFiles))
	for i := range sourceFilesSorted {
		sourceFilesSorted[i] = i
	}

	const USE_BEST_FIT = false
	if USE_BEST_FIT {
		// sort source files by descending size for best-fit allocation
		sort.Slice(sourceFilesSorted, func(i, j int) bool {
			a, b := sourceFileInfos[sourceFilesSorted[i]], sourceFileInfos[sourceFilesSorted[j]]
			if a != nil && b != nil {
				return a.Size() > b.Size()
			} else if a == nil {
				return false
			} else {
				return true
			}
		})

		// cluster source files from largest to smallest
		for _, sourceFileIndex := range sourceFilesSorted {
			info := sourceFileInfos[sourceFileIndex]
			if info == nil {
				continue // this file was isolated
			}

			// find smallest unity
			unityIndex := 0
			for i := 1; i < numUnityFiles; i++ {
				if unityFiles[i].TotalSize < unityFiles[unityIndex].TotalSize {
					unityIndex = i
				}
			}

			// record source in smallest unity and update its size
			unityFile := &unityFiles[unityIndex]
			unityFile.TotalSize += info.Size()
			unityFile.Inputs.Append(sourceFiles[sourceFileIndex])
		}
	} else {
		// sort source files by ascending modified date
		sort.Slice(sourceFilesSorted, func(i, j int) bool {
			a, b := sourceFileInfos[sourceFilesSorted[i]], sourceFileInfos[sourceFilesSorted[j]]
			if a != nil && b != nil {
				return a.ModTime().Before(b.ModTime())
			} else if a == nil {
				return false
			} else {
				return true
			}
		})

		// cluster source files from oldest to newest
		unityIndex := 0
		numSourceFilesPerUnity := (len(sourceFiles) + numUnityFiles - 1) / numUnityFiles
		for _, sourceFileIndex := range sourceFilesSorted {
			info := sourceFileInfos[sourceFileIndex]
			if info == nil {
				continue // this file was isolated
			}

			// record source in current unity and update its size
			unityFile := &unityFiles[unityIndex]
			unityFile.TotalSize += info.Size()
			unityFile.Inputs.Append(sourceFiles[sourceFileIndex])

			// jump to next unity if full or too large
			if unityIndex+1 < numUnityFiles && (unityFile.TotalSize > int64(unit.SizePerUnity) || len(unityFile.Inputs) >= numSourceFilesPerUnity) {
				unityIndex++
			}
		}
	}

	// check for modified files if adaptive unity is enabled
	adaptiveUnityFiles := utils.FileSet{}
	if unit.AdaptiveUnity.Get() {
		var scm *utils.SourceControlModifiedFiles
		if scm, err = utils.BuildSourceControlModifiedFiles(unit.ModuleDir).Need(bc); err != nil {
			return
		}

		for _, file := range sourceFiles {
			if scm.HasUnversionedModifications(file) && !isolatedFiles.Contains(file) {
				base.LogVerbose(LogCompile, "%v: adaptive unity isolated %q", unit.TargetAlias, file)
				adaptiveUnityFiles.Append(file)
			}
		}

		base.Assert(adaptiveUnityFiles.IsUniq)
	}

	// replace source fileset by generated unity + isolated files + adaptive unity files
	sourceFiles = append(isolatedFiles, adaptiveUnityFiles...)

	for _, unityFile := range unityFiles {
		base.Assert(unityFile.Inputs.IsUniq)
		unityFile.Inputs.Sort()
		unityFile.Excludeds = base.Intersect(unityFile.Inputs, adaptiveUnityFiles)
		sourceFiles.Append(unityFile.Output)

		if _, err = bc.OutputFactory(utils.MakeBuildFactory(func(bi utils.BuildInitializer) (UnityFile, error) {
			staticDeps := utils.NewFileSet(unityFile.Inputs...)
			staticDeps.Remove(unityFile.Excludeds...)
			return unityFile.UnityFile, bi.NeedFile(staticDeps...)
		})); err != nil {
			return
		}
	}

	sourceFiles.Sort()
	base.Assert(sourceFiles.IsUniq)
	return
}

/***************************************
 * Unity File
 ***************************************/

type UnityFile struct {
	Output    utils.Filename
	Includes  base.StringSet
	Inputs    utils.FileSet
	Excludeds utils.FileSet
}

func MakeUnityFileAlias(output utils.Filename) utils.BuildAlias {
	return utils.MakeBuildAlias("Unity", output.Dirname.Path, output.Basename)
}
func FindUnityFile(output utils.Filename) (*UnityFile, error) {
	// easier to debug with a separated function, since this function is expected to fail
	// return FindGlobalBuildable[*UnityFile](MakeUnityFileAlias(output))
	alias := MakeUnityFileAlias(output)
	if node := utils.CommandEnv.BuildGraph().Find(alias); node != nil {
		return node.GetBuildable().(*UnityFile), nil
	} else {
		return nil, utils.BuildableNotFound{Alias: alias}
	}
}

func (x *UnityFile) Alias() utils.BuildAlias {
	return MakeUnityFileAlias(x.Output)
}
func (x *UnityFile) GetInputsWithoutExcludeds() utils.FileSet {
	return base.RemoveUnless(func(i utils.Filename) bool {
		return !x.Excludeds.Contains(i)
	}, x.Inputs...)
}
func (x *UnityFile) Build(bc utils.BuildContext) error {
	base.AssertNotIn(len(x.Inputs), 0)

	timestamp := time.Time{}

	err := utils.UFS.CreateBuffered(x.Output, func(w io.Writer) error {
		cpp := internal_io.NewCppFile(w, true)
		for _, it := range x.Includes {
			cpp.Include(it)
		}
		for _, it := range x.Inputs {
			isExcluded := x.Excludeds.Contains(it)
			if isExcluded {
				cpp.BeginBlockComment()
			}

			cpp.Pragma("message(\"unity: \" %q)", it)
			cpp.Include(utils.SanitizePath(it.Relative(utils.UFS.Source), '/'))

			if isExcluded {
				cpp.EndBlockComment()
			}

			if info, err := it.Info(); err == nil {
				if timestamp.Before(info.ModTime()) {
					timestamp = info.ModTime()
				}
			} else {
				return err
			}
		}
		return nil
	})

	if err == nil {
		bc.Annotate(fmt.Sprintf("%d files", len(x.Inputs)-len(x.Excludeds)))
		bc.Timestamp(timestamp)
		if err = utils.UFS.SetMTime(x.Output, timestamp); err == nil {
			err = bc.OutputFile(x.Output)
		}
	}
	return err
}
func (x *UnityFile) Serialize(ar base.Archive) {
	ar.Serializable(&x.Output)
	ar.Serializable(&x.Includes)
	ar.Serializable(&x.Inputs)
	ar.Serializable(&x.Excludeds)
}
