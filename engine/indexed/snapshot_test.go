package indexed_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

func TestExportSnapshotEmptyEngineErrors(t *testing.T) {
	e := indexed.New()
	if _, err := e.ExportSnapshot(); !errors.Is(err, indexed.ErrSnapshotEmpty) {
		t.Fatalf("want ErrSnapshotEmpty, got %v", err)
	}
}

func TestExportSnapshotHookBearingEngineErrors(t *testing.T) {
	e := indexed.New().WithPostFilterHook(func(parser.Condition) bool { return false })
	_ = e.AddRule(indexed.Rule{
		Name:  "r",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})
	if _, err := e.ExportSnapshot(); !errors.Is(err, indexed.ErrSnapshotIncompatibleHook) {
		t.Fatalf("want ErrSnapshotIncompatibleHook, got %v", err)
	}
}

func TestExportSnapshotHookOnBuiltEngineErrors(t *testing.T) {
	e := indexed.New().WithPostFilterHook(func(parser.Condition) bool { return false })
	_ = e.AddRule(indexed.Rule{
		Name:  "r",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})
	if err := e.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := e.ExportSnapshot(); !errors.Is(err, indexed.ErrSnapshotIncompatibleHook) {
		t.Fatalf("want ErrSnapshotIncompatibleHook on built engine, got %v", err)
	}
}

func TestExportSnapshotFormatVersionIsCurrent(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "r",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})
	snap, err := e.ExportSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if snap.FormatVersion != indexed.SnapshotFormatVersion {
		t.Fatalf("FormatVersion = %d, want %d", snap.FormatVersion, indexed.SnapshotFormatVersion)
	}
}

func TestLoadSnapshotNilErrors(t *testing.T) {
	if _, err := indexed.LoadSnapshot(nil, nil); !errors.Is(err, indexed.ErrSnapshotMalformed) {
		t.Fatalf("want ErrSnapshotMalformed on nil snapshot, got %v", err)
	}
}

func TestLoadSnapshotFormatVersionMismatchErrors(t *testing.T) {
	snap := &indexed.Snapshot{FormatVersion: 9999}
	if _, err := indexed.LoadSnapshot(snap, nil); !errors.Is(err, indexed.ErrSnapshotFormatVersionMismatch) {
		t.Fatalf("want ErrSnapshotFormatVersionMismatch, got %v", err)
	}
}

func TestLoadSnapshotMalformedUnknownTypeErrors(t *testing.T) {
	snap := &indexed.Snapshot{
		FormatVersion: indexed.SnapshotFormatVersion,
		Rules: []indexed.SnapshotRule{{
			Name:  "r",
			Match: indexed.SnapshotCondition{Type: "no-such-type"},
		}},
	}
	if _, err := indexed.LoadSnapshot(snap, nil); !errors.Is(err, indexed.ErrSnapshotMalformed) {
		t.Fatalf("want ErrSnapshotMalformed, got %v", err)
	}
}

func TestLoadSnapshotMalformedRangeMinErrors(t *testing.T) {
	snap := &indexed.Snapshot{
		FormatVersion: indexed.SnapshotFormatVersion,
		Rules: []indexed.SnapshotRule{{
			Name: "r",
			Match: indexed.SnapshotCondition{
				Type: "and",
				Children: []indexed.SnapshotCondition{
					{Type: "string", Field: "k", Op: parser.OpEq, Value: "v"},
					{Type: "range", Field: "n", Min: "not-a-float", Max: "10"},
				},
			},
		}},
	}
	if _, err := indexed.LoadSnapshot(snap, nil); !errors.Is(err, indexed.ErrSnapshotMalformed) {
		t.Fatalf("want ErrSnapshotMalformed on bad Min, got %v", err)
	}
}

func TestLoadSnapshotMalformedRangeMaxErrors(t *testing.T) {
	snap := &indexed.Snapshot{
		FormatVersion: indexed.SnapshotFormatVersion,
		Rules: []indexed.SnapshotRule{{
			Name: "r",
			Match: indexed.SnapshotCondition{
				Type: "and",
				Children: []indexed.SnapshotCondition{
					{Type: "string", Field: "k", Op: parser.OpEq, Value: "v"},
					{Type: "range", Field: "n", Min: "1", Max: "junk"},
				},
			},
		}},
	}
	if _, err := indexed.LoadSnapshot(snap, nil); !errors.Is(err, indexed.ErrSnapshotMalformed) {
		t.Fatalf("want ErrSnapshotMalformed on bad Max, got %v", err)
	}
}

func TestDecodeConditionNestedChildErrorPropagates(t *testing.T) {
	snap := &indexed.Snapshot{
		FormatVersion: indexed.SnapshotFormatVersion,
		Rules: []indexed.SnapshotRule{{
			Name: "r",
			Match: indexed.SnapshotCondition{
				Type: "and",
				Children: []indexed.SnapshotCondition{
					{Type: "string", Field: "k", Op: parser.OpEq, Value: "v"},
					{Type: "and", Children: []indexed.SnapshotCondition{
						{Type: "range", Field: "n", Min: "not-a-float", Max: "10"},
					}},
				},
			},
		}},
	}
	if _, err := indexed.LoadSnapshot(snap, nil); !errors.Is(err, indexed.ErrSnapshotMalformed) {
		t.Fatalf("want ErrSnapshotMalformed from nested-and-child error, got %v", err)
	}
}

func TestLoadSnapshotMalformedNestedChildErrors(t *testing.T) {
	snap := &indexed.Snapshot{
		FormatVersion: indexed.SnapshotFormatVersion,
		Rules: []indexed.SnapshotRule{{
			Name: "r",
			Match: indexed.SnapshotCondition{
				Type: "and",
				Children: []indexed.SnapshotCondition{
					{Type: "bogus-leaf"},
				},
			},
		}},
	}
	if _, err := indexed.LoadSnapshot(snap, nil); !errors.Is(err, indexed.ErrSnapshotMalformed) {
		t.Fatalf("want ErrSnapshotMalformed for nested bad child, got %v", err)
	}
}

func TestLoadSnapshotPropagatesAddRuleErrors(t *testing.T) {
	snap := &indexed.Snapshot{
		FormatVersion: indexed.SnapshotFormatVersion,
		Rules: []indexed.SnapshotRule{{
			Name: "", // empty name -> ErrEmptyRuleName from AddRule
			Match: indexed.SnapshotCondition{
				Type: "string", Field: "k", Op: parser.OpEq, Value: "v",
			},
		}},
	}
	if _, err := indexed.LoadSnapshot(snap, nil); !errors.Is(err, indexed.ErrEmptyRuleName) {
		t.Fatalf("want ErrEmptyRuleName, got %v", err)
	}
}

func TestRoundTripStringEquality(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:  "br",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})

	snap, err := orig.ExportSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	loaded, err := indexed.LoadSnapshot(snap, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	assertSameMatchesAcrossInputs(t, orig, loaded, []map[string]string{
		{"country": "BR"},
		{"country": "AR"},
	})
}

func TestRoundTripSetMembership(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:  "mercosul",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR", "UY"}},
	})

	snap, err := orig.ExportSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	loaded, err := indexed.LoadSnapshot(snap, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	assertSameMatchesAcrossInputs(t, orig, loaded, []map[string]string{
		{"country": "BR"},
		{"country": "AR"},
		{"country": "UY"},
		{"country": "PY"},
	})
}

func TestRoundTripStringNegationPostFilter(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name: "br-not-corporate",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpNeq, Value: "corporate"},
		}},
	})

	snap, err := orig.ExportSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	loaded, err := indexed.LoadSnapshot(snap, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	assertSameMatchesAcrossInputs(t, orig, loaded, []map[string]string{
		{"country": "BR", "tier": "consumer"},
		{"country": "BR", "tier": "corporate"},
		{"country": "AR", "tier": "consumer"},
	})
}

func TestRoundTripSetNegationPostFilter(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name: "br-not-blocked-tier",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.SetCondition{Field: "tier", Op: parser.OpNotIn, Values: []string{"corporate", "fraud"}},
		}},
	})

	snap, err := orig.ExportSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	loaded, err := indexed.LoadSnapshot(snap, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	assertSameMatchesAcrossInputs(t, orig, loaded, []map[string]string{
		{"country": "BR", "tier": "consumer"},
		{"country": "BR", "tier": "corporate"},
		{"country": "BR", "tier": "fraud"},
	})
}

func TestRoundTripRangeFiniteBounds(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name: "mid-range",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.RangeCondition{Field: "amount", Min: 100, Max: 500},
		}},
	})

	snap, err := orig.ExportSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	loaded, err := indexed.LoadSnapshot(snap, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	assertSameMatchesAcrossInputs(t, orig, loaded, []map[string]string{
		{"country": "BR", "amount": "50"},
		{"country": "BR", "amount": "100"},
		{"country": "BR", "amount": "300"},
		{"country": "BR", "amount": "500"},
		{"country": "BR", "amount": "600"},
	})
}

func TestRoundTripRangeInfinityBounds(t *testing.T) {
	for _, tc := range []struct {
		name     string
		min, max float64
	}{
		{"unbounded-below", math.Inf(-1), 500},
		{"unbounded-above", 100, math.Inf(+1)},
		{"both-infinite", math.Inf(-1), math.Inf(+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			orig := indexed.New()
			_ = orig.AddRule(indexed.Rule{
				Name: "inf-range",
				Match: parser.AndCondition{Children: []parser.Condition{
					parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
					parser.RangeCondition{Field: "amount", Min: tc.min, Max: tc.max},
				}},
			})

			snap, err := orig.ExportSnapshot()
			if err != nil {
				t.Fatalf("Export: %v", err)
			}
			loaded, err := indexed.LoadSnapshot(snap, nil)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			assertSameMatchesAcrossInputs(t, orig, loaded, []map[string]string{
				{"country": "BR", "amount": "-1e9"},
				{"country": "BR", "amount": "0"},
				{"country": "BR", "amount": "300"},
				{"country": "BR", "amount": "1e9"},
			})
		})
	}
}

func TestRoundTripMultiConditionAnd(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name: "br-consumer-mid",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "segment", Op: parser.OpEq, Value: "consumer"},
			parser.SetCondition{Field: "channel", Op: parser.OpIn, Values: []string{"web", "mobile"}},
			parser.RangeCondition{Field: "amount", Min: 100, Max: 1000},
		}},
	})

	snap, err := orig.ExportSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	loaded, err := indexed.LoadSnapshot(snap, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	assertSameMatchesAcrossInputs(t, orig, loaded, []map[string]string{
		{"country": "BR", "segment": "consumer", "channel": "web", "amount": "500"},
		{"country": "BR", "segment": "consumer", "channel": "store", "amount": "500"},
		{"country": "BR", "segment": "corporate", "channel": "web", "amount": "500"},
		{"country": "BR", "segment": "consumer", "channel": "web", "amount": "50"},
		{"country": "AR", "segment": "consumer", "channel": "web", "amount": "500"},
	})
}

func TestRoundTripPreservesInsertionOrder(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:  "br-specific",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	_ = orig.AddRule(indexed.Rule{
		Name:  "br-broader",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR", "UY"}},
	})

	snap, err := orig.ExportSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if snap.Rules[0].Name != "br-specific" || snap.Rules[1].Name != "br-broader" {
		t.Fatalf("snapshot rule order = %v / %v", snap.Rules[0].Name, snap.Rules[1].Name)
	}
	loaded, err := indexed.LoadSnapshot(snap, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := loaded.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got.Matched) != 1 || got.Matched[0] != "br-specific" {
		t.Fatalf("first-match should be br-specific (insertion order preserved), got %v", got.Matched)
	}
}

func TestRoundTripPreservesDescriptionAndTags(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:        "r",
		Description: "the brazilian rule",
		Tags:        []string{"geo", "country", "br"},
		Match:       parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})

	snap, err := orig.ExportSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if snap.Rules[0].Description != "the brazilian rule" {
		t.Fatalf("Description lost: %q", snap.Rules[0].Description)
	}
	if strings.Join(snap.Rules[0].Tags, ",") != "geo,country,br" {
		t.Fatalf("Tags lost or reordered: %v", snap.Rules[0].Tags)
	}

	loaded, err := indexed.LoadSnapshot(snap, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	infos := loaded.RuleInfos()
	if len(infos) != 1 {
		t.Fatalf("want 1 rule, got %d", len(infos))
	}
	if infos[0].Description != "the brazilian rule" {
		t.Fatalf("loaded Description = %q", infos[0].Description)
	}
	if strings.Join(infos[0].Tags, ",") != "geo,country,br" {
		t.Fatalf("loaded Tags = %v", infos[0].Tags)
	}

	snap.Rules[0].Tags[0] = "MUTATED"
	if infos[0].Tags[0] == "MUTATED" {
		t.Fatalf("loaded engine aliases the snapshot's Tags slice")
	}
}

func TestRoundTripCallbacksAttachByName(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:   "r",
		Match:  parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
		Action: func(input interface{}) interface{} { return "original" },
	})

	snap, err := orig.ExportSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	loaded, err := indexed.LoadSnapshot(snap, map[string]indexed.RuleCallbacks{
		"r": {Action: func(input interface{}) interface{} { return "rebuilt" }},
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

func TestRoundTripCallbackActionContextAttachByName(t *testing.T) {
	snap := &indexed.Snapshot{
		FormatVersion: indexed.SnapshotFormatVersion,
		Rules: []indexed.SnapshotRule{{
			Name:  "r",
			Match: indexed.SnapshotCondition{Type: "string", Field: "k", Op: parser.OpEq, Value: "v"},
		}},
	}

	loaded, err := indexed.LoadSnapshot(snap, map[string]indexed.RuleCallbacks{
		"r": {ActionContext: func(_ context.Context, _ interface{}) interface{} { return "ctx-out" }},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := loaded.Execute(context.Background(), engine.Request{Input: map[string]string{"k": "v"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Output != "ctx-out" {
		t.Fatalf("ActionContext callback should have fired, got %v", got.Output)
	}
}

func TestRoundTripWithoutCallbacks(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:  "r",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})

	snap, err := orig.ExportSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	loaded, err := indexed.LoadSnapshot(snap, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := loaded.Execute(context.Background(), engine.Request{Input: map[string]string{"k": "v"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got.Matched) != 1 || got.Matched[0] != "r" {
		t.Fatalf("loaded engine should match rule, got %v", got.Matched)
	}
	if got.Output != nil {
		t.Fatalf("no callback registered -> Output must be nil, got %v", got.Output)
	}
}

func TestLoadedEngineIsBuilt(t *testing.T) {
	snap := &indexed.Snapshot{
		FormatVersion: indexed.SnapshotFormatVersion,
		Rules: []indexed.SnapshotRule{{
			Name:  "r",
			Match: indexed.SnapshotCondition{Type: "string", Field: "k", Op: parser.OpEq, Value: "v"},
		}},
	}
	loaded, err := indexed.LoadSnapshot(snap, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.Built() {
		t.Fatalf("LoadSnapshot must return a Built engine")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name: "r",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.RangeCondition{Field: "amount", Min: 100, Max: math.Inf(+1)},
		}},
	})

	snap, err := orig.ExportSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var roundTrip indexed.Snapshot
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	loaded, err := indexed.LoadSnapshot(&roundTrip, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	assertSameMatchesAcrossInputs(t, orig, loaded, []map[string]string{
		{"country": "BR", "amount": "50"},
		{"country": "BR", "amount": "500"},
		{"country": "AR", "amount": "500"},
	})
}

func TestSnapshotOfSnapshotIsIdempotent(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name: "r1",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.SetCondition{Field: "tier", Op: parser.OpNotIn, Values: []string{"fraud"}},
		}},
	})
	_ = orig.AddRule(indexed.Rule{
		Name:  "r2",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"AR", "UY"}},
	})

	first, err := orig.ExportSnapshot()
	if err != nil {
		t.Fatalf("Export 1: %v", err)
	}
	loaded, err := indexed.LoadSnapshot(first, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	second, err := loaded.ExportSnapshot()
	if err != nil {
		t.Fatalf("Export 2: %v", err)
	}

	a, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("Marshal first: %v", err)
	}
	b, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("Marshal second: %v", err)
	}
	if string(a) != string(b) {
		t.Fatalf("snapshot-of-snapshot drifted:\n  first:  %s\n  second: %s", a, b)
	}
}

func TestExportSnapshotTriggersImplicitBuild(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "r",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})
	if e.Built() {
		t.Fatalf("engine should not be Built before Export")
	}
	if _, err := e.ExportSnapshot(); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !e.Built() {
		t.Fatalf("ExportSnapshot must trigger implicit Build")
	}
}

func assertSameMatchesAcrossInputs(t *testing.T, a, b *indexed.Engine, inputs []map[string]string) {
	t.Helper()
	for i, in := range inputs {
		ra, err := a.Execute(context.Background(), engine.Request{Input: in})
		if err != nil {
			t.Fatalf("input %d (%v) Execute on original: %v", i, in, err)
		}
		rb, err := b.Execute(context.Background(), engine.Request{Input: in})
		if err != nil {
			t.Fatalf("input %d (%v) Execute on loaded: %v", i, in, err)
		}
		if !sameMatched(ra.Matched, rb.Matched) {
			t.Fatalf("input %d (%v): original matched %v, loaded matched %v", i, in, ra.Matched, rb.Matched)
		}
	}
}

func sameMatched(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
