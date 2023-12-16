package generic

import (
	"bufio"
	"io"
	"path/filepath"
	"unicode"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/internal/base"

	. "github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * GnuDepFile
 ***************************************/

type GnuDepFile struct {
	Dependencies FileSet
}

func (x *GnuDepFile) Load(src Filename) error {
	x.Dependencies = FileSet{}

	return UFS.Open(src, func(rd io.Reader) error {
		var rb bufio.Reader
		rb.Reset(rd)

		buf := base.TransientBuffer.Allocate()
		defer base.TransientBuffer.Release(buf)

		var err error
		for {
			if _, err = rb.ReadString(':'); err != nil {
				break
			}

			appendFile := func() {
				filename := base.UnsafeStringFromBuffer(buf)
				if len(filename) > 0 {
					if filepath.IsLocal(filename) {
						x.Dependencies.AppendUniq(UFS.Root.AbsoluteFile(filename).Normalize())
					} else {
						x.Dependencies.AppendUniq(MakeFilename(filename))
					}
					// LogDebug("gnu-dep-file: parsed source file name %q", x.Dependencies[len(x.Dependencies)-1])
				}
				buf.Reset()
			}

			for {
				ch, _, err := rb.ReadRune()
				if err != nil {
					break
				}
				if ch == '\\' {
					ch2, _, err := rb.ReadRune()

					if err == nil {
						if ch2 == ' ' {
							if _, err := buf.WriteRune(ch2); err != nil {
								return err
							}
							continue
						} else if unicode.IsSpace(ch2) {
							appendFile()
							continue // skip endline
						} else {
							if err := rb.UnreadRune(); err != nil {
								return err
							}
						}
					}
				}

				if unicode.IsSpace(ch) {
					appendFile()
				} else {
					if _, err := buf.WriteRune(ch); err != nil {
						return err
					}
				}
			}

			appendFile()
		}
		if err == io.EOF {
			err = nil
		}
		return nil
	})
}

/***************************************
 * GnuDepFileAction
 ***************************************/

type GnuSourceDependenciesAction struct {
	action.ActionRules
	GnuDepFile Filename
}

func (x *GnuSourceDependenciesAction) Alias() BuildAlias {
	return MakeBuildAlias("Action", "Gnu", x.Outputs.Join(";"))
}
func (x *GnuSourceDependenciesAction) Build(bc BuildContext) error {
	// compile the action with /sourceDependencies
	if err := x.ActionRules.Build(bc); err != nil {
		return err
	}

	// track json file as an output dependency (check file exists)
	if err := bc.OutputFile(x.GnuDepFile); err != nil {
		return err
	}

	// parse source dependencies outputted by cl.exe
	var sourceDeps GnuDepFile
	if err := sourceDeps.Load(x.GnuDepFile); err != nil {
		return err
	}

	// add all parsed filenames as dynamic dependencies: when a header is modified, this action will have to be rebuild
	base.LogDebug(LogGeneric, "gnu-dep-file: parsed output in %q\n%v", x.GnuDepFile, base.MakeStringer(func() string {
		return base.PrettyPrint(sourceDeps.Dependencies)
	}))

	return bc.NeedFiles(sourceDeps.Dependencies...)
}
func (x *GnuSourceDependenciesAction) Serialize(ar base.Archive) {
	ar.Serializable(&x.ActionRules)
	ar.Serializable(&x.GnuDepFile)
}
