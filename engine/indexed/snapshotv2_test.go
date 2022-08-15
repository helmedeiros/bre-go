package indexed_test

import (
	"context"
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

func TestExportPreClassifiedRefusesHook(t *testing.T) {
	e := indexed.New().WithPostFilterHook(func(parser.Condition) bool { return false })
	_ = e.AddRule(indexed.Rule{Name: "r", Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"}})
	if _, err := e.ExportPreClassifiedRules(); !errors.Is(err, indexed.ErrSnapshotIncompatibleHook) {
		t.Fatalf("want ErrSnapshotIncompatibleHook, got %v", err)
	}
}

func TestExportPreClassifiedRefusesHookAfterBuild(t *testing.T) {
	e := indexed.New().WithPostFilterHook(func(parser.Condition) bool { return false })
	_ = e.AddRule(indexed.Rule{Name: "r", Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"}})
	_ = e.Build()
	if _, err := e.ExportPreClassifiedRules(); !errors.Is(err, indexed.ErrSnapshotIncompatibleHook) {
		t.Fatalf("want ErrSnapshotIncompatibleHook after Build, got %v", err)
	}
}

func TestPreClassifiedRoundTrip(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:        "br",
		Description: "brazilian",
		Tags:        []string{"geo"},
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.SetCondition{Field: "tier", Op: parser.OpNotIn, Values: []string{"corporate"}},
		}},
	})
	_ = orig.AddRule(indexed.Rule{
		Name:  "mercosul",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"AR", "UY"}},
	})

	pre, err := orig.ExportPreClassifiedRules()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	loaded := indexed.New()
	for _, r := range pre {
		if err := loaded.AddPreClassifiedRule(r); err != nil {
			t.Fatalf("AddPreClassifiedRule %q: %v", r.Name, err)
		}
	}
	if err := loaded.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, in := range []map[string]string{
		{"country": "BR", "tier": "consumer"},
		{"country": "BR", "tier": "corporate"},
		{"country": "AR", "tier": "consumer"},
		{"country": "UY", "tier": "consumer"},
		{"country": "DE", "tier": "consumer"},
	} {
		ro, err := orig.Execute(context.Background(), engine.Request{Input: in})
		if err != nil {
			t.Fatalf("orig Execute %v: %v", in, err)
		}
		rl, err := loaded.Execute(context.Background(), engine.Request{Input: in})
		if err != nil {
			t.Fatalf("loaded Execute %v: %v", in, err)
		}
		if !sameMatched(ro.Matched, rl.Matched) {
			t.Fatalf("input %v: orig=%v loaded=%v", in, ro.Matched, rl.Matched)
		}
	}
}

func TestAddPreClassifiedRuleValidation(t *testing.T) {
	e := indexed.New()
	if err := e.AddPreClassifiedRule(indexed.PreClassifiedRule{Name: ""}); !errors.Is(err, indexed.ErrEmptyRuleName) {
		t.Fatalf("want ErrEmptyRuleName, got %v", err)
	}
	if err := e.AddPreClassifiedRule(indexed.PreClassifiedRule{Name: "r"}); !errors.Is(err, indexed.ErrNoIndexableTerms) {
		t.Fatalf("want ErrNoIndexableTerms, got %v", err)
	}
	r := indexed.PreClassifiedRule{Name: "r", Sets: []indexed.FieldValueSet{{Field: "k", Values: []string{"v"}}}}
	if err := e.AddPreClassifiedRule(r); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := e.AddPreClassifiedRule(r); !errors.Is(err, indexed.ErrDuplicateRuleName) {
		t.Fatalf("want ErrDuplicateRuleName, got %v", err)
	}
}

func TestAddPreClassifiedRuleFanoutTooLarge(t *testing.T) {
	e := indexed.New()
	vals := make([]string, 33)
	for i := range vals {
		vals[i] = string(rune('a' + i))
	}
	r := indexed.PreClassifiedRule{
		Name: "big",
		Sets: []indexed.FieldValueSet{
			{Field: "a", Values: vals},
			{Field: "b", Values: vals},
		},
	}
	err := e.AddPreClassifiedRule(r)
	var fe *indexed.FanoutTooLargeError
	if !errors.As(err, &fe) {
		t.Fatalf("want FanoutTooLargeError, got %v", err)
	}
}

func TestAddPreClassifiedRuleSortsSetsByField(t *testing.T) {
	e := indexed.New()
	if err := e.AddPreClassifiedRule(indexed.PreClassifiedRule{
		Name: "r",
		Sets: []indexed.FieldValueSet{
			{Field: "z", Values: []string{"v"}},
			{Field: "a", Values: []string{"v"}},
		},
	}); err != nil {
		t.Fatalf("AddPreClassifiedRule: %v", err)
	}
	if err := e.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got, err := e.Execute(context.Background(), engine.Request{Input: map[string]string{"a": "v", "z": "v"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got.Matched) != 1 || got.Matched[0] != "r" {
		t.Fatalf("expected match on rule r, got %v", got.Matched)
	}
}

func TestLoadCompiledSnapshotMalformedRuleMatchErrors(t *testing.T) {
	cs := &indexed.CompiledSnapshot{
		KeysetOrder: []string{},
		Buckets:     map[string]indexed.CompiledBucket{},
		RulesInOrder: []indexed.SnapshotRule{{
			Name:  "r",
			Match: indexed.SnapshotCondition{Type: "no-such-type"},
		}},
	}
	if _, err := indexed.LoadCompiledSnapshot(cs, nil); !errors.Is(err, indexed.ErrSnapshotMalformed) {
		t.Fatalf("want ErrSnapshotMalformed for unknown Match type, got %v", err)
	}
}

func TestAddPreClassifiedRuleAfterBuild(t *testing.T) {
	e := indexed.New()
	_ = e.Build()
	err := e.AddPreClassifiedRule(indexed.PreClassifiedRule{
		Name: "r",
		Sets: []indexed.FieldValueSet{{Field: "k", Values: []string{"v"}}},
	})
	if !errors.Is(err, indexed.ErrEngineBuilt) {
		t.Fatalf("want ErrEngineBuilt, got %v", err)
	}
}

func TestExportCompiledRefusesHook(t *testing.T) {
	e := indexed.New().WithPostFilterHook(func(parser.Condition) bool { return false })
	_ = e.AddRule(indexed.Rule{Name: "r", Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"}})
	if _, err := e.ExportCompiledSnapshot(); !errors.Is(err, indexed.ErrSnapshotIncompatibleHook) {
		t.Fatalf("want ErrSnapshotIncompatibleHook, got %v", err)
	}
}

func TestExportCompiledRefusesHookAfterBuild(t *testing.T) {
	e := indexed.New().WithPostFilterHook(func(parser.Condition) bool { return false })
	_ = e.AddRule(indexed.Rule{Name: "r", Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"}})
	_ = e.Build()
	if _, err := e.ExportCompiledSnapshot(); !errors.Is(err, indexed.ErrSnapshotIncompatibleHook) {
		t.Fatalf("want ErrSnapshotIncompatibleHook after Build, got %v", err)
	}
}

func TestCompiledRoundTrip(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:  "br-specific",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	_ = orig.AddRule(indexed.Rule{
		Name:  "mercosul",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"AR", "UY"}},
	})

	cs, err := orig.ExportCompiledSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	loaded, err := indexed.LoadCompiledSnapshot(cs, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.Built() {
		t.Fatalf("loaded engine must be Built")
	}

	for _, in := range []map[string]string{
		{"country": "BR"},
		{"country": "AR"},
		{"country": "UY"},
		{"country": "DE"},
	} {
		ro, err := orig.Execute(context.Background(), engine.Request{Input: in})
		if err != nil {
			t.Fatalf("orig Execute: %v", err)
		}
		rl, err := loaded.Execute(context.Background(), engine.Request{Input: in})
		if err != nil {
			t.Fatalf("loaded Execute: %v", err)
		}
		if !sameMatched(ro.Matched, rl.Matched) {
			t.Fatalf("input %v: orig=%v loaded=%v", in, ro.Matched, rl.Matched)
		}
	}
}

func TestLoadCompiledSnapshotNilErrors(t *testing.T) {
	if _, err := indexed.LoadCompiledSnapshot(nil, nil); err == nil {
		t.Fatal("expected error on nil snapshot")
	}
}

func TestCompiledRoundTripWithCallbacks(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:   "r",
		Match:  parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
		Action: func(input interface{}) interface{} { return "orig-out" },
	})
	cs, err := orig.ExportCompiledSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	loaded, err := indexed.LoadCompiledSnapshot(cs, map[string]indexed.RuleCallbacks{
		"r": {Action: func(interface{}) interface{} { return "rebuilt" }},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := loaded.Execute(context.Background(), engine.Request{Input: map[string]string{"k": "v"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Output != "rebuilt" {
		t.Fatalf("rebuild callback should have fired, got %v", got.Output)
	}
}

func TestCompiledRoundTripExportableAgain(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:  "r",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})
	cs, _ := orig.ExportCompiledSnapshot()
	loaded, _ := indexed.LoadCompiledSnapshot(cs, nil)
	if _, err := loaded.ExportCompiledSnapshot(); err != nil {
		t.Fatalf("re-export from loaded: %v", err)
	}
}
