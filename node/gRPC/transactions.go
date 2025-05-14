package gRPC

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"slices"

	"github.com/cybrarymin/btcblockchain/chain"
	"github.com/cybrarymin/btcblockchain/helpers"
	"github.com/cybrarymin/btcblockchain/protogen/pb"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	grpcCode "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type TransactionApplier interface {
	ApplyTx(ctx context.Context, stx *chain.SignedTransaction) error
	Nonce(addr chain.Address) (uint64, bool)
}

type TransactionRelayer interface {
	RelayTx(ctx context.Context, stx *chain.SignedTransaction) error // TODO
}

type TransactionService struct {
	logger        *zerolog.Logger
	keyStoreDir   string
	blockStoreDir string
	txApplier     TransactionApplier
	txRelayer     TransactionRelayer
	pb.TransactionServiceServer
}

func NewTransactionService(logger *zerolog.Logger, KeyStoreDir string, txapplier TransactionApplier) *TransactionService {
	return &TransactionService{
		logger:      logger,
		keyStoreDir: KeyStoreDir,
		txApplier:   txapplier,
	}
}

func (t *TransactionService) SignTransaction(ctx context.Context, req *pb.TxSignReq) (*pb.TxSignRes, error) {
	ctx, span := otel.Tracer("SignTransaction.Grpc.Tracer").Start(ctx, "SignTransaction.Grpc.Tracer")
	defer span.End()
	accountfilePath := filepath.Join(t.keyStoreDir, req.FromAddress)

	userAcc, err := chain.ReadAccount(ctx, accountfilePath, req.Password)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to fetch account informations to sign the transaction")
		return nil, status.Error(grpcCode.Internal, err.Error())
	}

	nonce, exists := t.txApplier.Nonce(chain.Address(req.FromAddress))
	if !exists {
		err = errors.New("account doesn't exists")
		span.RecordError(err)
		span.SetStatus(codes.Error, "couldn't get the nonce of the account because account doesn't exist")
		return nil, status.Error(grpcCode.NotFound, err.Error())
	}

	nTx := chain.NewTransaction(
		chain.Address(req.FromAddress),
		chain.Address(req.ToAddress),
		nonce+1,
		req.Value)

	nStx, err := userAcc.SignTx(ctx, nTx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to sign the transaction")
		return nil, status.Error(grpcCode.Internal, err.Error())
	}
	byteStx, err := helpers.JsonMarshaller(ctx, nStx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to serialize singed transaction to json")
		return nil, status.Error(grpcCode.Internal, err.Error())
	}

	return &pb.TxSignRes{
		SignedTransaction: byteStx,
	}, nil
}

func (t *TransactionService) SendTransaction(ctx context.Context, req *pb.TxSendReq) (*pb.TxSendRes, error) {
	ctx, span := otel.Tracer("SendTransaction.Grpc.Tracer").Start(ctx, "SendTransaction.Grpc.Span")
	defer span.End()

	sTx, err := helpers.JsonUnMarshaller[*chain.SignedTransaction](ctx, req.SignedTransaction)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "couldn't deserialize the signed transaction from json format")
		return nil, status.Error(grpcCode.InvalidArgument, err.Error())
	}
	err = t.txApplier.ApplyTx(ctx, sTx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "couldn't apply the transaction to the pending state")
		return nil, status.Error(grpcCode.FailedPrecondition, err.Error())
	}

	if t.txRelayer != nil {
		t.txRelayer.RelayTx(ctx, sTx)
	}

	tHash, err := sTx.Tx.Hash(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "couldn't calculate the transaction hash to provide as a response")
		return nil, status.Error(grpcCode.FailedPrecondition, err.Error())
	}

	return &pb.TxSendRes{
		Hash: tHash.String(),
	}, nil

}

func (t *TransactionService) ProveTransaction(ctx context.Context, req *pb.TxProveReq) (*pb.TxProveRes, error) {
	ctx, span := otel.Tracer("ProveTransaction.Grpc.Tracer").Start(ctx, "ProveTransaction.Grpc.Span")
	defer span.End()

	sTx, err := helpers.JsonUnMarshaller[*chain.SignedTransaction](ctx, req.SignedTransaction)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to deserialize the json formated signed transaction")
		return nil, status.Error(grpcCode.Internal, err.Error())
	}
	sTxHash, err := sTx.Hash(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to calculate the signed transaction hash")
		return nil, status.Error(grpcCode.Internal, err.Error())
	}

	iterator, close, err := chain.ReadBlocks(t.blockStoreDir)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to open the blockStore for finding and verifying the transaction")
		return nil, status.Error(grpcCode.Internal, err.Error())
	}
	defer close()

	var sBlk *chain.SignedBlock
	for {
		if err := ctx.Err(); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Ok, "operation canceled")
			return nil, status.Error(grpcCode.Canceled, err.Error())
		}
		sBlk, err = iterator.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			span.RecordError(err)
			continue
		}

		exists := slices.ContainsFunc(sBlk.Blk.Txs, func(bsTx chain.SignedTransaction) bool {
			hash, _ := bsTx.Hash(ctx)
			return hash == sTxHash
		})

		if exists {
			merkleRoot, err := chain.MerkleHash(ctx, sBlk.Blk.Txs, chain.TxHash, chain.TxPairHash)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to calculate the merkle root for the list of transaction in the block")
				return nil, status.Error(grpcCode.Internal, err.Error())
			}
			proofs, _, err := chain.GenerateProofs(ctx, sTxHash, merkleRoot, chain.TxHash)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to calculate the merkle proofs for the transaction")
				return nil, status.Error(grpcCode.Internal, err.Error())
			}

			jsonProofs, err := helpers.JsonMarshaller(ctx, proofs)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to serialize the merkle proof to json format")
				return nil, status.Error(grpcCode.Internal, err.Error())
			}

			return &pb.TxProveRes{
				MerkleProof: jsonProofs,
			}, nil
		}
	}
	return nil, status.Error(grpcCode.NotFound, "couldn't find the transaction")
}

func (t *TransactionService) VerifyTransaction(ctx context.Context, req *pb.TxVerifyReq) (*pb.TxVerifyRes, error) {
	ctx, span := otel.Tracer("VerifyTransaction.Grpc.Tracer").Start(ctx, "VerifyTransaction.Grpc.Span")
	defer span.End()
	txHashBytes, err := chain.DecodeHash(ctx, req.TxHash)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to decode transaction hexadecimal formatted hash")
		return nil, status.Error(grpcCode.InvalidArgument, err.Error())
	}
	merkleRootHashByte, err := chain.DecodeHash(ctx, req.MerkleRootHash)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to decode merkle root hexadecimal formatted hash")
		return nil, status.Error(grpcCode.InvalidArgument, err.Error())
	}
	merkleRootNode := chain.NewMerkleNode(nil, nil, merkleRootHashByte, false)

	proofList := make([]chain.Hash, 0, len(req.MerkleProofs))
	for _, proof := range req.MerkleProofs {
		proof, err := chain.DecodeHash(ctx, proof)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to decode merkle proofs hexadecimal formatted hash")
			span.SetAttributes(attribute.String("merkle proof:", proof.String()))
			return nil, status.Error(grpcCode.InvalidArgument, err.Error())
		}
		proofList = append(proofList, proof)
	}

	valid, _, err := chain.VerifyMerkle(ctx, txHashBytes, merkleRootNode, proofList, req.MerkleProofPositions, chain.TxPairHash)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "transaction verification process failed")
		return nil, status.Error(grpcCode.Internal, err.Error())
	}

	return &pb.TxVerifyRes{
		Valid: valid,
	}, nil
}
