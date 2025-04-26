package chain

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cybrarymin/btcblockchain/helpers"
	"github.com/dustinxie/ecc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const Blocksfile = "block.store"

type Block struct {
	BlockNum        uint64              `json:"block_number"`
	ParentBlockHash Hash                `json:"parent_block_hash"`
	Txs             []SignedTransaction `json:"transactions"`
	MerkleTree      *MerkleNode         `json:"-"`
	MerkleTreeRoot  Hash                `json:"transactions_merkle_tree_root"`
	Time            time.Time           `json:"time"`
}

func NeWBlock(ctx context.Context, parenBlockHash Hash, Txs []SignedTransaction, BlockNum uint64) (*Block, error) {
	merkleTreeRoot, err := MerkleHash(ctx, Txs, TxHash, TxPairHash)
	if err != nil {
		return nil, err
	}
	return &Block{
		BlockNum:        BlockNum,
		ParentBlockHash: parenBlockHash,
		Txs:             Txs,
		MerkleTree:      merkleTreeRoot,
		MerkleTreeRoot:  merkleTreeRoot.Hash,
		Time:            time.Now(),
	}, nil
}

func (b *Block) Hash(ctx context.Context) (Hash, error) {
	hash, err := NewHash(ctx, b)
	if err != nil {
		return Hash{}, err
	}
	return hash, nil
}

type SignedBlock struct {
	Blk *Block
	Sig []byte
}

func NewSinedBlock(blk *Block, sig []byte) *SignedBlock {
	return &SignedBlock{
		Blk: blk,
		Sig: sig,
	}
}

func (sigBlock *SignedBlock) Hash(ctx context.Context) (Hash, error) {
	hash, err := NewHash(ctx, sigBlock)
	if err != nil {
		return Hash{}, err
	}
	return hash, nil
}

func (sigBlock *SignedBlock) String() string {
	var bld strings.Builder
	hash, _ := sigBlock.Blk.Hash(context.Background())
	bld.WriteString(
		fmt.Sprintf(
			"blk %7d: %.7s -> %.7s   mrk %.7s\n",
			sigBlock.Blk.BlockNum, hash, sigBlock.Blk.ParentBlockHash, sigBlock.Blk.MerkleTreeRoot,
		),
	)
	for _, tx := range sigBlock.Blk.Txs {
		bld.WriteString(fmt.Sprintf("%v\n", tx))
	}
	return bld.String()
}

func (acc *Account) SignBlock(ctx context.Context, blk *Block) (*SignedBlock, error) {
	ctx, span := otel.Tracer("SignBlock.Tracer").Start(ctx, "SignBlock.Span")
	defer span.End()
	blkHash, err := blk.Hash(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to calculate hash of the block to sign the block")
		return nil, err
	}
	sigBlock, err := ecc.SignBytes(acc.Priv, blkHash.Bytes(), ecc.LowerS|ecc.RecID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to encrypt the hash of the block and make digital signature")
		return nil, err
	}
	return &SignedBlock{
		Blk: blk,
		Sig: sigBlock,
	}, nil
}

func VerifyBlock(ctx context.Context, sigBlock *SignedBlock, authority Address) (bool, error) {
	ctx, span := otel.Tracer("VerifyBlock.Tracer").Start(ctx, "VerifyBlock.Span")
	defer span.End()
	blkHash, err := sigBlock.Blk.Hash(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to calculate hash of the block to verify the signature")
		return false, err
	}
	userPubKey, err := ecc.RecoverPubkey("P521", blkHash.Bytes(), sigBlock.Sig)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to recover public key from digital signature")
		return false, err
	}

	accAddress, err := NewAddress(ctx, userPubKey)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to calculate the user address to verify authority account of the signed block")
		return false, err
	}

	return accAddress == authority, nil
}

func (sigBlock *SignedBlock) Persist(ctx context.Context, dirPath string) error {
	blkfile, err := os.OpenFile(filepath.Join(dirPath, Blocksfile), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer blkfile.Close()
	return json.NewEncoder(blkfile).Encode(sigBlock)
}

type ReadBlocksIterator struct {
	scanner *bufio.Scanner
}

func NewReadBlocksIterator(scanner *bufio.Scanner) *ReadBlocksIterator {
	return &ReadBlocksIterator{
		scanner: scanner,
	}
}

func (it *ReadBlocksIterator) Next() (*SignedBlock, error) {
	ctx := context.Background()
	ok := it.scanner.Scan()

	if err := it.scanner.Err(); err != nil {
		return nil, err
	}

	if it.scanner.Err() == nil && !ok {
		return nil, io.EOF
	}

	nsigBlock, err := helpers.JsonUnMarshaller[*SignedBlock](ctx, it.scanner.Bytes())
	if err != nil {
		return nil, err
	}
	return nsigBlock, nil
}

func ReadBlocks(dir string) (*ReadBlocksIterator, func(), error) {
	file, err := os.Open(filepath.Join(dir, Blocksfile))
	if err != nil {
		return nil, nil, err
	}
	close := func() {
		file.Close()
	}

	sca := bufio.NewScanner(file)
	nIterator := NewReadBlocksIterator(sca)
	return nIterator, close, nil
}
