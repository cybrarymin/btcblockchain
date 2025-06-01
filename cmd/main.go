package cmd

import (
	"context"
	"os"

	"github.com/cybrarymin/btcblockchain/node"
	obs "github.com/cybrarymin/btcblockchain/obeservability"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
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
	// otelHandler := otelgrpc.NewServerHandler()
	nodeCfg := node.NewNodeCfg(CmdGrpcHost+":"+CmdGrpcPort, CmdBootstrap, CmdBootstrapAddr, CmdKeyStoreDir, CmdBlockStore, CmdChainName, CmdAuthAccountPass, CmdAuthAccountPass, CmdChainBalance, CmdDiscoveryInterval)
	node := node.NewNode(pCtx, &logger, nodeCfg)
	err = node.Start()
	if err != nil {
		logger.Error().Err(err).Msg("failed to start the server")
	}
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

	//otelHandler := otelgrpc.NewClientHandler()

}
