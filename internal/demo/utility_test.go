package demo

import "testing"

func TestUtilityHealthDamageDoesNotSubstituteOverkill(t *testing.T) {
	for _, tt := range []struct {
		taken, reported int
		known           bool
		want            int
	}{
		{5, 100, true, 5}, {0, 0, true, 0}, {0, 100, false, 0}, {-1, 2, false, 0},
	} {
		got := utilityHealthDamage(tt.taken, tt.reported)
		if (got != nil) != tt.known || (got != nil && *got != tt.want) {
			t.Fatalf("damage(%d,%d) = %v", tt.taken, tt.reported, got)
		}
	}
}
