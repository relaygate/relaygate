// Package xds hosts the in-process Aggregated Discovery Service (ADS) for Envoy.
//
// Design (docs/hot-update-xds.md, docs/fleet-scale-control-plane.md):
//   - Envoy connects only to loopback ADS (127.0.0.1:XDS_PORT); never remote Istiod.
//   - Management Primary may embed ADS in Panel; Secondary uses relaygate xds serve|apply
//     (D13) — do NOT run a full Panel UI/API on every fleet node.
//   - HotApply publishes CDS+LDS snapshots without docker restart; HardReload remains
//     for bootstrap / image / admin meta changes.
//
// With XDS_ENABLED=0, ops never starts this server.
package xds
