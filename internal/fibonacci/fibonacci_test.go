package fibonacci_test

import (
	"fmt"
	"math/big"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Mattiaslndstrm/fibonacci-service/internal/fibonacci"
)

// Expected values are from OEIS A000045, https://oeis.org/A000045/b000045.txt.
const (
	fib99  = "218922995834555169026"
	fib999 = "2686381002448535938614672720214292396761660931898695234012317599761798" +
		"1700247881689338369654483356564191827856161443356312976673642210350324" +
		"634850410377680367334151172899169723197082763985615764450078474174626"
)

func TestSequenceReturnsTheFirstNNumbersStartingFromZero(t *testing.T) {
	tests := []struct {
		name string
		want []string
		n    int
	}{
		{name: "one number", n: 1, want: []string{"0"}},
		{name: "two numbers", n: 2, want: []string{"0", "1"}},
		{name: "the PDF's example", n: 5, want: []string{"0", "1", "1", "2", "3"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, toStrings(fibonacci.Sequence(tt.n)))
		})
	}
}

func TestSequenceIsExactBeyondSixtyFourBits(t *testing.T) {
	tests := []struct {
		wantLast string
		n        int
	}{
		{n: 100, wantLast: fib99},
		{n: 1000, wantLast: fib999},
	}

	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.n), func(t *testing.T) {
			got := fibonacci.Sequence(tt.n)
			require.Len(t, got, tt.n)
			assert.Equal(t, tt.wantLast, got[tt.n-1].String())
		})
	}
}

func TestSequenceZeroIsEmptyAndNotNil(t *testing.T) {
	got := fibonacci.Sequence(0)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestSequencePanicsOnNegativeN(t *testing.T) {
	assert.PanicsWithValue(t, "fibonacci: negative Sequence count", func() { fibonacci.Sequence(-1) })
}

func ExampleSequence() {
	fmt.Println(fibonacci.Sequence(5))
	// Output: [0 1 1 2 3]
}

func toStrings(values []*big.Int) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = v.String()
	}

	return out
}
