# Clean Code Conventions

Every contribution is expected to follow these rules. They are short on purpose; when a rule and an idiomatic Go convention disagree, name the conflict in the PR description and pick the cleaner option for the case.

## Naming

- Choose descriptive and unambiguous names.
- Make meaningful distinction (two different things must read differently).
- Use pronounceable names.
- Use searchable names (avoid one-letter and overly generic names).
- Replace magic numbers with named constants.
- Avoid encodings. No prefixes (`strName`, `iCount`, `errFoo` + `ErrFoo` duplicate) and no embedded type info.

## Functions

- Small.
- Do one thing.
- Use descriptive names.
- Prefer fewer arguments.
- Have no side effects (pure where possible; if side effects exist, the name says so).
- Don't use flag arguments. Split into separate functions callers pick from.

## Comments

- Try to explain yourself in code first.
- Don't be redundant.
- No obvious noise.
- No closing-brace comments.
- Don't comment out code -- remove it.
- Use comments for: intent, clarification, warning of consequences.

## Source code structure

- Separate concepts vertically.
- Related code stays vertically dense.
- Declare variables close to their usage.
- Dependent functions stay close.
- Similar functions stay close.
- Place functions in the downward direction (caller above callee).
- Keep lines short.
- No horizontal alignment.
- Use whitespace to associate related things; disassociate weakly related.
- Don't break indentation.

## Objects and data structures

- Hide internal structure.
- Prefer transparent data structures over hybrid object/data.
- Small.
- Do one thing.
- Small number of instance variables.
- Base class knows nothing about derivatives.
- Many functions over passing flags to select behavior.
- Prefer non-static methods to static methods.

## Tests

- One assert (or one logical behavior) per test.
- Readable.
- Fast.
- Independent.
- Repeatable.
