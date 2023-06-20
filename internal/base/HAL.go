package base

import (
	"fmt"
	"strings"
)

/***************************************
 * Host Id
 ***************************************/

type HostId string

const (
	HOST_WINDOWS HostId = "WINDOWS"
	HOST_LINUX   HostId = "LINUX"
	HOST_DARWIN  HostId = "DARWIN"
)

func HostIds() []HostId {
	return []HostId{
		HOST_WINDOWS,
		HOST_LINUX,
		HOST_DARWIN,
	}
}

func (id HostId) String() string {
	return (string)(id)
}
func (x *HostId) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case HOST_WINDOWS.String():
		*x = HOST_WINDOWS
	case HOST_LINUX.String():
		*x = HOST_LINUX
	case HOST_DARWIN.String():
		*x = HOST_DARWIN
	default:
		err = MakeUnexpectedValueError(x, in)
	}
	return err
}
func (id HostId) Compare(o HostId) int {
	return strings.Compare(id.String(), o.String())
}
func (id *HostId) Serialize(ar Archive) {
	ar.String((*string)(id))
}

/***************************************
 * Host Platform
 ***************************************/

type HostPlatform struct {
	Id   HostId
	Name string
}

func (x *HostPlatform) Serialize(ar Archive) {
	ar.Serializable(&x.Id)
	ar.String(&x.Name)
}
func (x HostPlatform) String() string {
	return fmt.Sprint(x.Id, x.Name)
}

var currentHost *HostPlatform

func CurrentHost() *HostPlatform {
	return currentHost
}
func SetCurrentHost(host *HostPlatform) {
	currentHost = host
}

func IfWindows(block func()) {
	if CurrentHost().Id == HOST_WINDOWS {
		block()
	}
}
func IfLinux(block func()) {
	if CurrentHost().Id == HOST_LINUX {
		block()
	}
}
func IfDarwin(block func()) {
	if CurrentHost().Id == HOST_DARWIN {
		block()
	}
}
