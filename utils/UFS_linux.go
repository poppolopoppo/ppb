//go:build linux

package utils

import (
	"fmt"
	"path/filepath"

	"github.com/poppolopoppo/ppb/internal/base"
)

func CleanPath(in string) string {
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
		base.LogPanicErr(LogUFS, err)
	}

	return in
}
