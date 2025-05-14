package chain

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMerkleVerify(t *testing.T) {
	transactionNum := 4

	ctx := context.Background()
	sourceAcc, err := NewAccount(ctx)
	assert.NoError(t, err, "shouldn't have an error for creating a source account")
	dstAcc, err := NewAccount(ctx)
	assert.NoError(t, err, "shouldn't have an error for creating a destination account")

	sTxList := make([]SignedTransaction, 0, transactionNum)
	for i := 0; i < transactionNum; i++ {
		nTx := NewTransaction(sourceAcc.Addr, dstAcc.Addr, uint64(i), 100)
		sTx, err := sourceAcc.SignTx(ctx, nTx)
		assert.NoError(t, err, "shouldn't have an error for signing a new transaction")
		sTxList = append(sTxList, *sTx)
	}

	rootNode, err := MerkleHash(ctx, sTxList, TxHash, TxPairHash)
	assert.NoError(t, err, "shouldn't have an error when creating the merkleTree of the transactions")

	t.Logf("MerkleRootHash: %s\n\n\n", rootNode.Hash)

	// Print the entire tree structure
	t.Log("MerkleTreeStructure")
	printMerkleTree(t, rootNode, 0, "")

	// Print the hashes of individual transactions for reference
	t.Log("\nTransaction Hashes:")
	for i, tx := range sTxList {
		txHash, err := TxHash(ctx, tx)
		assert.NoError(t, err)
		t.Logf("Transaction %d: %s", i, txHash)
	}

	// Generate Proofs
	txHash, _ := sTxList[0].Hash(ctx)
	Proofs, Positions, err := GenerateProofs(ctx, txHash, rootNode, TxHash)
	assert.NoError(t, err, "shouldn't fail during generating the merkleProofs")
	t.Log("\nMerkleProofs")
	for i, proof := range Proofs {
		t.Logf("Proof%d: %s", i, proof)
	}

	t.Log("MerkleTransactionRelativePositions", Positions)

	valid, calculatedRoot, err := VerifyMerkle(ctx, txHash, rootNode, Proofs, Positions, TxPairHash)
	t.Log("Calculated MerkleRoot From Proofs:", calculatedRoot)
	assert.NoError(t, err, "souldn't have error when verifying a valid transaction in the merkleTree")
	assert.Equal(t, valid, true)
}

// printMerkleTree prints the Merkle tree in a readable format
func printMerkleTree(t *testing.T, node *MerkleNode, depth int, prefix string) {
	if node == nil {
		return
	}

	indent := strings.Repeat("  ", depth)
	nodeType := "Branch"
	if node.IsLeaf {
		nodeType = "Leaf"
	}

	t.Logf("%s%s└─ %s: %s", indent, prefix, nodeType, node.Hash)

	if node.Left != nil {
		printMerkleTree(t, node.Left, depth+3, "L")
	}

	if node.Right != nil {
		printMerkleTree(t, node.Right, depth+3, "R")
	}
}
