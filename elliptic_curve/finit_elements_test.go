package elliptic_curve

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtendedGCD(t *testing.T) {
	tests := []struct {
		Name           string
		Input1         int
		Input2         int
		expectedResult int
		expectError    bool
	}{
		{
			Name:           "right greatest common divisor",
			Input1:         29,
			Input2:         3,
			expectedResult: 1,
			expectError:    false,
		},
		{
			Name:           "right greatest common divisor",
			Input1:         14,
			Input2:         21,
			expectedResult: 7,
			expectError:    false,
		},
		{
			Name:           "wrong greatest common divisor",
			Input1:         29,
			Input2:         3,
			expectedResult: 2,
			expectError:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			g, _, _ := ExtendedGCD(tc.Input1, tc.Input2)
			if !tc.expectError {
				assert.Equal(t, tc.expectedResult, g)
			} else {
				assert.NotEqual(t, tc.expectedResult, g)
			}

		})
	}

}

// func TestAdditiveInverse(t *testing.T) {

// }

func TestMultiplicativeInverse(t *testing.T) {
	tests := []struct {
		Name   string
		Input  FieldElement
		Output FieldElement
	}{
		{
			"right multiplicative inverse calculation",
			*NewFieldElement(29, 12),
			*NewFieldElement(29, 17),
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			got, err := tc.Input.MultiplicativeInverse()
			if err != nil {
				assert.NoError(t, err, "MultiplicativeInverse shouldn't return an error")
			}
			fmt.Println(got, tc.Output)
			assert.True(t, got.EqualTo(&tc.Output))
		})
	}
}
