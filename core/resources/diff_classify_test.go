package resources

import "testing"

func TestChangeSummaryClassify(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		summary   ChangeSummary
		wantReload bool
		wantFW     bool
	}{
		{name: "empty", summary: ChangeSummary{}},
		{
			name:       "acl only",
			summary:    ChangeSummary{ACLChanged: []string{"+deny 1.2.3.4/32"}},
			wantFW:     true,
		},
		{
			name:       "nftables defaults only",
			summary:    ChangeSummary{DefaultsChanged: []string{"nftables.tcp_new_conn_per_ip 30/second→60/second"}},
			wantFW:     true,
		},
		{
			name:       "envoy defaults only",
			summary:    ChangeSummary{DefaultsChanged: []string{"tcp_idle_timeout 1h→2h"}},
			wantReload: true,
		},
		{
			name:       "server change",
			summary:    ChangeSummary{ServersChanged: []string{"server-01: address a→b"}},
			wantReload: true,
		},
		{
			name:       "port change both",
			summary:    ChangeSummary{PortChanges: []string{"forward-a TCP/10001→TCP/10002"}},
			wantReload: true,
			wantFW:     true,
		},
		{
			name:       "enabled rule added both",
			summary:    ChangeSummary{RulesAdded: []string{"forward-a (prod TCP/10001 → server-01, enabled)"}},
			wantReload: true,
			wantFW:     true,
		},
		{
			name:       "disabled rule added reload only",
			summary:    ChangeSummary{RulesAdded: []string{"forward-a (prod TCP/10001 → server-01, disabled)"}},
			wantReload: true,
		},
		{
			name:       "rule toggled both",
			summary:    ChangeSummary{RulesToggled: []string{"forward-a off→on"}},
			wantReload: true,
			wantFW:     true,
		},
		{
			name: "mixed acl and server",
			summary: ChangeSummary{
				ServersAdded: []string{"server-02"},
				ACLChanged:   []string{"+allow 10.0.0.0/8"},
			},
			wantReload: true,
			wantFW:     true,
		},
		{
			name: "first snapshot summarizeDefaults",
			summary: ChangeSummary{
				DefaultsChanged: []string{"tcp_rl=100/20 max_conn=1000 nftables.tcp=30/second nftables.udp=500/second"},
			},
			wantReload: true,
			wantFW:     true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.summary.Classify()
			if got.NeedsReload != tc.wantReload || got.NeedsFirewall != tc.wantFW {
				t.Fatalf("Classify() = %+v, want reload=%v firewall=%v", got, tc.wantReload, tc.wantFW)
			}
		})
	}
}
