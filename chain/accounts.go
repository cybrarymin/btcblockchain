package chain

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"

	"github.com/cybrarymin/btcblockchain/helpers"
	"github.com/dustinxie/ecc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/sha3"
)

const (
	pbkdSalt      = "chainSalt"
	encKeyLen     = 32
	hashIteration = 256
)

type P521PublicKey struct {
	Curve string   `json:"curve"`
	X     *big.Int `json:"x"`
	Y     *big.Int `json:"y"`
}

/*
Create a P521PublicKey from ecdsa.PublicKey. P521PublicKey type has been created to make encoding of the key easier by providing string type as a curve rather than elliptic.Curve type
*/
func NewP521PublicKey(pub *ecdsa.PublicKey) *P521PublicKey {
	return &P521PublicKey{
		Curve: "P-521k1",
		X:     pub.X,
		Y:     pub.Y,
	}
}

type P521PrivateKey struct {
	*P521PublicKey
	D *big.Int
}

/*
Create a P521PrivateKey from ecdsa.PrivateKey.
*/
func NewP521PrivateKey(prv *ecdsa.PrivateKey) *P521PrivateKey {
	return &P521PrivateKey{
		NewP521PublicKey(&prv.PublicKey),
		prv.D,
	}
}

/*
This method will provide the ecdsa.PublicKey type of the custom type P521PrivateKey.
*/
func (k *P521PrivateKey) Public() *ecdsa.PublicKey {
	return &ecdsa.PublicKey{
		Curve: elliptic.P521(),
		X:     k.X,
		Y:     k.Y,
	}
}

/*
This method will provide the ecdsa.PrivateKey type of the custom type P521PrivateKey.
*/
func (k *P521PrivateKey) Private() *ecdsa.PrivateKey {
	return &ecdsa.PrivateKey{
		PublicKey: *k.Public(),
		D:         k.D,
	}
}

/*
Address type
*/
type Address string

/*
Generate a new address for user's account from sha3 sum of the public key encoded format.
*/
func NewAddress(ctx context.Context, pub *ecdsa.PublicKey) (Address, error) {
	_, span := otel.Tracer("NewAddress.Tracer").Start(ctx, "NewAddress.Span")
	defer span.End()

	newPub := NewP521PublicKey(pub)
	jPub, err := helpers.JsonMarshaller(ctx, newPub)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to serialize the public key into json")
		return "", err
	}
	jPub = bytes.TrimSuffix(jPub, []byte("\n"))

	hash := sha3.Sum256(jPub)

	return Address(hex.EncodeToString(hash[:encKeyLen])), nil
}

/*
Account type which has account address and privateKey as attributes
*/
type Account struct {
	Priv *ecdsa.PrivateKey
	Addr Address
}

/*
Create a new user account by generating a key pair
*/
func NewAccount(ctx context.Context) (*Account, error) {
	_, span := otel.Tracer("NewAccount.Tracer").Start(ctx, "NewAccount.Span")
	defer span.End()

	privKey, err := ecdsa.GenerateKey(ecc.P521(), rand.Reader)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to generate an ecdsa key pair")
		return nil, err
	}
	addr, err := NewAddress(ctx, &privKey.PublicKey)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to generate a new account address for the user")
		return nil, err
	}
	return &Account{
		Priv: privKey,
		Addr: addr,
	}, nil
}

/*
Encrypting the account's private key using the AES-256-GCM with user passphrase. encodedKey is a json encoding of the account's private key and passphrase is the user created password
*/
func encryptKeyWithPass(ctx context.Context, encodedPrivKey []byte, passphrase string) ([]byte, error) {
	_, span := otel.Tracer("encryptKeyWithPass.Tracer").Start(ctx, "encryptKeyWithPass.Span")
	defer span.End()

	salt := make([]byte, encKeyLen)
	var encryptedKeyWithSalt []byte
	// Reading a random salt
	_, err := rand.Read(salt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read a random salt for the hashing the user private key")
		return nil, err
	}

	// Using password based key derivation function to get user defined passphrase and turn that to a hash with encKeyLen byte length ( 256 bit ) which make it compatible with AES-256 which needs 256bit key
	// In this method we won't need to force user to provide a encKeyLen byte length password anymore.
	// hashIteration is 256 which is number of times hashing will occur.
	// Important Salt will be attached to the begining of the key
	dk := pbkdf2.Key([]byte(passphrase), salt, hashIteration, encKeyLen, sha1.New)

	block, err := aes.NewCipher(dk)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to initialize new aes cipher")
		return nil, err
	}

	// Use GCM mode of aes for encryption
	gcmCipher, err := cipher.NewGCM(block)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create a GCM mode aes cipher")
		return nil, err
	}

	// Create a nonce. Nonce should be based on GCM compatibility.
	// Nonce: stands for number used once. It is a unique number used to avoid replay attacks.
	// for aes encryption this unique number will be prepended to ciphertext.
	// we can store nonce in a local store if we want but cause the replay attack is not going be effective in this logic we won't do this
	nonce := make([]byte, gcmCipher.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create a nonce for aes cipher")
		return nil, err
	}

	// Encrypt the data
	// Add the salt at the end of cipher bytes to be able to read the salt for decryption.
	encryptedKeyWithSalt = gcmCipher.Seal(nonce, nonce, encodedPrivKey, nil)
	encryptedKeyWithSalt = append(encryptedKeyWithSalt, salt...)

	return encryptedKeyWithSalt, nil
}

/*
Decrypting the account's private key using the AES-256-GCM with user passphrase. accountPath is the private key file and passphrase is the user password
*/
func decryptKeyWithPass(ctx context.Context, accountPath string, passphrase string) ([]byte, error) {
	_, span := otel.Tracer("decryptKeyWithPass.Tracer").Start(ctx, "decryptKeyWithPass.Span")
	defer span.End()

	encryptedKeyWithSalt, err := os.ReadFile(accountPath)
	if err != nil {
		if err != io.EOF {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to read the user encrypted private key file")
			return nil, err
		}
	}

	// Fetching salt from ciphertext and recreate the key of aes-256 using user's passphrase
	salt, encryptedKey := encryptedKeyWithSalt[len(encryptedKeyWithSalt)-32:], encryptedKeyWithSalt[:len(encryptedKeyWithSalt)-32]

	fmt.Println("encryptedKeyWithSalt in decryption function", encryptedKeyWithSalt) // TSHOOT
	fmt.Println("Salt in decryption function", salt)                                 // TSHOOT
	dk := pbkdf2.Key([]byte(passphrase), salt, hashIteration, encKeyLen, sha1.New)

	block, err := aes.NewCipher(dk)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to initialize new aes cipher")
		return nil, err
	}

	//Create a new GCM
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create GMC mode aes cipher")
		return nil, err
	}
	//Get the nonce size
	nonceSize := aesGCM.NonceSize()

	//Extract the nonce from the encrypted data
	nonce, ciphertext := encryptedKey[:nonceSize], encryptedKey[nonceSize:]

	//Decrypt the data
	encodedKey, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to decrypt the provided cipher text")
		return nil, err
	}
	return encodedKey, nil
}

/*
This function will encode the private key of the user account, then encrypts it using the encryptKeyWithPass function and stores it in a file.
*/
func (a *Account) Persist(ctx context.Context, directoryPath string, pass string) error {
	ctx, span := otel.Tracer("Persist.Tracer").Start(ctx, "Persist.Span")
	defer span.End()

	jPriv, err := helpers.JsonMarshaller(ctx, NewP521PrivateKey(a.Priv))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to serialize the private key into json")
		return err
	}

	encryptedKey, err := encryptKeyWithPass(ctx, jPriv, pass)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to encrypt the user's private key using it's passphrase")
		return err
	}

	err = os.MkdirAll(directoryPath, 0700)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create a directory for storing the user's encrypted private key")
		return err
	}

	err = os.WriteFile(filepath.Join(directoryPath, string(a.Addr)), encryptedKey, 0500)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to persist user's encrypted private key in file")
		return err
	}
	return nil
}

func ReadAccount(ctx context.Context, accountPath string, passphrase string) (*Account, error) {
	ctx, span := otel.Tracer("ReadAccount.Tracer").Start(ctx, "ReadAccount.Span")
	defer span.End()

	encodedKey, err := decryptKeyWithPass(ctx, accountPath, passphrase)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to decrypt the user's private key using provided passphrase")
		return nil, err
	}

	privKey, err := helpers.JsonUnMarshaller[P521PrivateKey](ctx, encodedKey)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to deserialize the user's privatekey from json")
		return nil, err
	}

	accAddr, err := NewAddress(ctx, privKey.Public())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to recreate the user's account address")
		return nil, err
	}

	return &Account{
		Priv: privKey.Private(),
		Addr: accAddr,
	}, nil
}
