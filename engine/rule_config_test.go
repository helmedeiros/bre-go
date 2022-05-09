package engine_test

import (
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
)

type testRuleConfig struct {
	Name string
}

func (c testRuleConfig) RuleName() string { return c.Name }

type sliceProvider struct {
	configs []testRuleConfig
	err     error
}

func (s *sliceProvider) RuleConfigs() ([]testRuleConfig, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.configs, nil
}

func TestRuleConfigInterfaceIsSatisfiedByStructWithRuleNameMethod(t *testing.T) {
	var _ engine.RuleConfig = testRuleConfig{Name: "x"}
}

func TestRuleConfigProviderInterfaceIsSatisfiedBySliceProvider(t *testing.T) {
	var _ engine.RuleConfigProvider[testRuleConfig] = &sliceProvider{}
}

func TestLoadCallsAddOnceForEachConfig(t *testing.T) {
	provider := &sliceProvider{configs: []testRuleConfig{
		{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"},
	}}
	var seen []string

	err := engine.Load[testRuleConfig](provider, func(c testRuleConfig) error {
		seen = append(seen, c.Name)
		return nil
	})

	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if len(seen) != 3 {
		t.Fatalf("seen: want 3 configs, got %d", len(seen))
	}
}

func TestLoadPreservesInsertionOrder(t *testing.T) {
	provider := &sliceProvider{configs: []testRuleConfig{
		{Name: "first"}, {Name: "second"}, {Name: "third"},
	}}
	var seen []string

	_ = engine.Load[testRuleConfig](provider, func(c testRuleConfig) error {
		seen = append(seen, c.Name)
		return nil
	})

	want := []string{"first", "second", "third"}
	for i, w := range want {
		if seen[i] != w {
			t.Fatalf("seen[%d]: want %q, got %q", i, w, seen[i])
		}
	}
}

func TestLoadPropagatesProviderError(t *testing.T) {
	sentinel := errors.New("provider broke")
	provider := &sliceProvider{err: sentinel}

	err := engine.Load[testRuleConfig](provider, func(testRuleConfig) error {
		t.Fatalf("add should not be called when provider returns error")
		return nil
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("Load err: want sentinel, got %v", err)
	}
}

func TestLoadShortCircuitsOnAddError(t *testing.T) {
	provider := &sliceProvider{configs: []testRuleConfig{
		{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"},
	}}
	sentinel := errors.New("add rejected")
	calls := 0

	err := engine.Load[testRuleConfig](provider, func(c testRuleConfig) error {
		calls++
		if c.Name == "beta" {
			return sentinel
		}
		return nil
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("Load err: want sentinel, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls: want 2 (alpha then beta short-circuits), got %d", calls)
	}
}

func TestChainProvidersWithZeroArgsReturnsEmpty(t *testing.T) {
	chain := engine.ChainProviders[testRuleConfig]()

	configs, err := chain.RuleConfigs()

	if err != nil {
		t.Fatalf("RuleConfigs: unexpected error: %v", err)
	}
	if len(configs) != 0 {
		t.Fatalf("configs: want empty for zero-arg chain, got %d", len(configs))
	}
}

func TestChainProvidersWithOneProviderReturnsItsConfigs(t *testing.T) {
	inner := &sliceProvider{configs: []testRuleConfig{{Name: "alpha"}, {Name: "beta"}}}
	chain := engine.ChainProviders[testRuleConfig](inner)

	configs, _ := chain.RuleConfigs()

	if len(configs) != 2 || configs[0].Name != "alpha" || configs[1].Name != "beta" {
		t.Fatalf("configs: want [alpha beta], got %v", configs)
	}
}

func TestChainProvidersConcatenatesInOrder(t *testing.T) {
	first := &sliceProvider{configs: []testRuleConfig{{Name: "a"}, {Name: "b"}}}
	second := &sliceProvider{configs: []testRuleConfig{{Name: "c"}, {Name: "d"}}}
	chain := engine.ChainProviders[testRuleConfig](first, second)

	configs, _ := chain.RuleConfigs()

	want := []string{"a", "b", "c", "d"}
	for i, w := range want {
		if configs[i].Name != w {
			t.Fatalf("configs[%d].Name: want %q, got %q", i, w, configs[i].Name)
		}
	}
}

func TestChainProvidersShortCircuitsOnProviderError(t *testing.T) {
	sentinel := errors.New("boom")
	first := &sliceProvider{configs: []testRuleConfig{{Name: "a"}}}
	second := &sliceProvider{err: sentinel}
	third := &sliceProvider{configs: []testRuleConfig{{Name: "z"}}}
	chain := engine.ChainProviders[testRuleConfig](first, second, third)

	_, err := chain.RuleConfigs()

	if !errors.Is(err, sentinel) {
		t.Fatalf("err: want sentinel, got %v", err)
	}
}

func TestLoadAcceptsEmptyProvider(t *testing.T) {
	provider := &sliceProvider{configs: nil}
	calls := 0

	err := engine.Load[testRuleConfig](provider, func(testRuleConfig) error {
		calls++
		return nil
	})

	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("calls: want 0 on empty provider, got %d", calls)
	}
}
