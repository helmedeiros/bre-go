package indexed

import (
	"sort"

	"github.com/helmedeiros/bre-go/engine/parser"
)

// DiagnoseReport is the result of Engine.Diagnose.
type DiagnoseReport struct {
	DeadRules []DeadRule
}

// DeadRule names a rule that can never fire because an earlier rule shadows it.
type DeadRule struct {
	Name       string
	ShadowedBy string
	Reason     string
}

const deadRuleReason = "every input matching this rule also matches an earlier, less-constrained rule"

// Diagnose returns rules that can never fire because an earlier rule
// shadows them. Earlier rules with post-filter terms are skipped
// (their firing isn't decidable from shape alone). Complexity is
// O(N^2 * F); call from startup validation, not per-request.
func (e *Engine) Diagnose() DiagnoseReport {
	rules := e.rulesView()
	if len(rules) < 2 {
		return DiagnoseReport{}
	}

	shapes := make([]ruleShape, len(rules))
	for i, r := range rules {
		shapes[i] = extractRuleShape(r.Match)
	}

	var dead []DeadRule
	for laterIdx := 1; laterIdx < len(rules); laterIdx++ {
		later := shapes[laterIdx]
		if later.unanalyzable {
			continue
		}
		for earlierIdx := 0; earlierIdx < laterIdx; earlierIdx++ {
			if shadows(shapes[earlierIdx], later) {
				dead = append(dead, DeadRule{
					Name:       rules[laterIdx].Name,
					ShadowedBy: rules[earlierIdx].Name,
					Reason:     deadRuleReason,
				})
				break
			}
		}
	}
	return DiagnoseReport{DeadRules: dead}
}

type ruleShape struct {
	fields        []string
	values        map[string][]string
	hasPostFilter bool
	unanalyzable  bool
}

func extractRuleShape(c parser.Condition) ruleShape {
	var sets []fieldValueSet
	var post []parser.Condition
	if err := collectSets(c, &sets, &post, nil); err != nil {
		return ruleShape{unanalyzable: true}
	}
	out := ruleShape{
		values:        make(map[string][]string, len(sets)),
		hasPostFilter: len(post) > 0,
	}
	out.fields = make([]string, len(sets))
	for i, s := range sets {
		out.fields[i] = s.field
		out.values[s.field] = canonicalizeValues(s.values)
	}
	sort.Strings(out.fields)
	return out
}

func shadows(earlier, later ruleShape) bool {
	if earlier.unanalyzable || later.unanalyzable || earlier.hasPostFilter {
		return false
	}
	for _, f := range earlier.fields {
		laterValues, ok := later.values[f]
		if !ok {
			return false
		}
		if !valueSubsetOf(laterValues, earlier.values[f]) {
			return false
		}
	}
	return true
}

// valueSubsetOf assumes both slices are canonicalized (sorted, deduped).
func valueSubsetOf(sub, sup []string) bool {
	if len(sub) > len(sup) {
		return false
	}
	i := 0
	for _, v := range sub {
		for i < len(sup) && sup[i] < v {
			i++
		}
		if i >= len(sup) || sup[i] != v {
			return false
		}
		i++
	}
	return true
}
