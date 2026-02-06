package enrichment

import "context"

type EnrichmentSourcer interface {
	Lookup(ctx context.Context, inputs map[string]string, outputs []string) (map[string]any, error)
}
