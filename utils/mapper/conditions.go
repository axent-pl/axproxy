package mapper

import (
	"fmt"
	"reflect"
	"strings"
)

// Condition represents a leaf condition or a logical group.
type Condition struct {
	Left  string `yaml:"left,omitempty"`
	Op    string `yaml:"op,omitempty"`
	Right string `yaml:"right,omitempty"`

	And []Condition `yaml:"and,omitempty"`
	Or  []Condition `yaml:"or,omitempty"`
	Not *Condition  `yaml:"not,omitempty"`
}

// EvalCondition evaluates a single condition against the provided source map.
func EvalCondition(cond Condition, src map[string]any) (bool, error) {
	hasAnd := len(cond.And) > 0
	hasOr := len(cond.Or) > 0
	hasNot := cond.Not != nil
	hasOp := cond.Op != "" || cond.Left != "" || cond.Right != ""

	if hasOp && (hasAnd || hasOr || hasNot) {
		return false, fmt.Errorf("invalid condition: cannot combine op with logical groups")
	}
	if (hasAnd && (hasOr || hasNot)) || (hasOr && hasNot) {
		return false, fmt.Errorf("invalid condition: multiple logical groups set")
	}

	switch {
	case hasAnd:
		for i, child := range cond.And {
			ok, err := EvalCondition(child, src)
			if err != nil {
				return false, fmt.Errorf("and[%d]: %w", i, err)
			}
			if !ok {
				return false, nil
			}
		}
		return true, nil
	case hasOr:
		for i, child := range cond.Or {
			ok, err := EvalCondition(child, src)
			if err != nil {
				return false, fmt.Errorf("or[%d]: %w", i, err)
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	case hasNot:
		ok, err := EvalCondition(*cond.Not, src)
		if err != nil {
			return false, fmt.Errorf("not: %w", err)
		}
		return !ok, nil
	default:
		return evalOp(cond, src)
	}
}

func evalOp(c Condition, src map[string]any) (bool, error) {
	if c.Op == "" {
		return false, fmt.Errorf("invalid condition: missing op")
	}
	switch strings.ToLower(c.Op) {
	case "eq":
		return evalEq(c.Left, c.Right, src)
	case "empty":
		return evalEmpty(c.Left, src)
	default:
		return false, fmt.Errorf("invalid condition: unsupported op %q", c.Op)
	}
}

func evalEq(leftExpr, rightExpr string, src map[string]any) (bool, error) {
	if leftExpr == "" || rightExpr == "" {
		return false, fmt.Errorf("invalid eq condition: left/right required")
	}
	left, ok, err := resolveExpr(src, leftExpr)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	right, ok, err := resolveExpr(src, rightExpr)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return reflect.DeepEqual(left, right), nil
}

func evalEmpty(expr string, src map[string]any) (bool, error) {
	if expr == "" {
		return false, fmt.Errorf("invalid empty condition: left required")
	}
	val, ok, err := resolveExpr(src, expr)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	return isEmpty(val), nil
}

func resolveExpr(src map[string]any, expr string) (any, bool, error) {
	path, hasPath, def, hasDefault, err := parseExpr(expr)
	if err != nil {
		return nil, false, err
	}
	if !hasPath {
		if !hasDefault {
			return nil, false, nil
		}
		return def, true, nil
	}
	val, ok, err := get(src, path)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		if hasDefault {
			return def, true, nil
		}
		return nil, false, nil
	}
	return val, true, nil
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Interface || rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return true
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len() == 0
	}
	return false
}
