// Package fibonacci computes the Fibonacci sequence.
package fibonacci

import "math/big"

// Sequence returns the first n Fibonacci numbers, starting from 0.
// For n = 0 it returns an empty, non-nil slice. It panics if n is negative.
func Sequence(n int) []*big.Int {
	if n < 0 {
		panic("fibonacci: negative Sequence count")
	}

	seq := make([]*big.Int, n)
	a, b := big.NewInt(0), big.NewInt(1)

	for i := range seq {
		seq[i] = a
		a, b = b, new(big.Int).Add(a, b)
	}

	return seq
}
