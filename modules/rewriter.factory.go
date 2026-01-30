package modules

import (
	"fmt"
	"log/slog"

	"github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	"gopkg.in/yaml.v3"
)

type RewriterModuleV1 struct {
	manifest.TypeMeta `yaml:",inline"`
	Metadata          manifest.ObjectMeta `yaml:"metadata"`
	Spec              RewriterModule      `yaml:"spec"`
}

type RewriterHandler struct{}

func (RewriterHandler) Kind() string { return KIND_REWRITER }

func (RewriterHandler) Unmarshal(apiVersion string, rawYAML []byte) (module.Module, error) {
	switch apiVersion {
	case "v1":
		var obj RewriterModuleV1
		if err := yaml.Unmarshal(rawYAML, &obj); err != nil {
			return &RewriterModule{}, err
		}
		obj.Spec.Metadata = obj.Metadata
		return &obj.Spec, nil
	default:
		return &RewriterModule{}, fmt.Errorf("unsupported apiVersion %q", apiVersion)
	}
}

func init() {
	if err := manifest.RegisterHandler(&RewriterHandler{}); err != nil {
		slog.Error("init RewriterHandler", "error", err)
	}
}
