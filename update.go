package dema

import (
	"context"
	"reflect"
	"strings"

	"github.com/amatsagu/lumo"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type setClause struct {
	col string
	val any
}

type UpdateBuilder[T any] struct {
	db     *DB
	sets   []setClause
	cond   Condition
	hasRow bool
	row    T
}

func (db *DB) Update[T any](rows ...T) *UpdateBuilder[T] {
	b := &UpdateBuilder[T]{
		db: db,
	}
	if len(rows) > 0 {
		b.hasRow = true
		b.row = rows[0]
	}
	return b
}

func (b *UpdateBuilder[T]) Set[V any](field Field[T, V], val V) *UpdateBuilder[T] {
	b.sets = append(b.sets, setClause{
		col: field.Name,
		val: val,
	})
	return b
}

func (b *UpdateBuilder[T]) Where(cond Condition) *UpdateBuilder[T] {
	b.cond = cond
	return b
}

func (b *UpdateBuilder[T]) Row(row T) *UpdateBuilder[T] {
	b.hasRow = true
	b.row = row
	return b
}

func (b *UpdateBuilder[T]) Run(args ...any) error {
	ctx := context.Background()
	cache := true

	for _, arg := range args {
		switch a := arg.(type) {
		case context.Context:
			if a != nil {
				ctx = a
			}
		case bool:
			cache = a
		}
	}

	var zero T
	typ := reflect.TypeOf(zero)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	table, err := b.db.getTableInfo(typ)
	if err != nil {
		return err
	}

	conn, err := b.db.pool.Take(ctx)
	if err != nil {
		return lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "connection from pool")
	}
	defer b.db.pool.Put(conn)

	return b.executeWithConn(conn, table, cache)
}

func (b *UpdateBuilder[T]) executeWithConn(conn *sqlite.Conn, table *tableInfo, cache bool) error {
	if b.hasRow {
		if table.onUpdate != nil {
			if err := table.onUpdate(&b.row); err != nil {
				return lumo.WrapError(err).
					Include("dema_table", table.name).
					Include("dema_operation", "UPDATE").
					Include("dema_expected", "onUpdate hook validation")
			}
		}
		return executeUpdateRow(conn, table, b.row, cache)
	}

	if len(b.sets) == 0 {
		return lumo.WrapString("no fields specified for update (call Set or pass row)").
			Include("dema_table", table.name).
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "at least one field to update")
	}
	if b.cond == nil {
		return lumo.WrapString("WHERE condition required for update").
			Include("dema_table", table.name).
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "explicit WHERE condition")
	}

	if table.onUpdate != nil {
		if err := table.onUpdate(nil); err != nil {
			return lumo.WrapError(err).
				Include("dema_table", table.name).
				Include("dema_operation", "UPDATE").
				Include("dema_expected", "onUpdate hook validation")
		}
	}

	var sb strings.Builder
	var args []any

	sb.WriteString(`UPDATE "`)
	sb.WriteString(table.name)
	sb.WriteString(`" SET `)
	for i, s := range b.sets {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(`"`)
		sb.WriteString(s.col)
		sb.WriteString(`" = ?`)
		args = append(args, encodeValue(s.val))
	}

	sb.WriteString(" WHERE ")
	b.cond.toSQL(&sb, &args)
	sb.WriteString(";")

	opts := &sqlitex.ExecOptions{Args: args}
	var err error
	if cache {
		err = sqlitex.Execute(conn, sb.String(), opts)
	} else {
		err = sqlitex.ExecuteTransient(conn, sb.String(), opts)
	}

	if err != nil {
		return lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "successful execution")
	}
	return nil
}

func (db *DB) UpdateRow(args ...any) error {
	ctx := context.Background()
	var rowVal reflect.Value
	var found bool

	for _, arg := range args {
		if c, ok := arg.(context.Context); ok && c != nil {
			ctx = c
		} else if arg != nil {
			v := reflect.ValueOf(arg)
			if v.Kind() == reflect.Pointer {
				v = v.Elem()
			}
			if v.Kind() == reflect.Struct {
				rowVal = v
				found = true
			}
		}
	}

	if !found {
		return lumo.WrapString("no struct row provided for update").
			Include("dema_table", "").
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "model struct")
	}

	typ := rowVal.Type()
	table, err := db.getTableInfo(typ)
	if err != nil {
		return err
	}

	if table.onUpdate != nil {
		rowPtr := reflect.New(typ)
		rowPtr.Elem().Set(rowVal)
		if err := table.onUpdate(rowPtr.Interface()); err != nil {
			return lumo.WrapError(err).
				Include("dema_table", table.name).
				Include("dema_operation", "UPDATE").
				Include("dema_expected", "onUpdate hook validation")
		}
		rowVal = rowPtr.Elem()
	}

	conn, err := db.pool.Take(ctx)
	if err != nil {
		return lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "connection from pool")
	}
	defer db.pool.Put(conn)

	return executeUpdateRowVal(conn, table, rowVal, true)
}

func executeUpdateRow[T any](conn *sqlite.Conn, table *tableInfo, row T, cache bool) error {
	rowVal := reflect.ValueOf(&row).Elem()
	return executeUpdateRowVal(conn, table, rowVal, cache)
}

func executeUpdateRowVal(conn *sqlite.Conn, table *tableInfo, rowVal reflect.Value, cache bool) error {
	if !rowVal.CanAddr() {
		addr := reflect.New(rowVal.Type()).Elem()
		addr.Set(rowVal)
		rowVal = addr
	}

	if len(table.pkCols) == 0 {
		return lumo.WrapString("update row requires primary key fields marked with 'pk' tag").
			Include("dema_table", table.name).
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "primary key definition")
	}

	var setCols []string
	var args []any

	for _, col := range table.nonPkCols {
		val, include, err := table.extractColValue(col, rowVal)
		if err != nil {
			return lumo.WrapError(err).
				Include("dema_table", table.name).
				Include("dema_operation", "UPDATE").
				Include("dema_expected", "field value extraction")
		}
		if !include {
			continue
		}
		setCols = append(setCols, col.name)
		args = append(args, encodeValue(val))
	}

	if len(setCols) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString(`UPDATE "`)
	sb.WriteString(table.name)
	sb.WriteString(`" SET `)
	for i, name := range setCols {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(`"`)
		sb.WriteString(name)
		sb.WriteString(`" = ?`)
	}

	sb.WriteString(` WHERE `)
	for i, pk := range table.pkCols {
		if i > 0 {
			sb.WriteString(` AND `)
		}
		sb.WriteString(`"`)
		sb.WriteString(pk.name)
		sb.WriteString(`" = ?`)

		val, _, err := table.extractColValue(pk, rowVal)
		if err != nil {
			return lumo.WrapError(err).
				Include("dema_table", table.name).
				Include("dema_operation", "UPDATE").
				Include("dema_expected", "primary key value extraction")
		}
		args = append(args, encodeValue(val))
	}
	sb.WriteString(`;`)

	opts := &sqlitex.ExecOptions{Args: args}
	var err error
	if cache {
		err = sqlitex.Execute(conn, sb.String(), opts)
	} else {
		err = sqlitex.ExecuteTransient(conn, sb.String(), opts)
	}

	if err != nil {
		return lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "row update")
	}
	return nil
}

func Update(args ...any) error {
	for i, arg := range args {
		if db, ok := arg.(*DB); ok {
			remaining := append(append([]any(nil), args[:i]...), args[i+1:]...)
			return db.UpdateRow(remaining...)
		}
	}
	return lumo.WrapString("no *DB provided to Update").
		Include("dema_table", "").
		Include("dema_operation", "UPDATE").
		Include("dema_expected", "*DB instance")
}
