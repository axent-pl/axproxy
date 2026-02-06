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
		Left:  "${session.user}",
		Op:    "eq",
		Right: "alice",
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
		Left: "${session.items}",
		Op:   "empty",
	}
	ok, err := mapper.EvalCondition(cond, src)
	if err != nil {
		t.Fatalf("EvalCondition error: %v", err)
	}
	if !ok {
		t.Fatalf("expected condition to be true")
	}
}

func TestEvalConditionAndOrNot(t *testing.T) {
	src := map[string]any{
		"session": map[string]any{
			"user":  "alice",
			"items": []any{},
		},
	}
	cond := mapper.Condition{
		And: []mapper.Condition{
			{
				Left:  "${session.user}",
				Op:    "eq",
				Right: "alice",
			},
			{
				Not: &mapper.Condition{
					Or: []mapper.Condition{
						{
							Left: "${session.items}",
							Op:   "empty",
						},
						{
							Left:  "${session.user}",
							Op:    "eq",
							Right: "bob",
						},
					},
				},
			},
		},
	}
	ok, err := mapper.EvalCondition(cond, src)
	if err != nil {
		t.Fatalf("EvalCondition error: %v", err)
	}
	if ok {
		t.Fatalf("expected condition to be false")
	}
}
