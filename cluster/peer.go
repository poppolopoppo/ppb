package cluster

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/Showmax/go-fqdn"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * Peer version
 ***************************************/

type PeerVersion = string

const (
	PEERVERSION_1_0 PeerVersion = "1.0"
)

const CURRENT_PEERVERSION = PEERVERSION_1_0

/***************************************
 * Peer mode
 ***************************************/

type PeerMode int32

const (
	PEERMODE_DISABLED PeerMode = iota
	PEERMODE_IDLE
	PEERMODE_DEDICATED
	PEERMODE_PROPORTIONAL
)

func PeerModes() []PeerMode {
	return []PeerMode{
		PEERMODE_DISABLED,
		PEERMODE_IDLE,
		PEERMODE_DEDICATED,
		PEERMODE_PROPORTIONAL,
	}
}
func (x PeerMode) Equals(o PeerMode) bool {
	return (x == o)
}
func (x PeerMode) String() string {
	switch x {
	case PEERMODE_DISABLED:
		return "DISABLED"
	case PEERMODE_IDLE:
		return "IDLE"
	case PEERMODE_DEDICATED:
		return "DEDICATED"
	case PEERMODE_PROPORTIONAL:
		return "PROPORTIONAL"
	default:
		UnexpectedValue(x)
		return ""
	}
}
func (x *PeerMode) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case PEERMODE_DISABLED.String():
		*x = PEERMODE_DISABLED
	case PEERMODE_IDLE.String():
		*x = PEERMODE_IDLE
	case PEERMODE_DEDICATED.String():
		*x = PEERMODE_DEDICATED
	case PEERMODE_PROPORTIONAL.String():
		*x = PEERMODE_PROPORTIONAL
	default:
		err = MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *PeerMode) Serialize(ar Archive) {
	ar.Int32((*int32)(x))
}
func (x PeerMode) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *PeerMode) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}
func (x *PeerMode) AutoComplete(in AutoComplete) {
	for _, it := range PeerModes() {
		in.Add(it.String())
	}
}

/***************************************
 * Peer informations
 ***************************************/

type PeerInfo struct {
	Version     PeerVersion
	Addr        net.IPNet
	FQDN        string
	Hardware    PeerHardware
	PeerPort    string
	Compression CompressionFormat
}

func CurrentPeerInfo(iface net.Interface, localAddr net.IPNet, tunnelPort string, compression CompressionFormat) (peer PeerInfo, err error) {
	peer.Version = CURRENT_PEERVERSION
	peer.PeerPort = tunnelPort
	peer.Addr = localAddr
	peer.Compression = compression

	// Retrieve the FQDN (Fully Qualified Domain Name)
	peer.FQDN, err = fqdn.FqdnHostname()
	if err != nil {
		return
	}
	LogVerbose(LogCluster, "peer FQDN: %q, addr: %q, compression: %q, iface[%d]: %q (%v)", peer.FQDN, peer.Addr.IP, compression, iface.Index, iface.Name, iface.Flags)

	// Retrieve hardware survey
	peer.Hardware, err = CurrentPeerHardware()
	return
}
func (x *PeerInfo) GetAddress() string {
	return net.JoinHostPort(x.FQDN, x.PeerPort)
}
func (x *PeerInfo) String() string {
	return fmt.Sprintf("%s:%s -> %v", x.FQDN, x.PeerPort, x.Hardware)
}
func (x *PeerInfo) Load(rd io.Reader) error {
	return JsonDeserialize(x, rd)
}
func (x *PeerInfo) Save(wr *os.File) error {
	if err := wr.Chmod(0644); err != nil {
		return err
	}
	return JsonSerialize(x, wr)
}
