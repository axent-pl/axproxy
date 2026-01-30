package modules

import (
	"fmt"
	"log/slog"

	"github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	"gopkg.in/yaml.v3"
)

type CustomHeadersModuleV1 struct {
	manifest.TypeMeta `yaml:",inline"`
	Metadata          manifest.ObjectMeta `yaml:"metadata"`
	Spec              CustomHeadersModule `yaml:"spec"`
}

type CustomHeadersHandler struct{}

func (CustomHeadersHandler) Kind() string { return KIND_CUSTOMHEADERS }

func (CustomHeadersHandler) Unmarshal(apiVersion string, rawYAML []byte) (module.Module, error) {
	switch apiVersion {
	case "v1":
		var obj CustomHeadersModuleV1
		if err := yaml.Unmarshal(rawYAML, &obj); err != nil {
			return &CustomHeadersModule{}, err
		}
		obj.Spec.Metadata = obj.Metadata
		return &obj.Spec, nil
	default:
		return &CustomHeadersModule{}, fmt.Errorf("unsupported apiVersion %q", apiVersion)
	}
}

func init() {
	if err := manifest.RegisterHandler(&CustomHeadersHandler{}); err != nil {
		slog.Error("init CustomHeadersHandler", "error", err)
	}
}
