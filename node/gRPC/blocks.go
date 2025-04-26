package gRPC

import (
	"context"
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
	logger  *zerolog.Logger
	dirPath string
	pb.BlockServiceServer
}

func NewBlockService(logger *zerolog.Logger, dirPath string) *BlockService {
	return &BlockService{
		logger:  logger,
		dirPath: dirPath,
	}
}

func (s *BlockService) SearchBlock(req *pb.SearchBlockReq, res grpc.ServerStreamingServer[pb.SearchBlockRes]) error {
	ctx, span := otel.Tracer("SearchBlock.Grpc.Tracer").Start(context.Background(), "SearchBlock.Grpc.Span")
	defer span.End()
	blkIterator, closeBlocks, err := chain.ReadBlocks(s.dirPath)
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
