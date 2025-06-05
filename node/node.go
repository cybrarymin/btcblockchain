package node

import (
	"context"
	"net"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/cybrarymin/btcblockchain/chain"
	"github.com/cybrarymin/btcblockchain/node/gRPC"
	"github.com/cybrarymin/btcblockchain/protogen/pb"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type NodeCfg struct {
	// Addressing
	NodeAddr  string
	Bootstrap bool
	SeedAddr  string
	// Stores
	KeyStoreDir   string
	BlockStoreDir string
	// Genesis
	ChainName string
	AuthPass  string // password for creating and initializing the authority account
	OwnerPass string
	Balance   uint64 // initial balance for the authority account
	// Processes
	Period time.Duration // time internval which nodes on p2p network will send discovery requests to each other
}

func NewNodeCfg(nodeAddr string, bootstrap bool, seedAddr string, KeyStoreDir string, blockStoreDir string, chainName string, authPass string, ownerPass string, balance uint64, discoveryInternal time.Duration) *NodeCfg {
	if bootstrap {
		seedAddr = nodeAddr
	}
	return &NodeCfg{
		NodeAddr:      nodeAddr,
		Bootstrap:     bootstrap,
		SeedAddr:      seedAddr,
		KeyStoreDir:   KeyStoreDir,
		BlockStoreDir: blockStoreDir,
		ChainName:     chainName,
		AuthPass:      authPass,
		OwnerPass:     ownerPass, // owner account is the account we use to only hold balances of the treasury.
		Balance:       balance,
		Period:        discoveryInternal,
	}
}

type Node struct {
	logger *zerolog.Logger
	cfg    *NodeCfg
	// Graceful shutdown
	ctx       context.Context
	ctxCancel func()
	wg        *sync.WaitGroup
	chErr     chan error
	// Node components
	// evStream  *EventStream
	state     *chain.State
	stateSync *StateSync
	grpcSrv   *grpc.Server
	peerDisc  *PeerDiscovery
	txRelay   *MsgRelay[*chain.SignedTransaction, GRPCMsgRelay[*chain.SignedTransaction]]
	blockProp *BlockProposer
	blkRelay  *MsgRelay[*chain.SignedBlock, GRPCMsgRelay[*chain.SignedBlock]]
}

func NewNode(ctx context.Context, logger *zerolog.Logger, nodecfg *NodeCfg) *Node {
	ctx, cancelFunc := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	wg := new(sync.WaitGroup)

	// initialize the peer discovery
	peerDiscCfg := NewPeerDiscoveryCfg(nodecfg.NodeAddr, nodecfg.Bootstrap, nodecfg.SeedAddr)
	peerDisc := NewPeerDiscovery(ctx, logger, wg, peerDiscCfg)
	// initialize transaction relay
	txRelay := NewMsgRelay(ctx, logger, wg, 100, GRPCTxRelay, false, peerDisc)
	// // initialize block relay
	blkRelay := NewMsgRelay(ctx, logger, wg, 10, GRPCBlockRelay, true, peerDisc)

	// initialize blockproposer
	blockProp := NewBlockProposer(ctx, logger, wg, blkRelay)

	// initializing the state synchroniztion
	stateSync := NewStateSync(ctx, logger, nodecfg, peerDisc)

	// initialize the grpc server
	nServer := grpc.NewServer()

	return &Node{
		logger:    logger,
		cfg:       nodecfg,
		ctx:       ctx,
		ctxCancel: cancelFunc,
		wg:        wg,
		chErr:     make(chan error, 1),
		stateSync: stateSync,
		grpcSrv:   nServer,
		peerDisc:  peerDisc,
		txRelay:   txRelay,
		blkRelay:  blkRelay,
		blockProp: blockProp,
	}
}

func (n *Node) Start() error {
	state, err := n.stateSync.SyncState(n.ctx)
	if err != nil {
		return err
	}
	n.state = state
	n.wg.Add(1)
	go n.grpcRun()

	n.wg.Add(1)
	go n.peerDisc.DiscoverPeers(n.cfg.Period)

	n.wg.Add(1)
	go n.txRelay.RelayMsgs(n.cfg.Period)

	n.wg.Add(1)
	go n.blkRelay.RelayMsgs(n.cfg.Period)

	if n.cfg.Bootstrap {
		path := filepath.Join(n.cfg.KeyStoreDir, string(n.state.Authroity()))
		auth, err := chain.ReadAccount(n.ctx, path, n.cfg.AuthPass)
		if err != nil {
			return err
		}
		n.blockProp.SetAuthority(*auth)
		n.blockProp.SetState(n.state)
		n.wg.Add(1)
		go n.blockProp.ProposeBlock(n.cfg.Period * 2)
	}

	<-n.ctx.Done()
	n.grpcStop(10 * time.Second)

	n.wg.Wait()
	return err

}

func (n *Node) grpcRun() error {
	defer n.wg.Done()

	// create new grpc accoutnSrv
	nAccSrv := gRPC.NewAccountSrv(n.logger, n.cfg.KeyStoreDir, n.state) // TODO
	nTxSrv := gRPC.NewTransactionService(n.logger, n.cfg.KeyStoreDir, n.cfg.BlockStoreDir, n.state.Pending, n.txRelay)
	nBlockSrv := gRPC.NewBlockService(n.logger, n.cfg.BlockStoreDir, n.state, n.blkRelay)
	nP2PSrv := gRPC.NewP2PService(n.logger, n.peerDisc)

	// register the grpc services
	pb.RegisterAccountServiceServer(n.grpcSrv, nAccSrv)
	pb.RegisterTransactionServiceServer(n.grpcSrv, nTxSrv)
	pb.RegisterBlockServiceServer(n.grpcSrv, nBlockSrv)
	pb.RegisterP2PServiceServer(n.grpcSrv, nP2PSrv)
	reflection.Register(n.grpcSrv)

	listenAddr, err := net.Listen("tcp4", n.cfg.NodeAddr)
	if err != nil {
		return err
	}

	n.logger.Info().Msgf("started grpc server on %s", n.cfg.NodeAddr)
	err = n.grpcSrv.Serve(listenAddr)
	if err != nil {
		n.logger.Error().Err(err).Msgf("failed to start grpc server on %s", n.cfg.NodeAddr)
		return err
	}
	return nil
}

func (n *Node) grpcStop(duration time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	stopped := make(chan error)

	go func() {
		select {
		case <-stopped:
			n.logger.Info().Msg("grpc server shutdown gracefully")
		case <-ctx.Done():
			n.logger.Warn().Msg("couldn't shutdown grpc server gracefully. force shutdown")
			n.grpcSrv.Stop()
		}
	}()
	n.grpcSrv.GracefulStop()
	close(stopped)
	return nil
}
