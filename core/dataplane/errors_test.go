package dataplane

import (
	"errors"
	"strings"
	"testing"
)

func TestUserFacingErrorACK(t *testing.T) {
	t.Parallel()
	got := UserFacingError(errors.New("xds: ACK timeout after 15s (version 3)（已回滚上一快照）"))
	if !strings.Contains(got, "未在时限内确认") {
		t.Fatalf("got %q", got)
	}
	if strings.HasPrefix(got, "xds:") {
		t.Fatalf("should not keep raw xds prefix as primary message: %q", got)
	}
}

func TestUserFacingErrorStandby(t *testing.T) {
	t.Parallel()
	got := UserFacingError(errors.New("standby 节点只读，禁止写操作"))
	if !strings.Contains(got, "主控") {
		t.Fatalf("got %q", got)
	}
}
