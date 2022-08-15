package format

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

// C5 — pre-compiled (pre-bucketed) snapshot binary.
//
// Encodes the engine's compiled state: keyset order + per-keyset
// bucket (fields + byValueKey -> rule-refs) + rules-in-order.
// Each per-bucket rule-ref carries the rule name and its
// post-filter condition list (same encoding as C4). Loader
// constructs the CompiledSnapshot in memory, hands it to
// indexed.LoadCompiledSnapshot, which atomic-stores it directly.
// No AddRule, no fan-out, no Build.

const (
	c5Magic          = "BRG5"
	c5Version uint16 = 1
)

// EncodeC5 writes cs to w.
func EncodeC5(w io.Writer, cs *indexed.CompiledSnapshot) error {
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	if _, err := bw.WriteString(c5Magic); err != nil {
		return err
	}
	if err := binary.Write(bw, binary.BigEndian, c5Version); err != nil {
		return err
	}

	// keyset order
	if err := writeVarint(bw, len(cs.KeysetOrder)); err != nil {
		return err
	}
	for _, k := range cs.KeysetOrder {
		if err := writeString(bw, k); err != nil {
			return err
		}
	}

	// buckets
	if err := writeVarint(bw, len(cs.Buckets)); err != nil {
		return err
	}
	for keyset, bucket := range cs.Buckets {
		if err := writeString(bw, keyset); err != nil {
			return err
		}
		if err := writeVarint(bw, len(bucket.Fields)); err != nil {
			return err
		}
		for _, f := range bucket.Fields {
			if err := writeString(bw, f); err != nil {
				return err
			}
		}
		if err := writeVarint(bw, len(bucket.ByValueKey)); err != nil {
			return err
		}
		for vk, refs := range bucket.ByValueKey {
			if err := writeString(bw, vk); err != nil {
				return err
			}
			if err := writeVarint(bw, len(refs)); err != nil {
				return err
			}
			for _, ref := range refs {
				if err := writeString(bw, ref.Name); err != nil {
					return err
				}
				if err := writeVarint(bw, len(ref.PostFilter)); err != nil {
					return err
				}
				for _, p := range ref.PostFilter {
					if err := writeParserCondC4(bw, p); err != nil {
						return err
					}
				}
			}
		}
	}

	// rules in order (carry the Match for round-trip integrity + meta)
	if err := writeVarint(bw, len(cs.RulesInOrder)); err != nil {
		return err
	}
	for _, r := range cs.RulesInOrder {
		if err := writeString(bw, r.Name); err != nil {
			return err
		}
		if err := writeString(bw, r.Description); err != nil {
			return err
		}
		if err := writeVarint(bw, len(r.Tags)); err != nil {
			return err
		}
		for _, t := range r.Tags {
			if err := writeString(bw, t); err != nil {
				return err
			}
		}
		if err := writeSnapshotCondC5(bw, r.Match); err != nil {
			return err
		}
	}
	return nil
}

// DecodeC5 reads cs from r.
func DecodeC5(r io.Reader) (*indexed.CompiledSnapshot, error) {
	br := bufio.NewReader(r)
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(br, hdr); err != nil {
		return nil, fmt.Errorf("c5: header: %w", err)
	}
	if string(hdr) != c5Magic {
		return nil, errors.New("c5: bad magic")
	}
	var v uint16
	if err := binary.Read(br, binary.BigEndian, &v); err != nil {
		return nil, err
	}
	if v != c5Version {
		return nil, fmt.Errorf("c5: unsupported version %d", v)
	}

	keyN, err := readVarint(br)
	if err != nil {
		return nil, err
	}
	keysetOrder := make([]string, keyN)
	for i := 0; i < keyN; i++ {
		keysetOrder[i], err = readString(br)
		if err != nil {
			return nil, err
		}
	}

	bucketN, err := readVarint(br)
	if err != nil {
		return nil, err
	}
	buckets := make(map[string]indexed.CompiledBucket, bucketN)
	for i := 0; i < bucketN; i++ {
		keyset, err := readString(br)
		if err != nil {
			return nil, err
		}
		fldN, err := readVarint(br)
		if err != nil {
			return nil, err
		}
		fields := make([]string, fldN)
		for j := 0; j < fldN; j++ {
			fields[j], err = readString(br)
			if err != nil {
				return nil, err
			}
		}
		vkN, err := readVarint(br)
		if err != nil {
			return nil, err
		}
		byVk := make(map[string][]indexed.CompiledRuleRef, vkN)
		for j := 0; j < vkN; j++ {
			vk, err := readString(br)
			if err != nil {
				return nil, err
			}
			refN, err := readVarint(br)
			if err != nil {
				return nil, err
			}
			refs := make([]indexed.CompiledRuleRef, refN)
			for k := 0; k < refN; k++ {
				name, err := readString(br)
				if err != nil {
					return nil, err
				}
				postN, err := readVarint(br)
				if err != nil {
					return nil, err
				}
				var post []parser.Condition
				if postN > 0 {
					post = make([]parser.Condition, postN)
					for m := 0; m < postN; m++ {
						post[m], err = readParserCondC4(br)
						if err != nil {
							return nil, err
						}
					}
				}
				refs[k] = indexed.CompiledRuleRef{Name: name, PostFilter: post}
			}
			byVk[vk] = refs
		}
		buckets[keyset] = indexed.CompiledBucket{Fields: fields, ByValueKey: byVk}
	}

	ruleN, err := readVarint(br)
	if err != nil {
		return nil, err
	}
	rules := make([]indexed.SnapshotRule, ruleN)
	for i := 0; i < ruleN; i++ {
		name, err := readString(br)
		if err != nil {
			return nil, err
		}
		desc, err := readString(br)
		if err != nil {
			return nil, err
		}
		tagN, err := readVarint(br)
		if err != nil {
			return nil, err
		}
		var tags []string
		if tagN > 0 {
			tags = make([]string, tagN)
			for j := 0; j < tagN; j++ {
				tags[j], err = readString(br)
				if err != nil {
					return nil, err
				}
			}
		}
		match, err := readSnapshotCondC5(br)
		if err != nil {
			return nil, err
		}
		rules[i] = indexed.SnapshotRule{Name: name, Description: desc, Tags: tags, Match: match}
	}

	return &indexed.CompiledSnapshot{
		KeysetOrder:  keysetOrder,
		Buckets:      buckets,
		RulesInOrder: rules,
	}, nil
}

func writeSnapshotCondC5(w *bufio.Writer, c indexed.SnapshotCondition) error {
	switch c.Type {
	case "string":
		if err := w.WriteByte(c3CondString); err != nil {
			return err
		}
		if err := writeString(w, c.Field); err != nil {
			return err
		}
		if err := w.WriteByte(opToByte(c.Op)); err != nil {
			return err
		}
		return writeString(w, c.Value)
	case "set":
		if err := w.WriteByte(c3CondSet); err != nil {
			return err
		}
		if err := writeString(w, c.Field); err != nil {
			return err
		}
		if err := w.WriteByte(opToByte(c.Op)); err != nil {
			return err
		}
		if err := writeVarint(w, len(c.Values)); err != nil {
			return err
		}
		for _, v := range c.Values {
			if err := writeString(w, v); err != nil {
				return err
			}
		}
		return nil
	case "range":
		if err := w.WriteByte(c3CondRange); err != nil {
			return err
		}
		if err := writeString(w, c.Field); err != nil {
			return err
		}
		if err := writeString(w, c.Min); err != nil {
			return err
		}
		return writeString(w, c.Max)
	case "and":
		if err := w.WriteByte(c3CondAnd); err != nil {
			return err
		}
		if err := writeVarint(w, len(c.Children)); err != nil {
			return err
		}
		for _, ch := range c.Children {
			if err := writeSnapshotCondC5(w, ch); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("c5: unsupported snapshot-cond type %q", c.Type)
	}
}

func readSnapshotCondC5(r *bufio.Reader) (indexed.SnapshotCondition, error) {
	t, err := r.ReadByte()
	if err != nil {
		return indexed.SnapshotCondition{}, err
	}
	switch t {
	case c3CondString:
		field, err := readString(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		op, err := r.ReadByte()
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		val, err := readString(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		return indexed.SnapshotCondition{Type: "string", Field: field, Op: opFromByte(op), Value: val}, nil
	case c3CondSet:
		field, err := readString(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		op, err := r.ReadByte()
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		n, err := readVarint(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		values := make([]string, n)
		for i := 0; i < n; i++ {
			values[i], err = readString(r)
			if err != nil {
				return indexed.SnapshotCondition{}, err
			}
		}
		return indexed.SnapshotCondition{Type: "set", Field: field, Op: opFromByte(op), Values: values}, nil
	case c3CondRange:
		field, err := readString(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		minS, err := readString(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		maxS, err := readString(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		return indexed.SnapshotCondition{Type: "range", Field: field, Min: minS, Max: maxS}, nil
	case c3CondAnd:
		n, err := readVarint(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		children := make([]indexed.SnapshotCondition, n)
		for i := 0; i < n; i++ {
			children[i], err = readSnapshotCondC5(r)
			if err != nil {
				return indexed.SnapshotCondition{}, err
			}
		}
		return indexed.SnapshotCondition{Type: "and", Children: children}, nil
	default:
		return indexed.SnapshotCondition{}, fmt.Errorf("c5: unknown snapshot-cond tag %d", t)
	}
}
