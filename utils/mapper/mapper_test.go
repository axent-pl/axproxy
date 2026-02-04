package mapper_test

import (
	"reflect"
	"testing"

	"github.com/axent-pl/axproxy/utils/mapper"
)

func TestApply(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		dst     map[string]any
		src     map[string]any
		rules   map[string]string
		wantErr bool
		wantDst map[string]any
	}{
		{
			name: "maps simple and nested",
			dst:  map[string]any{},
			src: map[string]any{
				"a": 1,
				"nested": map[string]any{
					"b": "hello",
				},
			},
			rules: map[string]string{
				"x":   "${a}",
				"y.z": "${nested.b}",
			},
			wantDst: map[string]any{
				"x": 1,
				"y": map[string]any{
					"z": "hello",
				},
			},
		},
		{
			name: "reads list indices from src",
			dst:  map[string]any{},
			src: map[string]any{
				"items": []any{"zero", "one"},
			},
			rules: map[string]string{
				"out.first": "${items[0]}",
				"out.last":  "${items[1]}",
			},
			wantDst: map[string]any{
				"out": map[string]any{
					"first": "zero",
					"last":  "one",
				},
			},
		},
		{
			name: "expands dst list and nested objects",
			dst:  map[string]any{},
			src: map[string]any{
				"user": map[string]any{
					"name": "Ada",
				},
			},
			rules: map[string]string{
				"users[1].name": "${user.name}",
			},
			wantDst: map[string]any{
				"users": []any{
					nil,
					map[string]any{
						"name": "Ada",
					},
				},
			},
		},
		{
			name: "default values with different types",
			dst:  map[string]any{},
			src:  map[string]any{},
			rules: map[string]string{
				"v.int":   "${missing|42}",
				"v.float": "${missing|3.14}",
				"v.bool":  "${missing|true}",
				"v.nil":   "${missing|null}",
				"v.str":   "${missing|'hello'}",
			},
			wantDst: map[string]any{
				"v": map[string]any{
					"int":   int64(42),
					"float": 3.14,
					"bool":  true,
					"nil":   nil,
					"str":   "hello",
				},
			},
		},
		{
			name: "missing src without default leaves dst untouched",
			dst: map[string]any{
				"keep": "value",
			},
			src: map[string]any{},
			rules: map[string]string{
				"missing": "${nope}",
			},
			wantDst: map[string]any{
				"keep": "value",
			},
		},
		{
			name:    "invalid expression",
			dst:     map[string]any{},
			src:     map[string]any{},
			rules:   map[string]string{"x": "${}"},
			wantErr: true,
		},
		{
			name: "invalid dst path type",
			dst: map[string]any{
				"x": "not-a-map",
			},
			src: map[string]any{
				"a": "value",
			},
			rules:   map[string]string{"x.y": "a"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := mapper.Apply(tt.dst, tt.src, tt.rules)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Apply() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Apply() succeeded unexpectedly")
			}
			if tt.wantDst != nil && !reflect.DeepEqual(tt.dst, tt.wantDst) {
				t.Fatalf("Apply() dst mismatch\n got: %#v\nwant: %#v", tt.dst, tt.wantDst)
			}
		})
	}
}
