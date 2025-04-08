package gRPC

import (
	"context"
	"fmt"

	"github.com/cybrarymin/btcblockchain/chain"
	"github.com/cybrarymin/btcblockchain/protogen/pb"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	grpcCode "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type BalanceChecker interface {
	Balance(context.Context, chain.Address) (uint64, bool)
}

type AccountSrv struct {
	logger      *zerolog.Logger
	keyStoreDir string
	BalChecker  BalanceChecker
	pb.AccountServiceServer
}

func NewAccountSrv(logger *zerolog.Logger, directory string, balChecker BalanceChecker) *AccountSrv {
	return &AccountSrv{
		logger:      logger,
		keyStoreDir: directory,
		BalChecker:  balChecker,
	}
}

func (s *AccountSrv) CreateAccount(ctx context.Context, req *pb.CreateAccountReq) (*pb.CreateAccountResp, error) {
	ctx, span := otel.Tracer("CreateAccount.Grpc.Tracer").Start(ctx, "CreateAccount.Grpc.Span")
	defer span.End()

	nAcc, err := chain.NewAccount(ctx)
	if err != nil {
		s.logger.Error().Err(err).
			Msg("failed to create a new account with a new address")

		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create a new account")
		return nil, status.Error(grpcCode.Internal, err.Error())
	}

	s.logger.Info().
		Str("account_address", string(nAcc.Addr)).
		Msg("persisting the new account")

	err = nAcc.Persist(ctx, s.keyStoreDir, req.Password)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to persist the new account")
		return nil, status.Error(grpcCode.Internal, err.Error())
	}

	return &pb.CreateAccountResp{
		Address: string(nAcc.Addr),
	}, nil
}

func (s *AccountSrv) AccountBalance(ctx context.Context, req *pb.AccountBalanceReq) (*pb.AccountBalanceResp, error) {
	ctx, span := otel.Tracer("AccountBalance.Grpc.Tracer").Start(ctx, "AccountBalance.Grpc.Span")
	defer span.End()

	accAddr := req.Address
	balance, exists := s.BalChecker.Balance(ctx, chain.Address(accAddr))
	if !exists {
		span.RecordError(fmt.Errorf("account %v not found or doesn't have transaction", accAddr))
		span.SetStatus(codes.Error, "failed to fetch required information")
		return nil, status.Errorf(grpcCode.NotFound, "account %v doesn't exists or doesn't have any transaction yet", accAddr)
	}

	return &pb.AccountBalanceResp{
		Balance: balance,
	}, nil
}
