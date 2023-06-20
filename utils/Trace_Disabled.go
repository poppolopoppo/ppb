//go:build !ppb_trace
// +build !ppb_trace

package utils

const TRACE_ENABLED = false

func StartTrace() func() {
	return func() {}
}
