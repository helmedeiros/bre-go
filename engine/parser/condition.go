package parser

// Op constants for StringCondition and SetCondition. Strings rather
// than a typed enum so the values JSON-marshal cleanly and read
// naturally in debug output.
const (
	OpEq    = "=="
	OpNeq   = "!="
	OpIn    = "IN"
	OpNotIn = "NOT IN"
)

// Condition is a Boolean predicate over a fact map. Concrete
// implementations (StringCondition, SetCondition, AndCondition,
// OrCondition, NotCondition) are typed structs that can be
// inspected, marshaled, and compared. Use ParseToCondition to
// build one from an expression string.
type Condition interface {
	Eval(fact map[string]interface{}) bool
}

// StringCondition compares the string value at Field against Value
// using Op. Op must be OpEq or OpNeq. A missing field, or a non-string
// value at the field, evaluates to false regardless of Op.
type StringCondition struct {
	Field string
	Op    string
	Value string
}

// Eval implements Condition.
func (c StringCondition) Eval(fact map[string]interface{}) bool {
	got, ok := fact[c.Field]
	if !ok {
		return false
	}
	s, ok := got.(string)
	if !ok {
		return false
	}
	switch c.Op {
	case OpEq:
		return s == c.Value
	case OpNeq:
		return s != c.Value
	default:
		return false
	}
}

// SetCondition tests set membership of the string value at Field
// against Values using Op. Op must be OpIn or OpNotIn. A missing
// field, or a non-string value at the field, evaluates to false
// regardless of Op.
type SetCondition struct {
	Field  string
	Op     string
	Values []string
}

// Eval implements Condition.
func (c SetCondition) Eval(fact map[string]interface{}) bool {
	got, ok := fact[c.Field]
	if !ok {
		return false
	}
	s, ok := got.(string)
	if !ok {
		return false
	}
	hit := false
	for _, v := range c.Values {
		if v == s {
			hit = true
			break
		}
	}
	switch c.Op {
	case OpIn:
		return hit
	case OpNotIn:
		return !hit
	default:
		return false
	}
}

// AndCondition is the logical conjunction of every child. An empty
// child list evaluates to true (the identity for conjunction).
// Evaluation short-circuits on the first false child.
type AndCondition struct {
	Children []Condition
}

// Eval implements Condition.
func (c AndCondition) Eval(fact map[string]interface{}) bool {
	for _, child := range c.Children {
		if !child.Eval(fact) {
			return false
		}
	}
	return true
}

// OrCondition is the logical disjunction of every child. An empty
// child list evaluates to false (the identity for disjunction).
// Evaluation short-circuits on the first true child.
type OrCondition struct {
	Children []Condition
}

// Eval implements Condition.
func (c OrCondition) Eval(fact map[string]interface{}) bool {
	for _, child := range c.Children {
		if child.Eval(fact) {
			return true
		}
	}
	return false
}

// NotCondition is the logical negation of Child.
type NotCondition struct {
	Child Condition
}

// Eval implements Condition.
func (c NotCondition) Eval(fact map[string]interface{}) bool {
	return !c.Child.Eval(fact)
}

// AsPredicate wraps a typed Condition tree as a Predicate. Same shape
// as Parse returns, so callers can drop the result into any code path
// that currently takes a Predicate.
func AsPredicate(c Condition) Predicate {
	return func(fact map[string]interface{}) bool { return c.Eval(fact) }
}

// AsRuleCondition wraps a typed Condition tree as a
// func(interface{}) bool suitable for any adapter's Rule.Condition
// field. factOf converts the engine input value into the fact map
// the condition reads. Mirror of AsCondition for the typed-tree
// case.
func AsRuleCondition(c Condition, factOf func(interface{}) map[string]interface{}) func(interface{}) bool {
	return func(in interface{}) bool { return c.Eval(factOf(in)) }
}
