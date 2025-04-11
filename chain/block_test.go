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

	nTx1 := NewTransaction(authorityAcc.Addr, authorityAcc.Addr, 1, 100)
	nTx2 := NewTransaction(authorityAcc.Addr, authorityAcc.Addr, 1, 200)
	signedTx1, err := authorityAcc.SignTx(ctx, nTx1)
	assert.NoError(t, err)
	signedTx2, err := authorityAcc.SignTx(ctx, nTx2)
	assert.NoError(t, err)
	txhash, _ := nTx2.Hash(ctx)
	t.Log(txhash)
	t.Log(TxPairHash(ctx, txhash, Hash{}))

	txList := []SignedTransaction{}
	txList = append(txList, *signedTx1, *signedTx2)

	blk, err := NeWBlock(ctx, genHash, txList, 1)
	assert.NoError(t, err)

	signedBlk, err := authorityAcc.SignBlock(ctx, blk)

	assert.NoError(t, err)
	err = signedBlk.Persist(ctx, dirPath)
	assert.NoError(t, err)
}
