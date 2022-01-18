// Package observability holds the structured-logging port adapters
// emit to.
package observability

// Logger receives structured log events. Implementations must be
// safe for concurrent use.
type Logger interface {
	Info(msg string, fields ...Field)
	Error(msg string, fields ...Field)
}

// Field is a key/value pair attached to a log line.
type Field struct {
	Key   string
	Value interface{}
}

// String returns a string-typed Field.
func String(key, value string) Field { return Field{Key: key, Value: value} }

// Int returns an int-typed Field.
func Int(key string, value int) Field { return Field{Key: key, Value: value} }

// Bool returns a bool-typed Field.
func Bool(key string, value bool) Field { return Field{Key: key, Value: value} }

// Err returns an error-typed Field with the fixed key "err".
func Err(value error) Field { return Field{Key: "err", Value: value} }

// NopLogger discards every message.
type NopLogger struct{}

// Info on a NopLogger discards the message.
func (NopLogger) Info(string, ...Field) {}

// Error on a NopLogger discards the message.
func (NopLogger) Error(string, ...Field) {}
