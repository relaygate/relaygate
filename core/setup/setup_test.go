package setup

import (
	"testing"
)

func TestRandomPasswordLength(t *testing.T) {
	pw, err := randomPassword()
	if err != nil {
		t.Fatal(err)
	}
	if len(pw) != 16 {
		t.Fatalf("randomPassword length = %d, want 16", len(pw))
	}
	for _, r := range pw {
		ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if !ok {
			t.Fatalf("unexpected char %q in password %q", r, pw)
		}
	}
}
