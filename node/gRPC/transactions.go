package gRPC

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/cybrarymin/btcblockchain/chain"
	"github.com/cybrarymin/btcblockchain/helpers"
	"github.com/cybrarymin/btcblockchain/protogen/pb"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
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
	logger      *zerolog.Logger
	keyStoreDir string
	txApplier   TransactionApplier
	txRelayer   TransactionRelayer
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
	ctx, span := otel.Tracer("SendTransaction.Tracer").Start(ctx, "SendTransaction.Span")
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
