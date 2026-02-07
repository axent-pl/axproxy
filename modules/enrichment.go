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
	When     *mapper.Condition   `yaml:"when"`
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

func (m *EnrichmentModule) Skip(next module.ProxyHandlerFunc, w http.ResponseWriter, r *http.Request, st *state.State) bool {
	if m.When != nil {
		src := mapper.BuildSourceMap(st.Session, r, nil)
		exec, err := mapper.EvalCondition(*m.When, src)
		if err != nil {
			http.Error(w, "could not eval step condition", http.StatusBadGateway)
			return true
		}
		if !exec {
			slog.Info("EnrichmentModule skipped", "request_id", st.RequestID)
			next(w, r, st)
			return true
		}
	}
	return false
}

func (m *EnrichmentModule) ProxyMiddleware(next module.ProxyHandlerFunc) module.ProxyHandlerFunc {
	return module.ProxyHandlerFunc(func(w http.ResponseWriter, r *http.Request, st *state.State) {
		if r == nil || st == nil {
			next(w, r, st)
			return
		}
		if m.Skip(next, w, r, st) {
			return
		}
		m.doLookup(r.Context(), st)
		next(w, r, st)
	})
}

func (m *EnrichmentModule) mapLookupInputs(_ context.Context, lookup EnrichmentLookup, st *state.State) (map[string]string, error) {
	src := mapper.BuildSourceMap(st.Session, nil, nil)
	dst := map[string]any{}
	if err := mapper.Apply(dst, src, lookup.Inputs); err != nil {
		return nil, err
	}

	inputs := map[string]string{}
	for k, v := range dst {
		inputs[k] = v.(string)
	}
	return inputs, nil
}

func (m *EnrichmentModule) doLookup(ctx context.Context, st *state.State) error {
	log_base := slog.With(
		"request_id", st.RequestID,
		"module_kind", KIND_ENRICHMENT,
		"module_name", m.Metadata.Name,
	)

	for _, lookup := range m.Lookups {
		log_lookup := log_base.With(
			"source_name", lookup.SourceName,
			"lookup_name", lookup.Name,
		)

		src, ok := m.srcInterfaces[lookup.SourceName]
		if !ok {
			err := fmt.Errorf("invalid enrichment source %s", lookup.SourceName)
			log_lookup.Error("lookup failed", "error", err)
			return fmt.Errorf("undefined enrichment source %s", lookup.SourceName)
		}

		lookupInputs, err := m.mapLookupInputs(ctx, lookup, st)
		if err != nil {
			log_lookup.Error("lookup failed", "error", fmt.Errorf("failed to map input: %w", err))
			return fmt.Errorf("failed to map input: %w", err)
		}

		lookupOutputs, err := src.Lookup(ctx, lookupInputs, lookup.Outputs)
		if err != nil {
			log_lookup.Error("lookup failed", "error", fmt.Errorf("failed to call source: %w", err))
			return fmt.Errorf("failed to call source: %w", err)
		}

		dst := map[string]any{}
		if err := mapper.Apply(dst, lookupOutputs, lookup.Mappings); err != nil {
			log_lookup.Error("lookup failed", "error", fmt.Errorf("failed to map output: %w", err))
			return fmt.Errorf("failed to map output: %w", err)
		}

		if err := mapper.ApplyToTargets(dst, st.Session, nil, nil); err != nil {
			log_lookup.Error("lookup failed", "error", fmt.Errorf("failed to map output to targets: %w", err))
			return fmt.Errorf("failed to map output to targets: %w", err)
		}

		log_lookup.Info("lookup completed")
	}
	return nil
}

func (m *EnrichmentModule) Start() error {
	log_base := slog.With(
		"module_kind", KIND_ENRICHMENT,
		"module_name", m.Metadata.Name,
	)
	m.srcInterfaces = make(map[string]enrichment.EnrichmentSourcer)
	for _, source := range m.Sources {
		log_source := log_base.With(
			"source_type", source.Type,
			"source_name", source.Name,
		)
		switch source.Type {
		case "ldap":
			sourceInterface, err := enrichment.NewLdapEnrichmentSource(&source.LdapSourceConfig)
			if err != nil {
				log_source.Error("could not initialize enrichment source", "error", err)
				return fmt.Errorf("could not initialize enrichment source: %w", err)
			}
			m.srcInterfaces[source.Name] = sourceInterface
		case "dummy":
			m.srcInterfaces[source.Name] = enrichment.NewDummyEnrichmentSource()
		default:
			log_source.Error("invalid enrichment source")
			return fmt.Errorf("invalid enrichment source (%s:%s)", source.Type, source.Name)
		}
	}
	return nil
}
