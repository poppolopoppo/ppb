//go:build darwin

package hal

import (
	"net"

	"github.com/poppolopoppo/ppb/internal/hal/generic"
	"github.com/poppolopoppo/ppb/utils"
)

func InitHAL(env *utils.CommandEnv) {
	utils.SetCurrentHost(&utils.HostPlatform{
		Id:   HOST_DARWIN,
		Name: "TODO",
	})
	utils.FBUILD_BIN = utils.UFS.Build.Folder("hal", "darwin", "bin").File("fbuild")
	generic.InitGenericHAL()
}

func InitCompile() {
	generic.InitGenericCompile()
}
