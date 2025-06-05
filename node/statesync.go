package node

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/cybrarymin/btcblockchain/chain"
	"github.com/cybrarymin/btcblockchain/helpers"
	"github.com/cybrarymin/btcblockchain/protogen/pb"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type StateSync struct {
	logger     *zerolog.Logger
	ctx        context.Context
	cfg        *NodeCfg
	state      *chain.State
	peerReader PeerReader
}

func NewStateSync(ctx context.Context, logger *zerolog.Logger, cfg *NodeCfg, pr PeerReader) *StateSync {
	return &StateSync{
		logger:     logger,
		ctx:        ctx,
		cfg:        cfg,
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
					span.RecordError(err)
					span.SetStatus(codes.Error, "failed to create a new genesis")
					return nil, err
				}
			} else {
				sigGen, err = s.SyncGenesis(ctx)
				if err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "failed to sync the genesis")
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

	s.state, err = chain.NewState(*sigGen, s.logger)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create a new state from signed genesis")
		return nil, err
	}

	err = chain.InitBlockStore(ctx, s.cfg.BlockStoreDir)
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

/*
CreateGenesis is used by the boostrap node. if the genesis doesn't exist bootstrap node will create it for the first time it comes up.
*/
func (s *StateSync) CreateGenesis(ctx context.Context) (*chain.SignedGenesis, error) {
	ctx, span := otel.Tracer("CreateGenesis.Tracer").Start(ctx, "CreateGenesis.Span")
	defer span.End()

	// Create and persist the authority account
	authAcc, err := chain.NewAccount(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create the authority account")
		return nil, err
	}
	err = authAcc.Persist(ctx, s.cfg.KeyStoreDir, s.cfg.AuthPass)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to persist the authority account")
		return nil, err
	}

	if s.cfg.Balance == 0 {
		span.RecordError(fmt.Errorf("genesis balance shouldn't be negative"))
		span.SetStatus(codes.Error, "failed to create genesis block because of negative balance")
		err := errors.New("balance must be positive")
		return nil, err
	}

	// Create an owner account and persist the account
	ownerAcc, err := chain.NewAccount(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create the owner account")
		return nil, err
	}
	err = ownerAcc.Persist(ctx, s.cfg.KeyStoreDir, s.cfg.OwnerPass)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to persist the owner account")
		return nil, err
	}

	gen := chain.NewGenesis(s.cfg.ChainName, authAcc.Addr, ownerAcc.Addr, s.cfg.Balance)
	sigGen, err := authAcc.SignGenesis(ctx, gen)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed sign the genesis block using authority account private key")
		return nil, err
	}
	err = sigGen.Persist(ctx, s.cfg.BlockStoreDir)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to persist and write the genesis block to the file")
		return nil, err
	}

	return sigGen, nil
}

/*
grpcGenesisSync is the function for non-boostrap nodes which they are getting connected to the boostrap node to synchronize the genesisBlock
*/
func (s *StateSync) grpcGenesisSync(ctx context.Context) ([]byte, error) {
	ctx, span := otel.Tracer("grpcGenesisSync.Grpc.Tracer").Start(ctx, "grpcGenesisSync.Grpc.Span")
	defer span.End()

	gConn, err := grpc.NewClient(s.cfg.SeedAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "couldn't establish new connection with grpc server")
		return nil, err
	}
	defer gConn.Close()

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
The genesis sync process is performed once for every new node joins the already initialized blockchain with the running bootstrap node.
*/
func (s *StateSync) SyncGenesis(ctx context.Context) (*chain.SignedGenesis, error) {
	ctx, span := otel.Tracer("SyncGenesis.Tracer").Start(ctx, "SyncGenesis.Span")
	defer span.End()

	jGen, err := s.grpcGenesisSync(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to sync and fetch genesis from bootstrap nodes")
		return nil, err
	}

	span.SetAttributes(attribute.String("synced_genesis", string(jGen)))
	sigGen, err := helpers.JsonUnMarshaller[*chain.SignedGenesis](ctx, jGen)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed unmarshal the synced genesis to the struct type")
		return nil, err
	}
	valid, err := chain.VerifyGenesis(ctx, sigGen)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "genesis verification failed")
		return nil, err
	}
	if !valid {
		span.RecordError(fmt.Errorf("invalid genesis signature"))
		span.SetStatus(codes.Error, "genesis signature is invalid. the synced genesis is corrupted")
		err = errors.New("invalid genesis signature")
		return nil, err
	}
	err = sigGen.Persist(ctx, s.cfg.BlockStoreDir)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to persist the synced genesis on file")
		return nil, err
	}
	return sigGen, nil
}

/*
everytime nodes restarts the genesis will be read and initialized then all the blocks will be read and brought to the confirmed state to bring the node back to the state it left off.
*/
func (s *StateSync) readBlocks(ctx context.Context) error {
	ctx, span := otel.Tracer("readBlocks.Tracer").Start(ctx, "readBlocks.Span")
	defer span.End()
	iterator, close, err := chain.ReadBlocks(s.cfg.BlockStoreDir)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, fmt.Sprintf("failed to open the %v to read blocks from", s.cfg.BlockStoreDir))
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
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to apply the block to the current state")
			return err
		}
		// after successfull block application to clone state. apply the clonse state to the current state
		s.state.Apply(clone)
	}

	return nil
}

/*
Each node coming up is going to get connected to the peer and sync it's block from that last block number it had in its state after resyncing it's state locally.
*/
func (s *StateSync) grpcBlockSync(ctx context.Context, peer string) (grpc.ServerStreamingClient[pb.BlockSyncRes], error) {
	ctx, span := otel.Tracer("grpcBlockSync.Grpc.Tracer").Start(ctx, "grpcBlockSync.Grpc.Span")
	defer span.End()

	conn, err := grpc.NewClient(peer, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	grpClient := pb.NewBlockServiceClient(conn)

	var blkNum uint64

	switch {
	case s.state.LastBlock().Blk == nil:
		blkNum = 1
	default:
		blkNum = s.state.LastBlock().Blk.BlockNum
	}

	streamRes, err := grpClient.BlockSync(ctx, &pb.BlockSyncReq{
		BlockNumber: blkNum,
	})
	if err != nil {
		return nil, err
	}

	return streamRes, nil
}

/*
The block sync process propagates the recent confirmed blocks through the blockchain network during the initialization of a new node or the synchronization of an out-of-sync node on the blockchain.
For every known peer the block sync process fetches the new confirmed blocks starting from the block number next to the last confirmed block number on the requesting node
*/
func (s *StateSync) syncBlocks(ctx context.Context) error {
	ctx, span := otel.Tracer("syncBlocks.Tracer").Start(ctx, "syncBlock.Span")
	defer span.End()

	for _, peer := range s.peerReader.Peers() {
		stream, err := s.grpcBlockSync(ctx, peer)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to get the blocks through grpc stream from the boostrap nodes")
			return err
		}
		defer stream.CloseSend() // Close the stream after we're done receiving data

		for {
			resp, err := stream.Recv()
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to get the blocks through grpc stream from the boostrap nodes")
				if err == io.EOF {
					return nil
				}
				return err
			}

			sigBlock, err := helpers.JsonUnMarshaller[*chain.SignedBlock](ctx, resp.Block)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to unmarshal the received json block to the struct type")
				return err
			}

			clone := s.state.Clone()
			err = clone.ApplyBlock(ctx, sigBlock)
			if err != nil {
				span.RecordError(err)
				span.SetAttributes(attribute.String("invalid_block", string(resp.Block)))
				span.SetStatus(codes.Error, "failed to apply the block to the current state")
				return err
			}
			// after successfull block application to clone state. apply the clonse state to the current state
			s.state.Apply(clone)
			err = sigBlock.Persist(ctx, s.cfg.BlockStoreDir)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to persist the block in blockstore.store file")
				return err
			}
		}
	}
	return nil
}
