package engine_test

import (
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/engine/parser"
	"github.com/helmedeiros/bre-go/engine/priority"
)

// These tests live in package engine_test (not in any adapter) because
// they're the proof that the engine-level sentinels work as an
// umbrella across every adapter -- a single import of `engine` plus
// errors.Is(err, engine.ErrEmptyRuleName) is enough.

func TestErrEmptyRuleNameUmbrella(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"inmemory", addEmptyNameInmemory()},
		{"firstmatch", addEmptyNameFirstmatch()},
		{"priority", addEmptyNamePriority()},
		{"indexed", addEmptyNameIndexed()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !errors.Is(c.err, engine.ErrEmptyRuleName) {
				t.Fatalf("%s: errors.Is(err, engine.ErrEmptyRuleName) == false; got %v", c.name, c.err)
			}
		})
	}
}

func TestErrDuplicateRuleNameUmbrella(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"inmemory", addDuplicateInmemory()},
		{"firstmatch", addDuplicateFirstmatch()},
		{"priority", addDuplicatePriority()},
		{"indexed", addDuplicateIndexed()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !errors.Is(c.err, engine.ErrDuplicateRuleName) {
				t.Fatalf("%s: errors.Is(err, engine.ErrDuplicateRuleName) == false; got %v", c.name, c.err)
			}
		})
	}
}

func TestPerAdapterSentinelsStillWork(t *testing.T) {
	// Backward compat: pre-v0.19 code using the per-adapter sentinels
	// keeps working. We're not removing them; we're adding an umbrella
	// above them.
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"inmemory empty", addEmptyNameInmemory(), inmemory.ErrEmptyRuleName},
		{"firstmatch empty", addEmptyNameFirstmatch(), firstmatch.ErrEmptyRuleName},
		{"priority empty", addEmptyNamePriority(), priority.ErrEmptyRuleName},
		{"indexed empty", addEmptyNameIndexed(), indexed.ErrEmptyRuleName},
		{"inmemory dup", addDuplicateInmemory(), inmemory.ErrDuplicateRuleName},
		{"firstmatch dup", addDuplicateFirstmatch(), firstmatch.ErrDuplicateRuleName},
		{"priority dup", addDuplicatePriority(), priority.ErrDuplicateRuleName},
		{"indexed dup", addDuplicateIndexed(), indexed.ErrDuplicateRuleName},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !errors.Is(c.err, c.want) {
				t.Fatalf("backward-compat broken: %s err %v doesn't match %v", c.name, c.err, c.want)
			}
		})
	}
}

func TestPerAdapterMessagesUnchanged(t *testing.T) {
	// Error message strings are part of the loose public surface. We
	// don't promise them, but breaking them would silently break any
	// consumer doing log-grep on the message. Confirm they're
	// unchanged after the wrap.
	cases := []struct {
		err  error
		want string
	}{
		{inmemory.ErrEmptyRuleName, "inmemory: rule name must not be empty"},
		{firstmatch.ErrEmptyRuleName, "firstmatch: rule name must not be empty"},
		{priority.ErrEmptyRuleName, "priority: rule name must not be empty"},
		{indexed.ErrEmptyRuleName, "indexed: rule name must not be empty"},
		{inmemory.ErrDuplicateRuleName, "inmemory: rule name already registered"},
		{firstmatch.ErrDuplicateRuleName, "firstmatch: rule name already registered"},
		{priority.ErrDuplicateRuleName, "priority: rule name already registered"},
		{indexed.ErrDuplicateRuleName, "indexed: rule name already registered"},
	}
	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Fatalf("error message drift: got %q, want %q", got, c.want)
		}
	}
}

func addEmptyNameInmemory() error {
	e := inmemory.New()
	return e.AddRule(inmemory.Rule{Name: "", Condition: func(interface{}) bool { return true }})
}

func addEmptyNameFirstmatch() error {
	e := firstmatch.New()
	return e.AddRule(firstmatch.Rule{Name: "", Condition: func(interface{}) bool { return true }})
}

func addEmptyNamePriority() error {
	e := priority.New()
	return e.AddRule(priority.Rule{Name: "", Condition: func(interface{}) bool { return true }})
}

func addEmptyNameIndexed() error {
	e := indexed.New()
	return e.AddRule(indexed.Rule{Name: "", Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"}})
}

func addDuplicateInmemory() error {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{Name: "r", Condition: func(interface{}) bool { return true }})
	return e.AddRule(inmemory.Rule{Name: "r", Condition: func(interface{}) bool { return true }})
}

func addDuplicateFirstmatch() error {
	e := firstmatch.New()
	_ = e.AddRule(firstmatch.Rule{Name: "r", Condition: func(interface{}) bool { return true }})
	return e.AddRule(firstmatch.Rule{Name: "r", Condition: func(interface{}) bool { return true }})
}

func addDuplicatePriority() error {
	e := priority.New()
	_ = e.AddRule(priority.Rule{Name: "r", Condition: func(interface{}) bool { return true }})
	return e.AddRule(priority.Rule{Name: "r", Condition: func(interface{}) bool { return true }})
}

func addDuplicateIndexed() error {
	e := indexed.New()
	r := indexed.Rule{Name: "r", Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"}}
	_ = e.AddRule(r)
	return e.AddRule(r)
}
