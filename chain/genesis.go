package chain

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/cybrarymin/btcblockchain/helpers"
	"github.com/dustinxie/ecc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const genesisfile = "genesis.json"

type Genesis struct {
	Chain     string             `json:"chain"`
	Authority Address            `json:"authority_address"`
	Balances  map[Address]uint64 `json:"balances"`
	Time      time.Time          `json:"time"` // the genesis time of the genesis configuraion
}

func NewGenesis(chainName string, authorityAddress Address, balances map[Address]uint64) *Genesis {
	return &Genesis{
		Chain:     chainName,
		Authority: authorityAddress,
		Balances:  balances,
		Time:      time.Now(),
	}
}

func (g *Genesis) Hash(ctx context.Context) (Hash, error) {
	hash, err := NewHash(ctx, g)
	if err != nil {
		return Hash{}, err
	}
	return hash, nil
}

type SignedGenesis struct {
	Gen Genesis
	Sig []byte
}

func NewSignedGenesis(g Genesis, sig []byte) *SignedGenesis {
	return &SignedGenesis{
		Gen: g,
		Sig: sig,
	}
}

func (sg *SignedGenesis) Hash(ctx context.Context) (Hash, error) {
	hash, err := NewHash(ctx, sg)
	if err != nil {
		return Hash{}, err
	}
	return hash, nil
}

func (ac *Account) SignGenesis(ctx context.Context, gen *Genesis) (*SignedGenesis, error) {
	ctx, span := otel.Tracer("SignGenesis.Tracer").Start(ctx, "SignGenesis.Span")
	defer span.End()

	gHash, err := gen.Hash(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to calculate the hash of genesis block")
		return nil, err
	}

	sigGenesis, err := ecc.SignBytes(ac.Priv, gHash[:], ecc.LowerS|ecc.RecID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to calculate the signature of the genesis block")
		return nil, err
	}
	signedG := NewSignedGenesis(*gen, sigGenesis)
	return signedG, nil
}

func (ac *Account) VerifyGenesis(ctx context.Context, sigGen *SignedGenesis) (bool, error) {
	ctx, span := otel.Tracer("VerifyGenesis.Tracer").Start(ctx, "VerifyGenesis.Span")
	defer span.End()

	hash, err := sigGen.Gen.Hash(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to calculate the hash of genesis block for verification")
		return false, err
	}

	authorityUserPubKey, err := ecc.RecoverPubkey("P521", hash.Bytes(), sigGen.Sig)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed recover public key from digital signature")
		return false, err
	}

	addr, err := NewAddress(ctx, authorityUserPubKey)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to calculate the user address to verify sender in transaction")
		return false, err
	}

	return addr == sigGen.Gen.Authority, nil
}

func (sigGen *SignedGenesis) Persist(ctx context.Context, dirPath string) error {
	sigGenJson, err := helpers.JsonMarshaller(ctx, sigGen)
	if err != nil {
		return err
	}
	err = os.MkdirAll(dirPath, 0700)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(dirPath, genesisfile), sigGenJson, 0700)
	if err != nil {
		return err
	}
	return nil
}

func ReadGenesis(ctx context.Context, dirPath string) (*SignedGenesis, error) {
	signedGenJson, err := os.ReadFile(filepath.Join(dirPath, genesisfile))
	if err != nil {
		if err != io.EOF {
			return nil, err
		}
	}
	sigGen, err := helpers.JsonUnMarshaller[*SignedGenesis](ctx, signedGenJson)
	if err != nil {
		return nil, err
	}
	return sigGen, nil
}
