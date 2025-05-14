package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/cybrarymin/btcblockchain/node/gRPC"
	obs "github.com/cybrarymin/btcblockchain/obeservability"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

func cmdMain() {
	pCtx := context.Background()

	// setting up new zerolog logger
	logger := zerolog.New(os.Stdout)
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	if CmdLogLevelFlag == zerolog.LevelTraceValue {
		logger = logger.With().Timestamp().Stack().Caller().Logger().Level(zerolog.TraceLevel)
	} else {
		loglvl, err := zerolog.ParseLevel(CmdLogLevelFlag)
		if err != nil {
			logger.Error().Err(err).
				Msg("couldn't indentify the loglevel")
		}
		logger = logger.With().Timestamp().Logger().Level(loglvl)
	}

	// setup opentelemetry sdk to use global trace provider for span creation and propagation
	otelShut, err := obs.SetupOTelSDK(pCtx, CmdJaegerHostFlag, CmdJaegerPortFlag, CmdJaegerConnectionTimeout, CmdSpanExportInterval)
	if err != nil {
		logger.Error().Err(err).
			Msg("couldn't setup the otel sdk")
	}
	defer otelShut(pCtx)

	// Create a new grpc server and it's required interceptors
	otelHandler := otelgrpc.NewServerHandler()
	grpcSrvOptions := []grpc.ServerOption{
		grpc.StatsHandler(otelHandler),
	}

	gSrv := gRPC.NewGrpcServer(CmdGrpcHost, CmdGrpcPort, grpcSrvOptions, CmdKeyStoreDir, CmdBlockStore, &logger)
	go gSrv.Run()
	gSrvStopFunc := func() error {
		gSrv.Stop(pCtx, CmdGrpcGracefulShutdownTimeout)
		return nil
	}

	// graceful shutdown implementation
	shutdownErr := make(chan error)
	go graceFulShutdown(&logger, shutdownErr, gSrvStopFunc)
	for {
		err = <-shutdownErr
		if err == nil {
			break
		}
		logger.Error().Err(err).
			Msg("failed to shutdown component gracefully")
	}
}

func graceFulShutdown(logger *zerolog.Logger, shutdownErr chan error, shutdownFuncs ...func() error) {
	sChan := make(chan os.Signal, 1)

	// waiting for so signals
	signal.Notify(sChan, syscall.SIGTERM, syscall.SIGQUIT)
	s := <-sChan

	logger.Info().
		Msgf("catched os signal %s", s)

	for _, stopFunc := range shutdownFuncs {
		err := stopFunc()
		if err != nil {
			shutdownErr <- err
		}
	}
	shutdownErr <- nil
	logger.Info().Msg("stopped the server....")
}

func cmdClientMain() {
	// setting up new zerolog logger
	logger := zerolog.New(os.Stdout)
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	if CmdLogLevelFlag == zerolog.LevelTraceValue {
		logger = logger.With().Timestamp().Stack().Caller().Logger().Level(zerolog.TraceLevel)
	} else {
		loglvl, err := zerolog.ParseLevel(CmdLogLevelFlag)
		if err != nil {
			logger.Error().Err(err).
				Msg("couldn't indentify the loglevel")
		}
		logger = logger.With().Timestamp().Logger().Level(loglvl)
	}

	otelHandler := otelgrpc.NewClientHandler()
	Opts := []grpc.DialOption{
		grpc.WithStatsHandler(otelHandler),
	}
	gClient := gRPC.NewGrpcClient(CmdBootstrapGrpcHostFlag, CmdBootstrapGrpcPortFlag, Opts, &logger)

}
