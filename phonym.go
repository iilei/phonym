// Package phonym encodes non-negative integers as pronounceable names and back.
//
// A name has one or more given words and a family name: "manwudoma-durmav".
// The family name carries the release (the spec version the name was made with)
// and the ID's lowest bits, so a full form decodes without any configuration.
// The short form ("muruboganva") is the given name only; decoding it needs the
// release from context.
//
// The mapping is an order-preserving bijection over syllable sequences accepted
// by a finite automaton with phonotactic rules. See SPEC.md for the format.
package phonym

import (
	"fmt"
	"math/big"
)

// MaxBits is the default limit on ID size: Encode rejects larger IDs, and Decode
// rejects names longer than MaxBits letters (twice the longest name any release
// gives a MaxBits-bit ID) before doing any work, then IDs above the limit. The
// limit bounds memory and time for untrusted input; raise it with the MaxBits option.
const MaxBits = 512

// maxBytesPerLetter bounds the raw input before normalising: a UTF-8 rune has at
// most 4 bytes, so a name within the letter limit never needs more.
const maxBytesPerLetter = 4

type (
	// Decoded is the result of Decode.
	Decoded struct {
		// ID is the decoded identifier.
		ID *big.Int
		// Release is the label of the release the name belongs to.
		Release string
		// Short reports whether the input was a short form.
		Short bool
	}

	// Option changes how Encode, EncodeShort and Decode behave.
	Option func(*options)

	options struct {
		maxBits        int
		includeRetired bool
	}
)

// IncludeRetired lets Encode produce names in retired releases. It is meant for
// snapshots and tests: it shows which name an ID had in a retired release.
func IncludeRetired() Option {
	return func(o *options) { o.includeRetired = true }
}

// WithMaxBits replaces the MaxBits limit on ID size (and on name length when decoding).
func WithMaxBits(bits int) Option {
	return func(o *options) { o.maxBits = bits }
}

// resolve applies the options over the defaults.
func resolve(opts []Option) options {
	o := options{maxBits: MaxBits}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// Encode returns the full form of id in a release: lowercase, hyphen-separated,
// family name last.
func Encode(id *big.Int, release string, opts ...Option) (string, error) {
	slot, c, err := encodable(id, release, opts)
	if err != nil {
		return "", err
	}
	hi, lo := new(big.Int).DivMod(id, big.NewInt(Siblings), new(big.Int))
	given, err := encodeGiven(registry[slot].style, c, hi)
	if err != nil {
		return "", err
	}
	return given + "-" + familyName(slot, int(lo.Int64())), nil
}

// EncodeShort returns the short form of id in a release: given name(s) only,
// carrying the whole ID. Decoding a short form needs the release.
func EncodeShort(id *big.Int, release string, opts ...Option) (string, error) {
	slot, c, err := encodable(id, release, opts)
	if err != nil {
		return "", err
	}
	return encodeGiven(registry[slot].style, c, id)
}

// EncodeUint64 is Encode for a uint64 ID.
func EncodeUint64(id uint64, release string, opts ...Option) (string, error) {
	return Encode(new(big.Int).SetUint64(id), release, opts...)
}

// Decode decodes a full or short form, within the MaxBits limit (see WithMaxBits). A full form (its last word is a family
// name) decodes as the release in the name and ignores release; compare the
// returned label if the context expects a specific one. Anything else is a short
// form of release, and is rejected when release is empty.
func Decode(name, release string, opts ...Option) (Decoded, error) {
	o := resolve(opts)
	if len(name) > maxBytesPerLetter*o.maxBits { // cheap early bound, before normalising
		return Decoded{}, fmt.Errorf("%w: input longer than %d bytes", ErrInvalidName, maxBytesPerLetter*o.maxBits)
	}
	words, err := normalize(name)
	if err != nil {
		return Decoded{}, err
	}
	letters := 0
	for _, w := range words {
		letters += len(w)
	}
	if letters > o.maxBits {
		return Decoded{}, fmt.Errorf("%w: %d letters, more than the limit of %d", ErrInvalidName, letters, o.maxBits)
	}
	d, err := decodeWords(name, release, words)
	if err == nil && d.ID.BitLen() > o.maxBits {
		return Decoded{}, fmt.Errorf("%w: id has %d bits, more than the limit of %d", ErrOverflow, d.ID.BitLen(),
			o.maxBits)
	}
	return d, err
}

// decodeWords decodes normalised words as a full or short form.
func decodeWords(name, release string, words []string) (Decoded, error) {
	ref, ferr := familyParse(words[len(words)-1])
	if ferr != nil {
		return decodeShort(name, release, words)
	}
	given := words[:len(words)-1]
	if len(given) == 0 {
		return Decoded{}, fmt.Errorf("%w: %q: family name without a given name", ErrInvalidName, name)
	}
	slot := ref.slot
	if slot >= len(registry) {
		return Decoded{}, fmt.Errorf("%w: slot %d; this registry knows %d", ErrUnsupportedRelease, slot, len(registry))
	}
	if registry[slot].style == nil {
		return Decoded{}, fmt.Errorf("%w: slot %d is a placeholder and never issued names", ErrInvalidName, slot)
	}
	c, err := codeFor(slot)
	if err != nil {
		return Decoded{}, err
	}
	hi, err := decodeGiven(registry[slot].style, c, given)
	if err != nil {
		return Decoded{}, err
	}
	id := hi.Mul(hi, big.NewInt(Siblings)).Add(hi, big.NewInt(int64(ref.lo)))
	return Decoded{ID: id, Release: registry[slot].Label}, nil
}

// decodeShort decodes words that are not a full form as a short form of release.
func decodeShort(name, release string, words []string) (Decoded, error) {
	if release == "" {
		return Decoded{}, fmt.Errorf("%w: %q is not a full form (no family name); a short form needs a release",
			ErrInvalidName, name)
	}
	slot, ok := labelIndex[release]
	if !ok {
		return Decoded{}, fmt.Errorf("%w: %q", ErrUnknownRelease, release)
	}
	c, err := codeFor(slot)
	if err != nil {
		return Decoded{}, err
	}
	id, err := decodeGiven(registry[slot].style, c, words)
	if err != nil {
		return Decoded{}, err
	}
	return Decoded{ID: id, Release: release, Short: true}, nil
}

// encodable resolves a release for encoding and checks the ID and retirement.
func encodable(id *big.Int, release string, opts []Option) (int, *code, error) {
	o := resolve(opts)
	if id == nil || id.Sign() < 0 {
		return 0, nil, fmt.Errorf("%w: ids must be non-negative", ErrOverflow)
	}
	if id.BitLen() > o.maxBits {
		return 0, nil, fmt.Errorf("%w: id has %d bits, more than the limit of %d", ErrOverflow, id.BitLen(), o.maxBits)
	}
	slot, ok := labelIndex[release]
	if !ok {
		return 0, nil, fmt.Errorf("%w: %q", ErrUnknownRelease, release)
	}
	if registry[slot].Retired && !o.includeRetired {
		return 0, nil, fmt.Errorf("%w: %s: its names still decode, but it issues no new ones",
			ErrRetiredRelease, release)
	}
	c, err := codeFor(slot)
	return slot, c, err
}
