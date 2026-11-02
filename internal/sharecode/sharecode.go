// Package sharecode encodes and decodes CS2 match sharing codes
// (CSGO-xxxxx-xxxxx-xxxxx-xxxxx-xxxxx).
//
// A code packs three numbers the Game Coordinator needs to find a match's
// demo: the match ID, the outcome (reservation) ID and the TV port. They are
// laid out little-endian in 18 bytes, read as one big-endian integer and
// written in base 57, least significant digit first.
package sharecode

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// alphabet leaves out characters that are easy to mix up (I, l, 0, 1, g).
const alphabet = "ABCDEFGHJKLMNOPQRSTUVWXYZabcdefhijkmnopqrstuvwxyz23456789"

const (
	prefix = "CSGO-"
	digits = 25
	size   = 18
)

// ErrInvalid reports a string that is not a share code.
var ErrInvalid = errors.New("sharecode: invalid code")

// Code is a decoded share code.
type Code struct {
	MatchID   uint64
	OutcomeID uint64
	TVPort    uint16
}

// Decode parses a share code. It accepts the code with or without the
// CSGO- prefix and dashes.
func Decode(s string) (Code, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "-", "")
	// C, S, G and O are digits too, so the prefix is only cut from a code
	// that is longer than the 25 digits.
	if len(s) == len(prefix)-1+digits {
		s = strings.TrimPrefix(s, prefix[:len(prefix)-1])
	}
	if len(s) != digits {
		return Code{}, ErrInvalid
	}
	n, base := new(big.Int), big.NewInt(int64(len(alphabet)))
	for i := digits - 1; i >= 0; i-- {
		d := strings.IndexByte(alphabet, s[i])
		if d < 0 {
			return Code{}, ErrInvalid
		}
		n.Mul(n, base).Add(n, big.NewInt(int64(d)))
	}
	if n.BitLen() > size*8 {
		return Code{}, ErrInvalid
	}
	var b [size]byte
	n.FillBytes(b[:])
	return Code{
		MatchID:   binary.LittleEndian.Uint64(b[0:8]),
		OutcomeID: binary.LittleEndian.Uint64(b[8:16]),
		TVPort:    binary.LittleEndian.Uint16(b[16:18]),
	}, nil
}

// String encodes the code in its CSGO-xxxxx-... form.
func (c Code) String() string {
	var b [size]byte
	binary.LittleEndian.PutUint64(b[0:8], c.MatchID)
	binary.LittleEndian.PutUint64(b[8:16], c.OutcomeID)
	binary.LittleEndian.PutUint16(b[16:18], c.TVPort)
	n, base, d := new(big.Int).SetBytes(b[:]), big.NewInt(int64(len(alphabet))), new(big.Int)
	var sb strings.Builder
	sb.WriteString(prefix)
	for i := range digits {
		if i > 0 && i%5 == 0 {
			sb.WriteByte('-')
		}
		n.DivMod(n, base, d)
		sb.WriteByte(alphabet[d.Int64()])
	}
	return sb.String()
}

// Normalize returns the canonical form of a share code, or an error.
func Normalize(s string) (string, error) {
	c, err := Decode(s)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrInvalid, s)
	}
	return c.String(), nil
}
