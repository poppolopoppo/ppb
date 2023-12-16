//go:build darwin

package hal

import (
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/internal/hal/generic"
	"github.com/poppolopoppo/ppb/utils"
)

func InitHAL(env *utils.CommandEnv) {
	base.SetCurrentHost(&base.HostPlatform{
		Id:   HOST_DARWIN,
		Name: "TODO",
	})
	generic.InitGenericHAL()
}

func InitCompile() {
	generic.InitGenericCompile()
}
