package indexed

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/helmedeiros/bre-go/engine/parser"
)

// CompiledSnapshotFormatVersion identifies the binary wire-format
// schema for the pre-compiled snapshot. UnmarshalCompiledSnapshot
// refuses any other value with ErrCompiledSnapshotFormatVersionMismatch.
const CompiledSnapshotFormatVersion uint16 = 1

const compiledSnapshotMagic = "BRG5"

const (
	compiledCondString byte = 1
	compiledCondSet    byte = 2
	compiledCondRange  byte = 3
	compiledCondAnd    byte = 4

	compiledOpEq    byte = 1
	compiledOpNeq   byte = 2
	compiledOpIn    byte = 3
	compiledOpNotIn byte = 4
)

func compiledOpToByte(op string) byte {
	switch op {
	case parser.OpEq:
		return compiledOpEq
	case parser.OpNeq:
		return compiledOpNeq
	case parser.OpIn:
		return compiledOpIn
	case parser.OpNotIn:
		return compiledOpNotIn
	}
	return 0
}

func compiledOpFromByte(b byte) string {
	switch b {
	case compiledOpEq:
		return parser.OpEq
	case compiledOpNeq:
		return parser.OpNeq
	case compiledOpIn:
		return parser.OpIn
	case compiledOpNotIn:
		return parser.OpNotIn
	}
	return ""
}

// MarshalCompiledSnapshot serializes cs to w in the v0.16.0 binary
// format. Big-endian throughout for cross-architecture portability.
// Floats are decimal-string-encoded so IEEE-754 infinity bounds
// survive without an extra wire-level flag.
func MarshalCompiledSnapshot(w io.Writer, cs *CompiledSnapshot) error {
	if cs == nil {
		return fmt.Errorf("indexed: MarshalCompiledSnapshot called with nil snapshot")
	}
	bw := bufio.NewWriter(w)
	if _, err := bw.WriteString(compiledSnapshotMagic); err != nil {
		return err
	}
	if err := binary.Write(bw, binary.BigEndian, CompiledSnapshotFormatVersion); err != nil {
		return err
	}
	if err := wireWriteVarint(bw, len(cs.KeysetOrder)); err != nil {
		return err
	}
	for _, k := range cs.KeysetOrder {
		if err := wireWriteString(bw, k); err != nil {
			return err
		}
	}
	if err := wireWriteVarint(bw, len(cs.Buckets)); err != nil {
		return err
	}
	for keyset, bucket := range cs.Buckets {
		if err := wireWriteString(bw, keyset); err != nil {
			return err
		}
		if err := wireWriteVarint(bw, len(bucket.Fields)); err != nil {
			return err
		}
		for _, f := range bucket.Fields {
			if err := wireWriteString(bw, f); err != nil {
				return err
			}
		}
		if err := wireWriteVarint(bw, len(bucket.ByValueKey)); err != nil {
			return err
		}
		for vk, refs := range bucket.ByValueKey {
			if err := wireWriteString(bw, vk); err != nil {
				return err
			}
			if err := wireWriteVarint(bw, len(refs)); err != nil {
				return err
			}
			for _, ref := range refs {
				if err := wireWriteString(bw, ref.Name); err != nil {
					return err
				}
				if err := wireWriteVarint(bw, len(ref.PostFilter)); err != nil {
					return err
				}
				for _, p := range ref.PostFilter {
					if err := wireWriteParserCond(bw, p); err != nil {
						return err
					}
				}
			}
		}
	}
	if err := wireWriteVarint(bw, len(cs.RulesInOrder)); err != nil {
		return err
	}
	for _, r := range cs.RulesInOrder {
		if err := wireWriteString(bw, r.Name); err != nil {
			return err
		}
		if err := wireWriteString(bw, r.Description); err != nil {
			return err
		}
		if err := wireWriteVarint(bw, len(r.Tags)); err != nil {
			return err
		}
		for _, t := range r.Tags {
			if err := wireWriteString(bw, t); err != nil {
				return err
			}
		}
		if err := wireWriteSnapshotCond(bw, r.Match); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// UnmarshalCompiledSnapshot reads a CompiledSnapshot from r. Refuses
// ErrCompiledSnapshotFormatVersionMismatch on version mismatch and
// ErrCompiledSnapshotMalformed for truncated input or unknown tags.
func UnmarshalCompiledSnapshot(r io.Reader) (*CompiledSnapshot, error) {
	br := bufio.NewReader(r)
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(br, hdr); err != nil {
		return nil, fmt.Errorf("%w: header: %v", ErrCompiledSnapshotMalformed, err)
	}
	if string(hdr) != compiledSnapshotMagic {
		return nil, fmt.Errorf("%w: bad magic %q", ErrCompiledSnapshotMalformed, string(hdr))
	}
	var v uint16
	if err := binary.Read(br, binary.BigEndian, &v); err != nil {
		return nil, fmt.Errorf("%w: version: %v", ErrCompiledSnapshotMalformed, err)
	}
	if v != CompiledSnapshotFormatVersion {
		return nil, fmt.Errorf("%w: got %d want %d", ErrCompiledSnapshotFormatVersionMismatch, v, CompiledSnapshotFormatVersion)
	}

	keyN, err := wireReadVarint(br)
	if err != nil {
		return nil, fmt.Errorf("%w: keyset count: %v", ErrCompiledSnapshotMalformed, err)
	}
	keysetOrder := make([]string, keyN)
	for i := 0; i < keyN; i++ {
		s, err := wireReadString(br)
		if err != nil {
			return nil, fmt.Errorf("%w: keyset[%d]: %v", ErrCompiledSnapshotMalformed, i, err)
		}
		keysetOrder[i] = s
	}

	bucketN, err := wireReadVarint(br)
	if err != nil {
		return nil, fmt.Errorf("%w: bucket count: %v", ErrCompiledSnapshotMalformed, err)
	}
	buckets := make(map[string]CompiledBucket, bucketN)
	for i := 0; i < bucketN; i++ {
		keyset, err := wireReadString(br)
		if err != nil {
			return nil, fmt.Errorf("%w: bucket[%d] keyset: %v", ErrCompiledSnapshotMalformed, i, err)
		}
		fldN, err := wireReadVarint(br)
		if err != nil {
			return nil, fmt.Errorf("%w: bucket[%d] field count: %v", ErrCompiledSnapshotMalformed, i, err)
		}
		fields := make([]string, fldN)
		for j := 0; j < fldN; j++ {
			fields[j], err = wireReadString(br)
			if err != nil {
				return nil, fmt.Errorf("%w: bucket[%d].field[%d]: %v", ErrCompiledSnapshotMalformed, i, j, err)
			}
		}
		vkN, err := wireReadVarint(br)
		if err != nil {
			return nil, fmt.Errorf("%w: bucket[%d] vk count: %v", ErrCompiledSnapshotMalformed, i, err)
		}
		byVk := make(map[string][]CompiledRuleRef, vkN)
		for j := 0; j < vkN; j++ {
			vk, err := wireReadString(br)
			if err != nil {
				return nil, fmt.Errorf("%w: bucket[%d].vk[%d]: %v", ErrCompiledSnapshotMalformed, i, j, err)
			}
			refN, err := wireReadVarint(br)
			if err != nil {
				return nil, fmt.Errorf("%w: bucket[%d].vk[%d] ref count: %v", ErrCompiledSnapshotMalformed, i, j, err)
			}
			refs := make([]CompiledRuleRef, refN)
			for k := 0; k < refN; k++ {
				name, err := wireReadString(br)
				if err != nil {
					return nil, fmt.Errorf("%w: ref name: %v", ErrCompiledSnapshotMalformed, err)
				}
				postN, err := wireReadVarint(br)
				if err != nil {
					return nil, fmt.Errorf("%w: ref post count: %v", ErrCompiledSnapshotMalformed, err)
				}
				var post []parser.Condition
				if postN > 0 {
					post = make([]parser.Condition, postN)
					for m := 0; m < postN; m++ {
						pc, err := wireReadParserCond(br)
						if err != nil {
							return nil, fmt.Errorf("%w: ref[%d].post[%d]: %v", ErrCompiledSnapshotMalformed, k, m, err)
						}
						post[m] = pc
					}
				}
				refs[k] = CompiledRuleRef{Name: name, PostFilter: post}
			}
			byVk[vk] = refs
		}
		buckets[keyset] = CompiledBucket{Fields: fields, ByValueKey: byVk}
	}

	ruleN, err := wireReadVarint(br)
	if err != nil {
		return nil, fmt.Errorf("%w: rule count: %v", ErrCompiledSnapshotMalformed, err)
	}
	rules := make([]SnapshotRule, ruleN)
	for i := 0; i < ruleN; i++ {
		name, err := wireReadString(br)
		if err != nil {
			return nil, fmt.Errorf("%w: rules[%d] name: %v", ErrCompiledSnapshotMalformed, i, err)
		}
		desc, err := wireReadString(br)
		if err != nil {
			return nil, fmt.Errorf("%w: rules[%d] desc: %v", ErrCompiledSnapshotMalformed, i, err)
		}
		tagN, err := wireReadVarint(br)
		if err != nil {
			return nil, fmt.Errorf("%w: rules[%d] tag count: %v", ErrCompiledSnapshotMalformed, i, err)
		}
		var tags []string
		if tagN > 0 {
			tags = make([]string, tagN)
			for j := 0; j < tagN; j++ {
				tags[j], err = wireReadString(br)
				if err != nil {
					return nil, fmt.Errorf("%w: rules[%d].tag[%d]: %v", ErrCompiledSnapshotMalformed, i, j, err)
				}
			}
		}
		match, err := wireReadSnapshotCond(br)
		if err != nil {
			return nil, fmt.Errorf("%w: rules[%d].match: %v", ErrCompiledSnapshotMalformed, i, err)
		}
		rules[i] = SnapshotRule{Name: name, Description: desc, Tags: tags, Match: match}
	}

	return &CompiledSnapshot{
		KeysetOrder:  keysetOrder,
		Buckets:      buckets,
		RulesInOrder: rules,
	}, nil
}

func wireWriteSnapshotCond(w *bufio.Writer, c SnapshotCondition) error {
	switch c.Type {
	case "string":
		if err := w.WriteByte(compiledCondString); err != nil {
			return err
		}
		if err := wireWriteString(w, c.Field); err != nil {
			return err
		}
		if err := w.WriteByte(compiledOpToByte(c.Op)); err != nil {
			return err
		}
		return wireWriteString(w, c.Value)
	case "set":
		if err := w.WriteByte(compiledCondSet); err != nil {
			return err
		}
		if err := wireWriteString(w, c.Field); err != nil {
			return err
		}
		if err := w.WriteByte(compiledOpToByte(c.Op)); err != nil {
			return err
		}
		if err := wireWriteVarint(w, len(c.Values)); err != nil {
			return err
		}
		for _, v := range c.Values {
			if err := wireWriteString(w, v); err != nil {
				return err
			}
		}
		return nil
	case "range":
		if err := w.WriteByte(compiledCondRange); err != nil {
			return err
		}
		if err := wireWriteString(w, c.Field); err != nil {
			return err
		}
		if err := wireWriteString(w, c.Min); err != nil {
			return err
		}
		return wireWriteString(w, c.Max)
	case "and":
		if err := w.WriteByte(compiledCondAnd); err != nil {
			return err
		}
		if err := wireWriteVarint(w, len(c.Children)); err != nil {
			return err
		}
		for _, ch := range c.Children {
			if err := wireWriteSnapshotCond(w, ch); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("indexed: MarshalCompiledSnapshot: unsupported condition type %q", c.Type)
	}
}

func wireReadSnapshotCond(r *bufio.Reader) (SnapshotCondition, error) {
	t, err := r.ReadByte()
	if err != nil {
		return SnapshotCondition{}, err
	}
	switch t {
	case compiledCondString:
		field, err := wireReadString(r)
		if err != nil {
			return SnapshotCondition{}, err
		}
		op, err := r.ReadByte()
		if err != nil {
			return SnapshotCondition{}, err
		}
		val, err := wireReadString(r)
		if err != nil {
			return SnapshotCondition{}, err
		}
		return SnapshotCondition{Type: "string", Field: field, Op: compiledOpFromByte(op), Value: val}, nil
	case compiledCondSet:
		field, err := wireReadString(r)
		if err != nil {
			return SnapshotCondition{}, err
		}
		op, err := r.ReadByte()
		if err != nil {
			return SnapshotCondition{}, err
		}
		n, err := wireReadVarint(r)
		if err != nil {
			return SnapshotCondition{}, err
		}
		values := make([]string, n)
		for i := 0; i < n; i++ {
			values[i], err = wireReadString(r)
			if err != nil {
				return SnapshotCondition{}, err
			}
		}
		return SnapshotCondition{Type: "set", Field: field, Op: compiledOpFromByte(op), Values: values}, nil
	case compiledCondRange:
		field, err := wireReadString(r)
		if err != nil {
			return SnapshotCondition{}, err
		}
		minS, err := wireReadString(r)
		if err != nil {
			return SnapshotCondition{}, err
		}
		maxS, err := wireReadString(r)
		if err != nil {
			return SnapshotCondition{}, err
		}
		return SnapshotCondition{Type: "range", Field: field, Min: minS, Max: maxS}, nil
	case compiledCondAnd:
		n, err := wireReadVarint(r)
		if err != nil {
			return SnapshotCondition{}, err
		}
		children := make([]SnapshotCondition, n)
		for i := 0; i < n; i++ {
			children[i], err = wireReadSnapshotCond(r)
			if err != nil {
				return SnapshotCondition{}, err
			}
		}
		return SnapshotCondition{Type: "and", Children: children}, nil
	default:
		return SnapshotCondition{}, fmt.Errorf("unknown snapshot-cond tag %d", t)
	}
}

func wireWriteParserCond(w *bufio.Writer, c parser.Condition) error {
	switch v := c.(type) {
	case parser.StringCondition:
		if err := w.WriteByte(compiledCondString); err != nil {
			return err
		}
		if err := wireWriteString(w, v.Field); err != nil {
			return err
		}
		if err := w.WriteByte(compiledOpToByte(v.Op)); err != nil {
			return err
		}
		return wireWriteString(w, v.Value)
	case parser.SetCondition:
		if err := w.WriteByte(compiledCondSet); err != nil {
			return err
		}
		if err := wireWriteString(w, v.Field); err != nil {
			return err
		}
		if err := w.WriteByte(compiledOpToByte(v.Op)); err != nil {
			return err
		}
		if err := wireWriteVarint(w, len(v.Values)); err != nil {
			return err
		}
		for _, x := range v.Values {
			if err := wireWriteString(w, x); err != nil {
				return err
			}
		}
		return nil
	case parser.RangeCondition:
		if err := w.WriteByte(compiledCondRange); err != nil {
			return err
		}
		if err := wireWriteString(w, v.Field); err != nil {
			return err
		}
		if err := wireWriteString(w, formatFloat(v.Min)); err != nil {
			return err
		}
		return wireWriteString(w, formatFloat(v.Max))
	default:
		return fmt.Errorf("indexed: MarshalCompiledSnapshot: unsupported post-filter shape %T", c)
	}
}

func wireReadParserCond(r *bufio.Reader) (parser.Condition, error) {
	t, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch t {
	case compiledCondString:
		field, err := wireReadString(r)
		if err != nil {
			return nil, err
		}
		op, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		val, err := wireReadString(r)
		if err != nil {
			return nil, err
		}
		return parser.StringCondition{Field: field, Op: compiledOpFromByte(op), Value: val}, nil
	case compiledCondSet:
		field, err := wireReadString(r)
		if err != nil {
			return nil, err
		}
		op, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		n, err := wireReadVarint(r)
		if err != nil {
			return nil, err
		}
		values := make([]string, n)
		for i := 0; i < n; i++ {
			values[i], err = wireReadString(r)
			if err != nil {
				return nil, err
			}
		}
		return parser.SetCondition{Field: field, Op: compiledOpFromByte(op), Values: values}, nil
	case compiledCondRange:
		field, err := wireReadString(r)
		if err != nil {
			return nil, err
		}
		minS, err := wireReadString(r)
		if err != nil {
			return nil, err
		}
		maxS, err := wireReadString(r)
		if err != nil {
			return nil, err
		}
		minV, err := parseFloat(minS)
		if err != nil {
			return nil, err
		}
		maxV, err := parseFloat(maxS)
		if err != nil {
			return nil, err
		}
		return parser.RangeCondition{Field: field, Min: minV, Max: maxV}, nil
	default:
		return nil, fmt.Errorf("unknown post-filter tag %d", t)
	}
}

func wireWriteVarint(w *bufio.Writer, n int) error {
	buf := make([]byte, binary.MaxVarintLen64)
	m := binary.PutUvarint(buf, uint64(n))
	_, err := w.Write(buf[:m])
	return err
}

func wireReadVarint(r io.ByteReader) (int, error) {
	v, err := binary.ReadUvarint(r)
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

func wireWriteString(w *bufio.Writer, s string) error {
	if err := wireWriteVarint(w, len(s)); err != nil {
		return err
	}
	_, err := w.WriteString(s)
	return err
}

func wireReadString(r *bufio.Reader) (string, error) {
	n, err := wireReadVarint(r)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}
