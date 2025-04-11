package chain

import (
	"context"
)

func MerkleHash(ctx context.Context, Txs []SignedTransaction,
	TxHash func(ctx context.Context, tx SignedTransaction) (Hash, error),
	TxPairHash func(ctx context.Context, left, right Hash) (Hash, error)) ([]Hash, error) {

	txHashList := []Hash{}

	for _, Tx := range Txs {
		txHash, err := Tx.Hash(ctx)
		if err != nil {
			return nil, err
		}
		txHashList = append(txHashList, txHash)
	}

	if len(txHashList) == 1 {
		return txHashList, nil
	}

	hashlistPtr := new([]Hash)
	hashlistPtr = &txHashList
	for {
		nHashList, err := HashIterator(ctx, hashlistPtr)
		if err != nil {
			return nil, err
		}
		txHashList = append(txHashList, nHashList...)
		if len(nHashList) == 1 {
			break
		}
		hashlistPtr = &nHashList
	}

	return txHashList, nil
}

func HashIterator(ctx context.Context, hashList *[]Hash) ([]Hash, error) {
	newHashList := []Hash{}
	for i := 0; i < len(*hashList); i += 2 {
		if i+1 == len(*hashList) {
			nHash, err := TxPairHash(ctx, (*hashList)[i], Hash{})
			if err != nil {
				return nil, err
			}
			newHashList = append(newHashList, nHash)
			break
		}
		nHash, err := TxPairHash(ctx, (*hashList)[i], (*hashList)[i+1])
		if err != nil {
			return nil, err
		}
		newHashList = append(newHashList, nHash)
	}
	return newHashList, nil
}
