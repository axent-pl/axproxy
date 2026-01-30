package manifest

import (
	"fmt"
	"io"
	"reflect"

	"gopkg.in/yaml.v3"
)

func DecodeOne[K any](r io.Reader) (K, error) {
	var nilObj K
	objs, err := DecodeAll[K](r)
	if err != nil {
		return nilObj, err
	}
	if len(objs) != 1 {
		return nilObj, fmt.Errorf("unsupported number of manifests: want 1, got %d", len(objs))
	}
	return objs[0], nil
}

func DecodeAll[K any](r io.Reader) ([]K, error) {
	dec := yaml.NewDecoder(r)
	var out []K
	kType := reflect.TypeOf((*K)(nil)).Elem()

	for {
		var node yaml.Node
		err := dec.Decode(&node)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode yaml: %w", err)
		}
		if node.Kind == 0 {
			continue // skip empty docs
		}

		raw, err := yaml.Marshal(&node)
		if err != nil {
			return nil, fmt.Errorf("re-marshal yaml: %w", err)
		}

		var env Envelope
		if err := yaml.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("unmarshal envelope: %w", err)
		}
		if env.Kind == "" {
			return nil, fmt.Errorf("missing kind")
		}
		if env.APIVersion == "" {
			return nil, fmt.Errorf("missing apiVersion for kind %q", env.Kind)
		}

		handlerRegistryMu.RLock()
		t, ok := kindToTypeMap[env.Kind]
		handlerRegistryMu.RUnlock()
		if t != kType {
			continue
		}
		if !ok {
			return nil, fmt.Errorf("no handler registered for kind %q (apiVersion=%q)", env.Kind, env.APIVersion)
		}
		ha, ok := handlerRegistryByKind[env.Kind]
		if !ok {
			return nil, fmt.Errorf("no handler registered for kind %q (apiVersion=%q)", env.Kind, env.APIVersion)
		}
		h, ok := ha.(KindHandler[K])
		if !ok {
			return nil, fmt.Errorf("handler for kind %q (apiVersion=%q) does not implement %v", env.Kind, env.APIVersion, reflect.TypeOf((*KindHandler[K])(nil)).Elem())
		}

		obj, err := h.Unmarshal(env.APIVersion, raw)
		if err != nil {
			return nil, fmt.Errorf("kind %q apiVersion %q name %q: %w",
				env.Kind, env.APIVersion, env.Metadata.Name, err)
		}
		out = append(out, obj)
	}

	return out, nil
}
