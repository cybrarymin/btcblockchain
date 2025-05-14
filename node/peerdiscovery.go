package node

import (
	"context"
	"sync"

	"github.com/rs/zerolog"
)

type PeerReader interface {
	Peers() []string
	SelfPeers() []string
}

type PeerDiscoveryCfg struct {
	NodeAddr  string
	Bootstrap bool // is the node the bootstrap node of the p2p network
	SeedAddr  string
}

type PeerDiscovery struct {
	logger *zerolog.Logger
	cfg    PeerDiscoveryCfg
	ctx    context.Context
	wg     *sync.WaitGroup
	mtx    sync.RWMutex
	peers  map[string]struct{}
}

func NewPeerDiscovery(ctx context.Context, logger *zerolog.Logger, wg *sync.WaitGroup, cfg PeerDiscoveryCfg) *PeerDiscovery {
	peerDisc := &PeerDiscovery{
		logger: logger,
		cfg:    cfg,
		ctx:    ctx,
		wg:     wg,
		peers:  make(map[string]struct{}),
	}
	if peerDisc.cfg.Bootstrap {
		peerDisc.AddPeers(peerDisc.cfg.SeedAddr)
	}
	return peerDisc
}

/*
The add peers operation is concurrency safe and is executed every time the peer discovery algorithm fetches a list of peers from another node
Lock the list of known peers for writing
Iterate over the list of fetched peers from another node
Add only new, not yet known peers, to the list of known peers of the node
*/
func (d *PeerDiscovery) AddPeers(peers ...string) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	for _, peer := range peers {
		// if peer is not this node ip address
		if peer != d.cfg.NodeAddr {
			// if the peer doesn't exists in the list of peers this node has then add it to the list
			_, exists := d.peers[peer]
			if !exists {
				d.logger.Info().Msg("")
			}
			d.peers[peer] = struct{}{}
		}
	}
}

/*
 */
func (d *PeerDiscovery) Peers() []string {

}
