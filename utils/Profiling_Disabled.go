//go:build !ppb_profiling
// +build !ppb_profiling

package utils

const PROFILING_ENABLED = false

func StartProfiling() func() {
	return PurgeProfiling
}

func PurgeProfiling() {}
