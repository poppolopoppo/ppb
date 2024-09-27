package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
)

var LogSourceControl = base.NewLogCategory("SourceControl")

type SourceControlProvider interface {
	IsInRepository(Filename) bool
	GetRepositoryStatus(*SourceControlRepositoryStatus) error
	GetFileStatus(*SourceControlFileStatus) error
	GetFolderStatus(*SourceControlFolderStatus) error
}

type SourceControlRepositoryStatus struct {
	Files       map[Filename]SourceControlState
	Directories []struct {
		Directory
		State SourceControlState
	}
}

var GetSourceControlProvider = base.Memoize(func() SourceControlProvider {
	if git, err := NewGitSourceControl(UFS.Root); err == nil {
		base.LogVerbose(LogSourceControl, "found Git source control in %q", git.Repository)
		return git
	}
	return &DummySourceControl{}
})

/***************************************
 * Source Control State
 ***************************************/

type SourceControlState byte

const (
	SOURCECONTROL_IGNORED SourceControlState = iota
	SOURCECONTROL_UNTRACKED
	SOURCECONTROL_UPTODATE
	SOURCECONTROL_MODIFIED
	SOURCECONTROL_ADDED
	SOURCECONTROL_DELETED
	SOURCECONTROL_RENAMED
)

func GetSourceControlStates() []SourceControlState {
	return []SourceControlState{
		SOURCECONTROL_IGNORED,
		SOURCECONTROL_UNTRACKED,
		SOURCECONTROL_UPTODATE,
		SOURCECONTROL_MODIFIED,
		SOURCECONTROL_ADDED,
		SOURCECONTROL_DELETED,
		SOURCECONTROL_RENAMED,
	}
}
func (x SourceControlState) Ignored() bool {
	return x == SOURCECONTROL_IGNORED
}
func (x SourceControlState) Description() string {
	switch x {
	case SOURCECONTROL_IGNORED:
		return "ignored by source control"
	case SOURCECONTROL_UNTRACKED:
		return "not tracked by source control"
	case SOURCECONTROL_UPTODATE:
		return "all changes are in source control"
	case SOURCECONTROL_MODIFIED:
		return "local modifications present"
	case SOURCECONTROL_ADDED:
		return "added to source control"
	case SOURCECONTROL_DELETED:
		return "deleted from source control"
	case SOURCECONTROL_RENAMED:
		return "renamed locally"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x SourceControlState) String() string {
	switch x {
	case SOURCECONTROL_IGNORED:
		return "IGNORED"
	case SOURCECONTROL_UNTRACKED:
		return "UNVERSIONED"
	case SOURCECONTROL_UPTODATE:
		return "UPTODATE"
	case SOURCECONTROL_MODIFIED:
		return "MODIFIED"
	case SOURCECONTROL_ADDED:
		return "ADDED"
	case SOURCECONTROL_DELETED:
		return "DELETED"
	case SOURCECONTROL_RENAMED:
		return "RENAMED"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *SourceControlState) Set(in string) error {
	switch strings.ToUpper(in) {
	case SOURCECONTROL_IGNORED.String():
		*x = SOURCECONTROL_IGNORED
	case SOURCECONTROL_UNTRACKED.String():
		*x = SOURCECONTROL_UNTRACKED
	case SOURCECONTROL_UPTODATE.String():
		*x = SOURCECONTROL_UPTODATE
	case SOURCECONTROL_MODIFIED.String():
		*x = SOURCECONTROL_MODIFIED
	case SOURCECONTROL_ADDED.String():
		*x = SOURCECONTROL_ADDED
	case SOURCECONTROL_DELETED.String():
		*x = SOURCECONTROL_DELETED
	case SOURCECONTROL_RENAMED.String():
		*x = SOURCECONTROL_RENAMED
	}
	return nil
}
func (x *SourceControlState) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x SourceControlState) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *SourceControlState) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x SourceControlState) AutoComplete(in base.AutoComplete) {
	for _, it := range GetSourceControlStates() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * Source Control File Status
 ***************************************/

type SourceControlFileStatus struct {
	Path  Filename
	State SourceControlState
}

func ForeachLocalSourceControlModifications(bc BuildContext, each func(Filename, SourceControlState) error, files ...Filename) (count int, err error) {
	scm := GetSourceControlProvider()

	futures := make([]base.Future[BuildResult], 0, len(files))
	for _, it := range files {
		if scm.IsInRepository(it) {
			futures = append(futures, PrepareBuildFactory(bc, BuildFile(it)))
		}
	}

	err = base.ParallelJoin(func(i int, br BuildResult) error {
		bc.NeedBuildResult(br)
		if file := br.Buildable.(*FileDependency); !file.SourceControl.Ignored() {
			count++
			if each != nil {
				return each(file.Filename, file.SourceControl)
			}
		}
		return nil
	}, futures...)
	return
}

/***************************************
 * Source Control Folder Status
 ***************************************/

type SourceControlFolderStatus struct {
	Path      Directory
	Branch    string
	Revision  string
	Timestamp time.Time
}

func (x *SourceControlFolderStatus) GetSourceDirectory() Directory {
	return x.Path
}
func (x *SourceControlFolderStatus) Alias() BuildAlias {
	return MakeBuildAlias("SourceControl", x.Path.String())
}
func (x *SourceControlFolderStatus) Build(bc BuildContext) (err error) {
	err = GetSourceControlProvider().GetFolderStatus(x)
	if err == nil {
		base.LogVerbose(LogSourceControl, "%s: branch=%s, revision=%s, timestamp=%s", x.Path, x.Branch, x.Revision, x.Timestamp)
	}
	return
}
func (x *SourceControlFolderStatus) Serialize(ar base.Archive) {
	ar.Serializable(&x.Path)
	ar.String(&x.Branch)
	ar.String(&x.Revision)
	ar.Time(&x.Timestamp)
}

func BuildSourceControlFolderStatus(path Directory) BuildFactoryTyped[*SourceControlFolderStatus] {
	return MakeBuildFactory(func(bi BuildInitializer) (SourceControlFolderStatus, error) {
		return SourceControlFolderStatus{
			Path: path,
		}, nil
	})
}

/***************************************
 * Dummy source control
 ***************************************/

type DummySourceControl struct{}

func (x DummySourceControl) IsInRepository(Filename) bool {
	return false
}
func (x DummySourceControl) GetRepositoryStatus(repo *SourceControlRepositoryStatus) error {
	repo.Files = make(map[Filename]SourceControlState)
	repo.Directories = []struct {
		Directory
		State SourceControlState
	}{}
	return nil
}
func (x DummySourceControl) GetFileStatus(file *SourceControlFileStatus) error {
	file.State = SOURCECONTROL_IGNORED
	return nil
}
func (x DummySourceControl) GetFolderStatus(folder *SourceControlFolderStatus) error {
	folder.Branch = "Dummy"
	folder.Revision = "Unknown"
	folder.Timestamp = time.Now()
	return nil
}

/***************************************
 * Git source control
 ***************************************/

type GitSourceControl struct {
	Executable string
	Repository Directory

	status struct {
		once sync.Once
		err  error
		SourceControlRepositoryStatus
	}
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
func (git *GitSourceControl) IsInRepository(f Filename) bool {
	return f.IsIn(git.Repository)
}
func (git *GitSourceControl) Command(name string, args ...string) ([]byte, error) {
	args = append([]string{"--no-optional-locks", name}, args...)
	base.LogVeryVerbose(LogSourceControl, "run git command %v", base.MakeStringer(func() string {
		return strings.Join(args, " ")
	}))

	proc := exec.Command(git.Executable, args...)
	proc.Env = os.Environ()
	proc.Dir = git.Repository.String()

	output, err := proc.Output()
	if err != nil {
		base.LogError(LogSourceControl, "git command %v returned %v: %v", strings.Join(args, " "), err, output)
	}

	return output, err
}
func getGitRepositoryStatus(git *GitSourceControl) (repo SourceControlRepositoryStatus, err error) {
	repo.Files = make(map[Filename]SourceControlState)
	repo.Directories = []struct {
		Directory
		State SourceControlState
	}{}

	var status []byte
	status, err = git.Command("status",
		"--ignore-submodules", // Git will recursve into each submodule without this, which can be very slow (1.5s to 250ms on PPE)
		"-s", "--porcelain=v1")
	if err != nil {
		return
	}

	reader := bufio.NewScanner(bytes.NewReader(status))
	for {
		advance, token, err := bufio.ScanLines(status, true)
		if err != nil {
			return repo, err
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

		line := base.UnsafeStringFromBytes(token)

		status := SOURCECONTROL_IGNORED
		switch line[:2] {
		case "A ":
			status = SOURCECONTROL_ADDED
		case " M", "AM":
			status = SOURCECONTROL_MODIFIED
		case " R":
			status = SOURCECONTROL_RENAMED
		case " D":
			status = SOURCECONTROL_DELETED
			continue // deleted files are ignored by the build system, since they are now invalid
		case "??":
			status = SOURCECONTROL_UNTRACKED
		default:
			continue
		}
		path := strings.TrimSpace(line[3:])

		stat, err := os.Stat(path)
		if err != nil {
			base.LogError(LogSourceControl, "ignore invalid modified path: %v", err)
			continue
		}

		if stat.IsDir() {
			dir := git.Repository.AbsoluteFolder(path)
			FileInfos.SetDirectoryInfo(dir, stat, nil)

			repo.Directories = append(repo.Directories, struct {
				Directory
				State SourceControlState
			}{
				Directory: dir,
				State:     status,
			})

			base.LogVeryVerbose(LogSourceControl, "git sees directory %q as %v", dir, status)
		} else {
			file := git.Repository.AbsoluteFile(path)
			FileInfos.SetFileInfo(file, stat, nil)

			repo.Files[file] = status

			base.LogVeryVerbose(LogSourceControl, "git sees file %q as %v", file, status)
		}
	}

	sort.Slice(repo.Directories, func(i, j int) bool {
		return repo.Directories[i].Compare(repo.Directories[j].Directory) < 0
	})

	if err = reader.Err(); err == nil {
		base.LogVerbose(LogSourceControl, "git found %d modified files and %d modified directories in whole repository",
			len(repo.Files), len(repo.Directories))
	}

	return repo, err
}

func (git *GitSourceControl) GetRepositoryStatus(repo *SourceControlRepositoryStatus) error {
	git.status.once.Do(func() {
		git.status.SourceControlRepositoryStatus, git.status.err = getGitRepositoryStatus(git)
	})

	if git.status.err == nil {
		*repo = git.status.SourceControlRepositoryStatus
		return nil
	} else {
		repo.Files = map[Filename]SourceControlState{}
		repo.Directories = []struct {
			Directory
			State SourceControlState
		}{}
		return git.status.err
	}
}
func (git *GitSourceControl) GetFileStatus(file *SourceControlFileStatus) error {
	file.State = SOURCECONTROL_IGNORED

	var repo SourceControlRepositoryStatus
	if err := git.GetRepositoryStatus(&repo); err != nil {
		return err
	}

	if state, ok := repo.Files[file.Path]; ok {
		file.State = state
	} else if len(repo.Directories) > 0 {
		found := sort.Search(len(repo.Directories), func(i int) bool {
			return file.Path.Dirname.Compare(repo.Directories[i].Directory) >= 0
		})

		if found < len(repo.Directories) && file.Path.Dirname.IsIn(repo.Directories[found].Directory) {
			file.State = repo.Directories[found].State
		}
	}

	return nil
}
func (git *GitSourceControl) GetFolderStatus(dir *SourceControlFolderStatus) error {
	dir.Revision = "no-revision-available"
	dir.Branch = "no-branch-available"
	dir.Timestamp = CommandEnv.BuildTime()

	if outp, err := git.Command("log", "-1", "--format=\"%H;%ct;%D\"", "--", dir.Path.Relative(git.Repository)); err == nil {
		if len(outp) == 0 {
			return nil // output is empty when the path is known to Git (ignored or not git-added yet for instance)
		}

		line := strings.TrimSpace(base.UnsafeStringFromBytes(outp))
		line = strings.Trim(line, "\"")

		log := strings.SplitN(line, ";", 4)

		dir.Revision = strings.TrimSpace(log[0])
		timestamp := strings.TrimSpace(log[1])

		branchInfo := strings.Split(log[2], "->")
		dir.Branch = branchInfo[len(branchInfo)-1]
		dir.Branch = strings.Split(dir.Branch, `,`)[0]
		dir.Branch = strings.TrimSpace(dir.Branch)

		if unitT, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
			dir.Timestamp = time.Unix(unitT, 0)
			return nil
		} else {
			return err
		}
	} else {
		return err
	}
}
