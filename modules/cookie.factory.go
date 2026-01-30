package modules

import (
	"fmt"
	"log/slog"

	"github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	"gopkg.in/yaml.v3"
)

type CookieModuleV1 struct {
	manifest.TypeMeta `yaml:",inline"`
	Metadata          manifest.ObjectMeta `yaml:"metadata"`
	Spec              CookieModule        `yaml:"spec"`
}

type CookieHandler struct{}

func (CookieHandler) Kind() string { return KIND_COOKIE }

func (CookieHandler) Unmarshal(apiVersion string, rawYAML []byte) (module.Module, error) {
	switch apiVersion {
	case "v1":
		var obj CookieModuleV1
		if err := yaml.Unmarshal(rawYAML, &obj); err != nil {
			return &CookieModule{}, err
		}
		obj.Spec.Metadata = obj.Metadata
		return &obj.Spec, nil
	default:
		return &CookieModule{}, fmt.Errorf("unsupported apiVersion %q", apiVersion)
	}
}

func init() {
	if err := manifest.RegisterHandler(&CookieHandler{}); err != nil {
		slog.Error("init CookieHandler", "error", err)
	}
}
