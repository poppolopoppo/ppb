package cluster

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"

	"github.com/Showmax/go-fqdn"
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

type PeerMode byte

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
func (x PeerMode) Description() string {
	switch x {
	case PEERMODE_DISABLED:
		return "worker won't accept any remote task"
	case PEERMODE_IDLE:
		return "worker will accept remote task when idle"
	case PEERMODE_DEDICATED:
		return "worker will always accept remote task"
	case PEERMODE_PROPORTIONAL:
		return "worker will accept remote task until according to machine usage"
	default:
		base.UnexpectedValue(x)
		return ""
	}
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
		base.UnexpectedValue(x)
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
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *PeerMode) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x PeerMode) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *PeerMode) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *PeerMode) AutoComplete(in base.AutoComplete) {
	for _, it := range PeerModes() {
		in.Add(it.String(), it.Description())
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
	Compression base.CompressionFormat
}

func CurrentPeerInfo(iface net.Interface, localAddr net.IPNet, tunnelPort string, compression base.CompressionFormat) (peer PeerInfo, err error) {
	peer.Version = CURRENT_PEERVERSION
	peer.PeerPort = tunnelPort
	peer.Addr = localAddr
	peer.Compression = compression

	// Retrieve the FQDN (Fully Qualified Domain Name)
	peer.FQDN, err = fqdn.FqdnHostname()
	if err != nil {
		return
	}
	base.LogVerbose(LogCluster, "peer FQDN: %q, addr: %q, compression: %q, iface[%d]: %q (%v)", peer.FQDN, peer.Addr.IP, compression, iface.Index, iface.Name, iface.Flags)

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
	return base.JsonDeserialize(x, rd)
}
func (x *PeerInfo) Save(wr *os.File) error {
	if err := wr.Chmod(0644); err != nil {
		return err
	}
	return base.JsonSerialize(x, wr)
}
