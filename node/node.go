package node

import (
	"context"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cybrarymin/btcblockchain/chain"
	"github.com/cybrarymin/btcblockchain/node/gRPC"
	"github.com/rs/zerolog"
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
		OwnerPass:     ownerPass,
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
	grpcSrv   *gRPC.GrpcServer
	peerDisc  *PeerDiscovery
	// txRelay   *MsgRelay[chain.SigTx, GRPCMsgRelay[chain.SigTx]]
	// blockProp *BlockProposer
	// blkRelay  *MsgRelay[chain.SigBlock, GRPCMsgRelay[chain.SigBlock]]
}

func NewNode(ctx context.Context, logger *zerolog.Logger, nodecfg *NodeCfg) *Node {
	ctx, cancelFunc := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	wg := new(sync.WaitGroup)

	// initialize the peer discovery
	peerDiscCfg := NewPeerDiscoveryCfg(nodecfg.NodeAddr, nodecfg.Bootstrap, nodecfg.SeedAddr)
	peerDisc := NewPeerDiscovery(ctx, logger, wg, peerDiscCfg)
	// initializing the state synchroniztion
	stateSync := NewStateSync(ctx, logger, nodecfg, peerDisc)

	// initialize the grpc server
	nServer := gRPC.NewGrpcServer(nodecfg.NodeAddr, nil, nodecfg.KeyStoreDir, nodecfg.BlockStoreDir, peerDisc, logger)

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
	}
}

func (n *Node) Start() error {
	state, err := n.stateSync.SyncState(n.ctx)
	if err != nil {
		return err
	}
	n.state = state
	n.wg.Add(1)
	go n.grpcSrv.Run(n.ctx, n.wg)

	n.wg.Add(1)
	go n.peerDisc.DiscoverPeers(n.cfg.Period)
	// n.wg.Add(1)
	// go n.txRelay.RelayMsgs(n.cfg.Period)
	// if n.cfg.Bootstrap {
	// path := filepath.Join(n.cfg.KeyStoreDir, string(n.state.Authority()))
	// auth, err := chain.ReadAccount(path, []byte(n.cfg.AuthPass))
	// if err != nil {
	// 	return err
	// }
	// n.blockProp.SetAuthority(auth)
	// n.blockProp.SetState(n.state)
	// n.wg.Add(1)
	// go n.blockProp.ProposeBlocks(n.cfg.Period * 2)
	// }
	<-n.ctx.Done()
	n.grpcSrv.Stop(10 * time.Second)

	n.wg.Wait()
	return err

}
