package indexed

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine/parser"
)

type fakeUnsupportedCondition struct{}

func (fakeUnsupportedCondition) Eval(map[string]interface{}) bool { return false }

func TestEncodeConditionPanicsOnUnsupportedShape(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("encodeCondition must panic on an unsupported shape")
		}
	}()
	_ = encodeCondition(fakeUnsupportedCondition{})
}

func TestEncodeConditionPointerVariants(t *testing.T) {
	cases := []parser.Condition{
		&parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
		&parser.SetCondition{Field: "k", Op: parser.OpIn, Values: []string{"a"}},
		&parser.RangeCondition{Field: "k", Min: 0, Max: 1},
		&parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
		}},
	}
	for _, c := range cases {
		enc := encodeCondition(c)
		if enc.Type == "" {
			t.Fatalf("pointer variant %T encoded empty", c)
		}
	}
}
