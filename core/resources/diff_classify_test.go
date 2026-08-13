package resources

import "testing"

func TestChangeSummaryClassify(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		summary    ChangeSummary
		wantReload bool
		wantFW     bool
		wantHot    bool
		wantHard   bool
	}{
		{name: "empty", summary: ChangeSummary{}},
		{
			name:    "source allowlist only",
			summary: ChangeSummary{SecurityChanged: []string{"+deny 1.2.3.4/32"}},
			wantFW:  true,
		},
		{
			name:    "security policy nft params firewall only",
			summary: ChangeSummary{SecurityChanged: []string{"security.policies.firewall_new_conn_limit.params.tcp_per_ip 30/second→60/second"}},
			wantFW:  true,
		},
		{
			name:       "envoy defaults only",
			summary:    ChangeSummary{DefaultsChanged: []string{"tcp_idle_timeout 1h→2h"}},
			wantReload: true,
			wantHot:    true,
		},
		{
			name:       "server change",
			summary:    ChangeSummary{UpstreamsChanged: []string{"server-01: address a→b"}},
			wantReload: true,
			wantHot:    true,
		},
		{
			name:       "port change both",
			summary:    ChangeSummary{PortChanges: []string{"forward-a TCP/10001→TCP/10002"}},
			wantReload: true,
			wantFW:     true,
			wantHot:    true,
		},
		{
			name:       "enabled rule added both",
			summary:    ChangeSummary{ForwardsAdded: []string{"forward-a (prod TCP/10001 → server-01, enabled)"}},
			wantReload: true,
			wantFW:     true,
			wantHot:    true,
		},
		{
			name:       "disabled rule added reload only",
			summary:    ChangeSummary{ForwardsAdded: []string{"forward-a (prod TCP/10001 → server-01, disabled)"}},
			wantReload: true,
			wantHot:    true,
		},
		{
			name:       "rule toggled both",
			summary:    ChangeSummary{ForwardsToggled: []string{"forward-a off→on"}},
			wantReload: true,
			wantFW:     true,
			wantHot:    true,
		},
		{
			name: "mixed allowlist and upstream",
			summary: ChangeSummary{
				UpstreamsAdded:    []string{"server-02"},
				SecurityChanged: []string{"+allow 10.0.0.0/8"},
			},
			wantReload: true,
			wantFW:     true,
			wantHot:    true,
		},
		{
			name: "first snapshot summarizeDefaults",
			summary: ChangeSummary{
				DefaultsChanged: []string{"tcp_idle=3600s udp_idle=120s"},
			},
		},
		{
			name: "admin_port hard only",
			summary: ChangeSummary{
				MetaChanged: []string{"admin_port 9901→9902"},
			},
			wantReload: true,
			wantHard:   true,
		},
		{
			name: "envoy_image hard only",
			summary: ChangeSummary{
				MetaChanged: []string{"envoy_image a→b"},
			},
			wantReload: true,
			wantHard:   true,
		},
		{
			name: "server plus admin_port forces hard",
			summary: ChangeSummary{
				UpstreamsChanged: []string{"server-01: address a→b"},
				MetaChanged:    []string{"admin_address 127.0.0.1→0.0.0.0"},
			},
			wantReload: true,
			wantHard:   true,
		},
		{
			name:    "security policy nft disabled firewall only",
			summary: ChangeSummary{SecurityChanged: []string{"security.policies.firewall_new_conn_limit.enabled true→false"}},
			wantFW:  true,
		},
		{
			name:    "security policy allowlist firewall only",
			summary: ChangeSummary{SecurityChanged: []string{"security.policies.allowlist.enabled true→false"}},
			wantFW:  true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.summary.Classify()
			if got.NeedsReload != tc.wantReload || got.NeedsFirewall != tc.wantFW ||
				got.CanHotApply != tc.wantHot || got.NeedsHardReload != tc.wantHard {
				t.Fatalf("Classify() = %+v, want reload=%v firewall=%v hot=%v hard=%v",
					got, tc.wantReload, tc.wantFW, tc.wantHot, tc.wantHard)
			}
		})
	}
}

func TestDiffMetaHardReload(t *testing.T) {
	t.Parallel()
	before := &Resources{
		Meta: Meta{AdminPort: 9901, AdminAddress: "127.0.0.1", EnvoyImage: "envoy:v1"},
	}
	after := &Resources{
		Meta: Meta{AdminPort: 9902, AdminAddress: "127.0.0.1", EnvoyImage: "envoy:v1", GatewayName: "gw"},
	}
	sum := Diff(before, after)
	if len(sum.MetaChanged) != 1 || sum.MetaChanged[0] != "admin_port 9901→9902" {
		t.Fatalf("MetaChanged=%v", sum.MetaChanged)
	}
	plan := sum.Classify()
	if !plan.NeedsHardReload || plan.CanHotApply || !plan.NeedsReload {
		t.Fatalf("Classify()=%+v", plan)
	}

	after2 := &Resources{Meta: before.Meta}
	after2.Meta.GatewayName = "renamed"
	sum2 := Diff(before, after2)
	if len(sum2.MetaChanged) != 0 {
		t.Fatalf("gateway_name should not appear in MetaChanged: %v", sum2.MetaChanged)
	}
}
