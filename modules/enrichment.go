package modules

import (
	"context"
	"fmt"
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

	srcInterfaces map[string]enrichment.EnricheSourcer
}

func (m *EnrichmentModule) Kind() string {
	return KIND_ENRICHMENT
}

func (m *EnrichmentModule) Name() string {
	return m.Metadata.Name
}

func (m *EnrichmentModule) Middleware(next http.HandlerFunc) http.HandlerFunc {
	m.initSources() // will go to factory
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st := state.GetState(r.Context())
		if r == nil || st == nil {
			next.ServeHTTP(w, r)
			return
		}
		m.doLookup(r.Context(), st)
		next.ServeHTTP(w, r)
	})
}

func (m *EnrichmentModule) doLookup(ctx context.Context, st *state.State) error {
	dst := map[string]any{
		"session": st.Session.Values,
	}
	for _, lookup := range m.Lookups {
		if _, ok := m.srcInterfaces[lookup.SourceName]; !ok {
			return fmt.Errorf("undefined enrichment source %s", lookup.SourceName)
		}
		lookupOutput, err := m.srcInterfaces[lookup.SourceName].Lookup(ctx, lookup.Inputs, lookup.Outputs)
		if err != nil {
			return fmt.Errorf("enrichment lookup (source:%s, lookup:%s) failed: %w", lookup.SourceName, lookup.Name, err)
		}
		err = mapper.Apply(dst, lookupOutput, lookup.Mappings)
		if err != nil {
			return fmt.Errorf("enrichment mapping (source:%s, lookup:%s) failed: %w", lookup.SourceName, lookup.Name, err)
		}
	}
	return nil
}

func (m *EnrichmentModule) initSources() error {
	m.srcInterfaces = make(map[string]enrichment.EnricheSourcer)
	for _, source := range m.Sources {
		switch source.Type {
		case "ldap":
			sourceInterface, err := enrichment.NewLdapEnrichmentSource(&source.LdapSourceConfig)
			if err != nil {
				return fmt.Errorf("could not initialize enrichment source (%s:%s): %w", source.Type, source.Name, err)
			}
			m.srcInterfaces[source.Name] = sourceInterface
		default:
			return fmt.Errorf("could not initialize enrichment source (%s:%s): unknow source", source.Type, source.Name)
		}
	}
	return nil
}
