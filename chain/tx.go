package chain

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/cybrarymin/btcblockchain/helpers"
	"github.com/dustinxie/ecc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/crypto/sha3"
)

type Hash [32]byte

func NewHash(ctx context.Context, data any) (Hash, error) {
	ctx, span := otel.Tracer("NewHash.Tracer").Start(ctx, "NewHash.Span")
	defer span.End()

	var hash Hash
	dataBytes, err := helpers.JsonMarshaller(ctx, data)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to serialize data to json for creating sha3 hash")
		return Hash{}, err
	}
	nHash := sha3.New256()
	_, err = nHash.Write(dataBytes)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to write json encoded data to the sha3 hasher")
		return Hash{}, err
	}

	return hash, nil
}

// return the hex string of the [32]byte hash
func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

// return the []bytes of the [32]byte hash
func (h Hash) Bytes() []byte {
	return h[:]
}

// convert the [32]byte of hash to []byte of hex
func (h Hash) MarshalText() []byte {
	return []byte(hex.EncodeToString(h[:]))
}

// convert the []byte hex to [32]byte hash
func (h Hash) UnmarshalText(hashHex []byte) error {
	_, err := hex.Decode(h[:], hashHex)
	if err != nil {
		return err
	}
	return nil
}

// convert the hash string to type Hash which is [32]byte
func DecodeHash(ctx context.Context, hashString string) (Hash, error) {
	_, span := otel.Tracer("DecodeHash.Tracer").Start(ctx, "DecodeHash.Span")
	defer span.End()

	hashByte, err := hex.DecodeString(hashString)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to decode the hash hex string")
		return Hash{}, err
	}

	return Hash(hashByte), nil
}

type Transaction struct {
	FromAccount Address   `json:"from_account"`
	ToAccount   Address   `json:"to_account"`
	TxTime      time.Time `json:"time"`
	Nonce       uint64    `json:"nonce"`
	Value       uint64    `json:"value"`
}

func NewTransaction(fromAcc, toAcc Address, nonce, value uint64) *Transaction {
	return &Transaction{
		FromAccount: fromAcc,
		ToAccount:   toAcc,
		TxTime:      time.Now(),
		Nonce:       nonce,
		Value:       value,
	}
}

// this will hash the transaction using sha3 hashing and returns the hash type
func (tx *Transaction) Hash(ctx context.Context) (Hash, error) {
	hash, err := NewHash(ctx, tx)
	if err != nil {
		return Hash{}, err
	}
	return hash, nil
}

type SignedTransaction struct {
	Tx  Transaction
	Sig []byte `json:"sig"`
}

func NewSignedTransaction(tx Transaction, sig []byte) *SignedTransaction {
	return &SignedTransaction{
		Tx:  tx,
		Sig: sig,
	}
}

func (stx *SignedTransaction) Hash(ctx context.Context) (Hash, error) {
	hash, err := NewHash(ctx, stx)
	if err != nil {
		return Hash{}, err
	}
	return hash, nil
}

func (stx *SignedTransaction) String() string {
	hash, _ := stx.Hash(context.Background())
	return fmt.Sprintf("tx %.7s: %.7s -> %.7s %8d %8d", hash, stx.Tx.FromAccount, stx.Tx.ToAccount, stx.Tx.Value, stx.Tx.Nonce)
}

func TxHash(ctx context.Context, tx SignedTransaction) (Hash, error) {
	hash, err := NewHash(ctx, tx)
	if err != nil {
		return Hash{}, err
	}
	return hash, nil
}

// this function will get two transactions hash and combine them together and create a new hash
// this function will be used in merkle tree
func TxPairHash(ctx context.Context, left, right Hash) (Hash, error) {
	var nilHash Hash
	if right == nilHash {
		return left, nil
	}
	hash, err := NewHash(ctx, left.String()+right.String())
	if err != nil {
		return Hash{}, err
	}
	return hash, nil
}

func (acc *Account) SignTx(ctx context.Context, tx *Transaction) (*SignedTransaction, error) {
	ctx, span := otel.Tracer("SignTx.Tracer").Start(ctx, "SignTx.Span")
	defer span.End()

	hash, err := tx.Hash(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to calculate the transaction hash")
		return nil, err
	}

	sig, err := ecc.SignBytes(acc.Priv, hash.Bytes(), ecc.LowerS|ecc.RecID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to calculate digital signrature of the transaction")
		return nil, err
	}
	signedTx := NewSignedTransaction(*tx, sig)
	return signedTx, nil
}

func VerifyTx(ctx context.Context, stx *SignedTransaction) (bool, error) {
	ctx, span := otel.Tracer("VerifyTx.Tracer").Start(ctx, "VerifyTx.Span")
	defer span.End()

	hash, err := stx.Tx.Hash(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to calculate the transaction hash")
		return false, err
	}

	/* IMPORTANT
	What is SignatureRecovery verification method??
	Here we don't use the traditional verification method like old times.
	In traditional way we will get the public key of the user from user or we store it somewhere on blockchain
	Then we calculate the hash of transaction. we use public key to decrypt the Digital signature to retrive hash calculated by client side.
	comapre our hash with signature hash and if they are the same transaction is valid.
	In newer validation method which call it recovery we use ECC and hash of transaction to fetch the public key from digital signature itself
	In this method we leverage mathematical aspects of ECC to fetch public key.
	Benefit is we don't need to store the user's public key anywhere on the blockchain or client doesn't need to send it's public key to us.
	We calculate the transaction Hash then use the encrypted signature and hash to calculate the PublicKey
	Now we have a public key. If we hash this public key it should return us account address of the sender.
	If this transaction has been sent by an attacker then the public key we calculate is different then the account Address that we calculate is gonna be different from one specified so we understand the transaction is not valid */

	userpuBKey, err := ecc.RecoverPubkey("P521", hash.Bytes(), stx.Sig)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to recover public key from digital signature")
		return false, err
	}

	userAddr, err := NewAddress(ctx, userpuBKey)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to calculate the user address to verify sender in transaction")
		return false, err
	}

	return userAddr == stx.Tx.FromAccount, nil
}
