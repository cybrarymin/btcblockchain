package gRPC

import (
	"context"
	"time"

	"github.com/cybrarymin/btcblockchain/protogen/pb"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
)

type PeerDiscoverer interface {
	DiscoverPeers(period time.Duration)
	Peers() []string
	AddPeers(peerAddrs ...string)
	Bootstrap() bool
}

type P2PService struct {
	logger        *zerolog.Logger
	peerDiscovery PeerDiscoverer
	pb.P2PServiceServer
}

func NewP2PService(logger *zerolog.Logger, pd PeerDiscoverer) *P2PService {
	return &P2PService{
		logger:        logger,
		peerDiscovery: pd,
	}
}

func (s *P2PService) DiscoverPeers(ctx context.Context, req *pb.PeerDiscoveryReq) (*pb.PeerDiscoveryResp, error) {
	ctx, span := otel.Tracer("DiscoverPeers.Grpc.Tracer").Start(context.Background(), "DiscoverPeers.Grpc.Span")
	defer span.End()
	if s.peerDiscovery.Bootstrap() {
		s.peerDiscovery.AddPeers(req.Address)
	}
	// add the requesting node to the list of peers if it doesn't already exists
	s.peerDiscovery.AddPeers(req.Address)

	// return back list of all known peers of ourselves to the client
	resp := &pb.PeerDiscoveryResp{
		Peers: s.peerDiscovery.Peers(),
	}
	return resp, nil
}
