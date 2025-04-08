package gRPC

import (
	"context"
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
	ApplyTx(stx chain.SignedTransaction) error
	Nonce(acc chain.Address) uint64
}

type TransactionService struct {
	logger      *zerolog.Logger
	keyStoreDir string
	txApplier   TransactionApplier
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
		span.SetStatus(codes.Error, "faield to fetch account informations to sign the transaction")
		return nil, status.Error(grpcCode.Internal, err.Error())
	}

	nTx := chain.NewTransaction(
		chain.Address(req.FromAddress),
		chain.Address(req.ToAddress),
		t.txApplier.Nonce(chain.Address(req.FromAddress))+1,
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
