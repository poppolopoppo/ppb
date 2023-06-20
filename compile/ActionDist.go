package compile

import (
	"context"
	"io"
	"strings"
	"sync/atomic"

	"github.com/poppolopoppo/ppb/cluster"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var LogActionDist = NewLogCategory("ActionDist")

/***************************************
 * ActionDist
 ***************************************/

type ActionDist interface {
	GetBrokeragePath() Directory
	GetDistStats() *ActionDistStats

	CanDistribute(force bool) bool

	DistributeAction(a BuildAlias, executable Filename, arguments StringSet, options *ProcessOptions) (*cluster.PeerDiscovered, error)
	AsyncDistributeAction(a BuildAlias, executable Filename, arguments StringSet, options *ProcessOptions) (Future[int], bool)
}

type actionDist struct {
	cancel  context.CancelFunc
	cluster cluster.Cluster
	client  *cluster.Client
	stats   ActionDistStats
}

var GetActionDist = Memoize(func() ActionDist {
	var result *actionDist
	var err error

	result = new(actionDist)
	result.cluster = cluster.NewCluster(
		cluster.ClusterOptionStreamRead(func(r io.Reader, b []byte) (n int, err error) {
			stat := StartBuildStats()
			defer result.stats.StreamRead.Append(&stat)

			n, err = r.Read(b)
			result.stats.StatReadCompressed(uint64(n))
			return
		}),
		cluster.ClusterOptionUncompressRead(func(r io.Reader, b []byte) (n int, err error) {
			n, err = r.Read(b)
			result.stats.StatReadUncompressed(uint64(n))
			return
		}),
		cluster.ClusterOptionStreamWrite(func(w io.Writer, b []byte) (n int, err error) {
			stat := StartBuildStats()
			defer result.stats.StreamWrite.Append(&stat)

			n, err = w.Write(b)
			result.stats.StatWriteCompressed(uint64(n))
			return
		}),
		cluster.ClusterOptionCompressWrite(func(w io.Writer, b []byte) (n int, err error) {
			n, err = w.Write(b)
			result.stats.StatWriteUncompressed(uint64(n))
			return
		}))
	result.client, result.cancel, err = result.cluster.StartClient()
	LogPanicIfFailed(LogActionDist, err)

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
func (x *actionDist) DistributeAction(a BuildAlias, executable Filename, arguments StringSet, options *ProcessOptions) (*cluster.PeerDiscovered, error) {
	distributeStat := StartBuildStats()
	defer x.stats.DistributeAction.Append(&distributeStat)

	remoteOptions := *options
	x.client.GetMountedPaths(&remoteOptions)

	peer, ok, err := x.client.DispatchTask(executable, arguments, &remoteOptions)

	if peer != nil && ok {
		x.stats.AddRemoteAction(peer)
		LogVeryVerbose(LogActionDist, "action %q distributed to remote peer %v", a, peer)
	} else {
		x.stats.AddRemoteFailure()
		LogWarning(LogActionDist, "action %q could not be distributed: %v", a, err)
		peer = nil
	}

	return peer, err
}
func (x *actionDist) AsyncDistributeAction(a BuildAlias, executable Filename, arguments StringSet, options *ProcessOptions) (Future[int], bool) {
	distributeStat := StartBuildStats()
	defer x.stats.DistributeAction.Append(&distributeStat)

	remoteOptions := *options
	x.client.GetMountedPaths(&remoteOptions)

	peer, future := x.client.AsyncDispatchTask(executable, arguments, &remoteOptions)

	if peer != nil {
		x.stats.AddRemoteAction(peer)
		LogTrace(LogActionDist, "action %q distributed to remote peer %v", a, peer)
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

	WorkersSeen SharedMapT[string, int]

	StreamReadUncompressed uint64
	StreamReadCompressed   uint64

	StreamWriteUncompressed uint64
	StreamWriteCompressed   uint64
}

func (x *ActionDistStats) StatReadCompressed(n uint64) {
	atomic.AddUint64(&x.StreamReadCompressed, n)
}
func (x *ActionDistStats) StatReadUncompressed(n uint64) {
	atomic.AddUint64(&x.StreamReadUncompressed, n)
}
func (x *ActionDistStats) StatWriteCompressed(n uint64) {
	atomic.AddUint64(&x.StreamWriteCompressed, n)
}
func (x *ActionDistStats) StatWriteUncompressed(n uint64) {
	atomic.AddUint64(&x.StreamWriteUncompressed, n)
}
func (x *ActionDistStats) AddRemoteAction(peer *cluster.PeerDiscovered) {
	atomic.AddInt32(&x.RemoteActions, 1)
	x.WorkersSeen.FindOrAdd(peer.GetAddress(), 1)
}
func (x *ActionDistStats) AddRemoteFailure() {
	atomic.AddInt32(&x.RemoteFailures, 1)
}
func (x *ActionDistStats) Print() {
	LogForwardf("\nDistributed %d/%d actions on %d workers (%d errors)", x.RemoteActions, x.RemoteActions+x.RemoteFailures, x.WorkersSeen.Len(), x.RemoteFailures)
	LogForwardf("Spent %8.3f seconds dispatching %d tasks in network cluster", x.DistributeAction.InclusiveStart.Seconds(), x.DistributeAction.Count)

	LogForwardf("   READ <==  %8.3f seconds - %5d stream calls   - %8.3f MiB/Sec  - %8.3f MiB  ->> %9.3f MiB  (x%4.2f)",
		x.StreamRead.Duration.Exclusive.Seconds(), x.StreamRead.Count,
		MebibytesPerSec(uint64(x.StreamReadUncompressed), x.StreamRead.Duration.Exclusive),
		Mebibytes(x.StreamReadCompressed),
		Mebibytes(x.StreamReadUncompressed),
		float64(x.StreamReadUncompressed)/(float64(x.StreamReadCompressed)+0.00001))
	LogForwardf("  WRITE ==>  %8.3f seconds - %5d stream calls   - %8.3f MiB/Sec  - %8.3f MiB  ->> %9.3f MiB  (x%4.2f)",
		x.StreamWrite.Duration.Exclusive.Seconds(), x.StreamWrite.Count,
		MebibytesPerSec(uint64(x.StreamWriteUncompressed), x.StreamWrite.Duration.Exclusive),
		Mebibytes(x.StreamWriteCompressed),
		Mebibytes(x.StreamWriteUncompressed),
		float64(x.StreamWriteUncompressed)/(float64(x.StreamWriteCompressed)+0.00001))
}

/***************************************
 * DistModeType
 ***************************************/

type DistModeType int32

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
		UnexpectedValue(x)
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
		err = MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *DistModeType) Serialize(ar Archive) {
	ar.Int32((*int32)(x))
}
func (x DistModeType) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *DistModeType) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}
func (x *DistModeType) AutoComplete(in AutoComplete) {
	for _, it := range DistModeTypes() {
		in.Add(it.String())
	}
}

func (x DistModeType) Enabled() bool {
	switch x {
	case DIST_ENABLE, DIST_FORCE:
		return true
	case DIST_INHERIT, DIST_NONE:
	default:
		UnexpectedValuePanic(x, x)
	}
	return false
}
func (x DistModeType) Forced() bool {
	switch x {
	case DIST_FORCE:
		return true
	case DIST_INHERIT, DIST_NONE, DIST_ENABLE:
	default:
		UnexpectedValuePanic(x, x)
	}
	return false
}
