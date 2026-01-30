package modules

import (
	"fmt"
	"log/slog"

	"github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	"gopkg.in/yaml.v3"
)

type SessionModuleV1 struct {
	manifest.TypeMeta `yaml:",inline"`
	Metadata          manifest.ObjectMeta `yaml:"metadata"`
	Spec              SessionModule       `yaml:"spec"`
}

type SessionHandler struct{}

func (SessionHandler) Kind() string { return KIND_SESSION }

func (SessionHandler) Unmarshal(apiVersion string, rawYAML []byte) (module.Module, error) {
	switch apiVersion {
	case "v1":
		var obj SessionModuleV1
		if err := yaml.Unmarshal(rawYAML, &obj); err != nil {
			return &SessionModule{}, err
		}
		obj.Spec.Metadata = obj.Metadata
		return &obj.Spec, nil
	default:
		return &SessionModule{}, fmt.Errorf("unsupported apiVersion %q", apiVersion)
	}
}

func init() {
	if err := manifest.RegisterHandler(&SessionHandler{}); err != nil {
		slog.Error("init SessionHandler", "error", err)
	}
}
