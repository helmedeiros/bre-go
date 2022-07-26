// Package indexed is a sub-linear engine.Engine adapter. Rules use
// parser.Condition trees over a fact-map input; the adapter buckets
// rules by (key-set, value-tuple) and resolves Execute in O(K) hash
// lookups, where K is the number of distinct key-sets registered.
package indexed

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/internal/adapter"
	"github.com/helmedeiros/bre-go/engine/parser"
)

// ASCII unit separator -- safe to join field names and values without collision.
const unitSeparator = "\x1f"

// maxFanout caps OpIn Cartesian expansion at AddRule. Rules exceeding
// it return *FanoutTooLargeError so the caller fixes the shape
// rather than the engine eating unbounded memory.
const maxFanout = 1024

// Rule is the indexed adapter's rule. Description and Tags surface via
// engine.RuleInfoLister and do not influence matching.
type Rule struct {
	Name          string
	Description   string
	Tags          []string
	Match         parser.Condition
	Action        func(input interface{}) interface{}
	ActionContext func(ctx context.Context, input interface{}) interface{}
}

// New returns an empty engine in the builder phase.
func New() *Engine {
	return &Engine{
		builder: newBuilderState(),
	}
}

func newBuilderState() *builderState {
	return &builderState{
		buckets:   map[string]*keysetBucket{},
		ruleNames: map[string]struct{}{},
	}
}

// Engine is the indexed adapter. First-match semantics; insertion
// order breaks ties. Builder phase mutates under mu; built phase
// reads lockless from an atomic.Value snapshot.
type Engine struct {
	adapter.Notifier

	mu       sync.Mutex
	builder  *builderState
	snapshot atomic.Value
}

type builderState struct {
	buckets        map[string]*keysetBucket
	keysetOrder    []string
	ruleNames      map[string]struct{}
	rulesInOrder   []Rule
	postFilterHook PostFilterHook
}

type snapshot struct {
	buckets        map[string]*keysetBucket
	keysetOrder    []string
	rulesInOrder   []Rule
	postFilterHook PostFilterHook
}

// Build seals the builder into an immutable snapshot. Subsequent
// AddRule returns ErrEngineBuilt; subsequent Build returns
// ErrAlreadyBuilt; Execute becomes safe for concurrent calls.
func (e *Engine) Build() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.builder == nil {
		return ErrAlreadyBuilt
	}
	e.snapshot.Store(e.sealLocked())
	return nil
}

// Built reports whether Build (explicit or implicit) has finalized the engine.
func (e *Engine) Built() bool {
	return e.snapshot.Load() != nil
}

func (e *Engine) sealLocked() *snapshot {
	b := e.builder
	s := &snapshot{
		buckets:        b.buckets,
		keysetOrder:    b.keysetOrder,
		rulesInOrder:   b.rulesInOrder,
		postFilterHook: b.postFilterHook,
	}
	e.builder = nil
	return s
}

func (e *Engine) readSnapshot() *snapshot {
	if v := e.snapshot.Load(); v != nil {
		return v.(*snapshot)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	// Another goroutine may have sealed while we waited for the lock.
	if v := e.snapshot.Load(); v != nil {
		return v.(*snapshot)
	}
	s := e.sealLocked()
	e.snapshot.Store(s)
	return s
}

// PostFilterHook classifies Conditions the built-in classifier does
// not recognize. Returns true to admit the Condition as a post-filter.
type PostFilterHook func(c parser.Condition) (handled bool)

// WithPostFilterHook installs h. Panics if called on a built engine.
func (e *Engine) WithPostFilterHook(h PostFilterHook) *Engine {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.builder == nil {
		panic("indexed: WithPostFilterHook called on a built engine")
	}
	e.builder.postFilterHook = h
	return e
}

type keysetBucket struct {
	fields     []string
	byValueKey map[string][]indexedRule
}

type indexedRule struct {
	name       string
	action     func(input interface{}) interface{}
	ctxAct     func(ctx context.Context, input interface{}) interface{}
	postFilter []parser.Condition
}

// AddRule registers r. Returns one of: ErrEmptyRuleName, ErrNilMatch,
// ErrEngineBuilt, ErrNonIndexableCondition, ErrNoIndexableTerms,
// ErrDuplicateRuleName, *FanoutTooLargeError. Shape checks first,
// engine-state checks second.
func (e *Engine) AddRule(r Rule) error {
	if r.Name == "" {
		return ErrEmptyRuleName
	}
	if r.Match == nil {
		return ErrNilMatch
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.builder == nil {
		return ErrEngineBuilt
	}
	b := e.builder

	sets, postFilter, err := extractIndexablePairs(r.Match, b.postFilterHook)
	if err != nil {
		return err
	}
	if len(sets) == 0 {
		return ErrNoIndexableTerms
	}
	fanout := cartesianFanout(sets)
	if fanout > maxFanout {
		return &FanoutTooLargeError{Rule: r.Name, Cardinality: fanout, Limit: maxFanout}
	}
	if _, dup := b.ruleNames[r.Name]; dup {
		return ErrDuplicateRuleName
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

	b.rulesInOrder = append(b.rulesInOrder, r)
	ir := indexedRule{
		name:       r.Name,
		action:     r.Action,
		ctxAct:     r.ActionContext,
		postFilter: postFilter,
	}

	enumerateCombinations(sets, func(combo []fieldValuePair) {
		vk := buildValueKey(combo)
		bucket.byValueKey[vk] = append(bucket.byValueKey[vk], ir)
	})

	b.ruleNames[r.Name] = struct{}{}
	return nil
}

// RuleNames returns rule names in insertion order. Fresh copy.
func (e *Engine) RuleNames() []string {
	rules := e.rulesView()
	names := make([]string, len(rules))
	for i, r := range rules {
		names[i] = r.Name
	}
	return names
}

// RuleInfos returns rule metadata in insertion order. Tags are
// deep-copied so callers can mutate without affecting engine state.
func (e *Engine) RuleInfos() []engine.RuleInfo {
	rules := e.rulesView()
	infos := make([]engine.RuleInfo, len(rules))
	for i, r := range rules {
		infos[i] = engine.RuleInfo{
			Name:        r.Name,
			Description: r.Description,
			Tags:        copyTags(r.Tags),
		}
	}
	return infos
}

// rulesView returns the current rules slice without forcing implicit Build.
func (e *Engine) rulesView() []Rule {
	e.mu.Lock()
	defer e.mu.Unlock()
	if v := e.snapshot.Load(); v != nil {
		return v.(*snapshot).rulesInOrder
	}
	return e.builder.rulesInOrder
}

func copyTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	out := make([]string, len(tags))
	copy(out, tags)
	return out
}

// Execute returns the first matching rule. Input must be
// map[string]string or map[string]interface{}. Nil ctx is treated as
// context.Background(). Safe for concurrent calls after Build (or
// after the first Execute, which triggers implicit Build).
func (e *Engine) Execute(ctx context.Context, req engine.Request) (engine.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()

	snap := e.readSnapshot()

	fact, err := coerceInput(req.Input)
	if err != nil {
		e.NotifyErrored(req.Input, err)
		e.NotifyFinished(req.Input, nil, nil, time.Since(start))
		return engine.Result{}, err
	}

	e.NotifyStarted(req.Input)

	for _, keysetID := range snap.keysetOrder {
		if err := ctx.Err(); err != nil {
			e.NotifyErrored(req.Input, err)
			e.NotifyFinished(req.Input, nil, nil, time.Since(start))
			return engine.Result{}, err
		}

		bucket := snap.buckets[keysetID]
		valueKey, complete := projectFact(fact, bucket.fields)
		if !complete {
			continue
		}

		candidates := bucket.byValueKey[valueKey]
		var factMap map[string]interface{} // lazy-built only when needed
		for _, cand := range candidates {
			if len(cand.postFilter) > 0 {
				if factMap == nil {
					factMap = factToInterfaceMap(fact)
				}
				if !passesPostFilter(cand.postFilter, factMap) {
					continue
				}
			}
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

// extractIndexablePairs splits Match into bucket-key contributors
// (OpEq, OpIn) and post-filter terms (OpNeq, OpNotIn, RangeCondition,
// anything the hook claims). Returns ErrNonIndexableCondition for
// unrecognized shapes, empty value sets, or duplicate fields.
func extractIndexablePairs(c parser.Condition, hook PostFilterHook) ([]fieldValueSet, []parser.Condition, error) {
	var sets []fieldValueSet
	var postFilter []parser.Condition
	if err := collectSets(c, &sets, &postFilter, hook); err != nil {
		return nil, nil, err
	}
	if len(sets) == 0 && len(postFilter) == 0 {
		return nil, nil, ErrNonIndexableCondition
	}
	seen := make(map[string]struct{}, len(sets))
	for i, s := range sets {
		if _, dup := seen[s.field]; dup {
			return nil, nil, ErrNonIndexableCondition
		}
		seen[s.field] = struct{}{}
		canonical := canonicalizeValues(s.values)
		if len(canonical) == 0 {
			return nil, nil, ErrNonIndexableCondition
		}
		sets[i].values = canonical
	}
	return sets, postFilter, nil
}

func collectSets(c parser.Condition, sets *[]fieldValueSet, post *[]parser.Condition, hook PostFilterHook) error {
	switch v := c.(type) {
	case parser.StringCondition:
		return classifyStringCondition(v, sets, post)
	case *parser.StringCondition:
		return classifyStringCondition(*v, sets, post)
	case parser.SetCondition:
		return classifySetCondition(v, sets, post)
	case *parser.SetCondition:
		return classifySetCondition(*v, sets, post)
	case parser.RangeCondition:
		*post = append(*post, v)
		return nil
	case *parser.RangeCondition:
		*post = append(*post, *v)
		return nil
	case parser.AndCondition:
		for _, child := range v.Children {
			if err := collectSets(child, sets, post, hook); err != nil {
				return err
			}
		}
		return nil
	case *parser.AndCondition:
		for _, child := range v.Children {
			if err := collectSets(child, sets, post, hook); err != nil {
				return err
			}
		}
		return nil
	default:
		if hook != nil && hook(c) {
			*post = append(*post, c)
			return nil
		}
		return ErrNonIndexableCondition
	}
}

func classifyStringCondition(v parser.StringCondition, sets *[]fieldValueSet, post *[]parser.Condition) error {
	switch v.Op {
	case parser.OpEq:
		*sets = append(*sets, fieldValueSet{field: v.Field, values: []string{v.Value}})
		return nil
	case parser.OpNeq:
		*post = append(*post, v)
		return nil
	default:
		return ErrNonIndexableCondition
	}
}

func classifySetCondition(v parser.SetCondition, sets *[]fieldValueSet, post *[]parser.Condition) error {
	switch v.Op {
	case parser.OpIn:
		*sets = append(*sets, fieldValueSet{field: v.Field, values: v.Values})
		return nil
	case parser.OpNotIn:
		*post = append(*post, v)
		return nil
	default:
		return ErrNonIndexableCondition
	}
}

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

// cartesianFanout short-circuits past maxFanout to avoid scratch
// allocation for rejected pathological shapes.
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

// enumerateCombinations reuses combo between calls; visit must copy
// if retention is required (buildValueKey already copies into a string).
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

// projectFact returns ("", false) if any required field is missing.
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

// coerceInput accepts map[string]string or map[string]interface{};
// the latter is stringified via fmt.
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

func passesPostFilter(filter []parser.Condition, fact map[string]interface{}) bool {
	for _, c := range filter {
		if !c.Eval(fact) {
			return false
		}
	}
	return true
}

func factToInterfaceMap(fact map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(fact))
	for k, v := range fact {
		out[k] = v
	}
	return out
}
