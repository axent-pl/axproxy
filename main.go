package main

import (
	"bytes"
	"log/slog"
	"os"

	mf "github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	_ "github.com/axent-pl/axproxy/modules"
	"github.com/axent-pl/axproxy/proxy"
)

func init() {
	if value, ok := os.LookupEnv("LOG_FORMAT"); ok && value == "json" {
		jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		jsonLogger := slog.New(jsonHandler)
		slog.SetDefault(jsonLogger)
	}
}

func main() {
	data, err := os.ReadFile("assets/config.yaml")
	if err != nil {
		slog.Error("proxy reading configuration failed ", "error", err)
		os.Exit(1)
	}
	proxies, err := mf.DecodeAll[proxy.AuthProxy](bytes.NewReader(data))
	if err != nil {
		slog.Error("proxy initialization failed", "error", err)
		os.Exit(1)
	}
	mods, err := mf.DecodeAll[module.Module](bytes.NewReader(data))
	if err != nil {
		slog.Error("proxy modules initialization failed", "error", err)
		os.Exit(1)
	}
	for _, mod := range mods {
		module.Register(mod)
	}

	for _, p := range proxies {
		go func() {
			slog.Info("started proxy")
			if err := p.ListenAndServe(); err != nil {
				slog.Error("failed proxy", "error", err)
			}
		}()
	}
	select {}
}
