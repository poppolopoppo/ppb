package cluster

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"

	"github.com/quic-go/quic-go"
)

/***************************************
 * Worker Flags
 ***************************************/

type WorkerFlags struct {
	Broadcast     IntVar
	IdleCooldown  IntVar
	IdleThreshold IntVar
	Mode          PeerMode
	MaxThreads    IntVar
	MinFreeMemory base.SizeInBytes
}

var GetWorkerFlags = NewCommandParsableFlags(&WorkerFlags{
	Broadcast:     2,            // 2 seconds
	IdleCooldown:  5 * 60,       // 5 minutes
	IdleThreshold: 10,           // bellow 10% cpu usage
	MinFreeMemory: 4 * base.GiB, // min 4GiB available
	MaxThreads:    0,            // uncapped thread count
	Mode:          PEERMODE_PROPORTIONAL,
})

func (x *WorkerFlags) GetBroadcastDuraction() time.Duration {
	return time.Duration(x.Broadcast.Get()) * time.Second
}

func (x *WorkerFlags) Flags(cfv CommandFlagsVisitor) {
	cfv.Persistent("Broadcast", "set worker broadcast delay in seconds", &x.Broadcast)
	cfv.Persistent("PeerMode", "set peer mode", &x.Mode)
	cfv.Persistent("IdleCooldown", "set peer cooldown in seconds to be considered idle (used by IDLE peer mode)", &x.IdleCooldown)
	cfv.Persistent("IdleThreshold", "set peer maximum CPU usage percentage to be considered idle (used by IDLE peer mode)", &x.IdleThreshold)
	cfv.Persistent("MaxThreads", "set peer maximum concurrent threads for distributed tasks", &x.MaxThreads)
	cfv.Persistent("MinFreeMemory", "set peer minimum memory available to accept a job", &x.MinFreeMemory)
}

/***************************************
 * Worker
 ***************************************/

type Worker struct {
	await           base.Future[int]
	numJobsInFlight atomic.Int32

	Cluster *Cluster

	PeerAvaibility
	PeerDiscovered

	*WorkerFlags
}

func NewWorker(cluster *Cluster) (*Worker, error) {
	localPeer, err := cluster.GetPeerInfo()
	if err != nil {
		return nil, err
	}

	worker := &Worker{
		Cluster:        cluster,
		PeerAvaibility: newPeerAvaibility(),
		PeerDiscovered: PeerDiscovered{
			LastSeen: time.Now(),
			PeerInfo: localPeer,
		},
		WorkerFlags: GetWorkerFlags(),
	}

	if worker.MaxThreads.Get() > 0 && worker.Hardware.Threads > int32(worker.MaxThreads) {
		worker.Hardware.Threads = int32(worker.MaxThreads)
	}
	return worker, nil
}

func (x *Worker) Start() (context.CancelFunc, error) {
	x.numJobsInFlight.Store(0)

	listener, err := quic.ListenAddr(net.JoinHostPort("", x.PeerPort), generateServerTLSConfig(), nil)
	if err != nil {
		return nil, err
	}

	base.LogClaim(LogCluster, "start worker, listening on %q", listener.Addr())

	_, x.PeerPort, err = net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return nil, err
	}

	if err := x.updateDiscovery(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(x.Cluster.Context)

	x.await = base.MakeAsyncFuture[int](func() (int, error) {
		// broadcast discovery in a separate dependent go channel
		defer base.MakeAsyncFuture(func() (int, error) {
			broadcastTick := time.NewTicker(x.GetBroadcastDuraction())
			defer broadcastTick.Stop()

			for {
				select {
				case <-ctx.Done():
					return -1, ctx.Err()
				case <-broadcastTick.C:
					if err := x.updateDiscovery(); err != nil {
						if !os.IsTimeout(err) {
							base.LogWarning(LogCluster, "caught error during discovery: %v", err)
							return -2, err
						}
					}
				}
			}
		}).Join()

		defer x.Cluster.Disapear(&x.PeerInfo)
		defer listener.Close()
		defer cancel()

		wg := sync.WaitGroup{}
		defer wg.Wait()

		base.FlushLog()

		for {
			select {
			case <-ctx.Done():
				return -1, ctx.Err()
			default:
				if err := x.acceptClientConn(ctx, listener, &wg); err != nil {
					if !os.IsTimeout(err) {
						base.LogWarning(LogCluster, "caught error while accepting client: %v", err)
						return -3, err
					}
				}
			}
		}
	})

	return cancel, nil
}
func (x *Worker) updateDiscovery() error {
	changed, err := x.UpdateResources(x.Cluster.Context, x.WorkerFlags, &x.Hardware, x.numJobsInFlight.Load())
	if err != nil {
		return err
	}

	base.LogVeryVerbose(LogCluster, "available resources: mode=%v, %d jobs, threads=%d/%d, cpu=%5.2f%%, mem=%5.2f%% (%v / %v), idle since=%v",
		x.Mode,
		x.numJobsInFlight.Load(),
		x.AvailableThreads.Load(),
		x.Hardware.Threads,
		x.GetAverageCpuUsage()*100,
		x.GetAverageMemUsage()*100,
		base.SizeInBytes(x.AvailableVirtualMemory.Load()),
		base.SizeInBytes(x.Hardware.VirtualMemory),
		time.Since(x.idleSince))

	if x.ReadyForWork() {
		if changed {
			x.PeerDiscovered.LastSeen = time.Now()
			if err = x.Cluster.Announce(&x.PeerInfo); err != nil {
				err = fmt.Errorf("failed to announce worker in cluster: %v", err)
			}
		} else if time.Since(x.PeerDiscovered.LastSeen) > x.Cluster.GetTimeoutDuration()/2 {
			x.PeerDiscovered.LastSeen = time.Now()
			if err = x.Cluster.Touch(&x.PeerInfo); err != nil {
				err = fmt.Errorf("failed to touch worker in cluster: %v", err)
			}
		}
	} else {
		if changed {
			if err = x.Cluster.Disapear(&x.PeerInfo); err != nil {
				err = fmt.Errorf("failed to disapear in worker cluster: %v", err)
			}
		}
	}

	return err
}
func (x *Worker) Close() error {
	return x.await.Join().Failure()
}

func (x *Worker) acceptClientConn(ctx context.Context, listener *quic.Listener, wg *sync.WaitGroup) error {
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, x.Cluster.GetTimeoutDuration())
	defer timeoutCancel()

	conn, err := listener.Accept(timeoutCtx)
	if err != nil {
		return err
	}

	wg.Add(1) // wait for the new connection to be finished

	go func() {
		defer wg.Done() // signal connection closed for listen loop
		defer conn.CloseWithError(0, "close connection")

		tunnel, err := x.createAcceptTunnel(ctx, conn)
		if err != nil {
			base.LogError(LogCluster, "worker handshake with %v failed: %v", conn.RemoteAddr(), err)
			return
		}
		defer tunnel.Close()

		tunnel.OnReadyForWork = x.ReadyForWork

		base.LogInfo(LogCluster, "worker accepted connection from %v", conn.RemoteAddr())

		MessageLoop(tunnel, ctx, x.Cluster.GetTimeoutDuration())
	}()
	return nil
}

func (x *Worker) createAcceptTunnel(ctx context.Context, conn quic.Connection) (*Tunnel, error) {
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, x.Cluster.GetTimeoutDuration())
	defer timeoutCancel()

	tunnel, err := NewListenTunnel(x.Cluster, timeoutCtx, conn, x.Cluster.GetTimeoutDuration(), x.Cluster.Compression.Options)
	if err != nil {
		return nil, err
	}

	tunnel.OnTaskDispatch = func(executable Filename, arguments base.StringSet, options *internal_io.ProcessOptions) error {
		x.numJobsInFlight.Add(1)
		defer x.numJobsInFlight.Add(-1)
		return internal_io.RunProcess(executable, arguments, internal_io.OptionProcessStruct(options))
	}

	return tunnel, nil
}
