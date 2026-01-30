package proxy

import (
	"fmt"
	"log/slog"

	"github.com/axent-pl/axproxy/manifest"
	"gopkg.in/yaml.v3"
)

var _ manifest.KindHandler[AuthProxy] = ProxyHandler{}

type AuthProxyV1 struct {
	manifest.TypeMeta `yaml:",inline"`
	Metadata          manifest.ObjectMeta `yaml:"metadata"`
	Spec              AuthProxy           `yaml:"spec"`
}

type ProxyHandler struct{}

func (h ProxyHandler) Kind() string { return "AuthProxy" }

func (h ProxyHandler) Unmarshal(apiVersion string, rawYAML []byte) (AuthProxy, error) {
	switch apiVersion {
	case "v1":
		var obj AuthProxyV1
		if err := yaml.Unmarshal(rawYAML, &obj); err != nil {
			return AuthProxy{}, err
		}
		return obj.Spec, nil
	default:
		return AuthProxy{}, fmt.Errorf("unsupported apiVersion %q", apiVersion)
	}
}

func init() {
	if err := manifest.RegisterHandler(&ProxyHandler{}); err != nil {
		slog.Error("init ProxyHandler", "error", err)
	}
}
