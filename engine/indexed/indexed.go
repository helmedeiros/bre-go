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
	"sync"
	"sync/atomic"
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

// New returns an empty engine in the builder phase. Call AddRule to
// register rules; call Build to seal (or let the first Execute call
// implicitly seal). See ADR-0037 for the lifecycle.
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
// order breaks ties among rules in the same bucket (per ADR-0019).
//
// Concurrency model (ADR-0037): two phases.
//   - Builder phase: AddRule and WithPostFilterHook mutate the
//     engine's builder state under mu. Not safe for concurrent
//     Execute. Single-threaded "construct then use" pattern.
//   - Built phase: Build seals the builder into an immutable
//     snapshot held in an atomic.Value. Execute is lockless and
//     safe for arbitrary concurrent callers. AddRule returns
//     ErrEngineBuilt; WithPostFilterHook panics.
//
// An engine that never calls Build explicitly transitions on its
// first Execute call (implicit Build under mu, then atomic Store).
// Backward-compatible with v0.8.0–v0.11.0 callers.
type Engine struct {
	adapter.Notifier // promotes AddListener + Notify* (ADR-0029)

	mu       sync.Mutex // protects builder during the build phase
	builder  *builderState
	snapshot atomic.Value // holds *snapshot once Build (explicit or implicit) runs
}

// builderState carries the mutable rule-set under construction.
// Reads on this struct must hold the engine's mu; the struct is
// discarded once Build runs.
type builderState struct {
	buckets        map[string]*keysetBucket
	keysetOrder    []string
	ruleNames      map[string]struct{}
	rulesInOrder   []Rule
	postFilterHook PostFilterHook
}

// snapshot is the immutable read-side representation populated by
// Build. Held in Engine.snapshot via atomic.Value; Execute reads
// it lockless.
type snapshot struct {
	buckets        map[string]*keysetBucket
	keysetOrder    []string
	rulesInOrder   []Rule
	postFilterHook PostFilterHook
}

// Build finalizes the engine: the current builder state becomes the
// immutable snapshot that subsequent Execute calls read. Returns
// ErrAlreadyBuilt if called twice.
//
// After Build, AddRule returns ErrEngineBuilt and WithPostFilterHook
// panics. Execute is then safe for concurrent calls and runs
// lockless against the snapshot.
//
// Callers who care about deterministic seal timing (and want to
// avoid paying the build cost on a request) call Build explicitly
// during startup. Callers who don't care let the first Execute
// trigger Build implicitly. See ADR-0037.
func (e *Engine) Build() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.builder == nil {
		return ErrAlreadyBuilt
	}
	e.snapshot.Store(e.sealLocked())
	return nil
}

// Built reports whether Build (or an implicit Build on Execute)
// has finalized the engine. Useful for tests and lifecycle
// assertions.
func (e *Engine) Built() bool {
	return e.snapshot.Load() != nil
}

// sealLocked moves the builder state into a fresh snapshot. Caller
// must hold e.mu.
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

// readSnapshot returns the current snapshot, performing an
// implicit Build if one hasn't happened yet. Lockless on the hot
// path; takes mu only on the first call to perform the seal.
func (e *Engine) readSnapshot() *snapshot {
	if v := e.snapshot.Load(); v != nil {
		return v.(*snapshot)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	// Recheck under mu in case another goroutine raced us to Build.
	if v := e.snapshot.Load(); v != nil {
		return v.(*snapshot)
	}
	s := e.sealLocked()
	e.snapshot.Store(s)
	return s
}

// PostFilterHook is a caller-supplied classifier for typed
// parser.Conditions the indexed adapter does not natively recognize
// (the built-in StringCondition / SetCondition / RangeCondition
// shapes). When the hook returns true, the adapter treats the
// Condition as a post-filter (appended to indexedRule.postFilter
// and Eval'd against the input at Execute time).
//
// The hook is consulted only AFTER the built-in classifier has
// declined; it cannot override built-in behavior. See ADR-0036.
type PostFilterHook func(c parser.Condition) (handled bool)

// WithPostFilterHook installs h on the engine. Subsequent AddRule
// calls consult the hook before returning ErrNonIndexableCondition
// for an unknown shape. Returns the engine to allow method chaining.
//
// Only one hook is active at a time; calling WithPostFilterHook
// again replaces the previous hook. The hook does not affect
// already-registered rules (those retain whatever classification
// they had at the time AddRule was called).
//
// Must be called before Build (or before the first Execute that
// would trigger implicit Build). Calling WithPostFilterHook on a
// built engine panics -- the alternative would silently no-op,
// hiding a programming error. See ADR-0037.
func (e *Engine) WithPostFilterHook(h PostFilterHook) *Engine {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.builder == nil {
		panic("indexed: WithPostFilterHook called on a built engine; install the hook before Build / first Execute")
	}
	e.builder.postFilterHook = h
	return e
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
//
// postFilter is the list of non-indexable terms (OpNeq, OpNotIn)
// that must Eval to true on the candidate input. Empty / nil when
// the rule is pure-indexable, in which case the Execute hot path
// skips the Eval entirely. Added in v0.10.0 per ADR-0035.
type indexedRule struct {
	name       string
	action     func(input interface{}) interface{}
	ctxAct     func(ctx context.Context, input interface{}) interface{}
	postFilter []parser.Condition
}

// AddRule registers r. Returns ErrEmptyRuleName / ErrNilMatch /
// ErrDuplicateRuleName / ErrNonIndexableCondition /
// ErrNoIndexableTerms / *FanoutTooLargeError depending on which
// shape invariant the rule violates. Checks run shape-first,
// state-second.
//
// Rule shapes:
//   - OpEq / OpIn children contribute to the bucket key
//     (Cartesian-product fan-out per ADR-0034).
//   - OpNeq / OpNotIn children become post-filters evaluated at
//     Execute time per ADR-0035.
//   - A rule must have >= 1 indexable child; pure-negation rules
//     return ErrNoIndexableTerms.
//   - Empty value sets, duplicate indexable fields, and fan-outs
//     exceeding maxFanout are rejected.
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

// RuleNames returns the names of every registered rule in insertion
// order. The returned slice is a fresh copy. Safe to call in either
// the builder or the built phase.
func (e *Engine) RuleNames() []string {
	rules := e.rulesView()
	names := make([]string, len(rules))
	for i, r := range rules {
		names[i] = r.Name
	}
	return names
}

// RuleInfos returns metadata for every registered rule in insertion
// order. Safe to call in either phase. Tags are defensively copied
// so callers can mutate the returned slice without affecting
// engine state.
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

// rulesView returns the current rules slice without forcing an
// implicit Build. Always takes mu so the pre-Build and post-Build
// branches are deterministically reachable (no race-only dead
// code). RuleNames / RuleInfos aren't on the hot path, so the
// mutex cost is fine.
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
//
// Concurrency (ADR-0037): Execute is safe for arbitrary concurrent
// callers after Build (or after the first Execute on an
// unsealed engine, which triggers an implicit Build under mu).
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
			// Input lacks a field this key-set requires -- no rule
			// in this bucket can match.
			continue
		}

		candidates := bucket.byValueKey[valueKey]
		var factMap map[string]interface{} // lazy-built for post-filter Eval
		for _, cand := range candidates {
			// Indexable part of the rule is already known to match by
			// virtue of the bucket invariant. If the rule carries a
			// post-filter (OpNeq / OpNotIn terms per ADR-0035), evaluate
			// it now; skip the rule if any term returns false.
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

// extractIndexablePairs walks Match and splits it into:
//   - indexable terms (OpEq StringCondition, OpIn SetCondition)
//     returned as []fieldValueSet for bucket-key construction.
//   - post-filter terms (OpNeq StringCondition, OpNotIn SetCondition,
//     RangeCondition, anything the caller-installed PostFilterHook
//     claims) returned verbatim as []parser.Condition for runtime
//     evaluation.
//
// Anything else (Or, Not, unrecognized custom shape with no hook,
// empty value sets, duplicate indexable field) returns
// ErrNonIndexableCondition.
//
// The values list of each indexable set is canonicalized: sorted
// and deduplicated. This keeps the keyset / valueKey computation
// stable regardless of source order.
//
// Pure-negation rules (zero indexable terms) return a populated
// post-filter list with an empty index slice; AddRule then surfaces
// ErrNoIndexableTerms. Splitting that decision out of the walker
// keeps the walker focused on classification.
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
		// Built-in classifier doesn't recognize this shape. Consult
		// the caller-installed hook (if any) per ADR-0036.
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

// passesPostFilter returns true iff every condition in filter
// evaluates to true against fact. Used by Execute after a bucket
// hit to apply non-indexable predicates (OpNeq / OpNotIn) per
// ADR-0035.
func passesPostFilter(filter []parser.Condition, fact map[string]interface{}) bool {
	for _, c := range filter {
		if !c.Eval(fact) {
			return false
		}
	}
	return true
}

// factToInterfaceMap converts the canonical map[string]string fact
// representation into the map[string]interface{} shape that
// parser.Condition.Eval requires. Built lazily inside Execute and
// only when at least one candidate has a post-filter, so rules
// without negation pay zero cost.
func factToInterfaceMap(fact map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(fact))
	for k, v := range fact {
		out[k] = v
	}
	return out
}
