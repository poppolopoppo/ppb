package cmd

//lint:ignore ST1001 ignore dot imports warning
import . "github.com/poppolopoppo/ppb/utils"

func InitCmd() {
	RegisterSerializable(&BffBuilder{})
	RegisterSerializable(&VcxprojBuilder{})
	RegisterSerializable(&VscodeBuilder{})
}
