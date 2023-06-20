package utils

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
)

var LogProcess = NewLogCategory("Process")

/***************************************
 * Process Options
 ***************************************/

type FileAccessType int32

const (
	FILEACCESS_NONE      FileAccessType = 0
	FILEACCESS_READ      FileAccessType = 1
	FILEACCESS_WRITE     FileAccessType = 2
	FILEACCESS_EXECUTE   FileAccessType = 4
	FILEACCESS_READWRITE FileAccessType = FILEACCESS_READ | FILEACCESS_WRITE
)

func (x *FileAccessType) Append(other FileAccessType) {
	*x = FileAccessType(*x | other)
}
func (x FileAccessType) HasRead() bool {
	return (int32)(x)&int32(FILEACCESS_READ) == int32(FILEACCESS_READ)
}
func (x FileAccessType) HasWrite() bool {
	return (int32)(x)&int32(FILEACCESS_WRITE) == int32(FILEACCESS_WRITE)
}
func (x FileAccessType) HasExecute() bool {
	return (int32)(x)&int32(FILEACCESS_EXECUTE) == int32(FILEACCESS_EXECUTE)
}
func (x FileAccessType) String() string {
	var ch rune
	outp := strings.Builder{}
	outp.Grow(4)

	ch = '-'
	if x.HasRead() {
		ch = 'R'
	}
	outp.WriteRune(ch)
	ch = '-'
	if x.HasWrite() {
		ch = 'W'
	}
	outp.WriteRune(ch)
	ch = '-'
	if x.HasExecute() {
		ch = 'X'
	}
	outp.WriteRune(ch)
	return outp.String()
}
func (x *FileAccessType) Serialize(ar Archive) {
	ar.Int32((*int32)(x))
}

type FileAccessRecord struct {
	Path   Filename
	Access FileAccessType
}

func (x *FileAccessRecord) Serialize(ar Archive) {
	ar.Serializable(&x.Path)
	ar.Serializable(&x.Access)
}

type MountedPath struct {
	From Directory
	To   string
}

func (x *MountedPath) Serialize(ar Archive) {
	ar.Serializable(&x.From)
	ar.String(&x.To)
}

type ProcessOptions struct {
	Environment     ProcessEnvironment
	OnFileAccess    EventDelegate[FileAccessRecord]
	OnOutput        EventDelegate[string]
	WorkingDir      Directory
	MountedPaths    []MountedPath
	AttachDebugger  bool
	CaptureOutput   bool
	NoSpinner       bool
	UseResponseFile bool
	ExitCodeRef     *int32
}

type ProcessOptionFunc func(*ProcessOptions)

func (x *ProcessOptions) Init(options ...ProcessOptionFunc) {
	x.Environment = NewProcessEnvironment()
	for _, it := range options {
		it(x)
	}
}

func OptionProcessStruct(options *ProcessOptions) ProcessOptionFunc {
	return func(po *ProcessOptions) {
		*po = *options
	}
}
func OptionProcessEnvironment(environment ProcessEnvironment) ProcessOptionFunc {
	return func(po *ProcessOptions) {
		po.Environment.Overwrite(environment)
	}
}
func OptionProcessExitCode(exitCodeRef *int32) ProcessOptionFunc {
	return func(po *ProcessOptions) {
		po.ExitCodeRef = exitCodeRef
	}
}
func OptionProcessExport(name string, values ...string) ProcessOptionFunc {
	return func(po *ProcessOptions) {
		po.Environment.Append(name, values...)
	}
}
func OptionProcessFileAccess(onFileAccess EventDelegate[FileAccessRecord]) ProcessOptionFunc {
	return func(po *ProcessOptions) {
		po.OnFileAccess = onFileAccess
	}
}
func OptionProcessMountPath(from Directory, to string) ProcessOptionFunc {
	return func(po *ProcessOptions) {
		po.MountedPaths = append(po.MountedPaths, MountedPath{From: from, To: to})
	}
}
func OptionProcessMountedPath(mountedPaths ...MountedPath) ProcessOptionFunc {
	return func(po *ProcessOptions) {
		po.MountedPaths = append(po.MountedPaths, mountedPaths...)
	}
}
func OptionProcessOutput(onOuptut EventDelegate[string]) ProcessOptionFunc {
	return func(po *ProcessOptions) {
		po.OnOutput = onOuptut
	}
}
func OptionProcessWorkingDir(value Directory) ProcessOptionFunc {
	return func(po *ProcessOptions) {
		po.WorkingDir = value
	}
}
func OptionProcessAttachDebugger(po *ProcessOptions) {
	po.AttachDebugger = true
}
func OptionProcessAttachDebuggerIf(enabled bool) ProcessOptionFunc {
	return func(po *ProcessOptions) {
		po.AttachDebugger = enabled
	}
}
func OptionProcessCaptureOutput(po *ProcessOptions) {
	po.CaptureOutput = true
}
func OptionProcessCaptureOutputIf(enabled bool) ProcessOptionFunc {
	return func(po *ProcessOptions) {
		po.CaptureOutput = enabled
	}
}
func OptionProcessNoSpinner(po *ProcessOptions) {
	po.NoSpinner = true
}
func OptionProcessNoSpinnerIf(enabled bool) ProcessOptionFunc {
	return func(po *ProcessOptions) {
		po.NoSpinner = enabled
	}
}
func OptionProcessUseResponseFile(po *ProcessOptions) {
	po.UseResponseFile = true
}
func OptionProcessUseResponseFileIf(enabled bool) ProcessOptionFunc {
	return func(po *ProcessOptions) {
		po.UseResponseFile = enabled
	}
}

/***************************************
 * RunProcess
 ***************************************/

type RunProcessFunc = func(executable Filename, arguments StringSet, options ProcessOptions) error

var OnRunCommandWithDetours RunProcessFunc = nil
var OnRunCommandWithDebugger RunProcessFunc = nil

func RunProcess(executable Filename, arguments StringSet, userOptions ...ProcessOptionFunc) error {
	var options ProcessOptions
	options.Init(userOptions...)

	var pbar ProgressScope
	if !options.NoSpinner {
		pbar = LogSpinner(strings.Join(append([]string{executable.String()}, arguments...), " "))
		defer pbar.Close()
	}

	if options.UseResponseFile {
		tempFile, err := UFS.CreateTemp("ResponseFiles", func(w io.Writer) error {
			for i, a := range arguments {
				if i > 0 {
					if _, err := w.Write(UnsafeBytesFromString(" ")); err != nil {
						return err
					}
				}
				if _, err := w.Write(UnsafeBytesFromString(syscall.EscapeArg(a))); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		defer tempFile.Close()

		LogDebug(LogProcess, "use response file %q", tempFile.Path)
		arguments = []string{fmt.Sprint(`@`, tempFile.String())}

		if onFileAccess := options.OnFileAccess; onFileAccess != nil {
			options.OnFileAccess = func(far FileAccessRecord) error {
				if far.Path != tempFile.Path {
					// ignore response file in file accesses
					return onFileAccess(far)
				}
				return nil
			}
		}
	}

	defer LogBenchmark(LogProcess, "Run(%q, %q)", executable, MakeStringer(func() string {
		return JoinString("\", \"", arguments)
	})).Close()

	if options.OnFileAccess != nil && OnRunCommandWithDetours != nil {
		return OnRunCommandWithDetours(executable, arguments, options)
	} else if options.AttachDebugger && OnRunCommandWithDebugger != nil {
		return OnRunCommandWithDebugger(executable, arguments, options)
	} else {
		return RunProcess_Vanilla(executable, arguments, options)
	}
}

func RunProcess_Vanilla(executable Filename, arguments StringSet, options ProcessOptions) (err error) {
	cmd := exec.Command(executable.String(), arguments...)

	if len(options.WorkingDir.Path) > 0 {
		cmd.Dir = options.WorkingDir.String()
	}

	cmd.Env = append(cmd.Env, options.Environment.Export()...)

	LogTrace(LogProcess, "run %v:\n%#v", cmd, []any{cmd.Dir, executable, arguments, options})

	if options.CaptureOutput {
		var stdout io.ReadCloser
		if stdout, err = cmd.StdoutPipe(); err != nil {
			return err
		}
		defer stdout.Close()

		cmd.Stderr = cmd.Stdout

		if err = cmd.Start(); err != nil {
			return err
		}

		const capacity = LARGE_PAGE_CAPACITY / 2
		buf := TransientLargePage.Allocate()
		defer TransientLargePage.Release(buf)

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(buf, capacity)

		for scanner.Scan() {
			if line := scanner.Text(); len(line) > 0 {
				if options.OnOutput.Bound() {
					if er := options.OnOutput.Invoke(line); er != nil {
						return er
					}
				} else {
					LogForwardln(line)
				}
			}
		}

		err = cmd.Wait()

	} else {
		outputForError := TransientBuffer.Allocate()
		defer TransientBuffer.Release(outputForError)

		cmd.Stderr = outputForError
		cmd.Stdout = outputForError
		if err = cmd.Run(); err != nil {
			// print output if the command failed
			output := UnsafeStringFromBytes(outputForError.Bytes())
			if options.OnOutput.Bound() {
				if er := options.OnOutput.Invoke(output); er != nil {
					return er
				}
			} else {
				LogForward(output)
			}
		}
	}

	if exitCodeErr, ok := err.(*exec.ExitError); ok && options.ExitCodeRef != nil {
		*options.ExitCodeRef = int32(exitCodeErr.ExitCode())
	}

	return
}

/***************************************
 * Process Environment
 ***************************************/

type EnvironmentVar string

func (x EnvironmentVar) String() string { return (string)(x) }
func (x EnvironmentVar) Compare(other EnvironmentVar) int {
	return strings.Compare(x.String(), other.String())
}
func (x *EnvironmentVar) Serialize(ar Archive) {
	ar.String((*string)(x))
}

type EnvironmentDefinition struct {
	Name   EnvironmentVar
	Values StringSet
}

func (x EnvironmentDefinition) String() string {
	if len(x.Values) > 0 {
		sb := strings.Builder{}

		capacity := len(x.Name.String())
		for _, it := range x.Values {
			capacity += 1 + len(it)
		}
		sb.Grow(capacity)

		sb.WriteString(x.Name.String())
		sb.WriteRune('=')

		for i, it := range x.Values {
			if i > 0 {
				sb.WriteRune(';')
			}
			sb.WriteString(it)
		}

		return sb.String()
	} else {
		return x.Name.String()
	}
}
func (x *EnvironmentDefinition) Serialize(ar Archive) {
	ar.Serializable(&x.Name)
	ar.Serializable(&x.Values)
}

type ProcessEnvironment []EnvironmentDefinition

func NewProcessEnvironment() ProcessEnvironment {
	return ProcessEnvironment([]EnvironmentDefinition{})
}

func (x ProcessEnvironment) Export() []string {
	result := make([]string, len(x))
	for i, it := range x {
		result[i] = it.String()
	}
	return result
}
func (x ProcessEnvironment) IndexOf(name string) (int, bool) {
	for i, it := range x {
		if it.Name.String() == name {
			return i, true
		}
	}
	return len(x), false
}
func (x *ProcessEnvironment) Append(name string, values ...string) {
	if i, ok := x.IndexOf(name); ok {
		(*x)[i].Values.Append(values...)
	} else {
		*x = append(*x, EnvironmentDefinition{
			Name:   EnvironmentVar(name),
			Values: values,
		})
	}
}
func (x *ProcessEnvironment) Inherit(other ProcessEnvironment) {
	for _, it := range other {
		// #TODO: add precedence?
		x.Append(it.Name.String(), it.Values...)
	}
}
func (x *ProcessEnvironment) Overwrite(other ProcessEnvironment) {
	for _, it := range other {
		x.Append(it.Name.String(), it.Values...)
	}
}
func (x *ProcessEnvironment) Serialize(ar Archive) {
	SerializeSlice(ar, (*[]EnvironmentDefinition)(x))
}
