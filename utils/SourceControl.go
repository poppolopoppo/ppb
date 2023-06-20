package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var LogSourceControl = NewLogCategory("SourceControl")

type SourceControlProvider interface {
	GetModifiedItems(*SourceControlModifiedFiles) error
	GetStatus(*SourceControlStatus) error
}

/***************************************
 * Source Control Status
 ***************************************/

type SourceControlStatus struct {
	Path      Directory
	Branch    string
	Revision  string
	Timestamp time.Time
}

func (x *SourceControlStatus) Alias() BuildAlias {
	return MakeBuildAlias("SourceControl", "Status", x.Path.String())
}
func (x *SourceControlStatus) Build(bc BuildContext) (err error) {
	err = GetSourceControlProvider().GetStatus(x)
	if err == nil {
		LogVerbose(LogSourceControl, "branch=%s, revision=%s, timestamp=%s", x.Branch, x.Revision, x.Timestamp)
	}
	return
}
func (x *SourceControlStatus) Serialize(ar Archive) {
	ar.Serializable(&x.Path)
	ar.String(&x.Branch)
	ar.String(&x.Revision)
	ar.Time(&x.Timestamp)
}

func BuildSourceControlStatus(path Directory) BuildFactoryTyped[*SourceControlStatus] {
	return MakeBuildFactory(func(bi BuildInitializer) (SourceControlStatus, error) {
		return SourceControlStatus{
			Path: path,
		}, nil
	})
}

/***************************************
 * Source Control Modified Files
 ***************************************/

type SourceControlModifiedFiles struct {
	Path          Directory
	ModifiedDirs  DirSet
	ModifiedFiles FileSet
}

func (x *SourceControlModifiedFiles) Alias() BuildAlias {
	return MakeBuildAlias("SourceControl", "ModifiedFiles", x.Path.String())
}
func (x *SourceControlModifiedFiles) HasUnversionedModifications(files ...Filename) bool {
	for _, f := range files {
		if x.ModifiedFiles.Contains(f) {
			return true
		}
		for _, folder := range x.ModifiedDirs {
			if f.Dirname.IsIn(folder) {
				return true
			}
		}
	}
	return false
}
func (x *SourceControlModifiedFiles) Build(bc BuildContext) (err error) {
	if err = GetSourceControlProvider().GetModifiedItems(x); err != nil {
		return
	}

	// look for the date of the most recent modification
	var timestamp time.Time
	for _, dir := range x.ModifiedDirs {
		if st, err := dir.Info(); err == nil && timestamp.Before(st.ModTime()) {
			timestamp = st.ModTime()
		}
	}
	for _, file := range x.ModifiedFiles {
		if st, err := file.Info(); err == nil && timestamp.Before(st.ModTime()) {
			timestamp = st.ModTime()
		}
	}

	// explicit time tracking: avoid recompiling when nothing changed
	if timestamp != (time.Time{}) {
		bc.Timestamp(timestamp)
	}

	bc.Annotate(fmt.Sprintf("%d files, %d folders", len(x.ModifiedFiles), len(x.ModifiedDirs)))
	return
}
func (x *SourceControlModifiedFiles) Serialize(ar Archive) {
	ar.Serializable(&x.Path)
	ar.Serializable(&x.ModifiedDirs)
	ar.Serializable(&x.ModifiedFiles)
}

func BuildSourceControlModifiedFiles(path Directory) BuildFactoryTyped[*SourceControlModifiedFiles] {
	return MakeBuildFactory(func(bi BuildInitializer) (SourceControlModifiedFiles, error) {
		return SourceControlModifiedFiles{
			Path: path,
		}, nil
	})
}

/***************************************
 * Dummy source control
 ***************************************/

type DummySourceControl struct{}

func (x DummySourceControl) GetModifiedItems(modified *SourceControlModifiedFiles) error {
	modified.ModifiedDirs = DirSet{}
	modified.ModifiedFiles = FileSet{}
	return nil
}
func (x DummySourceControl) GetStatus(status *SourceControlStatus) error {
	status.Branch = "Dummy"
	status.Revision = "Unknown"
	status.Timestamp = time.Now()
	return nil
}

/***************************************
 * Git source control
 ***************************************/

type GitSourceControl struct {
	Executable string
	Repository Directory

	modifiedFilesInCache *SourceControlModifiedFiles
	modifiedFilesBarrier sync.Mutex
}

func NewGitSourceControl(repository Directory) (*GitSourceControl, error) {
	if gitDir := repository.Folder(".git"); !gitDir.Exists() {
		return nil, fmt.Errorf("invalid git repository %q", gitDir)
	}

	executable, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}

	return &GitSourceControl{
		Executable: executable,
		Repository: repository,
	}, nil
}

func (git *GitSourceControl) Command(name string, args ...string) ([]byte, error) {
	args = append([]string{"--no-optional-locks", name}, args...)
	LogVeryVerbose(LogSourceControl, "run git command %v", MakeStringer(func() string {
		return strings.Join(args, " ")
	}))

	proc := exec.Command(git.Executable, args...)
	proc.Env = os.Environ()
	proc.Dir = git.Repository.String()

	output, err := proc.Output()
	if err != nil {
		LogError(LogSourceControl, "git command %v returned %v: %v", strings.Join(args, " "), err, output)
	}

	return output, err
}
func (git *GitSourceControl) getModifiedFilesInCache() (*SourceControlModifiedFiles, error) {
	git.modifiedFilesBarrier.Lock()
	defer git.modifiedFilesBarrier.Unlock()

	if git.modifiedFilesInCache != nil {
		return git.modifiedFilesInCache, nil
	}

	modified := new(SourceControlModifiedFiles)
	modified.Path = git.Repository

	status, err := git.Command("status",
		"--ignore-submodules", // Git will recursve into each submodule without this, which can be very slow (1.5s to 250ms on PPE)
		"-s", "--porcelain=v1")
	if err != nil {
		return nil, err
	}

	reader := bufio.NewScanner(bytes.NewReader(status))
	for {
		advance, token, err := bufio.ScanLines(status, true)
		if err != nil {
			return nil, err
		}
		if advance == 0 {
			break
		}
		if advance <= len(status) {
			status = status[advance:]
		}
		if len(token) == 0 {
			continue
		}

		line := UnsafeStringFromBytes(token)

		if strings.HasPrefix(line, "A ") || strings.HasPrefix(line, " M") || strings.HasPrefix(line, "AM") || strings.HasPrefix(line, "??") {
			path := strings.TrimSpace(line[3:])
			stat, err := os.Stat(path)
			if err != nil {
				LogError(LogSourceControl, "ignore invalid modified path: %v", err)
				continue
			}

			if stat.IsDir() {
				dir := git.Repository.AbsoluteFolder(path)
				MakeDirectoryInfo(dir, &stat) // cache os.Stat() result

				modified.ModifiedDirs.Append(dir)

				LogVeryVerbose(LogSourceControl, "git sees directory %q as modified", dir)

			} else {
				file := git.Repository.AbsoluteFile(path)
				MakeFileInfo(file, &stat) // cache os.Stat() result

				modified.ModifiedFiles.Append(file)

				LogVeryVerbose(LogSourceControl, "git sees file %q as modified", file)
			}
		}
	}

	if err = reader.Err(); err == nil {
		LogVeryVerbose(LogSourceControl, "git found %d modified files and %d modified directories in whole repository", len(modified.ModifiedFiles), len(modified.ModifiedDirs))
		git.modifiedFilesInCache = modified
	}
	return git.modifiedFilesInCache, err
}
func (git *GitSourceControl) GetModifiedItems(modified *SourceControlModifiedFiles) error {
	modified.ModifiedDirs = NewDirSet()
	modified.ModifiedFiles = NewFileSet()

	// list modified files from Git only once for whole repo
	global, err := git.getModifiedFilesInCache()
	if err != nil {
		return err
	}

	// then filter global results by subfolder
	modified.ModifiedDirs = RemoveUnless(func(d Directory) bool { return d.IsIn(modified.Path) }, global.ModifiedDirs...)
	modified.ModifiedFiles = RemoveUnless(func(f Filename) bool { return f.IsIn(modified.Path) }, global.ModifiedFiles...)

	LogVeryVerbose(LogSourceControl, "git found %d modified files and %d modified directories in %v", len(modified.ModifiedFiles), len(modified.ModifiedDirs))
	return nil
}
func (git *GitSourceControl) GetStatus(status *SourceControlStatus) error {
	status.Revision = "no-revision-available"
	status.Branch = "no-branch-available"
	status.Timestamp = CommandEnv.BuildTime()

	if outp, err := git.Command("log", "-1", "--format=\"%H;%ct;%D\"", "--", status.Path.Relative(git.Repository)); err == nil {
		if len(outp) == 0 {
			return nil // output is empty when the path is known to Git (ignored or not git-added yet for instance)
		}

		line := strings.TrimSpace(UnsafeStringFromBytes(outp))
		line = strings.Trim(line, "\"")

		log := strings.SplitN(line, ";", 4)

		status.Revision = strings.TrimSpace(log[0])
		timestamp := strings.TrimSpace(log[1])

		branchInfo := strings.Split(log[2], "->")
		status.Branch = branchInfo[len(branchInfo)-1]
		status.Branch = strings.Split(status.Branch, `,`)[0]
		status.Branch = strings.TrimSpace(status.Branch)

		if unitT, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
			status.Timestamp = time.Unix(unitT, 0)
			return nil
		} else {
			return err
		}
	} else {
		return err
	}
}

var GetSourceControlProvider = Memoize(func() SourceControlProvider {
	if git, err := NewGitSourceControl(UFS.Root); err == nil {
		LogVerbose(LogSourceControl, "found Git source control in %q", git.Repository)
		return git
	}
	return &DummySourceControl{}
})
