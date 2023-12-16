package cluster

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

type PeerDiscovered struct {
	Revision int64
	LastSeen time.Time
	PeerInfo
}

/***************************************
 * Peer discovery
 ***************************************/

type PeerDiscovery struct {
	BrokeragePath utils.Directory
	Availables    []*PeerDiscovered
	Peers         map[string]*PeerDiscovered
	Version       PeerVersion

	barrier  sync.RWMutex
	revision int64
}

func NewPeerDiscovery(brokeragePath utils.Directory, version PeerVersion, maxPeers int) PeerDiscovery {
	base.LogVerbose(LogCluster, "new peer discovery with %q brokerage path", brokeragePath)
	return PeerDiscovery{
		BrokeragePath: brokeragePath.Folder(version),
		Availables:    make([]*PeerDiscovered, 0, maxPeers),
		Peers:         make(map[string]*PeerDiscovered, maxPeers),
		Version:       version,
	}
}

func (x *PeerDiscovery) MaxPeers() int { return cap(x.Availables) }
func (x *PeerDiscovery) RandomPeer(timeout time.Duration) (*PeerDiscovered, bool) {
	x.barrier.RLock()
	defer x.barrier.RUnlock()

	if len(x.Availables) == 0 {
		return nil, false
	}

	now := time.Now()
	for retry := 0; retry < 10; retry++ {
		peer := x.Availables[rand.Intn(len(x.Availables))]
		if peer.Revision != x.revision {
			base.LogDebug(LogCluster, "ignore worker %v from different revision: current=%v, worker=%v", peer.FQDN, x.revision, peer.Revision)
			continue
		}

		if now.Sub(peer.LastSeen) > timeout {
			base.LogDebug(LogCluster, "ignore timeouted worker %v", peer.FQDN)
			continue
		}

		base.LogVerbose(LogCluster, "selected random peer %v", peer.FQDN)
		return peer, true
	}

	return nil, false
}

func (x *PeerDiscovery) getPeerAnnounceFile(peer *PeerInfo) utils.Filename {
	announceFile := x.BrokeragePath.File(peer.FQDN)
	utils.UFS.Mkdir(announceFile.Dirname)
	return announceFile
}

func (x *PeerDiscovery) Announce(peer *PeerInfo) error {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	announceFile := x.getPeerAnnounceFile(peer)

	base.LogVerbose(LogCluster, "announce peer on brokerage %q", announceFile)

	return utils.UFS.CreateFile(announceFile, func(f *os.File) error {
		return peer.Save(f)
	})
}
func (x *PeerDiscovery) Touch(peer *PeerInfo) error {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	announceFile := x.getPeerAnnounceFile(peer)

	base.LogVerbose(LogCluster, "touch peer on brokerage %q", announceFile)

	return utils.UFS.Touch(announceFile)
}

func (x *PeerDiscovery) Disapear(peer *PeerInfo) error {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	base.LogVerbose(LogCluster, "remove %v peer from brokerage %q", peer.FQDN, x.BrokeragePath)
	return utils.UFS.Remove(x.BrokeragePath.File(peer.FQDN))
}

func (x *PeerDiscovery) Discover(retryCount int, timeout time.Duration) (int, error) {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	defer base.LogBenchmark(LogCluster, "peer discovery").Close()

	// update discovery revision (for GC-ing old peers)
	x.revision++

	// reset all previously available workers before updating discovery
	x.Availables = x.Availables[:0]

	if !x.BrokeragePath.Exists() {
		return 0, fmt.Errorf("invalid brokerage path: %q", x.BrokeragePath)
	}

	// refresh brokerage path file listing
	x.BrokeragePath.Invalidate()
	files, err := x.BrokeragePath.Files()
	if err != nil {
		return 0, err
	}

	// shuffle input files: we will only save MaxPeers records at most
	rand.Shuffle(len(files), func(i, j int) {
		files[i], files[j] = files[j], files[i]
	})

	// tag alive peers, un-tagged entries will then be deleted
	for _, file := range files {
		if peer, ok := x.Peers[file.Basename]; ok {
			peer.Revision = x.revision
		}
	}

	// clamp the number of available peers
	registerPeer := func(peer *PeerDiscovered) bool {
		if peer.Version == x.Version {
			x.Availables = append(x.Availables, peer)
			return cap(x.Availables) == len(x.Availables)
		}
		return false
	}

	// also clamp the total number of parse peer infos to avoid discovery complexity
	// scaling with cluster density increases
	maxPeersToTest := retryCount * x.MaxPeers()

	now := time.Now()
	for _, file := range files {
		var peer *PeerDiscovered
		peer, ok := x.Peers[file.Basename]
		if !ok {
			peer = &PeerDiscovered{Revision: x.revision}
			x.Peers[file.Basename] = peer
		}

		if st, err := file.Info(); err == nil {

			if since := now.Sub(st.ModTime()); since > timeout {
				base.LogWarning(LogCluster, "ignore peer timeout %q: %v (%s)", file, err, since)
				continue
			}

			if peer.LastSeen != st.ModTime() {
				peer.LastSeen = st.ModTime()
			} else {
				if registerPeer(peer) {
					break
				} else {
					continue
				}
			}

		} else {
			base.LogWarning(LogCluster, "ignore invalid peer stat %q: %v", file, err)
			continue
		}

		if err := utils.UFS.Open(file, func(r io.Reader) error {
			return peer.Load(r)
		}); err != nil {
			base.LogWarning(LogCluster, "ignore invalid peer info %q: %v", file, err)
			continue
		}

		if registerPeer(peer) {
			break
		}

		if maxPeersToTest--; maxPeersToTest <= 0 {
			break
		}
	}

	// delete all peer infos from previous revisions (in case a worker disappeared)
	for basename, peer := range x.Peers {
		if peer.Revision != x.revision {
			delete(x.Peers, basename)
		}
	}

	base.LogVerbose(LogCluster, "discovered %d peers", len(x.Peers))
	return len(x.Peers), nil
}
