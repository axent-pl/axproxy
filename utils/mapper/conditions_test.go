package mapper_test

import (
	"testing"

	"github.com/axent-pl/axproxy/utils/mapper"
)

func TestEvalConditionEq(t *testing.T) {
	src := map[string]any{
		"session": map[string]any{
			"user": "alice",
		},
	}
	cond := mapper.Condition{
		Eq: &mapper.EqCondition{
			Left:  "${session.user}",
			Right: "alice",
		},
	}
	ok, err := mapper.EvalCondition(cond, src)
	if err != nil {
		t.Fatalf("EvalCondition error: %v", err)
	}
	if !ok {
		t.Fatalf("expected condition to be true")
	}
}

func TestEvalConditionEmpty(t *testing.T) {
	src := map[string]any{
		"session": map[string]any{
			"items": []any{},
		},
	}
	cond := mapper.Condition{
		Empty: &mapper.EmptyCondition{
			Value: "${session.items}",
		},
	}
	ok, err := mapper.EvalCondition(cond, src)
	if err != nil {
		t.Fatalf("EvalCondition error: %v", err)
	}
	if !ok {
		t.Fatalf("expected condition to be true")
	}
}
