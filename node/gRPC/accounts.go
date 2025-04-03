package gRPC

import (
	"context"

	"github.com/cybrarymin/btcblockchain/chain"
	"github.com/cybrarymin/btcblockchain/protogen/pb"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AccountSrv struct {
	logger      *zerolog.Logger
	keyStoreDir string
	pb.AccountServiceServer
}

func NewAccountSrv(logger *zerolog.Logger, directory string) *AccountSrv {
	return &AccountSrv{
		logger:      logger,
		keyStoreDir: directory,
	}
}

func (s *AccountSrv) CreateAccount(ctx context.Context, req *pb.CreateAccountReq) (*pb.CreateAccountResp, error) {
	nAcc, err := chain.NewAccount()
	if err != nil {
		s.logger.Error().Err(err).
			Msg("failed to create a new account with a new address")

		return nil, status.Error(codes.Internal, err.Error())
	}

	s.logger.Info().
		Str("account_address", string(nAcc.Addr)).
		Msg("persisting the new account")

	err = nAcc.Persist(s.keyStoreDir, req.Password)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.CreateAccountResp{
		Address: string(nAcc.Addr),
	}, nil
}
