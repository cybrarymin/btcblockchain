package node

import (
	"context"
	"errors"
	"io"

	"github.com/cybrarymin/btcblockchain/chain"
	"github.com/cybrarymin/btcblockchain/helpers"
	"github.com/cybrarymin/btcblockchain/node/gRPC"
	"github.com/cybrarymin/btcblockchain/protogen/pb"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc"
)

type StateSync struct {
	cfg        NodeCfg
	state      *chain.State
	peerReader *PeerReader
	grpcClient *gRPC.GrpcClient
}

func NewStateSync(cfg NodeCfg, state *chain.State, pr *PeerReader) *StateSync {
	return &StateSync{
		cfg:        cfg,
		state:      state,
		peerReader: pr,
	}
}

/*
The StateSync type implements the state sync algorithm to initialize the bootstrap node or synchronize an out-of-sync node.
*/
func (s *StateSync) SyncState(ctx context.Context) (*chain.State, error) {
	ctx, span := otel.Tracer("SyncState.Tracer").Start(ctx, "SyncState.Span")
	defer span.End()
	var sigGen *chain.SignedGenesis

	sigGen, err := chain.ReadGenesis(ctx, s.cfg.BlockStoreDir)
	if err != nil {
		if err != io.EOF {
			if s.cfg.Bootstrap {
				sigGen, err = s.CreateGenesis(ctx)
				if err != nil {
					return nil, err
				}
			} else {
				sigGen, err = s.SyncGenesis(ctx)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	valid, err := chain.VerifyGenesis(ctx, sigGen)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to verify genesis signature")
		return nil, err
	}
	if !valid {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid genesis signature")
		err = errors.New("invalid genesis signature")
		return nil, err
	}

	s.state, err = chain.NewState(*sigGen)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create a new state from signed genesis")
		return nil, err
	}
	err = chain.InitBlockStore(s.cfg.BlockStoreDir)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to initialize the blockstore")
		return nil, err
	}

	err = s.readBlocks(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read the blocks from blockstore to reach to local latest state")
		return nil, err
	}

	err = s.syncBlocks(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to sync blocks from other peers to reach to the latest state")
		return nil, err
	}

	return s.state, nil
}

func (s *StateSync) CreateGenesis(ctx context.Context) (*chain.SignedGenesis, error) {
	// Create and persist the authority account
	authAcc, err := chain.NewAccount(ctx)
	if err != nil {
		return nil, err
	}
	err = authAcc.Persist(ctx, s.cfg.KeyStoreDir, s.cfg.AuthPass)
	if err != nil {
		return nil, err
	}

	if s.cfg.Balance == 0 {
		err := errors.New("balance must be positive")
		return nil, err
	}

	// Create an owner account and persist the account
	ownerAcc, err := chain.NewAccount(ctx)
	if err != nil {
		return nil, err
	}
	err = ownerAcc.Persist(ctx, s.cfg.KeyStoreDir, s.cfg.OwnerPass)
	if err != nil {
		return nil, err
	}

	gen := chain.NewGenesis(s.cfg.ChainName, authAcc.Addr, ownerAcc.Addr, s.cfg.Balance)
	sigGen, err := authAcc.SignGenesis(ctx, gen)
	if err != nil {
		return nil, err
	}
	err = sigGen.Persist(ctx, s.cfg.BlockStoreDir)
	if err != nil {
		return nil, err
	}

	return sigGen, nil
}

func (s *StateSync) grpcGenesisSync(ctx context.Context) ([]byte, error) {
	ctx, span := otel.Tracer("grpcGenesisSync.Grpc.Tracer").Start(ctx, "grpcGenesisSync.Grpc.Span")
	defer span.End()

	gConn, err := grpc.NewClient(s.grpcClient.GrpcHost+":"+s.grpcClient.GrpcPort, s.grpcClient.Opts...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "couldn't establish new connection with grpc server")
		return nil, err
	}
	grpcBlkSvcClient := pb.NewBlockServiceClient(gConn)

	nReq := &pb.GenesisSynReq{}
	res, err := grpcBlkSvcClient.GenesisSync(ctx, nReq)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to sync the gensis block from bootstrap node")
		return nil, err
	}
	return res.Genesis, nil
}

/*
The genesis sync process is performed once for every new node when the new node joins the already initialized blockchain with the running bootstrap node.
*/
func (s *StateSync) SyncGenesis(ctx context.Context) (*chain.SignedGenesis, error) {
	jGen, err := s.grpcGenesisSync(ctx)
	if err != nil {
		return nil, err
	}
	sigGen, err := helpers.JsonUnMarshaller[*chain.SignedGenesis](ctx, jGen)
	if err != nil {
		return nil, err
	}
	valid, err := chain.VerifyGenesis(ctx, sigGen)
	if err != nil {
		return nil, err
	}
	if !valid {
		err = errors.New("invalid genesis signature")
		return nil, err
	}
	err = sigGen.Persist(ctx, s.cfg.BlockStoreDir)
	if err != nil {
		return nil, err
	}
	return sigGen, nil
}

func (s *StateSync) grpcBlockSync(ctx context.Context) error {
	ctx, span := otel.Tracer("grpcBlockSync.Grpc.Tracer").Start(ctx, "grpcBlockSync.Grpc.Span")
	defer span.End()

	return nil
}

/*
everytime nodes restarts the genesis will be read and initialized then all the blocks will be read and brought to the confirmed state to bring the node back to the state it left off.
*/
func (s *StateSync) readBlocks(ctx context.Context) error {
	iterator, close, err := chain.ReadBlocks(s.cfg.BlockStoreDir)
	if err != nil {
		return err
	}
	defer close()
	// read everyblock from blockstore and apply them to the cloned state
	for {
		sigBlock, err := iterator.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			continue
		}
		clone := s.state.Clone()
		err = clone.ApplyBlock(ctx, sigBlock)
		if err != nil {
			return err
		}
		// after successfull block application to clone state. apply the clonse state to the current state
		s.state.Apply(clone)
	}

	return nil
}

/*
The block sync process propagates the recent confirmed blocks through the blockchain network during the initialization of a new node or the synchronization of an out-of-sync node on the blockchain.
For every known peer the block sync process fetches the new confirmed blocks starting from the block number next to the last confirmed block number on the requesting node
*/
func (s *StateSync) syncBlocks(ctx context.Context) error {
	for _, peer := range s.peerReader {
		iterator, close, err := s.grpcBlockSync(peer)
		if err != nil {
			return err
		}
		defer close()
		for {
			sigBlock, err := iterator.Next()
			if err != nil {
				if err == io.EOF {
					break
				}
				continue
			}
			clone := s.state.Clone()
			err = clone.ApplyBlock(ctx, sigBlock)
			if err != nil {
				return err
			}
			// after successfull block application to clone state. apply the clonse state to the current state
			s.state.Apply(clone)
			err = sigBlock.Persist(ctx, s.cfg.BlockStoreDir)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
