package sharecode

import (
	"errors"
	"testing"
)

func TestDecodeKnownCode(t *testing.T) {
	// Vector from the csgo-sharecode library's README.
	got, err := Decode("CSGO-GADqf-jjyJ8-cSP2r-smZRo-TO2xK")
	if err != nil {
		t.Fatal(err)
	}
	want := Code{MatchID: 3230642215713767580, OutcomeID: 3230647599455273103, TVPort: 55788}
	if got != want {
		t.Fatalf("Decode = %+v, want %+v", got, want)
	}
}

func TestRoundTrip(t *testing.T) {
	for _, c := range []Code{
		{},
		{MatchID: 1, OutcomeID: 2, TVPort: 3},
		{MatchID: 3230642215713767580, OutcomeID: 3230647599455273103, TVPort: 55788},
		{MatchID: ^uint64(0), OutcomeID: ^uint64(0), TVPort: ^uint16(0)},
	} {
		s := c.String()
		got, err := Decode(s)
		if err != nil {
			t.Fatalf("Decode(%q): %v", s, err)
		}
		if got != c {
			t.Errorf("Decode(%q) = %+v, want %+v", s, got, c)
		}
	}
}

func TestNormalize(t *testing.T) {
	const want = "CSGO-GADqf-jjyJ8-cSP2r-smZRo-TO2xK"
	for _, s := range []string{want, "  GADqfjjyJ8cSP2rsmZRoTO2xK ", "CSGOGADqfjjyJ8cSP2rsmZRoTO2xK"} {
		got, err := Normalize(s)
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", s, got, err, want)
		}
	}
}

func TestDecodeInvalid(t *testing.T) {
	for _, s := range []string{
		"",
		"CSGO-GADqf-jjyJ8-cSP2r-smZRo",       // too short
		"CSGO-GADqf-jjyJ8-cSP2r-RJBOM-fH8V0", // 0 is not in the alphabet
		"CSGO-99999-99999-99999-99999-99999", // more than 144 bits
	} {
		if _, err := Decode(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("Decode(%q) err = %v, want ErrInvalid", s, err)
		}
	}
}
