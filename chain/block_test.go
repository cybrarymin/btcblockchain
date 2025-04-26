package chain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSignVerifyPersistReadBlocks(t *testing.T) {
	dirPath := "/tmp/blockchain"
	ctx := context.Background()
	authorityAcc, err := NewAccount(ctx)
	assert.NoError(t, err)

	genBlock := NewGenesis("blockchain", authorityAcc.Addr, map[Address]uint64{
		authorityAcc.Addr: 1_000_000_000_000_000,
	})
	signedGen, err := authorityAcc.SignGenesis(ctx, genBlock)

	assert.NoError(t, err)
	err = signedGen.Persist(ctx, dirPath)
	assert.NoError(t, err)

	genHash, err := signedGen.Gen.Hash(ctx)

	assert.NoError(t, err)

	txList := []SignedTransaction{}
	for i := 0; i < 5; i++ {
		nTx1 := NewTransaction(authorityAcc.Addr, authorityAcc.Addr, uint64(i), 100)
		signedTx, err := authorityAcc.SignTx(ctx, nTx1)
		assert.NoError(t, err)
		txList = append(txList, *signedTx)
	}

	blk, err := NeWBlock(ctx, genHash, txList, 1)
	assert.NoError(t, err)

	signedBlk, err := authorityAcc.SignBlock(ctx, blk)
	assert.NoError(t, err)

	blk2, err := NeWBlock(ctx, genHash, txList, 2)
	assert.NoError(t, err)

	signedBlk2, err := authorityAcc.SignBlock(ctx, blk2)

	assert.NoError(t, err)
	err = signedBlk.Persist(ctx, dirPath)
	assert.NoError(t, err)
	err = signedBlk2.Persist(ctx, dirPath)
	assert.NoError(t, err)
}
