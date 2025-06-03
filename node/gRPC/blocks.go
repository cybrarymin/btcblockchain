package gRPC

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cybrarymin/btcblockchain/chain"
	"github.com/cybrarymin/btcblockchain/helpers"
	"github.com/cybrarymin/btcblockchain/protogen/pb"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc"
	grpcCode "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type BlockService struct {
	logger   *zerolog.Logger
	BlockDir string
	state    *chain.State
	pb.BlockServiceServer
}

func NewBlockService(logger *zerolog.Logger, BlockDir string, state *chain.State) *BlockService {
	return &BlockService{
		logger:   logger,
		BlockDir: BlockDir,
		state:    state,
	}
}

func (s *BlockService) SearchBlock(req *pb.SearchBlockReq, res grpc.ServerStreamingServer[pb.SearchBlockRes]) error {
	ctx, span := otel.Tracer("SearchBlock.Grpc.Tracer").Start(context.Background(), "SearchBlock.Grpc.Span")
	defer span.End()
	blkIterator, closeBlocks, err := chain.ReadBlocks(s.BlockDir)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read block from block store")
		return status.Error(grpcCode.NotFound, "failed to read block from block store")
	}
	defer closeBlocks()

	for {
		nSignedBlock, err := blkIterator.Next()
		if err != nil {
			if err != io.EOF {
				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to read block from block store")
				return status.Error(grpcCode.NotFound, "failed to read block from block store")
			}
			return nil
		}

		blkHash, err := nSignedBlock.Blk.Hash(ctx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to calculate block hash to comply it with requested hash")
			return status.Error(grpcCode.NotFound, "failed to calculate block hash to comply it with requested hash")
		}

		if req.BlockNumber != 0 && nSignedBlock.Blk.BlockNum == req.BlockNumber ||
			len(req.BlockHash) > 0 && blkHash.String() == req.BlockHash ||
			len(req.ParentBlockHash) > 0 && req.ParentBlockHash == nSignedBlock.Blk.ParentBlockHash.String() {

			signedBlk, err := helpers.JsonMarshaller(ctx, nSignedBlock)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to serialize the singed block to json as a response")
				return status.Error(grpcCode.NotFound, "failed to serialize the singed block to json as a response")
			}
			nRes := &pb.SearchBlockRes{
				Block: signedBlk,
			}
			err = res.Send(nRes)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to send grpc response")
				return status.Error(grpcCode.NotFound, "failed to sent the grpc response")
			}
		} else {
			return status.Error(grpcCode.NotFound, "requested block doesn't exist")
		}
	}
}

func (s *BlockService) GenesisSync(ctx context.Context, req *pb.GenesisSynReq) (*pb.GenesisSyncRes, error) {
	ctx, span := otel.Tracer("GenesisSync.Grpc.Tracer").Start(ctx, "GenesisSync.Grpc.Span")
	defer span.End()

	sigGen, err := chain.ReadGenesisBytes(ctx, s.BlockDir)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read the genesis block from local blockstore")
		return nil, status.Error(grpcCode.Internal, err.Error())
	}
	return &pb.GenesisSyncRes{
		Genesis: sigGen,
	}, nil
}

func (s *BlockService) BlockSync(req *pb.BlockSyncReq, stream grpc.ServerStreamingServer[pb.BlockSyncRes]) error {
	_, span := otel.Tracer("BlockSync.Grpc.Tracer").Start(context.Background(), "BlockSync.Grpc.Span")
	defer span.End()

	iterator, close, err := chain.ReadBlocks(s.BlockDir)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read blockstore to create a block iterator")
		return status.Error(grpcCode.Internal, err.Error())
	}
	defer close()
	var counter uint64 = 1
	for {
		sigBlkBytes, err := iterator.NextBytes()
		if err != nil {
			if err == io.EOF {
				break
			}
			continue
		}
		if counter < req.BlockNumber {
			continue
		}

		nRes := &pb.BlockSyncRes{
			Block: sigBlkBytes,
		}
		stream.Send(nRes)
	}
	return nil
}

func (s *BlockService) BlockReceive(stream grpc.ClientStreamingServer[pb.BlockReceiveReq, pb.BlockReceiveRes]) error {
	ctx, span := otel.Tracer("BlockSync.Grpc.Tracer").Start(context.Background(), "BlockSync.Grpc.Span")
	defer span.End()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			res := &pb.BlockReceiveRes{}
			return stream.SendAndClose(res)
		}
		if err != nil {
			return status.Errorf(grpcCode.Internal, err.Error())
		}
		var blk chain.SignedBlock
		err = json.Unmarshal(req.Block, &blk)
		if err != nil {
			fmt.Println(err)
			continue
		}
		s.logger.Info().Msgf("received a new block: %v", blk)
		err = s.state.ApplyBlockToState(ctx, &blk)
		if err != nil {
			s.logger.Error().Err(err).Msg("failed to apply the received proposed block to the current state of the node")
			continue
		}
		err = blk.Persist(ctx, s.BlockDir)
		if err != nil {
			s.logger.Error().Err(err).Msg("failed to persist the received proposed block to the blockstore.store file")
			continue
		}
		if s.state. != nil {
			s.blkRelayer.RelayBlock(blk)
		}
		if s.eventPub != nil {
			s.publishBlockAndTxs(blk)
		}
	}
}
