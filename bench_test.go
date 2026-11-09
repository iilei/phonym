package phonym_test

import (
	"math/big"
	"testing"

	"github.com/iilei/phonym"
)

// benchIDs are a typical 64-bit ID and the largest ID within the default limit.
var benchIDs = []struct {
	name string
	id   *big.Int
}{
	{"64bit", new(big.Int).SetUint64(0xdeadbeefcafe1234)},
	{"512bit", new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), phonym.MaxBits), big.NewInt(1))},
}

func BenchmarkEncode(b *testing.B) {
	for _, tc := range benchIDs {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				if _, err := phonym.Encode(tc.id, softL); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDecode(b *testing.B) {
	for _, tc := range benchIDs {
		name, err := phonym.Encode(tc.id, softL)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				if _, err := phonym.Decode(name, ""); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
