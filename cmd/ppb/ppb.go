package main

import (
	"github.com/poppolopoppo/ppb"
	"github.com/poppolopoppo/ppb/internal/base"
)

/***************************************
 * Launch Command (program entry point)
 ***************************************/

func main() {
	err := ppb.LaunchCommand("ppb")
	base.LogPanicIfFailed(ppb.LogPPB, err)
}
