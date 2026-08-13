package config

import "testing"

func TestHostSecurityAutoApplyDefaults(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		panel string
		auto  string
		want  bool
	}{
		{"node default on", "0", "", true},
		{"control default off", "1", "", false},
		{"control empty panel treated off", "", "", false},
		{"explicit on on control", "1", "1", true},
		{"explicit off on node", "0", "0", false},
		{"explicit true", "1", "true", true},
		{"explicit false", "0", "false", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := Env{PanelEnabled: tc.panel, SecurityAutoApply: tc.auto}
			if got := e.HostSecurityAutoApply(); got != tc.want {
				t.Fatalf("HostSecurityAutoApply()=%v want %v", got, tc.want)
			}
		})
	}
}
