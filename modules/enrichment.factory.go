package modules

import (
	"fmt"
	"log/slog"

	"github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	"gopkg.in/yaml.v3"
)

type EnrichmentModuleV1 struct {
	manifest.TypeMeta `yaml:",inline"`
	Metadata          manifest.ObjectMeta `yaml:"metadata"`
	Spec              EnrichmentModule    `yaml:"spec"`
}

type EnrichmentHandler struct{}

func (EnrichmentHandler) Kind() string { return KIND_ENRICHMENT }

func (EnrichmentHandler) Unmarshal(apiVersion string, rawYAML []byte) (module.Module, error) {
	switch apiVersion {
	case "v1":
		var obj EnrichmentModuleV1
		if err := yaml.Unmarshal(rawYAML, &obj); err != nil {
			return &EnrichmentModule{}, err
		}
		obj.Spec.Metadata = obj.Metadata
		if err := obj.Spec.Start(); err != nil {
			return &EnrichmentModule{}, err
		}
		return &obj.Spec, nil
	default:
		return &EnrichmentModule{}, fmt.Errorf("unsupported apiVersion %q", apiVersion)
	}
}

func init() {
	if err := manifest.RegisterHandler(&EnrichmentHandler{}); err != nil {
		slog.Error("init EnrichmentHandler", "error", err)
	}
}
