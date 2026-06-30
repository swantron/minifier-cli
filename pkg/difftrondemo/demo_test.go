package difftrondemo

import "testing"

func TestCovered(t *testing.T) {
	if Covered(2, 3) != 5 {
		t.Fatalf("Covered(2,3) = %d, want 5", Covered(2, 3))
	}
}
