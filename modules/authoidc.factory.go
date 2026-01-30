package modules

import (
	"fmt"
	"log/slog"

	"github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	"gopkg.in/yaml.v3"
)

type AuthOIDCModuleV1 struct {
	manifest.TypeMeta `yaml:",inline"`
	Metadata          manifest.ObjectMeta `yaml:"metadata"`
	Spec              AuthOIDCModule      `yaml:"spec"`
}

// Manifest handler

type AuthOIDCHandler struct{}

func (AuthOIDCHandler) Kind() string { return KIND_AUTHOIDC }

func (AuthOIDCHandler) Unmarshal(apiVersion string, rawYAML []byte) (module.Module, error) {
	switch apiVersion {
	case "v1":
		var obj AuthOIDCModuleV1
		if err := yaml.Unmarshal(rawYAML, &obj); err != nil {
			return &AuthOIDCModule{}, err
		}
		obj.Spec.Metadata = obj.Metadata
		return &obj.Spec, nil
	default:
		return &AuthOIDCModule{}, fmt.Errorf("unsupported apiVersion %q", apiVersion)
	}
}

func init() {
	if err := manifest.RegisterHandler(&AuthOIDCHandler{}); err != nil {
		slog.Error("init AuthOIDCHandler", "error", err)
	}
}
