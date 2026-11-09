package phonym

import "errors"

// Errors returned by phonym. Every error wraps exactly one of these, so callers
// can tell them apart with errors.Is.
var (
	// ErrInvalidName: the input is not a valid name (bad characters, wrong shape, no parse).
	ErrInvalidName = errors.New("invalid name")
	// ErrUnsupportedFormat: the last word is not a generation-1 family name.
	ErrUnsupportedFormat = errors.New("unsupported format")
	// ErrUnsupportedRelease: the name is from a release this registry doesn't know yet.
	ErrUnsupportedRelease = errors.New("unsupported release")
	// ErrUnknownRelease: a release label that isn't in the registry.
	ErrUnknownRelease = errors.New("unknown release")
	// ErrRetiredRelease: the release issues no new names (its names still decode).
	ErrRetiredRelease = errors.New("retired release")
	// ErrOverflow: the ID is negative or beyond a fixed-shape release's capacity.
	ErrOverflow = errors.New("id out of range")
)
