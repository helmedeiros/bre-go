package indexed

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/helmedeiros/bre-go/engine/parser"
)

// Snapshot v2 -- the v0.16.0 binary path documented in ADR-0041.
//
// Two intermediate forms, both validated by the harness in
// scientific/v0.15.0/experimental/ before promotion.
//
//   FieldValueSet + PreClassifiedRule + AddPreClassifiedRule:
//     skips the parser.Condition walk and validation that AddRule
//     performs. Caller has already split the match into bucket-key
//     contributors and post-filter terms; the engine runs cartesian
//     fan-out and bucket insertion only.
//
//   CompiledSnapshot + ExportCompiledSnapshot + LoadCompiledSnapshot:
//     skips AddRule entirely. The caller hands over an already-bucketed
//     state and the engine atomic-stores it as its sealed snapshot.
//     No fan-out, no insertion, no Build. Pair with
//     MarshalCompiledSnapshot / UnmarshalCompiledSnapshot for the
//     v0.16.0 binary wire format.

// FieldValueSet is a single bucket-key contributor: one field, the
// canonicalized set of values that admit it. v0.16.0 candidate API.
type FieldValueSet struct {
	Field  string
	Values []string
}

// PreClassifiedRule is a Rule after AddRule's classification step has
// already run: indexable terms split into Sets, non-indexable terms
// kept as the original parser.Conditions in PostFilter. Values inside
// Sets must be canonicalized (sorted, deduped, non-empty); the loader
// trusts the caller to have done this.
type PreClassifiedRule struct {
	Name          string
	Description   string
	Tags          []string
	Sets          []FieldValueSet
	PostFilter    []parser.Condition
	Action        func(input interface{}) interface{}
	ActionContext func(ctx context.Context, input interface{}) interface{}
}

// ExportPreClassifiedRules walks the engine's rules in insertion order
// and returns each rule's already-classified shape. Triggers implicit
// Build. Refuses hook-bearing engines (ErrSnapshotIncompatibleHook).
func (e *Engine) ExportPreClassifiedRules() ([]PreClassifiedRule, error) {
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
	out := make([]PreClassifiedRule, 0, len(s.rulesInOrder))
	for _, r := range s.rulesInOrder {
		sets, post, _ := extractIndexablePairs(r.Match, nil)
		fvs := make([]FieldValueSet, len(sets))
		for i, s := range sets {
			fvs[i] = FieldValueSet{Field: s.field, Values: append([]string(nil), s.values...)}
		}
		out = append(out, PreClassifiedRule{
			Name:          r.Name,
			Description:   r.Description,
			Tags:          copyTags(r.Tags),
			Sets:          fvs,
			PostFilter:    post,
			Action:        r.Action,
			ActionContext: r.ActionContext,
		})
	}
	return out, nil
}

// AddPreClassifiedRule inserts a rule into a builder-phase engine
// without running extractIndexablePairs. Validation, fan-out, and
// bucket insertion still run. Returns ErrEmptyRuleName,
// ErrNoIndexableTerms, ErrDuplicateRuleName, ErrEngineBuilt, or
// *FanoutTooLargeError.
func (e *Engine) AddPreClassifiedRule(r PreClassifiedRule) error {
	if r.Name == "" {
		return ErrEmptyRuleName
	}
	if len(r.Sets) == 0 {
		return ErrNoIndexableTerms
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.builder == nil {
		return ErrEngineBuilt
	}
	b := e.builder
	if _, dup := b.ruleNames[r.Name]; dup {
		return ErrDuplicateRuleName
	}

	sets := make([]fieldValueSet, len(r.Sets))
	for i, s := range r.Sets {
		sets[i] = fieldValueSet{field: s.Field, values: s.Values}
	}
	fanout := cartesianFanout(sets)
	if fanout > maxFanout {
		return &FanoutTooLargeError{Rule: r.Name, Cardinality: fanout, Limit: maxFanout}
	}

	sort.Slice(sets, func(i, j int) bool { return sets[i].field < sets[j].field })

	fields := make([]string, len(sets))
	for i, s := range sets {
		fields[i] = s.field
	}
	keysetID := strings.Join(fields, unitSeparator)

	bucket, ok := b.buckets[keysetID]
	if !ok {
		bucket = &keysetBucket{fields: fields, byValueKey: map[string][]indexedRule{}}
		b.buckets[keysetID] = bucket
		b.keysetOrder = append(b.keysetOrder, keysetID)
	}

	b.rulesInOrder = append(b.rulesInOrder, Rule{
		Name:          r.Name,
		Description:   r.Description,
		Tags:          r.Tags,
		Action:        r.Action,
		ActionContext: r.ActionContext,
	})
	ir := indexedRule{
		name:       r.Name,
		action:     r.Action,
		ctxAct:     r.ActionContext,
		postFilter: r.PostFilter,
	}

	enumerateCombinations(sets, func(combo []fieldValuePair) {
		vk := buildValueKey(combo)
		bucket.byValueKey[vk] = append(bucket.byValueKey[vk], ir)
	})

	b.ruleNames[r.Name] = struct{}{}
	return nil
}

// CompiledRuleRef is a per-bucket entry: the rule's name + the typed
// post-filter conditions that must Eval against the input.
type CompiledRuleRef struct {
	Name       string
	PostFilter []parser.Condition
}

// CompiledBucket is one keyset's bucket: the sorted field list that
// identifies the keyset, plus the value-key -> rule-refs mapping.
type CompiledBucket struct {
	Fields     []string
	ByValueKey map[string][]CompiledRuleRef
}

// CompiledSnapshot is the fully-bucketed engine state, ready for
// atomic-store directly into the engine's sealed snapshot value.
// LoadCompiledSnapshot performs no AddRule, no fan-out, no Build.
type CompiledSnapshot struct {
	KeysetOrder  []string
	Buckets      map[string]CompiledBucket
	RulesInOrder []SnapshotRule
}

// ExportCompiledSnapshot returns the engine's compiled state in the
// CompiledSnapshot form. Triggers implicit Build. Refuses hook-bearing
// engines.
func (e *Engine) ExportCompiledSnapshot() (*CompiledSnapshot, error) {
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
	out := &CompiledSnapshot{
		KeysetOrder:  append([]string(nil), s.keysetOrder...),
		Buckets:      make(map[string]CompiledBucket, len(s.buckets)),
		RulesInOrder: make([]SnapshotRule, 0, len(s.rulesInOrder)),
	}
	for keyset, bucket := range s.buckets {
		cb := CompiledBucket{
			Fields:     append([]string(nil), bucket.fields...),
			ByValueKey: make(map[string][]CompiledRuleRef, len(bucket.byValueKey)),
		}
		for vk, rules := range bucket.byValueKey {
			refs := make([]CompiledRuleRef, len(rules))
			for i, r := range rules {
				refs[i] = CompiledRuleRef{
					Name:       r.name,
					PostFilter: append([]parser.Condition(nil), r.postFilter...),
				}
			}
			cb.ByValueKey[vk] = refs
		}
		out.Buckets[keyset] = cb
	}
	for _, r := range s.rulesInOrder {
		out.RulesInOrder = append(out.RulesInOrder, SnapshotRule{
			Name:        r.Name,
			Description: r.Description,
			Tags:        copyTags(r.Tags),
			Match:       encodeCondition(r.Match),
		})
	}
	return out, nil
}

// LoadCompiledSnapshot reconstructs an engine from a CompiledSnapshot.
// The returned engine is already Built. No AddRule runs. rebuild may
// be nil; matching rules pick up their callbacks by name.
func LoadCompiledSnapshot(cs *CompiledSnapshot, rebuild map[string]RuleCallbacks) (*Engine, error) {
	if cs == nil {
		return nil, errors.New("indexed: LoadCompiledSnapshot called with nil snapshot")
	}
	e := New()

	rulesInOrder := make([]Rule, 0, len(cs.RulesInOrder))
	for _, sr := range cs.RulesInOrder {
		match, err := decodeCondition(sr.Match)
		if err != nil {
			return nil, err
		}
		cb := rebuild[sr.Name]
		rulesInOrder = append(rulesInOrder, Rule{
			Name:          sr.Name,
			Description:   sr.Description,
			Tags:          copyTags(sr.Tags),
			Match:         match,
			Action:        cb.Action,
			ActionContext: cb.ActionContext,
		})
	}

	buckets := make(map[string]*keysetBucket, len(cs.Buckets))
	for keyset, cb := range cs.Buckets {
		bucket := &keysetBucket{
			fields:     append([]string(nil), cb.Fields...),
			byValueKey: make(map[string][]indexedRule, len(cb.ByValueKey)),
		}
		for vk, refs := range cb.ByValueKey {
			entries := make([]indexedRule, len(refs))
			for i, ref := range refs {
				cb := rebuild[ref.Name]
				entries[i] = indexedRule{
					name:       ref.Name,
					action:     cb.Action,
					ctxAct:     cb.ActionContext,
					postFilter: append([]parser.Condition(nil), ref.PostFilter...),
				}
			}
			bucket.byValueKey[vk] = entries
		}
		buckets[keyset] = bucket
	}

	e.mu.Lock()
	e.snapshot.Store(&snapshot{
		buckets:      buckets,
		keysetOrder:  append([]string(nil), cs.KeysetOrder...),
		rulesInOrder: rulesInOrder,
	})
	e.builder = nil
	e.mu.Unlock()
	return e, nil
}
