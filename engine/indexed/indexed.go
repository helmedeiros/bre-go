// Package indexed is the first sub-linear engine.Engine adapter.
// Rules must be pure conjunctions of equality conditions (typed
// parser.Condition trees) over a fact-map input; the adapter buckets
// rules by (key-set, value-tuple) and resolves Execute via O(K) hash
// lookups where K is the number of distinct key sets registered.
//
// See ADR-0033 for the design rationale and ADR-0031's
// BENCHMARKS.md for the success bar versus the linear adapters.
package indexed

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/internal/adapter"
	"github.com/helmedeiros/bre-go/engine/parser"
)

// unitSeparator joins field names and value keys; the ASCII unit
// separator (0x1F) does not appear in normal fact strings, so it
// cannot collide with field names or values.
const unitSeparator = "\x1f"

// maxFanout caps the bucket-expansion cost of a single rule whose
// Match contains OpIn set-membership conditions. ADR-0034 §3 picked
// 1024 empirically; future ADR revises if a real workload runs into
// the cap. A rule exceeding it returns *FanoutTooLargeError at
// AddRule -- caller fixes the rule rather than the engine eating
// unbounded memory.
const maxFanout = 1024

// Rule is a typed-Condition rule for the indexed adapter. Description
// and Tags surface through engine.RuleInfoLister; they do not
// influence Execute. Match must be a pure conjunction of OpEq
// StringConditions.
//
// Condition and Action follow the same shape the other adapters use:
// Condition is the Match's compiled form; Action is the optional
// outcome closure; ActionContext is the context-aware variant.
type Rule struct {
	Name          string
	Description   string
	Tags          []string
	Match         parser.Condition
	Action        func(input interface{}) interface{}
	ActionContext func(ctx context.Context, input interface{}) interface{}
}

// New returns an empty engine.
func New() *Engine {
	return &Engine{
		buckets:     map[string]*keysetBucket{},
		keysetOrder: nil,
		ruleNames:   map[string]struct{}{},
	}
}

// Engine is the indexed adapter. First-match semantics; insertion
// order breaks ties among rules in the same bucket (per ADR-0019).
type Engine struct {
	adapter.Notifier // promotes AddListener + Notify* (ADR-0029)

	buckets     map[string]*keysetBucket
	keysetOrder []string // insertion order of first-seen key-set IDs
	ruleNames   map[string]struct{}

	// rulesInOrder mirrors AddRule order across all key-sets so
	// RuleInfos can return rules in registration order even when
	// buckets reorganize them.
	rulesInOrder []Rule
}

// keysetBucket holds every rule whose Match constrains exactly the
// fields in keysetID. Lookup is by the canonicalized value tuple.
type keysetBucket struct {
	fields     []string                 // sorted field names this key-set covers
	byValueKey map[string][]indexedRule // value-tuple → rules
}

// indexedRule is the per-bucket form of a Rule. It carries the
// minimum information Execute needs at lookup time; the full Rule
// (Description, Tags, etc.) lives in Engine.rulesInOrder.
type indexedRule struct {
	name   string
	action func(input interface{}) interface{}
	ctxAct func(ctx context.Context, input interface{}) interface{}
}

// AddRule registers r. Returns ErrEmptyRuleName / ErrNilMatch /
// ErrDuplicateRuleName / ErrNonIndexableCondition / *FanoutTooLargeError
// depending on which shape invariant the rule violates. Checks run
// shape-first, state-second.
//
// A rule whose Match contains OpIn set-membership conditions
// expands into a Cartesian product of bucket entries -- one per
// combination of values. Empty value sets, duplicate fields, and
// fan-outs exceeding maxFanout are rejected. See ADR-0034 §1 for
// the fan-out rationale.
func (e *Engine) AddRule(r Rule) error {
	if r.Name == "" {
		return ErrEmptyRuleName
	}
	if r.Match == nil {
		return ErrNilMatch
	}
	sets, err := extractIndexablePairs(r.Match)
	if err != nil {
		return err
	}
	fanout := cartesianFanout(sets)
	if fanout > maxFanout {
		return &FanoutTooLargeError{Rule: r.Name, Cardinality: fanout, Limit: maxFanout}
	}
	if _, dup := e.ruleNames[r.Name]; dup {
		return ErrDuplicateRuleName
	}

	sort.Slice(sets, func(i, j int) bool { return sets[i].field < sets[j].field })

	fields := make([]string, len(sets))
	for i, s := range sets {
		fields[i] = s.field
	}
	keysetID := strings.Join(fields, unitSeparator)

	bucket, ok := e.buckets[keysetID]
	if !ok {
		bucket = &keysetBucket{fields: fields, byValueKey: map[string][]indexedRule{}}
		e.buckets[keysetID] = bucket
		e.keysetOrder = append(e.keysetOrder, keysetID)
	}

	e.rulesInOrder = append(e.rulesInOrder, r)
	ir := indexedRule{name: r.Name, action: r.Action, ctxAct: r.ActionContext}

	enumerateCombinations(sets, func(combo []fieldValuePair) {
		vk := buildValueKey(combo)
		bucket.byValueKey[vk] = append(bucket.byValueKey[vk], ir)
	})

	e.ruleNames[r.Name] = struct{}{}
	return nil
}

// RuleNames returns the names of every registered rule in insertion
// order. The returned slice is a fresh copy.
func (e *Engine) RuleNames() []string {
	names := make([]string, len(e.rulesInOrder))
	for i, r := range e.rulesInOrder {
		names[i] = r.Name
	}
	return names
}

// RuleInfos returns metadata for every registered rule in insertion
// order. Mirror of the other adapters' implementation.
func (e *Engine) RuleInfos() []engine.RuleInfo {
	infos := make([]engine.RuleInfo, len(e.rulesInOrder))
	for i, r := range e.rulesInOrder {
		infos[i] = engine.RuleInfo{
			Name:        r.Name,
			Description: r.Description,
			Tags:        copyTags(r.Tags),
		}
	}
	return infos
}

func copyTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	out := make([]string, len(tags))
	copy(out, tags)
	return out
}

// Execute resolves a single matching rule via indexed lookup.
//
//  1. Coerce req.Input to a fact map (map[string]string preferred;
//     map[string]interface{} stringified via fmt).
//  2. For each registered key-set in insertion order, project the
//     fact onto that key-set, look up the bucket, walk the
//     candidates in insertion order, return on first match.
//
// A nil ctx is treated as context.Background(). ctx.Err() is checked
// at the top and between key-sets.
func (e *Engine) Execute(ctx context.Context, req engine.Request) (engine.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()

	fact, err := coerceInput(req.Input)
	if err != nil {
		e.NotifyErrored(req.Input, err)
		e.NotifyFinished(req.Input, nil, nil, time.Since(start))
		return engine.Result{}, err
	}

	e.NotifyStarted(req.Input)

	for _, keysetID := range e.keysetOrder {
		if err := ctx.Err(); err != nil {
			e.NotifyErrored(req.Input, err)
			e.NotifyFinished(req.Input, nil, nil, time.Since(start))
			return engine.Result{}, err
		}

		bucket := e.buckets[keysetID]
		valueKey, complete := projectFact(fact, bucket.fields)
		if !complete {
			// Input lacks a field this key-set requires -- no rule
			// in this bucket can match.
			continue
		}

		candidates := bucket.byValueKey[valueKey]
		for _, cand := range candidates {
			// No runtime Eval here: the AddRule contract (pure OpEq
			// conjunction, no duplicate fields) means every rule in
			// this bucket matches the value key we just probed.
			// Future ADRs that widen the indexable shape re-introduce
			// the check with their own coverage.
			out := engine.Result{Matched: []string{cand.name}}
			if cand.action != nil || cand.ctxAct != nil {
				output, panicErr := runAction(ctx, cand, req.Input)
				if panicErr != nil {
					e.NotifyErrored(req.Input, panicErr)
					e.NotifyFinished(req.Input, out.Output, out.Matched, time.Since(start))
					return out, panicErr
				}
				out.Output = output
			}
			e.NotifyMatched(cand.name, req.Input, out.Output)
			e.NotifyFinished(req.Input, out.Output, out.Matched, time.Since(start))
			return out, nil
		}
	}

	e.NotifyFinished(req.Input, nil, nil, time.Since(start))
	return engine.Result{}, nil
}

func runAction(ctx context.Context, r indexedRule, input interface{}) (output interface{}, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = &ActionPanicError{Rule: r.name, Value: rec}
		}
	}()
	if r.ctxAct != nil {
		output = r.ctxAct(ctx, input)
	} else {
		output = r.action(input)
	}
	return output, nil
}

// fieldValuePair is a single (field, value) pair -- the "flat" form
// used as a step in building a bucket value key.
type fieldValuePair struct {
	field string
	value string
}

// fieldValueSet groups a field with the set of values it constrains.
// Length-1 values is the OpEq shape; length-N is OpIn.
type fieldValueSet struct {
	field  string
	values []string
}

// extractIndexablePairs walks Match and returns one fieldValueSet
// per constrained field. Match must be a conjunction whose children
// are each an OpEq StringCondition or an OpIn SetCondition. Anything
// else (OpNeq, OpNotIn, Or, Not, range predicates, custom shapes,
// empty value sets, duplicate fields) returns ErrNonIndexableCondition.
//
// The values list of each returned set is canonicalized: sorted and
// deduplicated. This keeps the keyset / valueKey computation stable
// regardless of the source order.
func extractIndexablePairs(c parser.Condition) ([]fieldValueSet, error) {
	var out []fieldValueSet
	if err := collectSets(c, &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, ErrNonIndexableCondition
	}
	seen := make(map[string]struct{}, len(out))
	for i, s := range out {
		if _, dup := seen[s.field]; dup {
			return nil, ErrNonIndexableCondition
		}
		seen[s.field] = struct{}{}
		canonical := canonicalizeValues(s.values)
		if len(canonical) == 0 {
			return nil, ErrNonIndexableCondition
		}
		out[i].values = canonical
	}
	return out, nil
}

func collectSets(c parser.Condition, out *[]fieldValueSet) error {
	switch v := c.(type) {
	case parser.StringCondition:
		if v.Op != parser.OpEq {
			return ErrNonIndexableCondition
		}
		*out = append(*out, fieldValueSet{field: v.Field, values: []string{v.Value}})
		return nil
	case *parser.StringCondition:
		if v.Op != parser.OpEq {
			return ErrNonIndexableCondition
		}
		*out = append(*out, fieldValueSet{field: v.Field, values: []string{v.Value}})
		return nil
	case parser.SetCondition:
		if v.Op != parser.OpIn {
			return ErrNonIndexableCondition
		}
		*out = append(*out, fieldValueSet{field: v.Field, values: v.Values})
		return nil
	case *parser.SetCondition:
		if v.Op != parser.OpIn {
			return ErrNonIndexableCondition
		}
		*out = append(*out, fieldValueSet{field: v.Field, values: v.Values})
		return nil
	case parser.AndCondition:
		for _, child := range v.Children {
			if err := collectSets(child, out); err != nil {
				return err
			}
		}
		return nil
	case *parser.AndCondition:
		for _, child := range v.Children {
			if err := collectSets(child, out); err != nil {
				return err
			}
		}
		return nil
	default:
		return ErrNonIndexableCondition
	}
}

// canonicalizeValues returns a sorted, deduplicated copy of values.
// Empty input returns nil so the empty-set rejection in
// extractIndexablePairs catches it.
func canonicalizeValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sorted := make([]string, len(values))
	copy(sorted, values)
	sort.Strings(sorted)
	n := 1
	for i := 1; i < len(sorted); i++ {
		if sorted[i] != sorted[i-1] {
			sorted[n] = sorted[i]
			n++
		}
	}
	return sorted[:n]
}

// cartesianFanout returns the number of bucket entries the rule will
// produce -- the product of len(values) across all sets. Short-
// circuits once the product crosses maxFanout so a pathological
// rule does not allocate proportional scratch state before the
// rejection.
func cartesianFanout(sets []fieldValueSet) int {
	n := 1
	for _, s := range sets {
		n *= len(s.values)
		if n > maxFanout {
			return n
		}
	}
	return n
}

// enumerateCombinations calls visit once per Cartesian-product
// element across sets. The combo slice is reused between calls --
// callers that need to retain the slice must copy it inside visit
// (buildValueKey copies into a string already, so it's fine).
func enumerateCombinations(sets []fieldValueSet, visit func(combo []fieldValuePair)) {
	combo := make([]fieldValuePair, len(sets))
	for i, s := range sets {
		combo[i].field = s.field
	}
	var recurse func(int)
	recurse = func(i int) {
		if i == len(sets) {
			visit(combo)
			return
		}
		for _, v := range sets[i].values {
			combo[i].value = v
			recurse(i + 1)
		}
	}
	recurse(0)
}

// buildValueKey canonicalizes pairs (already sorted by field) into a
// single string suitable for map lookup. Format:
//
//	field0 \x1f value0 \x1f field1 \x1f value1 ...
//
// Including the field names makes the value key self-describing and
// guards against the (rare) case of two key-sets sharing a value
// permutation.
func buildValueKey(pairs []fieldValuePair) string {
	var b strings.Builder
	for i, p := range pairs {
		if i > 0 {
			b.WriteString(unitSeparator)
		}
		b.WriteString(p.field)
		b.WriteString(unitSeparator)
		b.WriteString(p.value)
	}
	return b.String()
}

// projectFact builds the value key for a fact restricted to fields.
// Returns complete=false if any required field is missing or carries
// a non-string value.
func projectFact(fact map[string]string, fields []string) (string, bool) {
	var b strings.Builder
	for i, f := range fields {
		v, ok := fact[f]
		if !ok {
			return "", false
		}
		if i > 0 {
			b.WriteString(unitSeparator)
		}
		b.WriteString(f)
		b.WriteString(unitSeparator)
		b.WriteString(v)
	}
	return b.String(), true
}

// coerceInput converts the engine's opaque Input into a fact map.
// map[string]string is the canonical shape; map[string]interface{}
// is accepted and stringified via fmt.Sprintf("%v", ...).
func coerceInput(in interface{}) (map[string]string, error) {
	switch v := in.(type) {
	case map[string]string:
		return v, nil
	case map[string]interface{}:
		out := make(map[string]string, len(v))
		for k, val := range v {
			if val == nil {
				continue
			}
			if s, ok := val.(string); ok {
				out[k] = s
				continue
			}
			out[k] = fmt.Sprintf("%v", val)
		}
		return out, nil
	default:
		return nil, ErrIncompatibleInput
	}
}
