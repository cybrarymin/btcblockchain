package gRPC

import (
	"context"
	"net"
	"time"

	"github.com/cybrarymin/btcblockchain/protogen/pb"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

type GrpcServer struct {
	GrpcHost    string
	GrpcPort    string
	Srv         *grpc.Server
	Logger      *zerolog.Logger
	Opts        []grpc.ServerOption
	KeystoreDir string
}

func NewGrpcServer(grpcHost string, grpcPort string, srvOpts []grpc.ServerOption, keyStoreDir string, logger *zerolog.Logger) *GrpcServer {
	// create a new grpc server
	srv := grpc.NewServer(srvOpts...)

	// create new grpc accoutnSrv
	nAccSrv := NewAccountSrv(logger, keyStoreDir)

	// register the grpc services
	pb.RegisterAccountServiceServer(srv, nAccSrv)
	return &GrpcServer{
		GrpcHost:    grpcHost,
		GrpcPort:    grpcPort,
		Srv:         srv,
		Logger:      logger,
		KeystoreDir: keyStoreDir,
	}
}

func (g *GrpcServer) Run() error {
	listenAddr, err := net.Listen("tcp4", g.GrpcHost+":"+g.GrpcPort)
	if err != nil {
		return err
	}

	g.Logger.Info().Msgf("started grpc server on %s:%s", g.GrpcHost, g.GrpcPort)
	err = g.Srv.Serve(listenAddr)
	if err != nil {
		return err
	}
	return nil
}

func (g *GrpcServer) Stop(ctx context.Context, duration time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, duration)
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
}
