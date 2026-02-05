package enrichment

import "context"

type EnricheSourcer interface {
	Lookup(ctx context.Context, inputs map[string]string, outputs []string) (map[string]any, error)
}
