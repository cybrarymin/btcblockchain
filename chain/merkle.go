package chain

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const (
	RightPosition = true
	LeftPosition  = false
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

/*
MerkleHash will get a list of signed Transactions and returns the merkleTree of those transactions.
Function returns the merkleRoot pointer which we can traverse the whole tree using that.
*/
func MerkleHash(ctx context.Context, sTxs []SignedTransaction,
	TxHash func(ctx context.Context, tx SignedTransaction) (Hash, error),
	TxPairHash func(ctx context.Context, left, right Hash) (Hash, error)) (*MerkleNode, error) {
	ctx, span := otel.Tracer("MerkleHash.Tracer").Start(ctx, "MerkleHash.Span")
	defer span.End()

	// Checking the transaction list
	if len(sTxs) == 0 {
		err := errors.New("cannot create merkle tree from empty transaction list")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Creating leaf hashes from all transactions
	leaves := make([]*MerkleNode, 0, len(sTxs))
	for _, tx := range sTxs {
		hash, err := TxHash(ctx, tx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
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

/*
BuildTree will get the leaves nodes and creates the tree and returns the MerkleRoot nodes as a pointer
*/
func buildTree(ctx context.Context, nodes []*MerkleNode, pairHashFunc func(ctx context.Context, left, right Hash) (Hash, error)) (*MerkleNode, error) {
	ctx, span := otel.Tracer("buildTree.Tracer").Start(ctx, "buildTree.Span")
	defer span.End()

	if len(nodes) == 0 {
		err := errors.New("no nodes to build the merkle tree")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
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
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		nNode := NewMerkleNode(left, right, nHash, false)
		nextLevel = append(nextLevel, nNode)
	}
	return buildTree(ctx, nextLevel, pairHashFunc)
}

/*
GenerateProofs will generate the proofs ( sibilings and positions we went ) for specific transaction.
It checks the hash of transaction to see if it already exists in the merkleTree and if yes it will create the proofs and their positions.
*/
func GenerateProofs(ctx context.Context, txHash Hash, root *MerkleNode,
	txHashFunc func(ctx context.Context, tx SignedTransaction) (Hash, error)) ([]Hash, []bool, error) {
	ctx, span := otel.Tracer("GenerateProofs.Tracer").Start(ctx, "GenerateProofs.Span")
	defer span.End()
	// check to see if the transaction already exists inside the merkleTree
	exists := Contains(ctx, root, txHash)
	if !exists {
		err := errors.New("transaction not found")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, nil, err
	}

	// Generate the proof
	siblings := []Hash{}
	path := []bool{}

	// Calculate the proof
	found := CollectProof(ctx, root, txHash, &siblings, &path)
	if !found {
		err := errors.New("failed to generate proof, transaction not in tree")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, nil, err
	}

	return siblings, path, nil
}

/*
Collect proof will get the MetkleRoot node and hash of the transaction. And it will traverse the tree on one side to find the hash and collects the sibilings and positions
*/
func CollectProof(ctx context.Context, node *MerkleNode, txHash Hash, siblings *[]Hash, path *[]bool) bool {
	ctx, span := otel.Tracer("CollectProofs.Tracer").Start(ctx, "CollectProofs.Span")
	defer span.End()

	// If leaf node, check if it's our target
	if node.Left == nil && node.Right == nil {
		return node.Hash == txHash
	}

	// Check left subtree
	if node.Left != nil && Contains(ctx, node.Left, txHash) {
		// Add right sibling to proof
		*siblings = append(*siblings, node.Right.Hash)
		*path = append(*path, LeftPosition) // We went left
		return CollectProof(ctx, node.Left, txHash, siblings, path)
	}

	// Check right subtree
	if node.Right != nil && Contains(ctx, node.Right, txHash) {
		// Add left sibling to proof
		*siblings = append(*siblings, node.Left.Hash)
		*path = append(*path, RightPosition) // We went right
		return CollectProof(ctx, node.Right, txHash, siblings, path)
	}

	return false
}

/*
Helper to check if a node or its children contains a transaction hash
*/
func Contains(ctx context.Context, node *MerkleNode, hash Hash) bool {

	if node == nil {
		return false
	}
	if node.Hash == hash {
		return true
	}
	return Contains(ctx, node.Left, hash) || Contains(ctx, node.Right, hash)
}

/*
Verifies that the transaction and it's proofs are able to create the correct merkleRoot
It will take the expected MerkleRoot node, the transaction hash that we want to verify the proofs we calculated for the transaction
*/
func VerifyMerkle(ctx context.Context, txHash Hash, expectedRoot *MerkleNode, siblings []Hash, path []bool, TxPairHash func(ctx context.Context, left, right Hash) (Hash, error)) (bool, Hash, error) {
	ctx, span := otel.Tracer("VerifyMerkle.Tracer").Start(ctx, "VerifyMerkle.Span")
	defer span.End()

	// check to see there is not mistake on gathering the proofs and number of positions are same as number of siblings
	if len(siblings) != len(path) {
		err := errors.New("siblings and path must have the same length")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, Hash{}, err
	}

	currentHash := txHash
	// Traverse the path from leaf to root
	for i := len(siblings) - 1; i >= 0; i-- {
		var left, right Hash
		// If path[i] is true, our current hash is on the right
		// Otherwise, it's on the left
		if path[i] {
			// We went right in the tree, so sibling is on the left
			left = siblings[i]
			right = currentHash
		} else {
			// We went left in the tree, so sibling is on the right
			left = currentHash
			right = siblings[i]
		}

		// Hash the pair to get the parent
		var err error
		currentHash, err = TxPairHash(ctx, left, right)
		if err != nil {
			return false, currentHash, fmt.Errorf("failed to hash pair: %w", err)
		}
	}

	// After processing all siblings, currentHash should be the root
	return currentHash == expectedRoot.Hash, currentHash, nil
}
