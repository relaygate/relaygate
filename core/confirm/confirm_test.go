package confirm

import "testing"

func TestMatch(t *testing.T) {
	ok := []string{"确认", "Confirm", " 确认 ", " Confirm\n"}
	for _, s := range ok {
		if !Match(s) {
			t.Fatalf("Match(%q) = false, want true", s)
		}
	}
	bad := []string{"", "confirm", "CONFIRM", "HOT_APPLY", "PUBLISH_FLEET", "yes", "Confirmed"}
	for _, s := range bad {
		if Match(s) {
			t.Fatalf("Match(%q) = true, want false", s)
		}
	}
}

func TestHint(t *testing.T) {
	if Hint() != "确认 / Confirm" {
		t.Fatalf("Hint()=%q", Hint())
	}
}
