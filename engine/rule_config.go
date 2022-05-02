package engine

// RuleConfig is the minimum contract any rule-loader configuration
// must satisfy. Concrete RC types carry whatever fields the rule
// needs (Amount, Currency, Priority, ...); the only required method
// is the rule's name.
type RuleConfig interface {
	RuleName() string
}

// RuleConfigProvider is the single-method interface every rule loader
// satisfies. A CSV loader, JSON loader, in-memory slice, HTTP client
// fetch -- all return their typed []RC (or an error if the underlying
// source is malformed).
//
// The interface is intentionally minimal: no Reload, Watch, or Close.
// Those capabilities land in future ADRs as optional interfaces if
// real callers need them.
type RuleConfigProvider[RC RuleConfig] interface {
	RuleConfigs() ([]RC, error)
}

// Load pulls configs from provider and calls add for each, in
// insertion order. Returns the first error from either the provider
// or add, short-circuiting the remaining configs.
//
// Typical wiring:
//
//	provider := csv.NewLoader[OrderRuleConfig]("rules.csv", parseRow)
//	eng := inmemory.New()
//	err := engine.Load(provider, func(c OrderRuleConfig) error {
//	    return eng.AddRule(toInmemoryRule(c))
//	})
//
// Load is a thin helper -- nothing magical happens inside it.
// Callers preferring a different bridging strategy (concat multiple
// providers, filter, parallelize) call provider.RuleConfigs() and
// loop themselves.
func Load[RC RuleConfig](provider RuleConfigProvider[RC], add func(RC) error) error {
	configs, err := provider.RuleConfigs()
	if err != nil {
		return err
	}
	for _, c := range configs {
		if err := add(c); err != nil {
			return err
		}
	}
	return nil
}
