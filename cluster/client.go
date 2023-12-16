package cluster

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * Client
 ***************************************/

type RemoteWorker struct {
	Available atomic.Bool
	*Tunnel
	Inbox chan MessageBody
}

type Client struct {
	Cluster *Cluster
	PeerAvaibility
	PeerInfo
	Webdav *WebdavServer
	*WorkerFlags

	async     base.Future[int]
	cancel    context.CancelFunc
	context   context.Context
	waitGroup sync.WaitGroup
}

func NewClient(cluster *Cluster) (*Client, error) {
	localPeer, err := cluster.GetPeerInfo()
	if err != nil {
		return nil, err
	}

	return &Client{
		Cluster:        cluster,
		PeerAvaibility: newPeerAvaibility(),
		PeerInfo:       localPeer,
		Webdav:         NewWebdavServer(cluster.Context),
		WorkerFlags:    GetWorkerFlags(),
	}, nil
}

func (x *Client) Start() (context.CancelFunc, error) {
	if _, err := x.Cluster.Discover(); err != nil {
		return nil, err
	}

	x.context, x.cancel = context.WithCancel(x.Cluster.Context)

	if err := x.Webdav.Start(x.context, x.Addr.IP, x.Cluster.GetWebdavPort()); err != nil {
		return nil, err
	}

	localCancel := x.cancel
	x.async = base.MakeFuture[int](func() (int, error) {
		ticker := time.NewTicker(x.Cluster.GetTimeoutDuration() / 3)
		defer func() {
			localCancel()
			x.waitGroup.Wait()
			ticker.Stop()
		}()

		for {
			select {
			case <-x.context.Done():
				return -1, x.context.Err()

			case <-ticker.C:
				if _, err := x.UpdateResources(x.context, x.WorkerFlags, &x.Hardware, int32(base.GetGlobalThreadPool().GetWorkload())); err != nil {
					base.LogError(LogCluster, "local peer update failed: %v", err)
					return -2, err
				}

				if _, err := x.Cluster.Discover(); err != nil {
					base.LogError(LogCluster, "client discovery failed: %v", err)
					return -3, err
				}
			}
		}
	})

	return x.cancel, nil
}
func (x *Client) Close() (err error) {
	if x.cancel != nil {
		x.cancel()
		x.cancel = nil
	}

	if x.async != nil {
		err = x.async.Join().Failure()
		x.async = nil
	}

	if x.Webdav != nil {
		if er := x.Webdav.Close(); er != nil && err == nil {
			err = er
		}
	}

	x.context = nil
	return err
}

func (x *Client) MountDir(d Directory) (bool, Directory) {
	for _, part := range x.Webdav.partitions {
		if part.Mountpoint.IsParentOf(d) {
			endpoint := part.GetEndpoint(x.Webdav)
			relative := SanitizePath(d.Path[len(part.Mountpoint.Path):], '/')
			return true, Directory{
				Path: endpoint + relative,
			}
		}
	}
	return false, d
}
func (x *Client) MountFile(f Filename) (bool, Filename) {
	if ok, d := x.MountDir(f.Dirname); ok {
		return true, Filename{
			Dirname:  d,
			Basename: f.Basename,
		}
	} else {
		return false, f
	}
}
func (x *Client) GetMountedPaths(po *internal_io.ProcessOptions) {
	for _, part := range x.Webdav.partitions {
		internal_io.OptionProcessMountPath(part.Mountpoint, part.GetEndpoint(x.Webdav))(po)
	}
}

func (x *Client) DispatchTask(executable Filename, arguments base.StringSet, options *internal_io.ProcessOptions) (*PeerDiscovered, bool, error) {
	timeoutSpan := x.Cluster.GetTimeoutDuration()
	timeoutCtx, timeoutCancel := context.WithTimeout(x.context, timeoutSpan)
	defer timeoutCancel()

	for r := 0; r < x.Cluster.RetryCount.Get(); r++ {
		peer, ok := x.Cluster.RandomPeer(timeoutSpan)
		if !ok {
			n, err := x.Cluster.Discover()
			if n == 0 || err != nil {
				return nil, false, err
			}
			continue
		}

		tunnel, err := x.connectToPeer(timeoutCtx, peer)
		if err != nil {
			base.LogError(LogCluster, "failed to connect to remote peer %v (RETRY=%d)",
				net.JoinHostPort(peer.FQDN, peer.PeerPort), r+1)
			continue
		}

		taskStarted := false
		tunnel.OnTaskStart.Add(func(mts *MessageTaskStart) error {
			taskStarted = mts.WasStarted()
			return mts.Err()
		})

		err = x.runTaskOnTunnelImmediate(tunnel, executable, arguments, options)
		if taskStarted {
			return peer, taskStarted, err
		}
	}

	return nil, false, nil // no worker available
}
func (x *Client) AsyncDispatchTask(executable Filename, arguments base.StringSet, options *internal_io.ProcessOptions) (*PeerDiscovered, base.Future[int]) {
	timeoutSpan := x.Cluster.GetTimeoutDuration()
	timeoutCtx, timeoutCancel := context.WithTimeout(x.context, timeoutSpan)
	defer timeoutCancel()

	for r := 0; r < x.Cluster.RetryCount.Get(); r++ {
		peer, ok := x.Cluster.RandomPeer(timeoutSpan)
		if !ok {
			n, err := x.Cluster.Discover()
			if n == 0 || err != nil {
				return nil, nil
			}
			continue
		}

		tunnel, err := x.connectToPeer(timeoutCtx, peer)
		if err != nil {
			base.LogError(LogCluster, "failed to connect to remote peer %v (RETRY=%d)",
				net.JoinHostPort(peer.FQDN, peer.PeerPort), r+1)
			continue
		}

		x.waitGroup.Add(1)

		return peer, base.MakeFuture(func() (int, error) {
			defer func() {
				tunnel.Close()
				x.waitGroup.Done()
			}()

			var exitCode int = -1

			tunnel.OnTaskStop.Add(func(mts *MessageTaskStop) error {
				exitCode = int(mts.ExitCode)
				return mts.Err()
			})

			return exitCode, x.runTaskOnTunnelImmediate(tunnel, executable, arguments, options)
		})
	}

	return nil, nil // no worker available
}

func (x *Client) runTaskOnTunnelImmediate(tunnel *Tunnel, executable Filename, arguments base.StringSet, options *internal_io.ProcessOptions) error {
	x.waitGroup.Add(1)
	defer func() {
		tunnel.Close()
		x.waitGroup.Done()
	}()

	tunnel.OnReadyForWork = x.ReadyForWork

	if options.OnOutput.Bound() {
		tunnel.OnTaskOutput.Add(func(mto *MessageTaskOutput) error {
			for _, outp := range mto.Outputs {
				if err := options.OnOutput.Invoke(outp); err != nil {
					return err
				}
			}
			return nil
		})
	}
	if options.OnFileAccess.Bound() {
		tunnel.OnTaskFileAccess.Add(func(mtfa *MessageTaskFileAccess) error {
			for _, far := range mtfa.Records {
				if err := options.OnFileAccess.Invoke(far); err != nil {
					return err
				}
			}
			return nil
		})
	}

	bootstrapDispatch := NewMessageTaskDispatch(executable, arguments,
		options.WorkingDir, options.Environment, options.MountedPaths, options.UseResponseFile)
	return MessageLoop(tunnel, x.context, x.Cluster.GetTimeoutDuration(), &bootstrapDispatch)
}

func (x *Client) connectToPeer(ctx context.Context, peer *PeerDiscovered) (*Tunnel, error) {
	addr := peer.GetAddress()
	base.LogInfo(LogCluster, "dialing remote worker %s", addr)

	tunnel, err := NewDialTunnel(x.Cluster, ctx, addr, x.Cluster.GetTimeoutDuration(),
		base.CompressionOptionFormat(peer.Compression),
		base.CompressionOptionLevel(x.Cluster.Compression.Level),
		base.CompressionOptionDictionary(x.Cluster.Compression.Dictionary))

	if err != nil {
		return nil, err
	}

	base.LogVerbose(LogCluster, "connected to remote worker %v (from %v):\n%v", addr, tunnel.conn.LocalAddr(), peer.Hardware)
	return tunnel, nil
}
