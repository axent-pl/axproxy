package mapper

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func Apply(dst, src map[string]any, rules map[string]string) error {
	for dstPath, srcExpr := range rules {
		path, hasPath, def, hasDef, err := parseExpr(srcExpr)
		if err != nil {
			return fmt.Errorf("mapper expression parse error %q: %w", dstPath, err)
		}

		if !hasPath {
			if !hasDef {
				continue
			}
			if err := set(dst, dstPath, def); err != nil {
				return fmt.Errorf("set dst %q: %w", dstPath, err)
			}
			continue
		}

		val, ok, err := get(src, path)
		if err != nil {
			return fmt.Errorf("get %q for dst %q: %w", path, dstPath, err)
		}
		if !ok {
			if !hasDef {
				continue
			}
			val = def
		}
		if err := set(dst, dstPath, val); err != nil {
			return fmt.Errorf("set dst %q: %w", dstPath, err)
		}

	}
	return nil
}

// ---------------- expr parsing ----------------

// parseExpr accepts:
//
//	${path|default}
//	${path}
//	default
func parseExpr(expr string) (path string, hasPath bool, def any, hasDefault bool, err error) {
	s := strings.TrimSpace(expr)

	// unwrap ${...} or return literal as default
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		s = strings.TrimSpace(s[2 : len(s)-1])
	} else {
		return "", false, parseDefault(s), true, nil
	}

	// split on first unescaped |
	parts := strings.SplitN(s, "|", 2)
	path = strings.TrimSpace(parts[0])
	if path == "" {
		return "", false, nil, false, fmt.Errorf("empty path")
	}
	hasPath = true

	if len(parts) == 2 {
		hasDefault = true
		defStr := strings.TrimSpace(parts[1])
		def = parseDefault(defStr)
	}
	return
}

// parseDefault tries to interpret common literals; otherwise returns string as-is.
// Supports: null, true/false, ints, floats, "quoted strings", 'quoted strings'
func parseDefault(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	switch strings.ToLower(s) {
	case "null", "nil":
		return nil
	case "true":
		return true
	case "false":
		return false
	}
	// quoted
	if (strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) ||
		(strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`)) {
		return s[1 : len(s)-1]
	}
	// number
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	// fallback: raw string
	return s
}

// ---------------- path parsing ----------------

type stepKind int

const (
	stepKey stepKind = iota
	stepIndex
)

type step struct {
	kind stepKind
	key  string
	idx  int
}

func parsePath(path string) ([]step, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return nil, fmt.Errorf("empty path")
	}
	var out []step
	var buf strings.Builder

	flushKey := func() {
		if buf.Len() > 0 {
			out = append(out, step{kind: stepKey, key: buf.String()})
			buf.Reset()
		}
	}

	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '.':
			flushKey()
		case '[':
			flushKey()
			end := strings.IndexByte(p[i:], ']')
			if end < 0 {
				return nil, fmt.Errorf("missing closing ] at pos %d", i)
			}
			end += i
			content := strings.TrimSpace(p[i+1 : end])
			n, err := strconv.Atoi(content)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("invalid index %q at pos %d", content, i)
			}
			out = append(out, step{kind: stepIndex, idx: n})
			i = end
		default:
			buf.WriteByte(p[i])
		}
	}
	flushKey()
	return out, nil
}

// ---------------- get/set ----------------

func get(root any, path string) (any, bool, error) {
	steps, err := parsePath(path)
	if err != nil {
		return nil, false, err
	}

	cur := root

	for _, st := range steps {
		v := reflect.ValueOf(cur)
		if !v.IsValid() {
			return nil, false, nil
		}

		// unwrap interfaces/pointers
		for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return nil, false, nil
			}
			v = v.Elem()
		}

		switch st.kind {
		case stepKey:
			if v.Kind() != reflect.Map || v.Type().Key().Kind() != reflect.String {
				return nil, false, nil
			}
			mv := v.MapIndex(reflect.ValueOf(st.key))
			if !mv.IsValid() {
				return nil, false, nil
			}
			cur = mv.Interface()

		case stepIndex:
			if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
				return nil, false, nil
			}
			if st.idx < 0 || st.idx >= v.Len() {
				return nil, false, nil
			}
			cur = v.Index(st.idx).Interface()

		default:
			return nil, false, fmt.Errorf("unknown step kind")
		}
	}

	return cur, true, nil
}

func set(dst map[string]any, dstPath string, val any) error {
	steps, err := parsePath(dstPath)
	if err != nil {
		return err
	}
	var cur any = dst

	for i := 0; i < len(steps); i++ {
		last := i == len(steps)-1
		st := steps[i]

		switch st.kind {
		case stepKey:
			m, ok := cur.(map[string]any)
			if !ok || m == nil {
				return fmt.Errorf("expected object at %q", st.key)
			}
			if last {
				m[st.key] = val
				return nil
			}
			nxt, exists := m[st.key]
			if !exists || nxt == nil {
				// create based on lookahead
				switch steps[i+1].kind {
				case stepKey:
					nxt = map[string]any{}
				case stepIndex:
					nxt = []any{}
				}
				m[st.key] = nxt
			}
			cur = nxt

		case stepIndex:
			a, ok := cur.([]any)
			if !ok {
				return fmt.Errorf("expected array at index [%d]", st.idx)
			}
			if st.idx >= len(a) {
				newA := make([]any, st.idx+1)
				copy(newA, a)
				a = newA
				// write back grown slice into its parent
				if err := replaceArrayInParent(dst, steps[:i], a); err != nil {
					return err
				}
			}
			if last {
				a[st.idx] = val
				_ = replaceArrayInParent(dst, steps[:i], a)
				return nil
			}
			nxt := a[st.idx]
			if nxt == nil {
				switch steps[i+1].kind {
				case stepKey:
					nxt = map[string]any{}
				case stepIndex:
					nxt = []any{}
				}
				a[st.idx] = nxt
				_ = replaceArrayInParent(dst, steps[:i], a)
			}
			cur = nxt

		default:
			return fmt.Errorf("unknown step kind")
		}
	}
	return nil
}

// replaceArrayInParent writes an updated slice back to its parent container.
// prefixSteps is the path to the array container itself (excluding the step that picked it).
func replaceArrayInParent(root map[string]any, prefixSteps []step, newArr []any) error {
	if len(prefixSteps) == 0 {
		// root is a map; arrays only live under some key/index
		return nil
	}

	// walk to parent of the array container selector
	var cur any = root
	for i := 0; i < len(prefixSteps)-1; i++ {
		st := prefixSteps[i]
		switch st.kind {
		case stepKey:
			m, _ := cur.(map[string]any)
			if m == nil {
				return nil
			}
			cur = m[st.key]
		case stepIndex:
			a, _ := cur.([]any)
			if a == nil || st.idx >= len(a) {
				return nil
			}
			cur = a[st.idx]
		}
	}

	last := prefixSteps[len(prefixSteps)-1]
	switch last.kind {
	case stepKey:
		m, _ := cur.(map[string]any)
		if m != nil {
			m[last.key] = newArr
		}
	case stepIndex:
		a, _ := cur.([]any)
		if a != nil && last.idx < len(a) {
			a[last.idx] = newArr
		}
	}
	return nil
}
