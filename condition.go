package dema

import (
	"strings"
)

type Condition interface {
	isCondition()
	toSQL(b *strings.Builder, args *[]any)
	writeSQL(qb *queryBuffer)
	appendArgs(args []any) []any
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
	val any
	col string
}

func (c equalCond) isCondition() { _ = c.col }
func (c equalCond) toSQL(b *strings.Builder, args *[]any) {
	qb := getQueryBuffer()
	c.writeSQL(qb)
	b.Write(qb.Bytes())
	putQueryBuffer(qb)
	*args = c.appendArgs(*args)
}
func (c equalCond) writeSQL(qb *queryBuffer) {
	qb.WriteString(c.col)
	qb.WriteString(" = ?")
}
func (c equalCond) appendArgs(args []any) []any {
	return append(args, encodeValue(c.val))
}

func Equal[T any, V any](field Field[T, V], val V) Condition {
	return equalCond{col: field.Name, val: val}
}

type greaterCond struct {
	val any
	col string
}

func (c greaterCond) isCondition() { _ = c.col }
func (c greaterCond) toSQL(b *strings.Builder, args *[]any) {
	qb := getQueryBuffer()
	c.writeSQL(qb)
	b.Write(qb.Bytes())
	putQueryBuffer(qb)
	*args = c.appendArgs(*args)
}
func (c greaterCond) writeSQL(qb *queryBuffer) {
	qb.WriteString(c.col)
	qb.WriteString(" > ?")
}
func (c greaterCond) appendArgs(args []any) []any {
	return append(args, encodeValue(c.val))
}

func Greater[T any, V any](field Field[T, V], val V) Condition {
	return greaterCond{col: field.Name, val: val}
}

type lesserCond struct {
	val any
	col string
}

func (c lesserCond) isCondition() { _ = c.col }
func (c lesserCond) toSQL(b *strings.Builder, args *[]any) {
	qb := getQueryBuffer()
	c.writeSQL(qb)
	b.Write(qb.Bytes())
	putQueryBuffer(qb)
	*args = c.appendArgs(*args)
}
func (c lesserCond) writeSQL(qb *queryBuffer) {
	qb.WriteString(c.col)
	qb.WriteString(" < ?")
}
func (c lesserCond) appendArgs(args []any) []any {
	return append(args, encodeValue(c.val))
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
	qb := getQueryBuffer()
	c.writeSQL(qb)
	b.Write(qb.Bytes())
	putQueryBuffer(qb)
	*args = c.appendArgs(*args)
}
func (c withinCond) writeSQL(qb *queryBuffer) {
	if len(c.vals) == 0 {
		qb.WriteString("0 = 1")
		return
	}
	qb.WriteString(c.col)
	qb.WriteString(" IN (")
	for i := range c.vals {
		if i > 0 {
			qb.WriteString(", ")
		}
		qb.WriteString("?")
	}
	qb.WriteString(")")
}
func (c withinCond) appendArgs(args []any) []any {
	for _, v := range c.vals {
		args = append(args, encodeValue(v))
	}
	return args
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
	qb := getQueryBuffer()
	c.writeSQL(qb)
	b.Write(qb.Bytes())
	putQueryBuffer(qb)
	*args = c.appendArgs(*args)
}
func (c notWithinCond) writeSQL(qb *queryBuffer) {
	if len(c.vals) == 0 {
		qb.WriteString("1 = 1")
		return
	}
	qb.WriteString(c.col)
	qb.WriteString(" NOT IN (")
	for i := range c.vals {
		if i > 0 {
			qb.WriteString(", ")
		}
		qb.WriteString("?")
	}
	qb.WriteString(")")
}
func (c notWithinCond) appendArgs(args []any) []any {
	for _, v := range c.vals {
		args = append(args, encodeValue(v))
	}
	return args
}

func NotWithin[T any, V any](field Field[T, V], values ...V) Condition {
	vals := make([]any, len(values))
	for i, v := range values {
		vals[i] = v
	}
	return notWithinCond{col: field.Name, vals: vals}
}

type rangeCond struct {
	min any
	max any
	col string
}

func (c rangeCond) isCondition() { _ = c.col }
func (c rangeCond) toSQL(b *strings.Builder, args *[]any) {
	qb := getQueryBuffer()
	c.writeSQL(qb)
	b.Write(qb.Bytes())
	putQueryBuffer(qb)
	*args = c.appendArgs(*args)
}
func (c rangeCond) writeSQL(qb *queryBuffer) {
	qb.WriteString(c.col)
	qb.WriteString(" BETWEEN ? AND ?")
}
func (c rangeCond) appendArgs(args []any) []any {
	return append(args, encodeValue(c.min), encodeValue(c.max))
}

func Range[T any, V any](field Field[T, V], min, max V) Condition {
	return rangeCond{col: field.Name, min: min, max: max}
}

type outsideRangeCond struct {
	min any
	max any
	col string
}

func (c outsideRangeCond) isCondition() { _ = c.col }
func (c outsideRangeCond) toSQL(b *strings.Builder, args *[]any) {
	qb := getQueryBuffer()
	c.writeSQL(qb)
	b.Write(qb.Bytes())
	putQueryBuffer(qb)
	*args = c.appendArgs(*args)
}
func (c outsideRangeCond) writeSQL(qb *queryBuffer) {
	qb.WriteString(c.col)
	qb.WriteString(" NOT BETWEEN ? AND ?")
}
func (c outsideRangeCond) appendArgs(args []any) []any {
	return append(args, encodeValue(c.min), encodeValue(c.max))
}

func OutsideRange[T any, V any](field Field[T, V], min, max V) Condition {
	return outsideRangeCond{col: field.Name, min: min, max: max}
}

type binaryCond struct {
	a  Condition
	b  Condition
	op string
}

func (c binaryCond) isCondition() { _ = c.op }
func (c binaryCond) toSQL(b *strings.Builder, args *[]any) {
	qb := getQueryBuffer()
	c.writeSQL(qb)
	b.Write(qb.Bytes())
	putQueryBuffer(qb)
	*args = c.appendArgs(*args)
}
func (c binaryCond) writeSQL(qb *queryBuffer) {
	if c.a == nil && c.b == nil {
		return
	}
	if c.a == nil {
		c.b.writeSQL(qb)
		return
	}
	if c.b == nil {
		c.a.writeSQL(qb)
		return
	}
	qb.WriteString("(")
	c.a.writeSQL(qb)
	qb.WriteString(") ")
	qb.WriteString(c.op)
	qb.WriteString(" (")
	c.b.writeSQL(qb)
	qb.WriteString(")")
}
func (c binaryCond) appendArgs(args []any) []any {
	if c.a != nil {
		args = c.a.appendArgs(args)
	}
	if c.b != nil {
		args = c.b.appendArgs(args)
	}
	return args
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
