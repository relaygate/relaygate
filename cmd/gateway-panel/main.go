package main

import (
	"log"
	"os"

	"github.com/robot/proxy/internal/panel"
)

func main() {
	cfg := panel.Config{
		Root:          env("PANEL_ROOT", ""),
		Bind:          env("PANEL_BIND", "127.0.0.1:8080"),
		AdminPassword: os.Getenv("PANEL_ADMIN_PASSWORD"),
		EnvoyAdminURL: env("ENVOY_ADMIN_URL", "http://127.0.0.1:9901"),
		PrometheusURL: env("PROMETHEUS_URL", "http://127.0.0.1:9090"),
	}
	srv, err := panel.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(srv.ListenAndServe())
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
