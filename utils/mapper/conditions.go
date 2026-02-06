package mapper

import (
	"fmt"
	"reflect"
)

// Condition represents a single basic condition.
// Exactly one of Eq or Empty should be set.
type Condition struct {
	Eq    *EqCondition    `yaml:"eq"`
	Empty *EmptyCondition `yaml:"empty"`
}

type EqCondition struct {
	Left  string `yaml:"left"`
	Right string `yaml:"right"`
}

type EmptyCondition struct {
	Value string `yaml:"value"`
}

// EvalCondition evaluates a single condition against the provided source map.
func EvalCondition(cond Condition, src map[string]any) (bool, error) {
	if cond.Eq != nil && cond.Empty != nil {
		return false, fmt.Errorf("invalid condition: both eq and empty set")
	}
	if cond.Eq == nil && cond.Empty == nil {
		return false, fmt.Errorf("invalid condition: no operator set")
	}
	if cond.Eq != nil {
		return evalEq(*cond.Eq, src)
	}
	return evalEmpty(*cond.Empty, src)
}

func evalEq(c EqCondition, src map[string]any) (bool, error) {
	left, ok, err := resolveExpr(src, c.Left)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	right, ok, err := resolveExpr(src, c.Right)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return reflect.DeepEqual(left, right), nil
}

func evalEmpty(c EmptyCondition, src map[string]any) (bool, error) {
	val, ok, err := resolveExpr(src, c.Value)
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
