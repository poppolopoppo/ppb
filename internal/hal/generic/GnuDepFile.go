package generic

import (
	"bufio"
	"io"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

var LogGnuDepFile = base.NewLogCategory("GnuDep")

/***************************************
 * GnuDepFile
 ***************************************/

type GnuDepFile struct {
	Dependencies utils.FileSet
}

func (x *GnuDepFile) Load(src utils.Filename) error {
	x.Dependencies = utils.FileSet{}
	// base.LogTrace(LogGnuDepFile, "%v: start parsing gnu dependency file", src)

	return utils.UFS.Open(src, func(rd io.Reader) error {
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
				if filename := buf.String(); len(filename) > 0 {
					if filepath.IsLocal(filename) {
						x.Dependencies.AppendUniq(utils.UFS.Root.AbsoluteFile(filename).Normalize())
					} else {
						x.Dependencies.AppendUniq(utils.MakeFilename(strings.Clone(filename)))
					}
					// base.LogTrace(LogGnuDepFile, "%v: parsed source file name %q", src, x.Dependencies[len(x.Dependencies)-1])
				}
				buf.Truncate(0)
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
	GnuDepFile utils.Filename
	action.ActionRules
}

func (x *GnuSourceDependenciesAction) Build(bc utils.BuildContext) error {
	// compile the action with /sourceDependencies
	return x.ActionRules.BuildWithSourceDependencies(bc, x)
}

func (x *GnuSourceDependenciesAction) GetActionSourceDependencies(bc utils.BuildContext) (sourceFiles utils.FileSet, err error) {
	// track json file as an output dependency (and check if file exists)
	if err = bc.OutputFile(x.GnuDepFile); err != nil {
		return
	}

	// parse source dependencies outputted by a GNU compiler
	var sourceDeps GnuDepFile
	if err = sourceDeps.Load(x.GnuDepFile); err != nil {
		return
	}

	// add all parsed filenames as dynamic dependencies: when a dependency is modified, this action will have to be rebuild
	base.LogDebug(LogGnuDepFile, "gnu-dep-file: parsed output in %q\n%v", x.GnuDepFile, base.MakeStringer(func() string {
		return base.PrettyPrint(sourceDeps.Dependencies)
	}))

	return sourceDeps.Dependencies, nil
}

func (x *GnuSourceDependenciesAction) Serialize(ar base.Archive) {
	ar.Serializable(&x.GnuDepFile)
	ar.Serializable(&x.ActionRules)
}
