// Package xds hosts the in-process Aggregated Discovery Service (ADS) for Envoy.
// Envoy connects only to loopback ADS; HotApply publishes CDS+LDS without docker restart.
// With XDS_ENABLED=0, ops never starts this server.
package xds
