//go:build ppb_trace
// +build ppb_trace

package base

import (
	"os"
	"runtime/trace"
)

const TRACE_ENABLED = true

func StartTrace() func() {
	if fd, err := os.Create("app.trace"); err == nil {
		trace.Start(fd)
		return func() {
			trace.Stop()
		}
	} else {
		LogPanic(err)
	}
}
