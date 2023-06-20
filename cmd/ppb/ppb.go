package main

import (
	"github.com/poppolopoppo/ppb"
	"github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * Launch Command (program entry point)
 ***************************************/

func main() {
	err := ppb.LaunchCommand("ppb")
	utils.LogPanicIfFailed(ppb.LogPPB, err)
}
