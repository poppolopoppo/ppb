package cluster

import (
	"context"
	"io"
	"net"
	"strconv"
	"time"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var LogCluster = NewLogCategory("Cluster")

func InitCluster() {
	RegisterSerializable(&MessagePing{})
	RegisterSerializable(&MessagePong{})
	RegisterSerializable(&MessageTaskDispatch{})
	RegisterSerializable(&MessageTaskStart{})
	RegisterSerializable(&MessageTaskFileAccess{})
	RegisterSerializable(&MessageTaskOutput{})
	RegisterSerializable(&MessageTaskStop{})
}

/***************************************
 * Cluster
 ***************************************/

type Cluster struct {
	PeerDiscovery
	ClusterOptions
}

func NewCluster(options ...ClusterOption) Cluster {
	settings := NewClusterOptions(options...)
	return Cluster{
		PeerDiscovery:  NewPeerDiscovery(settings.BrokeragePath, CURRENT_PEERVERSION, settings.MaxPeers.Get()),
		ClusterOptions: settings,
	}
}

func (x *Cluster) GetPeerInfo() (PeerInfo, error) {
	return CurrentPeerInfo(x.NetInterface, x.NetAddr, x.GetTunnelPort(), x.Compression.Format)
}

func (x *Cluster) StartClient() (client *Client, cancel context.CancelFunc, err error) {
	client, err = NewClient(x)
	if err == nil {
		cancel, err = client.Start()
	}
	return
}
func (x *Cluster) StartWorker() (worker *Worker, cancel context.CancelFunc, err error) {
	worker, err = NewWorker(x)
	if err == nil {
		cancel, err = worker.Start()
	}
	return
}

func (x *Cluster) Discover() (int, error) {
	return x.PeerDiscovery.Discover(x.RetryCount.Get(), x.GetTimeoutDuration())
}

/***************************************
 * Cluster Flags
 ***************************************/

type ClusterFlags struct {
	BrokeragePath Directory
	MaxPeers      IntVar
	IfIndex       IntVar
	RetryCount    IntVar
	Timeout       IntVar
	TunnelPort    IntVar
	WebdavPort    IntVar
}

var GetClusterFlags = NewCommandParsableFlags(&ClusterFlags{
	BrokeragePath: UFS.Transient.Folder("Brokerage"),
	MaxPeers:      32,
	IfIndex:       0,
	RetryCount:    5,
	Timeout:       3,
	TunnelPort:    0,
	WebdavPort:    0,
})

func (x *ClusterFlags) GetTimeoutDuration() time.Duration {
	return time.Duration(x.Timeout.Get()) * time.Second
}
func (x *ClusterFlags) GetTunnelPort() string {
	return strconv.Itoa(x.TunnelPort.Get())
}
func (x *ClusterFlags) GetWebdavPort() string {
	return strconv.Itoa(x.WebdavPort.Get())
}

func (x *ClusterFlags) Flags(cfv CommandFlagsVisitor) {
	cfv.Persistent("BrokeragePath", "set peer discovery brokerage path", &x.BrokeragePath)
	cfv.Persistent("MaxPeers", "set maximum number of connected peers allowed", &x.MaxPeers)
	cfv.Persistent("NetInterface", "set index of network interface for the cluster", &x.IfIndex)
	cfv.Persistent("RetryCount", "set peer retry count when an error occured", &x.RetryCount)
	cfv.Persistent("Timeout", "set peer tunnel timeout in seconds", &x.Timeout)
	cfv.Persistent("TunnelPort", "set peer TCP port used for communicating with cluster", &x.TunnelPort)
	cfv.Persistent("WebdavPort", "set peer TCP port used for webdav file sharing", &x.WebdavPort)
}

/***************************************
 * Cluster options
 ***************************************/

type StreamReadFunc = func(io.Reader, []byte) (int, error)
type StreamWriteFunc = func(io.Writer, []byte) (int, error)

type ClusterOptions struct {
	Context context.Context

	Compression CompressionOptions

	NetInterface net.Interface
	NetAddr      net.IPNet

	StreamRead  StreamReadFunc
	StreamWrite StreamWriteFunc

	UncompressRead StreamReadFunc
	CompressWrite  StreamWriteFunc

	*ClusterFlags
}

type ClusterOption = func(*ClusterOptions)

func NewClusterOptions(options ...ClusterOption) (result ClusterOptions) {
	result.ClusterFlags = GetClusterFlags()
	result.Context = context.Background()

	// setup compression for peer communication (zstd is more efficient for small network packets)
	result.Compression = NewCompressionOptions(
		CompressionOptionFormat(COMPRESSION_FORMAT_ZSTD),
		CompressionOptionLevel(COMPRESSION_LEVEL_BALANCED),
		// zstd is more efficient with small paylaods when using a pre-trained dictionary
		// https://github.com/facebook/zstd#dictionary-compression-how-to
		CompressionOptionDictionaryFile(UFS.Internal.Folder("zstd").File("ppb-message-dict.zstd")))

	// used for stats recording
	result.StreamRead = func(r io.Reader, b []byte) (int, error) {
		return r.Read(b)
	}
	result.UncompressRead = result.StreamRead
	result.StreamWrite = func(w io.Writer, b []byte) (int, error) {
		return w.Write(b)
	}
	result.CompressWrite = result.StreamWrite

	// select default network interface
	if result.ClusterFlags.IfIndex.IsInheritable() {
		if iface, addr, err := GetFirstNetInterface(); err == nil {
			result.NetInterface = iface
			result.NetAddr = addr

			// save results to avoid scanning for future runs
			result.ClusterFlags.IfIndex = InheritableInt(iface.Index)
		} else {
			LogPanicErr(LogCluster, err)
		}

	} else {
		interfaces, err := net.Interfaces()
		LogPanicIfFailed(LogCluster, err)

		// retrieve selected network interface by index
		for _, iface := range interfaces {
			if result.ClusterFlags.IfIndex.Get() == iface.Index {
				result.NetInterface = iface

				addrs, err := iface.Addrs()
				if err != nil {
					return
				}

				for _, addr := range addrs {
					if ip, ok := addr.(*net.IPNet); ok && ip.IP.To4() != nil {
						result.NetAddr = *ip
						break
					}
				}

				break
			}
		}
	}

	for _, opt := range options {
		opt(&result)
	}
	return
}

func ClusterOptionBrokeragePath(path Directory) ClusterOption {
	return func(co *ClusterOptions) {
		co.BrokeragePath = path
	}
}
func ClusterOptionContext(ctx context.Context) ClusterOption {
	return func(co *ClusterOptions) {
		co.Context = ctx
	}
}
func ClusterOptionCompression(comp CompressionOptions) ClusterOption {
	return func(co *ClusterOptions) {
		co.Compression = comp
	}
}
func ClusterOptionMaxPeers(n int) ClusterOption {
	return func(co *ClusterOptions) {
		co.MaxPeers.Assign(n)
	}
}
func ClusterOptionNetInterface(iface net.Interface) ClusterOption {
	return func(co *ClusterOptions) {
		co.NetInterface = iface
		co.NetAddr = net.IPNet{}

		addrs, err := iface.Addrs()
		if err != nil {
			return
		}

		for _, addr := range addrs {
			if ip, ok := addr.(*net.IPNet); ok && ip.IP.To4() != nil {
				co.NetAddr = *ip
				break
			}
		}
	}
}
func ClusterOptionRetryCount(n int) ClusterOption {
	return func(co *ClusterOptions) {
		co.RetryCount.Assign(n)
	}
}
func ClusterOptionStreamRead(read StreamReadFunc) ClusterOption {
	return func(co *ClusterOptions) {
		co.StreamRead = read
	}
}
func ClusterOptionStreamWrite(write StreamWriteFunc) ClusterOption {
	return func(co *ClusterOptions) {
		co.StreamWrite = write
	}
}
func ClusterOptionUncompressRead(read StreamReadFunc) ClusterOption {
	return func(co *ClusterOptions) {
		co.UncompressRead = read
	}
}
func ClusterOptionCompressWrite(write StreamWriteFunc) ClusterOption {
	return func(co *ClusterOptions) {
		co.CompressWrite = write
	}
}
func ClusterOptionTimeout(every time.Duration) ClusterOption {
	return func(co *ClusterOptions) {
		co.Timeout.Assign(int(every.Seconds()))
	}
}
func ClusterOptionTunnelPort(n int) ClusterOption {
	return func(co *ClusterOptions) {
		co.TunnelPort.Assign(n)
	}
}
func ClusterOptionWebdavPort(n int) ClusterOption {
	return func(co *ClusterOptions) {
		co.WebdavPort.Assign(n)
	}
}
