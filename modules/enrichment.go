package modules

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/axent-pl/axproxy/manifest"
	"github.com/axent-pl/axproxy/module"
	"github.com/axent-pl/axproxy/modules/enrichment"
	"github.com/axent-pl/axproxy/state"
	"github.com/axent-pl/axproxy/utils/mapper"
)

const KIND_ENRICHMENT string = "Enrichment"

type EnrichmentSource struct {
	Type             string                                `yaml:"type"`
	Name             string                                `yaml:"name"`
	LdapSourceConfig enrichment.LdapEnrichmentSourceConfig `yaml:"ldap"`
}

type EnrichmentLookup struct {
	Name       string            `yaml:"name"`
	SourceName string            `yaml:"source"`
	Inputs     map[string]string `yaml:"inputs"`
	Outputs    []string          `yaml:"outputs"`
	Mappings   map[string]string `yaml:"mappings"`
}

type EnrichmentModule struct {
	module.NoopModule
	Metadata manifest.ObjectMeta `yaml:"metadata"`
	Sources  []EnrichmentSource  `yaml:"sources"`
	Lookups  []EnrichmentLookup  `yaml:"lookups"`

	srcInterfaces map[string]enrichment.EnrichmentSourcer
}

func (m *EnrichmentModule) Kind() string {
	return KIND_ENRICHMENT
}

func (m *EnrichmentModule) Name() string {
	return m.Metadata.Name
}

func (m *EnrichmentModule) ProxyMiddleware(next module.ProxyHandlerFunc) module.ProxyHandlerFunc {
	m.initSources() // will go to factory
	return module.ProxyHandlerFunc(func(w http.ResponseWriter, r *http.Request, st *state.State) {
		if r == nil || st == nil {
			next(w, r, st)
			return
		}
		m.doLookup(r.Context(), st)
		next(w, r, st)
	})
}

func (m *EnrichmentModule) mapLookupInputs(_ context.Context, lookup EnrichmentLookup, st *state.State) (map[string]string, error) {
	src := map[string]any{
		"session": st.Session.GetValues(),
	}
	dst := map[string]any{}
	if err := mapper.Apply(dst, src, lookup.Inputs); err != nil {
		return nil, fmt.Errorf("enrichment mapping inputs (source:%s, lookup:%s) failed: %w", lookup.SourceName, lookup.Name, err)
	}

	inputs := map[string]string{}
	for k, v := range dst {
		inputs[k] = v.(string)
	}
	return inputs, nil
}

func (m *EnrichmentModule) doLookup(ctx context.Context, st *state.State) error {
	for _, lookup := range m.Lookups {

		if _, ok := m.srcInterfaces[lookup.SourceName]; !ok {
			slog.Error("enrichment", "error", fmt.Errorf("undefined enrichment source %s", lookup.SourceName))
			return fmt.Errorf("undefined enrichment source %s", lookup.SourceName)
		}

		lookupInputs, err := m.mapLookupInputs(ctx, lookup, st)
		if err != nil {
			slog.Error("enrichment", "error", err)
			return err
		}

		lookupOutputs, err := m.srcInterfaces[lookup.SourceName].Lookup(ctx, lookupInputs, lookup.Outputs)
		if err != nil {
			slog.Error("enrichment", "error", err)
			return fmt.Errorf("enrichment lookup (source:%s, lookup:%s) failed: %w", lookup.SourceName, lookup.Name, err)
		}

		dst := map[string]any{}
		err = mapper.Apply(dst, lookupOutputs, lookup.Mappings)
		if err != nil {
			slog.Error("enrichment", "error", err)
			return fmt.Errorf("enrichment mapping (source:%s, lookup:%s) failed: %w", lookup.SourceName, lookup.Name, err)
		}

		if sessionValues, ok := dst["session"].(map[string]any); ok {
			st.Session.SetValues(sessionValues)
		}
	}

	return nil
}

func (m *EnrichmentModule) initSources() error {
	m.srcInterfaces = make(map[string]enrichment.EnrichmentSourcer)
	for _, source := range m.Sources {
		switch source.Type {
		case "ldap":
			sourceInterface, err := enrichment.NewLdapEnrichmentSource(&source.LdapSourceConfig)
			if err != nil {
				return fmt.Errorf("could not initialize enrichment source (%s:%s): %w", source.Type, source.Name, err)
			}
			m.srcInterfaces[source.Name] = sourceInterface
		case "dummy":
			m.srcInterfaces[source.Name] = enrichment.NewDummyEnrichmentSource()
		default:
			return fmt.Errorf("could not initialize enrichment source (%s:%s): unknow source", source.Type, source.Name)
		}
	}
	return nil
}
