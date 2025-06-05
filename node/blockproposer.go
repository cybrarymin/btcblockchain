package node

import (
	"context"
	"crypto/rand"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/cybrarymin/btcblockchain/chain"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
)

type BlockProposer struct {
	logger     *zerolog.Logger
	ctx        context.Context
	wg         *sync.WaitGroup
	authority  chain.Account
	state      *chain.State
	blkRelayer BlockRelayer
}

func NewBlockProposer(ctx context.Context, logger *zerolog.Logger, wg *sync.WaitGroup, blkRelayer BlockRelayer) *BlockProposer {
	return &BlockProposer{
		logger:     logger,
		ctx:        ctx,
		wg:         wg,
		blkRelayer: blkRelayer,
	}
}

func (p *BlockProposer) SetAuthority(authority chain.Account) {
	p.authority = authority
}

func (p *BlockProposer) SetState(state *chain.State) {
	p.state = state
}

func randPeriod(maxPeriod time.Duration) time.Duration {
	minPeriod := maxPeriod / 2
	randSpan, _ := rand.Int(rand.Reader, big.NewInt(int64(maxPeriod)))
	return minPeriod + time.Duration(randSpan.Int64())
}

func (p *BlockProposer) ProposeBlock(maxPeriod time.Duration) {
	ctx, span := otel.Tracer("ProposeBlock.Tracer").Start(p.ctx, "ProposeBlock.Span")
	defer p.wg.Done()
	randPropose := time.NewTimer(randPeriod(maxPeriod))
	for {
		select {
		case <-ctx.Done():
			randPropose.Stop()
			return
		case <-randPropose.C:
			randPropose.Reset(randPeriod(maxPeriod))
			cloneState := p.state.Clone()
			nSigBlock, err := cloneState.CreateBlock(ctx, p.authority)
			if err != nil {
				if strings.Contains(err.Error(), "empty list of valid pending transactions") {
					p.logger.Info().Msg("skipping proposing a new block because of empty list of transactions")
					continue
				}
				p.logger.Error().Err(err).Msg("failed to create new block for proposal")
				continue
			}
			cloneState = p.state.Clone()
			err = cloneState.ApplyBlock(ctx, nSigBlock)
			if err != nil {
				p.logger.Error().Err(err).Msg("failed to apply the new proposed block to the clone of the latest state")
				continue
			}
			if p.blkRelayer != nil {
				// relay the block to the other peers
				p.logger.Info().Msgf("relaying the block to the validators: %v", nSigBlock)
				p.blkRelayer.RelayBlock(ctx, nSigBlock)
			}
			span.End()
			p.logger.Info().Msgf("new block proposed: %v", nSigBlock)
		}
	}
}
