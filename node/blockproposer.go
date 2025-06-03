package node

import (
	"context"
	"crypto/rand"
	"math/big"
	"sync"
	"time"

	"github.com/cybrarymin/btcblockchain/chain"
	"github.com/rs/zerolog"
)

type BlockProposer struct {
	logger     *zerolog.Logger
	ctx        context.Context
	wg         *sync.WaitGroup
	authority  chain.Account
	state      *chain.State
	blkRelayer BlockRelayer
}

func NewBlockProposer(ctx context.Context, wg *sync.WaitGroup, blkRelayer BlockRelayer) *BlockProposer {
	return &BlockProposer{
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
	defer p.wg.Done()
	randPropose := time.NewTimer(randPeriod(maxPeriod))
	for {
		select {
		case <-p.ctx.Done():
			randPropose.Stop()
			return
		case <-randPropose.C:
			randPropose.Reset(randPeriod(maxPeriod))
			cloneState := p.state.Clone()
			nSigBlock, err := cloneState.CreateBlock(p.ctx, p.authority)
			if err != nil {
				p.logger.Warn().Msg("failed to create new block for proposal")
				continue
			}
			if len(nSigBlock.Blk.Txs) == 0 {
				p.logger.Info().Msg("skipping proposing new block because of empty transactions")
				continue
			}
			err = cloneState.ApplyBlock(p.ctx, nSigBlock)
			if err != nil {
				p.logger.Error().Err(err).Msg("failed to apply the new proposed block to the clone of the latest state")
				continue
			}
			if p.blkRelayer != nil {
				// relay the block to the other peers
				p.blkRelayer.RelayBlock(p.ctx, nSigBlock)

				p.logger.Info().Msgf("relaying the block to the validators: %v", nSigBlock)
			}
			p.logger.Info().Msgf("new block proposed: %v", nSigBlock)
		}
	}
}
