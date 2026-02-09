package graph

import (
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
)

func field(name string, children ...*ast.Field) *ast.Field {
	var sel ast.SelectionSet
	for _, c := range children {
		sel = append(sel, c)
	}
	return &ast.Field{Alias: name, SelectionSet: sel}
}

func TestQueryDepth(t *testing.T) {
	tests := []struct {
		name string
		sel  ast.SelectionSet
		want int
	}{
		{"empty", nil, 0},
		{"single field", ast.SelectionSet{field("a")}, 1},
		{"nested 3", ast.SelectionSet{field("a", field("b", field("c")))}, 3},
		{"wide not deep", ast.SelectionSet{field("a"), field("b"), field("c")}, 1},
		{"mixed depth", ast.SelectionSet{
			field("a", field("b")),
			field("c", field("d", field("e"))),
		}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryDepth(tt.sel)
			if got != tt.want {
				t.Errorf("queryDepth() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDepthLimitValidate(t *testing.T) {
	if err := (DepthLimit{MaxDepth: 0}).Validate(nil); err == nil {
		t.Error("expected error for MaxDepth=0")
	}
	if err := (DepthLimit{MaxDepth: 7}).Validate(nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
