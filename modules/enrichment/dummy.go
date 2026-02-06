package enrichment

import "context"

type DummyEnrichmentSource struct{}

func NewDummyEnrichmentSource() *DummyEnrichmentSource {
	return &DummyEnrichmentSource{}
}

func (ds *DummyEnrichmentSource) Lookup(ctx context.Context, inputs map[string]string, outputs []string) (map[string]any, error) {
	_ = ctx
	results := map[string]any{}
	results["inputs"] = inputs
	results["outputs"] = outputs
	return results, nil
}
