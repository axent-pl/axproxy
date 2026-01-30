package modules

import (
	"fmt"
	"log/slog"

	"github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	"gopkg.in/yaml.v3"
)

type AuditModuleV1 struct {
	manifest.TypeMeta `yaml:",inline"`
	Metadata          manifest.ObjectMeta `yaml:"metadata"`
	Spec              AuditModule         `yaml:"spec"`
}

type AuditHandler struct{}

func (AuditHandler) Kind() string { return KIND_AUDIT }

func (AuditHandler) Unmarshal(apiVersion string, rawYAML []byte) (module.Module, error) {
	switch apiVersion {
	case "v1":
		var obj AuditModuleV1
		if err := yaml.Unmarshal(rawYAML, &obj); err != nil {
			return &AuditModule{}, err
		}
		obj.Spec.Metadata = obj.Metadata
		return &obj.Spec, nil
	default:
		return &AuditModule{}, fmt.Errorf("unsupported apiVersion %q", apiVersion)
	}
}

func init() {
	if err := manifest.RegisterHandler(&AuditHandler{}); err != nil {
		slog.Error("init AuditHandler", "error", err)
	}
}
