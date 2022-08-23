package indexed_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

func TestCompiledWireRoundTripSingleEquality(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:  "br",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	loaded := marshalRoundTrip(t, orig)
	assertSameAcrossInputs(t, orig, loaded, []map[string]string{
		{"country": "BR"}, {"country": "AR"},
	})
}

func TestCompiledWireRoundTripSetMembership(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:  "mercosul",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR", "UY"}},
	})
	loaded := marshalRoundTrip(t, orig)
	assertSameAcrossInputs(t, orig, loaded, []map[string]string{
		{"country": "BR"}, {"country": "PY"},
	})
}

func TestCompiledWireRoundTripNegationPostFilter(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name: "br-not-corp",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpNeq, Value: "corporate"},
		}},
	})
	_ = orig.AddRule(indexed.Rule{
		Name: "br-not-blocked",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.SetCondition{Field: "tier", Op: parser.OpNotIn, Values: []string{"corporate", "fraud"}},
		}},
	})
	loaded := marshalRoundTrip(t, orig)
	assertSameAcrossInputs(t, orig, loaded, []map[string]string{
		{"country": "BR", "tier": "consumer"},
		{"country": "BR", "tier": "corporate"},
		{"country": "BR", "tier": "fraud"},
	})
}

func TestCompiledWireRoundTripRangeWithInfinity(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name: "any-amount",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.RangeCondition{Field: "amount", Min: math.Inf(-1), Max: math.Inf(+1)},
		}},
	})
	_ = orig.AddRule(indexed.Rule{
		Name: "high-amount",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.RangeCondition{Field: "amount", Min: 100, Max: math.Inf(+1)},
		}},
	})
	loaded := marshalRoundTrip(t, orig)
	assertSameAcrossInputs(t, orig, loaded, []map[string]string{
		{"country": "BR", "amount": "50"},
		{"country": "BR", "amount": "100"},
		{"country": "BR", "amount": "1e308"},
	})
}

func TestCompiledWireRoundTripPreservesDescriptionAndTags(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:        "r",
		Description: "the brazilian rule",
		Tags:        []string{"geo", "country", "br"},
		Match:       parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	loaded := marshalRoundTrip(t, orig)
	infos := loaded.RuleInfos()
	if len(infos) != 1 || infos[0].Description != "the brazilian rule" {
		t.Fatalf("Description lost: %v", infos)
	}
	if len(infos[0].Tags) != 3 || infos[0].Tags[0] != "geo" {
		t.Fatalf("Tags lost: %v", infos[0].Tags)
	}
}

func TestCompiledWireMarshalNilErrors(t *testing.T) {
	if err := indexed.MarshalCompiledSnapshot(&bytes.Buffer{}, nil); err == nil {
		t.Fatal("expected error on nil snapshot")
	}
}

func TestCompiledWireBadMagicReturnsMalformed(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("XXXX")
	_ = binary.Write(&buf, binary.BigEndian, indexed.CompiledSnapshotFormatVersion)
	if _, err := indexed.UnmarshalCompiledSnapshot(&buf); !errors.Is(err, indexed.ErrCompiledSnapshotMalformed) {
		t.Fatalf("want ErrCompiledSnapshotMalformed, got %v", err)
	}
}

func TestCompiledWireFormatVersionMismatchReturnsTypedError(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("BRG5")
	_ = binary.Write(&buf, binary.BigEndian, uint16(9999))
	if _, err := indexed.UnmarshalCompiledSnapshot(&buf); !errors.Is(err, indexed.ErrCompiledSnapshotFormatVersionMismatch) {
		t.Fatalf("want ErrCompiledSnapshotFormatVersionMismatch, got %v", err)
	}
}

func TestCompiledWireTruncatedHeader(t *testing.T) {
	if _, err := indexed.UnmarshalCompiledSnapshot(bytes.NewReader([]byte("BRG"))); !errors.Is(err, indexed.ErrCompiledSnapshotMalformed) {
		t.Fatalf("want ErrCompiledSnapshotMalformed, got %v", err)
	}
}

func TestCompiledWireTruncatedVersion(t *testing.T) {
	buf := bytes.NewReader([]byte("BRG5"))
	if _, err := indexed.UnmarshalCompiledSnapshot(buf); !errors.Is(err, indexed.ErrCompiledSnapshotMalformed) {
		t.Fatalf("want ErrCompiledSnapshotMalformed, got %v", err)
	}
}

func TestCompiledWireTruncatedAfterHeader(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("BRG5")
	_ = binary.Write(&buf, binary.BigEndian, indexed.CompiledSnapshotFormatVersion)
	if _, err := indexed.UnmarshalCompiledSnapshot(&buf); !errors.Is(err, indexed.ErrCompiledSnapshotMalformed) {
		t.Fatalf("want ErrCompiledSnapshotMalformed on truncated keyset count, got %v", err)
	}
}

func TestCompiledWireRoundTripWithCallbacks(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:   "r",
		Match:  parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
		Action: func(input interface{}) interface{} { return "orig" },
	})
	cs, err := orig.ExportCompiledSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	var buf bytes.Buffer
	if err := indexed.MarshalCompiledSnapshot(&buf, cs); err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	decoded, err := indexed.UnmarshalCompiledSnapshot(&buf)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	loaded, err := indexed.LoadCompiledSnapshot(decoded, map[string]indexed.RuleCallbacks{
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

func TestCompiledWireFormatVersionConstant(t *testing.T) {
	if indexed.CompiledSnapshotFormatVersion != 1 {
		t.Fatalf("v0.16.0 ships CompiledSnapshotFormatVersion=1, got %d", indexed.CompiledSnapshotFormatVersion)
	}
}

func TestCompiledWireRoundTripDeepAndConditionTree(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name: "deep",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.AndCondition{Children: []parser.Condition{
				parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
				parser.StringCondition{Field: "k2", Op: parser.OpEq, Value: "v2"},
			}},
			parser.StringCondition{Field: "tier", Op: parser.OpNeq, Value: "fraud"},
		}},
	})
	loaded := marshalRoundTrip(t, orig)
	assertSameAcrossInputs(t, orig, loaded, []map[string]string{
		{"k": "v", "k2": "v2", "tier": "consumer"},
		{"k": "v", "k2": "v2", "tier": "fraud"},
		{"k": "v", "k2": "x", "tier": "consumer"},
	})
}

func TestCompiledWireRoundTripAllOps(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name: "all-ops",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "a", Op: parser.OpEq, Value: "1"},
			parser.SetCondition{Field: "b", Op: parser.OpIn, Values: []string{"x", "y"}},
			parser.StringCondition{Field: "c", Op: parser.OpNeq, Value: "no"},
			parser.SetCondition{Field: "d", Op: parser.OpNotIn, Values: []string{"forbidden"}},
		}},
	})
	loaded := marshalRoundTrip(t, orig)
	assertSameAcrossInputs(t, orig, loaded, []map[string]string{
		{"a": "1", "b": "x", "c": "yes", "d": "ok"},
		{"a": "1", "b": "x", "c": "no", "d": "ok"},
		{"a": "1", "b": "z", "c": "yes", "d": "ok"},
		{"a": "1", "b": "x", "c": "yes", "d": "forbidden"},
		{"a": "2", "b": "x", "c": "yes", "d": "ok"},
	})
}

func TestCompiledWireUnknownConditionTag(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("BRG5")
	_ = binary.Write(&buf, binary.BigEndian, indexed.CompiledSnapshotFormatVersion)
	buf.WriteByte(0) // keyset count varint = 0
	buf.WriteByte(0) // bucket count varint = 0
	buf.WriteByte(1) // rule count varint = 1
	buf.WriteByte(1) // rule name length varint = 1
	buf.WriteByte('r')
	buf.WriteByte(0)  // desc length = 0
	buf.WriteByte(0)  // tag count = 0
	buf.WriteByte(99) // unknown condition tag
	if _, err := indexed.UnmarshalCompiledSnapshot(&buf); !errors.Is(err, indexed.ErrCompiledSnapshotMalformed) {
		t.Fatalf("want ErrCompiledSnapshotMalformed on unknown cond tag, got %v", err)
	}
}

func TestCompiledWireTruncatedConditionString(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("BRG5")
	_ = binary.Write(&buf, binary.BigEndian, indexed.CompiledSnapshotFormatVersion)
	buf.WriteByte(0) // keyset count = 0
	buf.WriteByte(0) // bucket count = 0
	buf.WriteByte(1) // rule count = 1
	buf.WriteByte(1) // rule name len = 1
	buf.WriteByte('r')
	buf.WriteByte(0)           // desc = 0
	buf.WriteByte(0)           // tag count = 0
	buf.WriteByte(1)           // tag string cond
	buf.WriteByte(10)          // field length 10
	buf.Write([]byte("short")) // only 5 bytes provided
	if _, err := indexed.UnmarshalCompiledSnapshot(&buf); !errors.Is(err, indexed.ErrCompiledSnapshotMalformed) {
		t.Fatalf("want ErrCompiledSnapshotMalformed on truncated string, got %v", err)
	}
}

func TestCompiledWireRoundTripPreservesInsertionOrder(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:  "br-specific",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	_ = orig.AddRule(indexed.Rule{
		Name:  "mercosul",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR", "UY"}},
	})
	loaded := marshalRoundTrip(t, orig)
	got, err := loaded.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got.Matched) != 1 || got.Matched[0] != "br-specific" {
		t.Fatalf("first-match should be br-specific, got %v", got.Matched)
	}
}

type errAfterNBytesWriter struct {
	remaining int
}

func (w *errAfterNBytesWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, errInjected
	}
	if len(p) <= w.remaining {
		w.remaining -= len(p)
		return len(p), nil
	}
	n := w.remaining
	w.remaining = 0
	return n, errInjected
}

var errInjected = errors.New("injected I/O failure")

func TestCompiledWireMarshalSurfacesWriteErrorsAtEveryOffsetUnbuffered(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:        "br",
		Description: "brazilian",
		Tags:        []string{"geo", "country"},
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "channel", Op: parser.OpNeq, Value: "store"},
			parser.SetCondition{Field: "tier", Op: parser.OpNotIn, Values: []string{"corp", "fraud"}},
			parser.RangeCondition{Field: "amount", Min: 100, Max: math.Inf(+1)},
		}},
	})
	cs, err := orig.ExportCompiledSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	var goodBuf bytes.Buffer
	if err := indexed.MarshalCompiledSnapshot(&goodBuf, cs); err != nil {
		t.Fatalf("Marshal (control): %v", err)
	}
	total := goodBuf.Len()
	for cutoff := 0; cutoff < total; cutoff++ {
		// Pass a *bufio.Writer with buffer size 1 so every byte that
		// Marshal emits flushes immediately to the underlying writer.
		// Marshal detects pre-buffered writers and reuses our 1-byte
		// buffer, so its mid-stream `if err != nil { return err }`
		// branches all fire when the underlying writer errors.
		w := bufio.NewWriterSize(&errAfterNBytesWriter{remaining: cutoff}, 1)
		// Marshal returns nil for pre-buffered writers (caller owns
		// Flush), so check both Marshal and Flush errors.
		mErr := indexed.MarshalCompiledSnapshot(w, cs)
		fErr := w.Flush()
		if mErr == nil && fErr == nil {
			t.Fatalf("cutoff=%d: Marshal+Flush returned nil but underlying writer failed", cutoff)
		}
	}
}

func TestCompiledWireMarshalSurfacesWriteErrorsAtEveryOffset(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:        "br",
		Description: "brazilian",
		Tags:        []string{"geo", "country"},
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "channel", Op: parser.OpNeq, Value: "store"},
			parser.SetCondition{Field: "tier", Op: parser.OpNotIn, Values: []string{"corp", "fraud"}},
			parser.RangeCondition{Field: "amount", Min: 100, Max: math.Inf(+1)},
		}},
	})
	cs, err := orig.ExportCompiledSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	var goodBuf bytes.Buffer
	if err := indexed.MarshalCompiledSnapshot(&goodBuf, cs); err != nil {
		t.Fatalf("Marshal (control): %v", err)
	}
	total := goodBuf.Len()
	failures := 0
	for cutoff := 0; cutoff < total; cutoff++ {
		w := &errAfterNBytesWriter{remaining: cutoff}
		if err := indexed.MarshalCompiledSnapshot(w, cs); err == nil {
			t.Fatalf("cutoff=%d: Marshal returned nil but underlying writer failed", cutoff)
		} else {
			failures++
		}
	}
	if failures != total {
		t.Fatalf("expected every cutoff in [0,%d) to surface an error, got %d", total, failures)
	}
}

func TestCompiledWireUnmarshalSurfacesReadErrorsAtEveryOffset(t *testing.T) {
	orig := indexed.New()
	_ = orig.AddRule(indexed.Rule{
		Name:        "br",
		Description: "brazilian",
		Tags:        []string{"geo", "country"},
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "channel", Op: parser.OpNeq, Value: "store"},
			parser.SetCondition{Field: "tier", Op: parser.OpNotIn, Values: []string{"corp", "fraud"}},
			parser.RangeCondition{Field: "amount", Min: 100, Max: math.Inf(+1)},
		}},
	})
	cs, err := orig.ExportCompiledSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	var goodBuf bytes.Buffer
	if err := indexed.MarshalCompiledSnapshot(&goodBuf, cs); err != nil {
		t.Fatalf("Marshal (control): %v", err)
	}
	raw := goodBuf.Bytes()
	failures := 0
	for cutoff := 0; cutoff < len(raw); cutoff++ {
		if _, err := indexed.UnmarshalCompiledSnapshot(bytes.NewReader(raw[:cutoff])); err == nil {
			t.Fatalf("cutoff=%d: Unmarshal returned nil on truncated input", cutoff)
		}
		failures++
	}
	if failures != len(raw) {
		t.Fatalf("expected every cutoff in [0,%d) to surface an error, got %d", len(raw), failures)
	}
}

func TestCompiledWireBadOpByteForStringCond(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("BRG5")
	_ = binary.Write(&buf, binary.BigEndian, indexed.CompiledSnapshotFormatVersion)
	buf.WriteByte(0) // keyset count
	buf.WriteByte(0) // bucket count
	buf.WriteByte(1) // rule count
	buf.WriteByte(1) // name len
	buf.WriteByte('r')
	buf.WriteByte(0) // desc
	buf.WriteByte(0) // tag count
	buf.WriteByte(1) // string cond
	buf.WriteByte(1) // field len
	buf.WriteByte('k')
	buf.WriteByte(99) // unknown op byte
	buf.WriteByte(1)  // value len
	buf.WriteByte('v')
	if _, err := indexed.UnmarshalCompiledSnapshot(&buf); !errors.Is(err, indexed.ErrCompiledSnapshotMalformed) {
		t.Fatalf("want ErrCompiledSnapshotMalformed on bad op byte, got %v", err)
	}
}

func TestCompiledWireBadOpByteForSetCond(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("BRG5")
	_ = binary.Write(&buf, binary.BigEndian, indexed.CompiledSnapshotFormatVersion)
	buf.WriteByte(0)
	buf.WriteByte(0)
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('r')
	buf.WriteByte(0)
	buf.WriteByte(0)
	buf.WriteByte(2) // set cond
	buf.WriteByte(1)
	buf.WriteByte('k')
	buf.WriteByte(99) // unknown op byte
	buf.WriteByte(0)  // values count
	if _, err := indexed.UnmarshalCompiledSnapshot(&buf); !errors.Is(err, indexed.ErrCompiledSnapshotMalformed) {
		t.Fatalf("want ErrCompiledSnapshotMalformed on bad set op byte, got %v", err)
	}
}

func TestCompiledWireBadPostFilterOpByteForString(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("BRG5")
	_ = binary.Write(&buf, binary.BigEndian, indexed.CompiledSnapshotFormatVersion)
	buf.WriteByte(0) // keyset count
	buf.WriteByte(1) // bucket count
	buf.WriteByte(1) // keyset string len = 1
	buf.WriteByte('a')
	buf.WriteByte(1) // fields count
	buf.WriteByte(1) // field name len
	buf.WriteByte('a')
	buf.WriteByte(1) // value-key count
	buf.WriteByte(1) // value-key string len
	buf.WriteByte('x')
	buf.WriteByte(1) // ref count
	buf.WriteByte(1) // ref name len
	buf.WriteByte('r')
	buf.WriteByte(1) // postFilter count
	buf.WriteByte(1) // post-filter type: string
	buf.WriteByte(1) // field len
	buf.WriteByte('k')
	buf.WriteByte(99) // unknown op byte
	buf.WriteByte(1)
	buf.WriteByte('v')
	if _, err := indexed.UnmarshalCompiledSnapshot(&buf); !errors.Is(err, indexed.ErrCompiledSnapshotMalformed) {
		t.Fatalf("want ErrCompiledSnapshotMalformed, got %v", err)
	}
}

func TestCompiledWireBadPostFilterOpByteForSet(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("BRG5")
	_ = binary.Write(&buf, binary.BigEndian, indexed.CompiledSnapshotFormatVersion)
	buf.WriteByte(0)
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('a')
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('a')
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('x')
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('r')
	buf.WriteByte(1)
	buf.WriteByte(2) // post-filter type: set
	buf.WriteByte(1)
	buf.WriteByte('k')
	buf.WriteByte(99) // unknown op byte
	buf.WriteByte(0)
	if _, err := indexed.UnmarshalCompiledSnapshot(&buf); !errors.Is(err, indexed.ErrCompiledSnapshotMalformed) {
		t.Fatalf("want ErrCompiledSnapshotMalformed, got %v", err)
	}
}

func TestCompiledWireMarshalPanicsOnUnsupportedSnapshotCondType(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on unsupported SnapshotCondition Type")
		}
	}()
	cs := &indexed.CompiledSnapshot{
		KeysetOrder:  []string{},
		Buckets:      map[string]indexed.CompiledBucket{},
		RulesInOrder: []indexed.SnapshotRule{{Name: "r", Match: indexed.SnapshotCondition{Type: "nonsense"}}},
	}
	_ = indexed.MarshalCompiledSnapshot(&bytes.Buffer{}, cs)
}

func TestCompiledWireMarshalPanicsOnUnsupportedPostFilterShape(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on unsupported post-filter shape")
		}
	}()
	cs := &indexed.CompiledSnapshot{
		KeysetOrder: []string{"a"},
		Buckets: map[string]indexed.CompiledBucket{
			"a": {
				Fields: []string{"a"},
				ByValueKey: map[string][]indexed.CompiledRuleRef{
					"x": {{
						Name:       "r",
						PostFilter: []parser.Condition{unsupportedPostFilter{}},
					}},
				},
			},
		},
	}
	_ = indexed.MarshalCompiledSnapshot(&bytes.Buffer{}, cs)
}

func TestCompiledWireUnknownPostFilterTag(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("BRG5")
	_ = binary.Write(&buf, binary.BigEndian, indexed.CompiledSnapshotFormatVersion)
	buf.WriteByte(0)
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('a')
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('a')
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('x')
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('r')
	buf.WriteByte(1)
	buf.WriteByte(99) // unknown post-filter tag
	if _, err := indexed.UnmarshalCompiledSnapshot(&buf); !errors.Is(err, indexed.ErrCompiledSnapshotMalformed) {
		t.Fatalf("want ErrCompiledSnapshotMalformed on unknown post-filter tag, got %v", err)
	}
}

func TestCompiledWireMalformedRangePostFilterMin(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("BRG5")
	_ = binary.Write(&buf, binary.BigEndian, indexed.CompiledSnapshotFormatVersion)
	buf.WriteByte(0)
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('a')
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('a')
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('x')
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('r')
	buf.WriteByte(1) // 1 post-filter
	buf.WriteByte(3) // range cond
	buf.WriteByte(1) // field len
	buf.WriteByte('n')
	buf.WriteByte(3)
	buf.Write([]byte("xxx")) // unparseable min
	buf.WriteByte(2)
	buf.Write([]byte("10"))
	if _, err := indexed.UnmarshalCompiledSnapshot(&buf); !errors.Is(err, indexed.ErrCompiledSnapshotMalformed) {
		t.Fatalf("want ErrCompiledSnapshotMalformed on bad range min, got %v", err)
	}
}

func TestCompiledWireMalformedRangePostFilterMax(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("BRG5")
	_ = binary.Write(&buf, binary.BigEndian, indexed.CompiledSnapshotFormatVersion)
	buf.WriteByte(0)
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('a')
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('a')
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('x')
	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte('r')
	buf.WriteByte(1)
	buf.WriteByte(3) // range cond
	buf.WriteByte(1)
	buf.WriteByte('n')
	buf.WriteByte(2)
	buf.Write([]byte("10"))
	buf.WriteByte(3)
	buf.Write([]byte("xxx")) // unparseable max
	if _, err := indexed.UnmarshalCompiledSnapshot(&buf); !errors.Is(err, indexed.ErrCompiledSnapshotMalformed) {
		t.Fatalf("want ErrCompiledSnapshotMalformed on bad range max, got %v", err)
	}
}

func TestCompiledWireMarshalPanicsOnUnsupportedOp(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on unsupported Op")
		}
	}()
	cs := &indexed.CompiledSnapshot{
		KeysetOrder:  []string{},
		Buckets:      map[string]indexed.CompiledBucket{},
		RulesInOrder: []indexed.SnapshotRule{{Name: "r", Match: indexed.SnapshotCondition{Type: "string", Field: "k", Op: "BOGUS", Value: "v"}}},
	}
	_ = indexed.MarshalCompiledSnapshot(&bytes.Buffer{}, cs)
}

type unsupportedPostFilter struct{}

func (unsupportedPostFilter) Eval(map[string]interface{}) bool { return false }

func TestCompiledWireRoundTripLargeSnapshotExercisesBufioFlushes(t *testing.T) {
	orig := indexed.New()
	for i := 0; i < 5000; i++ {
		_ = orig.AddRule(indexed.Rule{
			Name:        labeli("rule", i),
			Description: labeli("description", i),
			Tags:        []string{labeli("t", i), labeli("u", i)},
			Match: parser.AndCondition{Children: []parser.Condition{
				parser.StringCondition{Field: "country", Op: parser.OpEq, Value: labeli("BR", i%50)},
				parser.SetCondition{Field: "tier", Op: parser.OpNotIn, Values: []string{labeli("blocked", i%10)}},
			}},
		})
	}

	cs, err := orig.ExportCompiledSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	var buf bytes.Buffer
	if err := indexed.MarshalCompiledSnapshot(&buf, cs); err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if buf.Len() < 100_000 {
		t.Fatalf("expected marshal output > 100KB to exercise bufio flushes, got %d", buf.Len())
	}
	decoded, err := indexed.UnmarshalCompiledSnapshot(&buf)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	loaded, err := indexed.LoadCompiledSnapshot(decoded, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.Built() {
		t.Fatalf("loaded engine should be Built")
	}
	if len(loaded.RuleNames()) != 5000 {
		t.Fatalf("expected 5000 loaded rules, got %d", len(loaded.RuleNames()))
	}
}

func TestCompiledWireMarshalErrorsOnLargeSnapshotMidStream(t *testing.T) {
	orig := indexed.New()
	for i := 0; i < 5000; i++ {
		_ = orig.AddRule(indexed.Rule{
			Name:  labeli("rule", i),
			Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: labeli("BR", i%50)},
		})
	}
	cs, err := orig.ExportCompiledSnapshot()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	var good bytes.Buffer
	if err := indexed.MarshalCompiledSnapshot(&good, cs); err != nil {
		t.Fatalf("Marshal (control): %v", err)
	}
	total := good.Len()
	// Sweep cutoffs in a few representative bands to exercise the
	// helper functions' err != nil branches once bufio flushes the
	// pending bytes.
	for _, cutoff := range []int{100, 1000, 5000, total / 2, total - 1000} {
		w := &errAfterNBytesWriter{remaining: cutoff}
		if err := indexed.MarshalCompiledSnapshot(w, cs); err == nil {
			t.Fatalf("cutoff=%d: expected error, got nil", cutoff)
		}
	}
}

func labeli(prefix string, i int) string {
	return fmt.Sprintf("%s-%05d", prefix, i)
}

func marshalRoundTrip(t *testing.T, orig *indexed.Engine) *indexed.Engine {
	t.Helper()
	cs, err := orig.ExportCompiledSnapshot()
	if err != nil {
		t.Fatalf("ExportCompiledSnapshot: %v", err)
	}
	var buf bytes.Buffer
	if err := indexed.MarshalCompiledSnapshot(&buf, cs); err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	decoded, err := indexed.UnmarshalCompiledSnapshot(&buf)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	loaded, err := indexed.LoadCompiledSnapshot(decoded, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return loaded
}

func assertSameAcrossInputs(t *testing.T, a, b *indexed.Engine, inputs []map[string]string) {
	t.Helper()
	for i, in := range inputs {
		ra, err := a.Execute(context.Background(), engine.Request{Input: in})
		if err != nil {
			t.Fatalf("input %d (%v) Execute on a: %v", i, in, err)
		}
		rb, err := b.Execute(context.Background(), engine.Request{Input: in})
		if err != nil {
			t.Fatalf("input %d (%v) Execute on b: %v", i, in, err)
		}
		if !sameMatched(ra.Matched, rb.Matched) {
			t.Fatalf("input %d (%v): a=%v b=%v", i, in, ra.Matched, rb.Matched)
		}
	}
}
