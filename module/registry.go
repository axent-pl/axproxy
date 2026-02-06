package module

import (
	"fmt"
	"log/slog"
	"sync"
)

type KindName struct {
	Kind string
	Name string
}

func (k KindName) Equal(other KindName) bool {
	return k.Kind == other.Kind && k.Name == other.Name
}

var (
	registryMU sync.RWMutex
	registry   = map[KindName]Module{}
)

func Register(module Module) {
	registryMU.Lock()
	defer registryMU.Unlock()
	kn := KindName{Kind: module.Kind(), Name: module.Name()}
	registry[kn] = module
	slog.Info("Module registered", "module_kind", module.Kind(), "module_name", module.Name())
}

func Get(kind string, name string) (Module, error) {
	registryMU.RLock()
	defer registryMU.RUnlock()
	kn := KindName{Kind: kind, Name: name}
	if module, ok := registry[kn]; ok {
		return module, nil
	}
	return nil, fmt.Errorf("module not found, kind: %s, name: %s", kind, name)
}
