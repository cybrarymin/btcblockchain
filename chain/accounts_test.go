package chain

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewAddress(t *testing.T) {
	pubKey := &ecdsa.PublicKey{
		Curve: elliptic.P521(),
		X:     big.NewInt(7782501172860074401), // random number
		Y:     big.NewInt(6754381619804888445), // random number
	}

	tests := []struct {
		Name           string
		Input          ecdsa.PublicKey
		expectedOutput string
		expectedError  bool
	}{
		{
			Name:           "right address calculation",
			Input:          *pubKey,
			expectedOutput: "06378b696afeed1417cddef21c2804a1f214d1c0140f66f7e164d8938c35257c",
			expectedError:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			addr, err := NewAddress(pubKey)
			switch tc.expectedError {
			case false:
				assert.NoError(t, err, "no error expected in creating a new address")
				assert.Equal(t, tc.expectedOutput, string(addr))
			case true:
				assert.Error(t, err, "expected error but no error returned for new address creation")
			}
		})
	}
}
