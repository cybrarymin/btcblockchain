package node

import (
	"context"
	"sync"
	"time"

	"github.com/cybrarymin/btcblockchain/protogen/pb"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type PeerReader interface {
	Peers() []string // list of peer address
	SelfPeers() []string
}

type PeerDiscoveryCfg struct {
	NodeAddr  string // the address of the node itself
	Bootstrap bool   // the bootstrap node of the p2p network or not. bootstrap node is the node that everynode joining to the cluster will send it a request to get the list of peers in the blockchain network
	SeedAddr  string // seed address is the boostrap node address
}

func NewPeerDiscoveryCfg(NodeAddr string, Boostrap bool, BoostrapNodeAddr string) *PeerDiscoveryCfg {
	return &PeerDiscoveryCfg{
		NodeAddr:  NodeAddr,
		Bootstrap: Boostrap,
		SeedAddr:  BoostrapNodeAddr,
	}
}

type PeerDiscovery struct {
	logger *zerolog.Logger
	Cfg    *PeerDiscoveryCfg
	ctx    context.Context
	wg     *sync.WaitGroup
	mtx    sync.RWMutex
	peers  map[string]struct{}
}

func NewPeerDiscovery(ctx context.Context, logger *zerolog.Logger, wg *sync.WaitGroup, cfg *PeerDiscoveryCfg) *PeerDiscovery {
	peerDisc := &PeerDiscovery{
		logger: logger,
		Cfg:    cfg,
		ctx:    ctx,
		wg:     wg,
		peers:  make(map[string]struct{}),
	}
	if peerDisc.Cfg.Bootstrap {
		peerDisc.AddPeers(peerDisc.Cfg.SeedAddr) // if node is the boostrap node it's going to add itself to the list of available peers
	}
	return peerDisc
}

// return if the peer is bootstrap node or not.
func (p *PeerDiscovery) Bootstrap() bool {
	return p.Cfg.Bootstrap
}

/*
The add peers operation is concurrency safe and is executed every time the peer discovery algorithm fetches a list of peers from another node
Lock the list of known peers for writing
Iterate over the list of fetched peers from another node
Add only new, not yet known peers, to the list of known peers of the node
*/
func (d *PeerDiscovery) AddPeers(peerAddrs ...string) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	for _, peer := range peerAddrs {
		// if peer is not this node ip address
		if peer != d.Cfg.NodeAddr {
			// if the peer doesn't exists in the list of peers this node has then add it to the list
			_, exists := d.peers[peer]
			if !exists {
				d.logger.Info().Msgf("adding new peer with %v", peer)
				d.peers[peer] = struct{}{}
			}
		}
	}
}

/*
returns list of peer address available on this node
*/
func (d *PeerDiscovery) Peers() []string {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	peerList := make([]string, 0, len(d.peers))
	for peerAddr, _ := range d.peers {
		peerList = append(peerList, peerAddr)
	}
	return peerList
}

/*
This one will return list of all existing peers plus the node address itself. So it's gonna be read peers with self-reference
*/
func (d *PeerDiscovery) SelfPeers() []string {
	return append(d.Peers(), d.Cfg.NodeAddr)
}

/*
Peer discovery algorithm to be used to discover peers in p2p network
*/
func (d *PeerDiscovery) DiscoverPeers(period time.Duration) {
	defer d.wg.Done()

	ticker := time.NewTicker(period)
	defer ticker.Stop()

	select {
	case <-d.ctx.Done():
		d.logger.Info().Msg("shutting down the peer discovery system")
		return
	case <-ticker.C:
		d.mtx.RLock()
		for peer, _ := range d.peers {
			if peer != d.Cfg.NodeAddr {
				npeers, err := d.grpcPeerDiscover(peer)
				if err != nil {
					d.logger.Warn().Msg("couldn't get a response from the peer. deleting peer from the list")
					continue
				}
				d.AddPeers(npeers...)
			}
		}

		d.mtx.RUnlock()
	}

}

/*
send a grpc request to the specified peer and provide your address and fetch list of the known peers by that peer
*/
func (d *PeerDiscovery) grpcPeerDiscover(peer string) ([]string, error) {
	conn, err := grpc.NewClient(peer, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	grpClient := pb.NewP2PServiceClient(conn)
	resp, err := grpClient.DiscoverPeers(d.ctx, &pb.PeerDiscoveryReq{
		Address: d.Cfg.NodeAddr,
	})
	if err != nil {
		return nil, err
	}
	return resp.Peers, nil
}
