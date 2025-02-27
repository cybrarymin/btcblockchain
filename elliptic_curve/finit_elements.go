package elliptic_curve

import (
	"errors"
)

var (
	errModulusMismatch = errors.New("field elements are not at the same set or the modulus primers are different")
	errNoInverseFound  = errors.New("no multiplicative inverse for number zero or non coprime numbers")
)

/*
FieldElement is each element of the Finit Field (Galois Field). Basically it is each number with its modulusPrime in the list or set of numbers
*/
type FieldElement struct {
	modulusPrime uint32 // the prime modulus p
	Value        uint32 // the value of the field element which is between [0...p-1]
}

/*
Initialized the FieldElement object.
*/
func NewFieldElement(modulusPrime uint32, value uint32) *FieldElement {
	return &FieldElement{
		modulusPrime,
		value,
	}
}

/*
Check if two FieldElements are the same
*/
func (fe *FieldElement) EqualTo(otherfe *FieldElement) bool {
	return fe.modulusPrime == otherfe.modulusPrime && fe.Value == otherfe.Value
}

/*
Add function for two FieldElements based on modulus arithmatic.
*/
func (fe *FieldElement) Add(otherfe *FieldElement) (*FieldElement, error) {
	if fe.modulusPrime != otherfe.modulusPrime {
		return nil, errModulusMismatch
	}
	return NewFieldElement(fe.modulusPrime, (fe.Value+otherfe.Value)%fe.modulusPrime), nil
}

/*
Subtraction function for two FieldElements based on modulus arithmatic.
*/
func (fe *FieldElement) Subtract(otherfe *FieldElement) (*FieldElement, error) {
	if fe.modulusPrime != otherfe.modulusPrime {
		return nil, errModulusMismatch
	}
	return NewFieldElement(fe.modulusPrime, (fe.Value-otherfe.Value)%fe.modulusPrime), nil
}

/*
Multiplication function for two FieldElements based on modulus arithmatic.
*/
func (fe *FieldElement) Multiply(otherfe *FieldElement) *FieldElement {
	return NewFieldElement(fe.modulusPrime, (fe.Value*otherfe.Value)%fe.modulusPrime)
}

/*
Division function for two FieldElements based on modulus arithmatic.
*/
func (fe *FieldElement) Divide(otherfe *FieldElement) (*FieldElement, error) {
	if fe.modulusPrime != otherfe.modulusPrime {
		return nil, errModulusMismatch
	}
	return NewFieldElement(fe.modulusPrime, (fe.Value/otherfe.Value)%fe.modulusPrime), nil
}

/*
AdditiveInverse function for two FieldElements based on modulus arithmatic. This function finds the FieldElement which is inverse of the other FieldElement. In another word ( a + b ) % modulusPrime = 0
*/
func (fe *FieldElement) AdditiveInverse() *FieldElement {

	return NewFieldElement(fe.modulusPrime, fe.modulusPrime-fe.Value)
}

/*
MultiplicativeInverse function for two FieldElements based on modulus arithmatic. This function finds the FieldElement which is inverse of the other FieldElement. In another word ( a . a^(-1) ) % modulusPrime = 1
*/
func (fe *FieldElement) MultiplicativeInverse() (*FieldElement, error) {
	if fe.Value == 0 {
		return nil, errNoInverseFound
	}
	g, x, _ := ExtendedGCD(int(fe.Value), int(fe.modulusPrime))
	if g != 1 {
		// No inverse if a and m aren't coprime.
		return nil, errNoInverseFound

	}
	inv := x % int(fe.modulusPrime)
	if inv < 0 {
		inv += int(fe.modulusPrime)
	}
	return NewFieldElement(fe.modulusPrime, uint32(inv)), nil
}

/*
ExtendedGCD calculatest the greates common divisor between two numbers.
g is the gcd, and other variables corresponds to a * x + b * y = g.
*/
func ExtendedGCD(a, b int) (g, x, y int) {
	if b == 0 {
		return a, 1, 0
	}
	g, x1, y1 := ExtendedGCD(b, a%b)
	// Now compute x, y based on recursion.
	x = y1
	y = x1 - (a/b)*y1
	return g, x, y
}
