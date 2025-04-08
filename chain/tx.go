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

/*

func TxHash(tx SigTx) Hash {
  return NewHash(tx)
}

func TxPairHash(l, r Hash) Hash {
  var nilHash Hash
  if r == nilHash {
    return l
  }
  return NewHash(l.String() + r.String())
}

*/

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

	userpuBKey, err := ecc.RecoverPubkey("P521", hash.Bytes(), stx.Sig)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed recover public key from digital signature")
		return false, err
	}

	userAddr, err := NewAddress(ctx, userpuBKey)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to calculate the user address to verify transaction")
		return false, err
	}

	return userAddr == stx.Tx.FromAccount, nil
}
