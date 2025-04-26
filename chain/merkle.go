package chain

import (
	"context"
	"fmt"
)

type MerkleNode struct {
	Left   *MerkleNode `json:"left,omitempty"`
	Right  *MerkleNode `json:"right,omitempty"`
	Hash   Hash        `json:"hash"`
	IsLeaf bool        `json:"isleaf"`
}

func NewMerkleNode(left, right *MerkleNode, currentNode Hash, isleaf bool) *MerkleNode {
	return &MerkleNode{
		Left:   left,
		Right:  right,
		Hash:   currentNode,
		IsLeaf: isleaf,
	}
}

func MerkleHash(ctx context.Context, Txs []SignedTransaction,
	TxHash func(ctx context.Context, tx SignedTransaction) (Hash, error),
	TxPairHash func(ctx context.Context, left, right Hash) (Hash, error)) (*MerkleNode, error) {

	// Checking the transaction list
	if len(Txs) == 0 {
		return nil, fmt.Errorf("cannot create merkle tree from empty transaction list")
	}

	// Creating leaf hashes from all transactions
	leaves := make([]*MerkleNode, 0, len(Txs))
	for _, tx := range Txs {
		hash, err := TxHash(ctx, tx)
		if err != nil {
			return nil, err
		}

		newNode := NewMerkleNode(nil, nil, hash, true)
		leaves = append(leaves, newNode)
	}

	if len(leaves) == 1 {
		return leaves[0], nil
	}

	return buildTree(ctx, leaves, TxPairHash)
}

func buildTree(ctx context.Context, nodes []*MerkleNode, pairHashFunc func(ctx context.Context, left, right Hash) (Hash, error)) (*MerkleNode, error) {

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes to build tree")
	}
	// If only one node remains, it's the root
	if len(nodes) == 1 {
		return nodes[0], nil
	}

	var nextLevel []*MerkleNode
	for i := 0; i < len(nodes); i += 2 {
		var left, right *MerkleNode
		left = nodes[i]

		if len(nodes) > i+1 {
			right = nodes[i+1]
		} else {
			right = left
		}

		nHash, err := pairHashFunc(ctx, left.Hash, right.Hash)
		if err != nil {
			return nil, err
		}
		nNode := NewMerkleNode(left, right, nHash, false)
		nextLevel = append(nextLevel, nNode)
	}
	return buildTree(ctx, nextLevel, pairHashFunc)
}

func GenerateProof(ctx context.Context, txHash Hash, root *MerkleNode,
	txHashFunc func(ctx context.Context, tx SignedTransaction) (Hash, error)) ([]Hash, []bool, error) {

	// check to see if the transaction already exists inside the merkleTree
	exists := Contains(root, txHash)
	if !exists {
		return nil, nil, fmt.Errorf("transaction not found")
	}

	// Generate the proof
	siblings := []Hash{}
	path := []bool{}

	// Calculate the proof
	found := CollectProof(root, txHash, &siblings, &path)
	if !found {
		return nil, nil, fmt.Errorf("failed to generate proof, transaction not in tree")
	}

	return siblings, path, nil
}

func CollectProof(node *MerkleNode, txHash Hash, siblings *[]Hash, path *[]bool) bool {
	// If leaf node, check if it's our target
	if node.Left == nil && node.Right == nil {
		return node.Hash == txHash
	}

	// Check left subtree
	if node.Left != nil && Contains(node.Left, txHash) {
		// Add right sibling to proof
		*siblings = append(*siblings, node.Right.Hash)
		*path = append(*path, false) // We went left
		return CollectProof(node.Left, txHash, siblings, path)
	}

	// Check right subtree
	if node.Right != nil && Contains(node.Right, txHash) {
		// Add left sibling to proof
		*siblings = append(*siblings, node.Left.Hash)
		*path = append(*path, true) // We went right
		return CollectProof(node.Right, txHash, siblings, path)
	}

	return false
}

// Helper to check if a node or its children contains a transaction hash
func Contains(node *MerkleNode, hash Hash) bool {
	if node == nil {
		return false
	}
	if node.Hash == hash {
		return true
	}
	return Contains(node.Left, hash) || Contains(node.Right, hash)
}

func VerifyMerkle(txHash Hash, root *MerkleNode) (bool, error) {

	exists := Contains(root, txHash)
	if !exists {
		return false, nil
	}

	return true, nil
}
