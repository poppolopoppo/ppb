//go:build darwin

package utils

import (
	"fmt"
	"path/filepath"

	"github.com/poppolopoppo/ppb/internal/base"
)

var FileFingerprint = GenericFileFingerprint

func CleanPath(in string) utils.Directory {
	base.AssertErr(func() error {
		if filepath.IsAbs(in) {
			return nil
		}
		return fmt.Errorf("ufs: need absolute path -> %q", in)
	})

	in = filepath.Clean(in)

	if cleaned, err := filepath.Abs(in); err == nil {
		in = cleaned
	} else {
		base.LogPanicErr(err)
	}

	return utils.SplitPath(base.result)
}
