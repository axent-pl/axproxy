package main

import (
	"bytes"
	"log/slog"
	"os"
	"strings"

	mf "github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	_ "github.com/axent-pl/axproxy/modules"
	"github.com/axent-pl/axproxy/proxy"
)

func init() {
	format := "text"
	if value, ok := os.LookupEnv("LOG_FORMAT"); ok && value != "" {
		format = value
	}

	level := slog.LevelInfo
	if value, ok := os.LookupEnv("LOG_LEVEL"); ok && value != "" {
		switch strings.ToLower(value) {
		case "debug":
			level = slog.LevelDebug
		case "info":
			level = slog.LevelInfo
		case "warn", "warning":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}

func main() {
	data, err := os.ReadFile("assets/config/config.yaml")
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
