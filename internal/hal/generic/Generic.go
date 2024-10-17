package generic

import (
	"github.com/poppolopoppo/ppb/internal/base"
)

var LogGeneric = base.NewLogCategory("Generic")

func InitGenericHAL() {

}

func InitGenericCompile() {
	base.RegisterSerializable[GnuSourceDependenciesAction]()

	base.RegisterSerializable[ExternalSDKHeaderGenerator]()
	base.RegisterSerializable[ExternalSDKGeneratedHeader]()

	base.RegisterSerializable[VulkanHeaderGenerator]()
	base.RegisterSerializable[VulkanGeneratedHeader]()

	base.RegisterSerializable[VulkanSourceGenerator]()
	base.RegisterSerializable[VulkanGeneratedSource]()

	base.RegisterSerializable[VulkanHeaders]()
	base.RegisterSerializable[VulkanBindings]()
	base.RegisterSerializable[VulkanInterface]()

	base.RegisterSerializable[VkFunctionPointer]()
}
