package action

import (
	"context"
	"io"
	"strings"
	"sync/atomic"

	"github.com/poppolopoppo/ppb/cluster"
	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var LogActionDist = base.NewLogCategory("ActionDist")

/***************************************
 * ActionDist
 ***************************************/

type ActionDist interface {
	GetBrokeragePath() Directory
	GetDistStats() *ActionDistStats

	CanDistribute(force bool) bool

	DistributeAction(a BuildAlias, executable Filename, arguments base.StringSet, options *internal_io.ProcessOptions) (*cluster.PeerDiscovered, error)
	AsyncDistributeAction(a BuildAlias, executable Filename, arguments base.StringSet, options *internal_io.ProcessOptions) (base.Future[int], bool)
}

type actionDist struct {
	cancel  context.CancelFunc
	cluster cluster.Cluster
	client  *cluster.Client
	stats   ActionDistStats
}

var GetActionDist = base.Memoize(func() ActionDist {
	var result *actionDist
	var err error

	result = new(actionDist)
	result.cluster = cluster.NewCluster(
		cluster.ClusterOptionStreamRead(func(r io.Reader, b []byte) (n int, err error) {
			stat := StartBuildStats()
			defer result.stats.StreamRead.Append(&stat)

			n, err = r.Read(b)
			result.stats.StatReadCompressed(n)
			return
		}),
		cluster.ClusterOptionUncompressRead(func(r io.Reader, b []byte) (n int, err error) {
			n, err = r.Read(b)
			result.stats.StatReadUncompressed(n)
			return
		}),
		cluster.ClusterOptionStreamWrite(func(w io.Writer, b []byte) (n int, err error) {
			stat := StartBuildStats()
			defer result.stats.StreamWrite.Append(&stat)

			n, err = w.Write(b)
			result.stats.StatWriteCompressed(n)
			return
		}),
		cluster.ClusterOptionCompressWrite(func(w io.Writer, b []byte) (n int, err error) {
			n, err = w.Write(b)
			result.stats.StatWriteUncompressed(n)
			return
		}))
	result.client, result.cancel, err = result.cluster.StartClient()
	base.LogPanicIfFailed(LogActionDist, err)

	CommandEnv.OnExit(func(cet *CommandEnvT) error {
		result.cancel()
		err := result.client.Close()
		if GetCommandFlags().Summary.Get() {
			result.stats.Print()
		}
		return err
	})

	return result
})

func (x *actionDist) GetBrokeragePath() Directory    { return x.cluster.BrokeragePath }
func (x *actionDist) GetDistStats() *ActionDistStats { return &x.stats }

func (x *actionDist) CanDistribute(force bool) bool {
	return force || !x.client.ReadyForWork()
}
func (x *actionDist) DistributeAction(a BuildAlias, executable Filename, arguments base.StringSet, options *internal_io.ProcessOptions) (*cluster.PeerDiscovered, error) {
	distributeStat := StartBuildStats()
	defer x.stats.DistributeAction.Append(&distributeStat)

	remoteOptions := *options
	x.client.GetMountedPaths(&remoteOptions)

	peer, ok, err := x.client.DispatchTask(executable, arguments, &remoteOptions)

	if peer != nil && ok {
		x.stats.AddRemoteAction(peer)
		base.LogVeryVerbose(LogActionDist, "action %q distributed to remote peer %v", a, peer)
	} else {
		x.stats.AddRemoteFailure()
		base.LogWarning(LogActionDist, "action %q could not be distributed: %v", a, err)
		peer = nil
	}

	return peer, err
}
func (x *actionDist) AsyncDistributeAction(a BuildAlias, executable Filename, arguments base.StringSet, options *internal_io.ProcessOptions) (base.Future[int], bool) {
	distributeStat := StartBuildStats()
	defer x.stats.DistributeAction.Append(&distributeStat)

	remoteOptions := *options
	x.client.GetMountedPaths(&remoteOptions)

	peer, future := x.client.AsyncDispatchTask(executable, arguments, &remoteOptions)

	if peer != nil {
		x.stats.AddRemoteAction(peer)
		base.LogTrace(LogActionDist, "action %q distributed to remote peer %v", a, peer)
	} else {
		x.stats.AddRemoteFailure()
	}

	return future, peer != nil
}

/***************************************
 * ActionDistStats
 ***************************************/

type ActionDistStats struct {
	StreamRead       BuildStats
	StreamWrite      BuildStats
	DistributeAction BuildStats

	RemoteActions  int32
	RemoteFailures int32

	WorkersSeen base.SharedMapT[string, int]

	StreamReadUncompressed int64
	StreamReadCompressed   int64

	StreamWriteUncompressed int64
	StreamWriteCompressed   int64
}

func (x *ActionDistStats) StatReadCompressed(n int) {
	atomic.AddInt64(&x.StreamReadCompressed, int64(n))
}
func (x *ActionDistStats) StatReadUncompressed(n int) {
	atomic.AddInt64(&x.StreamReadUncompressed, int64(n))
}
func (x *ActionDistStats) StatWriteCompressed(n int) {
	atomic.AddInt64(&x.StreamWriteCompressed, int64(n))
}
func (x *ActionDistStats) StatWriteUncompressed(n int) {
	atomic.AddInt64(&x.StreamWriteUncompressed, int64(n))
}
func (x *ActionDistStats) AddRemoteAction(peer *cluster.PeerDiscovered) {
	atomic.AddInt32(&x.RemoteActions, 1)
	x.WorkersSeen.FindOrAdd(peer.GetAddress(), 1)
}
func (x *ActionDistStats) AddRemoteFailure() {
	atomic.AddInt32(&x.RemoteFailures, 1)
}
func (x *ActionDistStats) Print() {
	base.LogForwardf("\nDistributed %d/%d actions on %d workers (%d errors)", x.RemoteActions, x.RemoteActions+x.RemoteFailures, x.WorkersSeen.Len(), x.RemoteFailures)
	base.LogForwardf("Spent %8.3f seconds dispatching %d tasks in network cluster", x.DistributeAction.InclusiveStart.Seconds(), x.DistributeAction.Count)

	base.LogForwardf("   READ <==  %8.3f seconds - %5d stream calls   - %8.3f MiB/Sec  - %8.3f MiB  ->> %9.3f MiB  (x%4.2f)",
		x.StreamRead.Duration.Exclusive.Seconds(), x.StreamRead.Count,
		base.MebibytesPerSec(x.StreamReadUncompressed, x.StreamRead.Duration.Exclusive),
		base.Mebibytes(x.StreamReadCompressed),
		base.Mebibytes(x.StreamReadUncompressed),
		float64(x.StreamReadUncompressed)/(float64(x.StreamReadCompressed)+0.00001))
	base.LogForwardf("  WRITE ==>  %8.3f seconds - %5d stream calls   - %8.3f MiB/Sec  - %8.3f MiB  ->> %9.3f MiB  (x%4.2f)",
		x.StreamWrite.Duration.Exclusive.Seconds(), x.StreamWrite.Count,
		base.MebibytesPerSec(x.StreamWriteUncompressed, x.StreamWrite.Duration.Exclusive),
		base.Mebibytes(x.StreamWriteCompressed),
		base.Mebibytes(x.StreamWriteUncompressed),
		float64(x.StreamWriteUncompressed)/(float64(x.StreamWriteCompressed)+0.00001))
}

/***************************************
 * DistModeType
 ***************************************/

type DistModeType byte

const (
	DIST_INHERIT DistModeType = iota
	DIST_NONE
	DIST_ENABLE
	DIST_FORCE
)

func DistModeTypes() []DistModeType {
	return []DistModeType{
		DIST_INHERIT,
		DIST_NONE,
		DIST_ENABLE,
		DIST_FORCE,
	}
}
func (x DistModeType) Description() string {
	switch x {
	case DIST_INHERIT:
		return "inherit default value from configuration"
	case DIST_NONE:
		return "disable task distribution"
	case DIST_ENABLE:
		return "enable task distribution when a worker is available"
	case DIST_FORCE:
		return "force task distribution, a worker need to be available"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x DistModeType) String() string {
	switch x {
	case DIST_INHERIT:
		return "INHERIT"
	case DIST_NONE:
		return "NONE"
	case DIST_ENABLE:
		return "ENABLE"
	case DIST_FORCE:
		return "FORCE"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x DistModeType) IsInheritable() bool {
	return x == DIST_INHERIT
}
func (x *DistModeType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case DIST_INHERIT.String():
		*x = DIST_INHERIT
	case DIST_NONE.String(), "FALSE":
		*x = DIST_NONE
	case DIST_ENABLE.String(), "TRUE":
		*x = DIST_ENABLE
	case DIST_FORCE.String():
		*x = DIST_FORCE
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *DistModeType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x DistModeType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *DistModeType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *DistModeType) AutoComplete(in base.AutoComplete) {
	for _, it := range DistModeTypes() {
		in.Add(it.String(), it.Description())
	}
}

func (x DistModeType) Enabled() bool {
	switch x {
	case DIST_ENABLE, DIST_FORCE:
		return true
	case DIST_INHERIT, DIST_NONE:
	default:
		base.UnexpectedValuePanic(x, x)
	}
	return false
}
func (x DistModeType) Forced() bool {
	switch x {
	case DIST_FORCE:
		return true
	case DIST_INHERIT, DIST_NONE, DIST_ENABLE:
	default:
		base.UnexpectedValuePanic(x, x)
	}
	return false
}
