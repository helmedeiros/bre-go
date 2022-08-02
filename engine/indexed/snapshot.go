package indexed

import (
	"context"
	"fmt"
	"strconv"

	"github.com/helmedeiros/bre-go/engine/parser"
)

// SnapshotFormatVersion identifies the on-disk schema. LoadSnapshot
// refuses snapshots with any other value.
const SnapshotFormatVersion = 1

// Snapshot is the JSON-serializable representation of a built engine's
// rule set. Insertion order is preserved.
type Snapshot struct {
	FormatVersion int            `json:"formatVersion"`
	Rules         []SnapshotRule `json:"rules"`
}

// SnapshotRule is a serialized rule. Action and ActionContext are not
// encoded; LoadSnapshot re-attaches them by name via the rebuild map.
type SnapshotRule struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Match       SnapshotCondition `json:"match"`
}

// SnapshotCondition is a tagged-union encoding of parser.Condition.
// Type is one of: "string", "set", "range", "and".
type SnapshotCondition struct {
	Type     string              `json:"type"`
	Field    string              `json:"field,omitempty"`
	Op       string              `json:"op,omitempty"`
	Value    string              `json:"value,omitempty"`
	Values   []string            `json:"values,omitempty"`
	Min      string              `json:"min,omitempty"`
	Max      string              `json:"max,omitempty"`
	Children []SnapshotCondition `json:"children,omitempty"`
}

// RuleCallbacks carries the per-rule action callbacks that LoadSnapshot
// re-attaches by rule name.
type RuleCallbacks struct {
	Action        func(input interface{}) interface{}
	ActionContext func(ctx context.Context, input interface{}) interface{}
}

const (
	snapshotTypeString = "string"
	snapshotTypeSet    = "set"
	snapshotTypeRange  = "range"
	snapshotTypeAnd    = "and"
)

// ExportSnapshot serializes the engine's rule set. Triggers implicit
// Build if not yet built.
func (e *Engine) ExportSnapshot() (*Snapshot, error) {
	e.mu.Lock()
	if e.builder != nil && e.builder.postFilterHook != nil {
		e.mu.Unlock()
		return nil, ErrSnapshotIncompatibleHook
	}
	e.mu.Unlock()

	s := e.readSnapshot()
	if s.postFilterHook != nil {
		return nil, ErrSnapshotIncompatibleHook
	}
	if len(s.rulesInOrder) == 0 {
		return nil, ErrSnapshotEmpty
	}

	out := &Snapshot{
		FormatVersion: SnapshotFormatVersion,
		Rules:         make([]SnapshotRule, 0, len(s.rulesInOrder)),
	}
	for _, r := range s.rulesInOrder {
		out.Rules = append(out.Rules, SnapshotRule{
			Name:        r.Name,
			Description: r.Description,
			Tags:        copyTags(r.Tags),
			Match:       encodeCondition(r.Match),
		})
	}
	return out, nil
}

// LoadSnapshot reconstructs an Engine from snap. The returned engine is
// already Built and ready to Execute. rebuild may be nil.
func LoadSnapshot(snap *Snapshot, rebuild map[string]RuleCallbacks) (*Engine, error) {
	if snap == nil {
		return nil, ErrSnapshotMalformed
	}
	if snap.FormatVersion != SnapshotFormatVersion {
		return nil, ErrSnapshotFormatVersionMismatch
	}

	e := New()
	for _, sr := range snap.Rules {
		match, err := decodeCondition(sr.Match)
		if err != nil {
			return nil, err
		}
		cb := rebuild[sr.Name]
		if err := e.AddRule(Rule{
			Name:          sr.Name,
			Description:   sr.Description,
			Tags:          copyTags(sr.Tags),
			Match:         match,
			Action:        cb.Action,
			ActionContext: cb.ActionContext,
		}); err != nil {
			return nil, err
		}
	}
	_ = e.Build()
	return e, nil
}

// encodeCondition panics if c is not a shape AddRule would have admitted.
// Built engines hold only admitted shapes, so the panic is unreachable
// in practice; it guards against silent encoder/classifier drift.
func encodeCondition(c parser.Condition) SnapshotCondition {
	switch v := c.(type) {
	case parser.StringCondition:
		return SnapshotCondition{Type: snapshotTypeString, Field: v.Field, Op: v.Op, Value: v.Value}
	case *parser.StringCondition:
		return encodeCondition(*v)
	case parser.SetCondition:
		return SnapshotCondition{Type: snapshotTypeSet, Field: v.Field, Op: v.Op, Values: append([]string(nil), v.Values...)}
	case *parser.SetCondition:
		return encodeCondition(*v)
	case parser.RangeCondition:
		return SnapshotCondition{
			Type:  snapshotTypeRange,
			Field: v.Field,
			Min:   strconv.FormatFloat(v.Min, 'g', -1, 64),
			Max:   strconv.FormatFloat(v.Max, 'g', -1, 64),
		}
	case *parser.RangeCondition:
		return encodeCondition(*v)
	case parser.AndCondition:
		children := make([]SnapshotCondition, 0, len(v.Children))
		for _, child := range v.Children {
			children = append(children, encodeCondition(child))
		}
		return SnapshotCondition{Type: snapshotTypeAnd, Children: children}
	case *parser.AndCondition:
		return encodeCondition(*v)
	default:
		panic(fmt.Sprintf("indexed: encodeCondition called with unsupported shape %T (build invariant violated)", c))
	}
}

func decodeCondition(sc SnapshotCondition) (parser.Condition, error) {
	switch sc.Type {
	case snapshotTypeString:
		return parser.StringCondition{Field: sc.Field, Op: sc.Op, Value: sc.Value}, nil
	case snapshotTypeSet:
		return parser.SetCondition{Field: sc.Field, Op: sc.Op, Values: append([]string(nil), sc.Values...)}, nil
	case snapshotTypeRange:
		minV, err := strconv.ParseFloat(sc.Min, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: range min %q: %v", ErrSnapshotMalformed, sc.Min, err)
		}
		maxV, err := strconv.ParseFloat(sc.Max, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: range max %q: %v", ErrSnapshotMalformed, sc.Max, err)
		}
		return parser.RangeCondition{Field: sc.Field, Min: minV, Max: maxV}, nil
	case snapshotTypeAnd:
		children := make([]parser.Condition, 0, len(sc.Children))
		for _, ch := range sc.Children {
			dec, err := decodeCondition(ch)
			if err != nil {
				return nil, err
			}
			children = append(children, dec)
		}
		return parser.AndCondition{Children: children}, nil
	default:
		return nil, fmt.Errorf("%w: unknown type %q", ErrSnapshotMalformed, sc.Type)
	}
}
