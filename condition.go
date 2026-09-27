package dema

import (
	"strings"
)

type Condition interface {
	isCondition()
	toSQL(b *strings.Builder, args *[]any)
}

func encodeValue(v any) any {
	if v == nil {
		return nil
	}
	if enc, ok := v.(FieldEncoder); ok {
		encoded, err := enc.EncodeDema()
		if err == nil {
			return encoded
		}
	}
	switch val := v.(type) {
	case []rune:
		return string(val)
	default:
		return v
	}
}

type equalCond struct {
	col string
	val any
}

func (c equalCond) isCondition() { _ = c.col }
func (c equalCond) toSQL(b *strings.Builder, args *[]any) {
	b.WriteString(c.col)
	b.WriteString(" = ?")
	*args = append(*args, encodeValue(c.val))
}

func Equal[T any, V any](field Field[T, V], val V) Condition {
	return equalCond{col: field.Name, val: val}
}

type greaterCond struct {
	col string
	val any
}

func (c greaterCond) isCondition() { _ = c.col }
func (c greaterCond) toSQL(b *strings.Builder, args *[]any) {
	b.WriteString(c.col)
	b.WriteString(" > ?")
	*args = append(*args, encodeValue(c.val))
}

func Greater[T any, V any](field Field[T, V], val V) Condition {
	return greaterCond{col: field.Name, val: val}
}

type lesserCond struct {
	col string
	val any
}

func (c lesserCond) isCondition() { _ = c.col }
func (c lesserCond) toSQL(b *strings.Builder, args *[]any) {
	b.WriteString(c.col)
	b.WriteString(" < ?")
	*args = append(*args, encodeValue(c.val))
}

func Lesser[T any, V any](field Field[T, V], val V) Condition {
	return lesserCond{col: field.Name, val: val}
}

type withinCond struct {
	col  string
	vals []any
}

func (c withinCond) isCondition() { _ = c.col }
func (c withinCond) toSQL(b *strings.Builder, args *[]any) {
	if len(c.vals) == 0 {
		b.WriteString("0 = 1")
		return
	}
	b.WriteString(c.col)
	b.WriteString(" IN (")
	for i, v := range c.vals {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("?")
		*args = append(*args, encodeValue(v))
	}
	b.WriteString(")")
}

func Within[T any, V any](field Field[T, V], values ...V) Condition {
	vals := make([]any, len(values))
	for i, v := range values {
		vals[i] = v
	}
	return withinCond{col: field.Name, vals: vals}
}

type notWithinCond struct {
	col  string
	vals []any
}

func (c notWithinCond) isCondition() { _ = c.col }
func (c notWithinCond) toSQL(b *strings.Builder, args *[]any) {
	if len(c.vals) == 0 {
		b.WriteString("1 = 1")
		return
	}
	b.WriteString(c.col)
	b.WriteString(" NOT IN (")
	for i, v := range c.vals {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("?")
		*args = append(*args, encodeValue(v))
	}
	b.WriteString(")")
}

func NotWithin[T any, V any](field Field[T, V], values ...V) Condition {
	vals := make([]any, len(values))
	for i, v := range values {
		vals[i] = v
	}
	return notWithinCond{col: field.Name, vals: vals}
}

type rangeCond struct {
	col string
	min any
	max any
}

func (c rangeCond) isCondition() { _ = c.col }
func (c rangeCond) toSQL(b *strings.Builder, args *[]any) {
	b.WriteString(c.col)
	b.WriteString(" BETWEEN ? AND ?")
	*args = append(*args, encodeValue(c.min), encodeValue(c.max))
}

func Range[T any, V any](field Field[T, V], min, max V) Condition {
	return rangeCond{col: field.Name, min: min, max: max}
}

type outsideRangeCond struct {
	col string
	min any
	max any
}

func (c outsideRangeCond) isCondition() { _ = c.col }
func (c outsideRangeCond) toSQL(b *strings.Builder, args *[]any) {
	b.WriteString(c.col)
	b.WriteString(" NOT BETWEEN ? AND ?")
	*args = append(*args, encodeValue(c.min), encodeValue(c.max))
}

func OutsideRange[T any, V any](field Field[T, V], min, max V) Condition {
	return outsideRangeCond{col: field.Name, min: min, max: max}
}

type binaryCond struct {
	op string
	a  Condition
	b  Condition
}

func (c binaryCond) isCondition() { _ = c.op }
func (c binaryCond) toSQL(b *strings.Builder, args *[]any) {
	if c.a == nil && c.b == nil {
		return
	}
	if c.a == nil {
		c.b.toSQL(b, args)
		return
	}
	if c.b == nil {
		c.a.toSQL(b, args)
		return
	}
	b.WriteString("(")
	c.a.toSQL(b, args)
	b.WriteString(") ")
	b.WriteString(c.op)
	b.WriteString(" (")
	c.b.toSQL(b, args)
	b.WriteString(")")
}

func Or(a, b Condition) Condition {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return binaryCond{op: "OR", a: a, b: b}
}

func And(a, b Condition) Condition {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return binaryCond{op: "AND", a: a, b: b}
}

func ConditionSQL(c Condition) (string, []any) {
	if c == nil {
		return "", nil
	}
	var b strings.Builder
	var args []any
	c.toSQL(&b, &args)
	return b.String(), args
}
