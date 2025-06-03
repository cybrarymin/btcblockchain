package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cybrarymin/btcblockchain/protogen/pb"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
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
	if !peerDisc.Cfg.Bootstrap {
		peerDisc.AddPeers(peerDisc.Cfg.SeedAddr)
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
			if exists {
				d.logger.Debug().
					Str("peer", peer).
					Msg("peer already exsits")
				continue
			}
			d.logger.Info().Msgf("adding new peer with %v", peer)
			d.peers[peer] = struct{}{}
		}
	}
}

func (d *PeerDiscovery) DeletePeers(peerAddrs ...string) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	for _, peer := range peerAddrs {
		// if peer is not this node ip address
		if peer != d.Cfg.NodeAddr {
			// if the peer doesn't exists in the list of peers this node has then add it to the list
			_, exists := d.peers[peer]
			if !exists {
				d.logger.Debug().
					Str("peer", peer).
					Msg("peer doesn't exsits")
				continue
			}
			d.logger.Info().Msgf("removing the peer %v from the known peers", peer)
			delete(d.peers, peer)
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
	ctx, span := otel.Tracer("DiscoverPeers.Tracer").Start(d.ctx, "DisoverPeers.Span")

	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			d.logger.Info().Msg("shutting down the peer discovery system")
			return
		case <-ticker.C:
			peers := d.Peers()
			d.logger.Debug().Strs("peers_list", peers).Msg("node current peers list")
			for _, peer := range peers {
				if peer != d.Cfg.NodeAddr {
					d.logger.Info().
						Str("peer", peer).
						Msgf("sending grpc request to peer %v to get full list of its peers", peer)

					npeers, err := d.grpcPeerDiscover(ctx, peer)
					if err != nil {
						span.RecordError(err)
						span.SetStatus(codes.Ok, fmt.Sprintf("failed to get connected to the peer: %v", peer))
						d.logger.Warn().
							Str("peer", peer).
							Msgf("couldn't get a response from the peer %v. removing the peer from the list", peer)
						d.DeletePeers(peer)
						continue
					}
					d.AddPeers(npeers...)
				}
			}
		}
		span.End()
	}
}

/*
send a grpc request to the specified peer and provide your address and fetch list of the known peers by that peer
*/
func (d *PeerDiscovery) grpcPeerDiscover(ctx context.Context, peer string) ([]string, error) {
	ctx, span := otel.Tracer("grpcPeerDiscovery.Tracer").Start(ctx, "grpcPeerDiscovery.Span")
	defer span.End()
	conn, err := grpc.NewClient(peer, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	grpClient := pb.NewP2PServiceClient(conn)
	resp, err := grpClient.DiscoverPeers(ctx, &pb.PeerDiscoveryReq{
		Address: d.Cfg.NodeAddr,
	})
	if err != nil {
		return nil, err
	}
	return resp.Peers, nil
}
