package chain

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

/*
State is the current situation or state of our blockchain.
Which represents the balances that we have in the blockchain.
Also we have some additional information such as all confirmed transactions the last confirmed block and the pending state waiting to be approved
*/
type State struct {
	logger      *zerolog.Logger
	mtx         sync.RWMutex
	authority   Address            // authority account address
	balances    map[Address]uint64 // all the account balances
	nonces      map[Address]uint64 // all the account nonces ( we use this nonce in transactions to avoid replay attack )
	lastBlock   SignedBlock        // last confirmed block = confirmed block is a block which is validated by different nodes
	genesisHash Hash               // hash of the genesis block
	txs         map[Hash]SignedTransaction
	Pending     *State
}

func NewState(sigGen SignedGenesis, logger *zerolog.Logger) (*State, error) {
	ctx := context.Background()
	hash, err := sigGen.Hash(ctx)
	if err != nil {
		return nil, err
	}

	return &State{
		logger:      logger,
		authority:   sigGen.Gen.Authority,
		balances:    maps.Clone(sigGen.Gen.Balances),
		nonces:      make(map[Address]uint64),
		genesisHash: hash,
		txs:         make(map[Hash]SignedTransaction),
		Pending: &State{
			authority:   sigGen.Gen.Authority,
			balances:    maps.Clone(sigGen.Gen.Balances),
			nonces:      make(map[Address]uint64),
			genesisHash: hash,
			txs:         make(map[Hash]SignedTransaction),
		},
	}, nil
}

func (s *State) Clone() *State {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return &State{
		authority:   s.authority,
		balances:    maps.Clone(s.balances),
		nonces:      maps.Clone(s.nonces),
		lastBlock:   s.lastBlock,
		genesisHash: s.genesisHash,
		txs:         s.txs,
		Pending: &State{
			txs: maps.Clone(s.Pending.txs),
		},
	}
}

func (s *State) Apply(clone *State) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.balances = clone.balances
	s.nonces = clone.nonces
	s.lastBlock = clone.lastBlock
	s.Pending.balances = maps.Clone(s.balances)
	s.Pending.nonces = maps.Clone(s.nonces)
	for _, tx := range clone.lastBlock.Blk.Txs {
		hash, err := tx.Hash(context.Background())
		if err != nil {
			return err
		}
		delete(s.Pending.txs, hash)
	}
	return nil
}

// returns the authotiry account address
func (s *State) Authroity() Address {
	return s.authority
}

// return balance of specific account address
func (s *State) Balance(ctx context.Context, addr Address) (uint64, bool) {
	_, span := otel.Tracer("Balance.Tracer").Start(ctx, "Balance.Span")
	defer span.End()
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	balance, exist := s.balances[addr]
	return balance, exist
}

// return nonce of specific account address
func (s *State) Nonce(addr Address) (uint64, bool) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	nonce, exists := s.nonces[addr]
	return nonce, exists
}

// return the lastblock of the state
func (s *State) LastBlock() *SignedBlock {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return &s.lastBlock
}

/*
will verify transaction and validate the transaction
Verification process will be checking the validity of transaction digital signature
validation porcess will be checking the source account has enough balance and also the nonce of the transaction is correct to avoid replay attacks
ApplyTX will be used on pendingState
*/
func (s *State) ApplyTx(ctx context.Context, stx *SignedTransaction) error {
	ctx, span := otel.Tracer("ApplyTx.Tracer").Start(ctx, "ApplyTx.Span")
	defer span.End()

	s.mtx.Lock()
	defer s.mtx.Unlock()
	valid, err := VerifyTx(ctx, stx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to verify signed transaction before applying it to the state")
		return err
	}
	if !valid {
		err = errors.New("invalid transaction")
		span.RecordError(err)
		span.SetAttributes(attribute.String("Transaction", stx.String()))
		span.SetStatus(codes.Error, "invalid transaction")
		return err
	}
	if stx.Tx.Nonce != s.nonces[stx.Tx.FromAccount] {
		err = errors.New("invalid transaction nonce received")
		span.RecordError(err)
		span.SetAttributes(attribute.String("Transaction", stx.String()))
		span.SetStatus(codes.Error, "invalid transaction nonce received")
		return err
	}
	if stx.Tx.Value > s.balances[stx.Tx.FromAccount] {
		err = errors.New("insufficient account funds")
		span.RecordError(err)
		span.SetAttributes(attribute.String("Transaction", stx.String()))
		span.SetStatus(codes.Error, "insufficient account fund")
		return err
	}
	s.balances[stx.Tx.FromAccount] -= stx.Tx.Value
	s.balances[stx.Tx.ToAccount] += stx.Tx.Value
	s.nonces[stx.Tx.FromAccount]++
	hash, err := stx.Hash(ctx)
	if err != nil {
		return err
	}
	s.txs[hash] = *stx
	return nil
}

// creates a new block
// this process will be getting all pending transactions and sort them based on time then apply ( apply process such as validation and verification ) them from pending queue to the current state transactions
// additionally we will create a new block with all transfered and valid transactions
func (s *State) CreateBlock(ctx context.Context, authority Account) (*SignedBlock, error) {
	ctx, span := otel.Tracer("CreateBlock.Tracer").Start(ctx, "CreateBlock.Span")
	defer span.End()

	pndTxs := make([]SignedTransaction, 0, len(s.Pending.txs))
	for _, tx := range s.Pending.txs {
		pndTxs = append(pndTxs, tx)
	}

	slices.SortFunc(pndTxs, func(a, b SignedTransaction) int {
		if a.Tx.TxTime.Before(b.Tx.TxTime) {
			return -1
		}
		if b.Tx.TxTime.Before(a.Tx.TxTime) {
			return 1
		}
		return 0
	})
	// applying valid transactions to the
	txs := make([]SignedTransaction, 0, len(pndTxs))
	for _, tx := range pndTxs {
		err := s.ApplyTx(ctx, &tx)
		if err != nil {
			s.logger.Error().Err(err).
				Str("transaction", tx.String()).Msg("transaction rejected")
			span.RecordError(err)
			continue
		}
		txs = append(txs, tx)
	}
	if len(txs) == 0 {
		return &SignedBlock{}, fmt.Errorf("empty list of valid pending transactions")
	}
	var parent Hash
	if s.lastBlock.Blk.BlockNum == 0 {
		parent = s.genesisHash
	} else {
		hash, err := s.lastBlock.Hash(ctx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to calculate hash of the parent block")
			return nil, err
		}
		parent = hash
	}
	blk, err := NeWBlock(ctx, parent, txs, s.lastBlock.Blk.BlockNum+1)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create a new block")
		return nil, err
	}
	sigBlk, err := authority.SignBlock(ctx, blk)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to sign the block")
		return nil, err
	}
	return sigBlk, nil
}

/*
Apply block is going apply the newly created or synced block to the current state.
The block signature is gonna be verified first. Then the block would be validated.
The validation process includes checking the blocknumber is right by comparing it to the LastBlock number.
and also it will check the parent block of the new block is right.
Applyblock will be used on pending state
*/
func (s *State) ApplyBlock(ctx context.Context, sBlk *SignedBlock) error {
	ctx, span := otel.Tracer("ApplyBlock.Tracer").Start(ctx, "ApplyBlock.Span")
	defer span.End()

	valid, err := VerifyBlock(ctx, sBlk, s.authority)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to verify signed block before applying it to the state")
		return err
	}
	if !valid {
		err = errors.New("invalid block")
		span.RecordError(err)
		span.SetAttributes(attribute.String("block", sBlk.String()))
		span.SetStatus(codes.Error, "invalid block")
		return err
	}
	if sBlk.Blk.BlockNum != s.lastBlock.Blk.BlockNum+1 {
		err = errors.New("invalid block number")
		span.RecordError(err)
		span.SetAttributes(attribute.String("block", sBlk.String()))
		span.SetStatus(codes.Error, "invalid block number")
		return err
	}

	var parent Hash
	if sBlk.Blk.BlockNum == 1 {
		parent = s.genesisHash
	} else {
		phash, err := s.lastBlock.Hash(ctx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "couldn't calculate the parent block hash for applying the current block to the state")
			return err
		}
		parent = phash
	}
	if parent != sBlk.Blk.ParentBlockHash {
		err = errors.New("invalid parent block hash")
		span.RecordError(err)
		span.SetAttributes(attribute.String("block", sBlk.String()))
		span.SetStatus(codes.Error, "invalid parent block hash")
		return err
	}

	merkleRoot, err := MerkleHash(ctx, sBlk.Blk.Txs, TxHash, TxPairHash)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "couldn't calculate the merkle tree of the block")
		return err
	}
	if sBlk.Blk.MerkleTreeRoot != merkleRoot.Hash {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid merkle root block")
		return err
	}

	for _, tx := range sBlk.Blk.Txs {
		err := s.ApplyTx(ctx, &tx)
		if err != nil {
			return err
		}
	}
	s.lastBlock = *sBlk
	return nil
}

/*
This will clone the current state then applies the block to the cloned state then reapplies the cloned state to the current state.
*/
func (s *State) ApplyBlockToState(ctx context.Context, sBlk *SignedBlock) error {
	clone := s.Clone()
	err := clone.ApplyBlock(ctx, sBlk)
	if err != nil {
		return err
	}
	s.Apply(clone)
	fmt.Printf("=== Block state\n%v", s)
	return nil
}
