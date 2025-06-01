package gRPC

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/cybrarymin/btcblockchain/protogen/pb"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type GrpcServer struct {
	NodeAddr    string
	Srv         *grpc.Server
	Logger      *zerolog.Logger
	KeystoreDir string
}

func NewGrpcServer(nodeAddr string, srvOpts []grpc.ServerOption, keyStoreDir string, BlockDir string, pd PeerDiscoverer, logger *zerolog.Logger) *GrpcServer {
	// create a new grpc server
	srv := grpc.NewServer(srvOpts...)

	// create new grpc accoutnSrv
	nAccSrv := NewAccountSrv(logger, keyStoreDir, nil) // TODO
	nTxSrv := NewTransactionService(logger, keyStoreDir, nil)
	nBlockSrv := NewBlockService(logger, BlockDir)
	nP2PSrv := NewP2PService(logger, pd)

	// register the grpc services
	pb.RegisterAccountServiceServer(srv, nAccSrv)
	pb.RegisterTransactionServiceServer(srv, nTxSrv)
	pb.RegisterBlockServiceServer(srv, nBlockSrv)
	pb.RegisterP2PServiceServer(srv, nP2PSrv)
	reflection.Register(srv)

	return &GrpcServer{
		NodeAddr:    nodeAddr,
		Srv:         srv,
		Logger:      logger,
		KeystoreDir: keyStoreDir,
	}
}

func (g *GrpcServer) Run(ctx context.Context, wg *sync.WaitGroup) error {
	defer wg.Done()
	listenAddr, err := net.Listen("tcp4", g.NodeAddr)
	if err != nil {
		return err
	}

	g.Logger.Info().Msgf("started grpc server on %s", g.NodeAddr)
	err = g.Srv.Serve(listenAddr)
	if err != nil {
		g.Logger.Error().Err(err).Msgf("failed to start grpc server on %s", g.NodeAddr)
		return err
	}
	return nil
}

func (g *GrpcServer) Stop(duration time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	stopped := make(chan error)

	go func() {
		select {
		case <-stopped:
			g.Logger.Info().Msg("grpc server shutdown gracefully")
		case <-ctx.Done():
			g.Logger.Warn().Msg("couldn't shutdown grpc server gracefully. force shutdown")
			g.Srv.Stop()
		}
	}()
	g.Srv.GracefulStop()
	close(stopped)
	return nil
}

type GrpcClient struct {
	NodeAddr string
	Opts     []grpc.DialOption
	Logger   *zerolog.Logger
}

func NewGrpcClient(nodeAddr string, dialOpts []grpc.DialOption, logger *zerolog.Logger) *GrpcClient {
	return &GrpcClient{
		NodeAddr: nodeAddr,
		Opts:     dialOpts,
		Logger:   logger,
	}
}
