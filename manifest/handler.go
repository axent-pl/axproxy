package manifest

import (
	"fmt"
	"reflect"
	"sync"
)

type KindHandler[K any] interface {
	Kind() string
	Unmarshal(apiVersion string, rawYAML []byte) (K, error)
}

var (
	handlerRegistryMu     sync.RWMutex
	kindToTypeMap         = map[string]reflect.Type{}
	handlerRegistryByKind = map[string]any{}
)

func RegisterHandler[K any](handler KindHandler[K]) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	t := reflect.TypeOf((*K)(nil)).Elem()
	handlerRegistryMu.Lock()
	defer handlerRegistryMu.Unlock()

	if _, exists := kindToTypeMap[handler.Kind()]; exists {
		return fmt.Errorf("duplicate handler for kind %s", handler.Kind())
	}
	kindToTypeMap[handler.Kind()] = t
	handlerRegistryByKind[handler.Kind()] = handler

	return nil
}
