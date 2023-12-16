//go:build windows

package windows

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
)

var LogDetours = base.NewLogCategory("Detours")

const USE_IO_DETOURING = true // %_NOCOMMIT%

func SetupWindowsIODetouring() {
	if USE_IO_DETOURING {
		if exe := getDetoursIOWrapperExecutable(); exe.Exists() {
			internal_io.OnRunCommandWithDetours = RunProcessWithDetoursWin32 // Comment to disable detouring %_NOCOMMIT%
		} else {
			base.LogWarning(LogDetours, "disable Win32 IO detouring: could not find %q", exe)
			internal_io.OnRunCommandWithDetours = nil
		}
	}
}

/***************************************
 * RunProcessWithDetoursWin32
 ***************************************/

var DETOURS_IGNORED_APPLICATIONS = []string{
	`cpptools.exe`,
	`cpptools-srv.exe`,
	`mspdbsrv.exe`,
	`vcpkgsrv.exe`,
	`vctip.exe`,
}

const DETOURS_IOWRAPPER_EXE = `Tools-IOWrapper-Win64-Devel.exe`

var getDetoursIOWrapperExecutable = base.Memoize(func() utils.Filename {
	return utils.UFS.Internal.AbsoluteFile(`hal`, `windows`, `bin`, DETOURS_IOWRAPPER_EXE)
})

func RunProcessWithDetoursWin32(executable utils.Filename, arguments base.StringSet, options *internal_io.ProcessOptions) error {
	tempFile, err := utils.UFS.CreateTemp("Detours", func(w io.Writer) error {
		return nil
	})

	m := sync.Mutex{}
	m.Lock()
	if err != nil {
		return err
	}
	defer tempFile.Close()

	options.Environment.Append("IOWRAPPER_IGNORED_APPLICATIONS", DETOURS_IGNORED_APPLICATIONS...)

	for i, mount := range options.MountedPaths {
		base.LogVeryVerbose(LogDetours, "mount %q as %q", mount.From, mount.To)

		if local, err := mountWebdavFolder(mount); err == nil {
			mount.To = local
			options.MountedPaths[i] = mount
		}

		options.Environment.Append("IOWRAPPER_MOUNTED_PATHS", mount.From.String(), mount.To)
	}

	// IOWrapper needs a valid executable file
	executable.Dirname.Path = expandWebdavPath(executable.Dirname.Path, options.MountedPaths)

	// IOWrapper should tolerate an invalid working directory though (even if it does not use detouring)
	// if options.WorkingDir.Valid() {
	// 	options.WorkingDir.Path = expandWebdavPath(options.WorkingDir.Path, options.MountedPaths)
	// }

	if err := internal_io.RunProcess_Vanilla(
		getDetoursIOWrapperExecutable(),
		append([]string{tempFile.String(), executable.String()}, arguments...),
		options); err != nil {
		return err
	}

	return utils.UFS.ReadLines(tempFile.Path, func(line string) error {
		var far internal_io.FileAccessRecord
		far.Path = utils.MakeFilename(unexpandWebdavPath(line[1:], options.MountedPaths))
		far.Access = internal_io.FILEACCESS_NONE

		mode := line[0] - '0'
		if (mode & 1) == 1 {
			far.Access.Append(internal_io.FILEACCESS_READ)
		}
		if (mode & 2) == 2 {
			far.Access.Append(internal_io.FILEACCESS_WRITE)
		}
		if (mode & 4) == 4 {
			far.Access.Append(internal_io.FILEACCESS_EXECUTE)
		}

		return options.OnFileAccess.Invoke(far)
	})
}

/***************************************
 * Mount webdav share on local folder for distributed tasks
 ***************************************/

// On Windows, one can mount an UNC path to a symlink, but it requires elevated privileges
// Elevated privileges can't be gained: we need to spawn a separated process with elevation
// This would cause a UAC panel to be shown, which we don't want obviously in an automated build-system
const detours_mount_webdav_with_symlink = false

// goal is detoured processes consuming webdav share transparently
func mountWebdavFolder(in internal_io.MountedPath) (string, error) {
	// parse HTTP webdav URL and translate it to a UNC path
	// this happens here because it's windows specific, while the webdav server is not (it could run on linux for instance)
	uri, err := url.Parse(in.To)
	if err != nil || (uri.Scheme != "http" && uri.Scheme != "https") {
		return in.To, nil // not a valid webdav url
	}

	hostname, port, err := net.SplitHostPort(uri.Host)
	if err != nil {
		return "", err // we ALWAYS provide a port, dafuq?
	}

	unc := fmt.Sprintf(`\\%s@%s\%s`, hostname, port, uri.Path)
	unc = strings.ReplaceAll(unc, `/`, `\`)

	if detours_mount_webdav_with_symlink {
		local := utils.UFS.Transient.Folder("DavWWWRoot", uri.Hostname())
		utils.UFS.Mkdir(local) // make sure parent directory exists, since Symlink won't create it
		local = local.Folder(base.SanitizeIdentifier(in.From.String()))

		// check if local path exists
		if st, err := os.Stat(local.String()); err == nil {
			// check if local path is a symbolic link
			if st.Mode().Type()&os.ModeSymlink != 0 {
				// check if symbolic target matches the UNC path we need
				if target, err := os.Readlink(local.String()); err == nil && target == unc {
					// nothing to do: path is already mounted :)
					return local.String(), nil
				}
			}

			// delete outdated/invalid symbolic link
			if err = os.Remove(local.String()); err != nil {
				return "", err // failed to delete previous link
			}
		}

		base.LogVeryVerbose(LogDetours, "mount webdav share %q as %q (UNC: %q)", in.To, local, unc)

		// need to create a new symbolic link pointing to UNC path we previously computed
		return local.String(), createWebdavSymbolicLink(unc, local.String())
	} else {
		return unc, nil
	}
}

func expandWebdavPath(path string, mounts []internal_io.MountedPath) string {
	for _, mount := range mounts {
		if prefix := mount.From.String(); strings.HasPrefix(path, prefix) {
			expanded := mount.To + path[len(prefix):]
			base.LogVeryVerbose(LogDetours, "expand webdav path %q to %q", path, expanded)
			return expanded
		}
	}
	return path
}
func unexpandWebdavPath(path string, mounts []internal_io.MountedPath) string {
	for _, mount := range mounts {
		if strings.HasPrefix(path, mount.To) {
			unexpanded := mount.From.String() + path[len(mount.To):]
			base.LogVeryVerbose(LogDetours, "unexpand webdav path %q to %q", path, unexpanded)
			return unexpanded
		}
	}
	return path
}

func createWebdavSymbolicLink(oldname, newname string) error {
	n, err := syscall.UTF16PtrFromString(newname)
	if err != nil {
		return &os.LinkError{Op: "createWebdavSymbolicLink", Old: oldname, New: newname, Err: err}
	}

	o, err := syscall.UTF16PtrFromString(oldname)
	if err != nil {
		return &os.LinkError{Op: "createWebdavSymbolicLink", Old: oldname, New: newname, Err: err}
	}

	// symlink support for CreateSymbolicLink() starting with Windows 10 (1703, v10.0.14972)
	const SYMBOLIC_LINK_FLAG_ALLOW_UNPRIVILEGED_CREATE = 0x2
	var flags uint32 = syscall.SYMBOLIC_LINK_FLAG_DIRECTORY |
		SYMBOLIC_LINK_FLAG_ALLOW_UNPRIVILEGED_CREATE

	if err = syscall.CreateSymbolicLink(n, o, flags); err != nil {
		// the unprivileged create flag is unsupported
		// below Windows 10 (1703, v10.0.14972). retry without it.
		flags &^= SYMBOLIC_LINK_FLAG_ALLOW_UNPRIVILEGED_CREATE

		if err = syscall.CreateSymbolicLink(n, o, flags); err != nil {
			return &os.LinkError{Op: "createWebdavSymbolicLink", Old: oldname, New: newname, Err: err}
		}
	}
	return nil
}
