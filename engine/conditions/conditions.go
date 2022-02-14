// Package conditions provides Boolean combinators and sentinels that
// produce predicates of the same shape as Rule.Condition. Drops into
// any adapter's AddRule without conversion.
package conditions

// And returns a predicate that is true when every argument is true.
// Short-circuits on the first false. An empty And is true (the
// identity for conjunction).
func And(preds ...func(interface{}) bool) func(interface{}) bool {
	return func(in interface{}) bool {
		for _, p := range preds {
			if !p(in) {
				return false
			}
		}
		return true
	}
}

// Or returns a predicate that is true when any argument is true.
// Short-circuits on the first true. An empty Or is false (the
// identity for disjunction).
func Or(preds ...func(interface{}) bool) func(interface{}) bool {
	return func(in interface{}) bool {
		for _, p := range preds {
			if p(in) {
				return true
			}
		}
		return false
	}
}

// Not returns a predicate that inverts pred.
func Not(pred func(interface{}) bool) func(interface{}) bool {
	return func(in interface{}) bool {
		return !pred(in)
	}
}

// Always returns the constant-true predicate.
func Always() func(interface{}) bool {
	return func(interface{}) bool { return true }
}

// Never returns the constant-false predicate.
func Never() func(interface{}) bool {
	return func(interface{}) bool { return false }
}
