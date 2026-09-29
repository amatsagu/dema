package dema

import (
	"context"
	"reflect"

	"github.com/amatsagu/lumo"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type CountBuilder[T any] struct {
	db     *DB
	table  *tableInfo
	cond   Condition
	column string
	limit  int
	offset int
	hasLim bool
	hasOff bool
}

func extractCountColumn(field []any) string {
	if len(field) == 0 || field[0] == nil {
		return ""
	}
	switch f := field[0].(type) {
	case string:
		if f == "*" {
			return ""
		}
		return f
	case interface{ ColumnName() string }:
		return f.ColumnName()
	case interface{ String() string }:
		s := f.String()
		if s == "*" {
			return ""
		}
		return s
	default:
		return ""
	}
}

func (db *DB) Count[T any](field ...any) CountBuilder[T] {
	typ, _ := getModelType[T]()
	var table *tableInfo
	if typ != nil {
		table = db.tableInfo(typ)
	}
	return CountBuilder[T]{
		db:     db,
		table:  table,
		column: extractCountColumn(field),
	}
}

func Count[T any](args ...any) CountBuilder[T] {
	var db *DB
	var field []any
	for _, arg := range args {
		if d, ok := arg.(*DB); ok {
			db = d
		} else {
			field = append(field, arg)
		}
	}
	if db != nil {
		return db.Count[T](field...)
	}
	typ, _ := getModelType[T]()
	var table *tableInfo
	if typ != nil && typ.Kind() == reflect.Struct {
		table, _ = getAdHocTableInfo(typ)
	}
	return CountBuilder[T]{
		table:  table,
		column: extractCountColumn(field),
	}
}

func (b CountBuilder[T]) Where(cond Condition) CountBuilder[T] {
	b.cond = cond
	return b
}

func (b CountBuilder[T]) Limit(amount int) CountBuilder[T] {
	if amount < 0 {
		amount = 0
	}
	b.limit = amount
	b.hasLim = true
	return b
}

func (b CountBuilder[T]) Offset(amount int) CountBuilder[T] {
	if amount < 0 {
		amount = 0
	}
	b.offset = amount
	b.hasOff = true
	return b
}

func (b CountBuilder[T]) Page(current, size int) CountBuilder[T] {
	if current < 1 {
		current = 1
	}
	if size < 0 {
		size = 0
	}
	b.limit = size
	b.offset = (current - 1) * size
	b.hasLim = true
	b.hasOff = true
	return b
}

func (b CountBuilder[T]) buildSQL(table *tableInfo, opts *sqlitex.ExecOptions) string {
	if b.cond == nil && !b.hasLim && !b.hasOff {
		if b.column == "" || b.column == "*" {
			if table.defaultCountSQL != "" {
				return table.defaultCountSQL
			}
		} else if col, ok := table.colByName[b.column]; ok && col.countSQL != "" {
			return col.countSQL
		}
	}

	qb := getQueryBuffer()
	defer putQueryBuffer(qb)

	qb.WriteString(`SELECT COUNT(`)
	if b.column == "" || b.column == "*" {
		qb.WriteString(`*`)
	} else {
		qb.WriteString(`"`)
		qb.WriteString(b.column)
		qb.WriteString(`"`)
	}
	qb.WriteString(`) FROM "`)
	qb.WriteString(table.name)
	qb.WriteString(`"`)

	if b.cond != nil {
		qb.WriteString(` WHERE `)
		b.cond.writeSQL(qb)
		opts.Args = b.cond.appendArgs(opts.Args)
	}

	if b.hasLim {
		qb.WriteString(` LIMIT `)
		qb.WriteInt(b.limit)
	}
	if b.hasOff {
		qb.WriteString(` OFFSET `)
		qb.WriteInt(b.offset)
	}
	qb.WriteString(`;`)

	return table.getSelectSQL(qb.buf)
}

func (b CountBuilder[T]) SQL() string {
	table := b.table
	if table == nil && b.db != nil {
		typ, _ := getModelType[T]()
		table = b.db.tableInfo(typ)
	}
	if table == nil {
		typ, _ := getModelType[T]()
		table, _ = getAdHocTableInfo(typ)
	}
	if table == nil {
		return ""
	}
	opts := getExecOptions()
	defer putExecOptions(opts)
	opts.Args = opts.Args[:0]
	return b.buildSQL(table, opts)
}

func (b CountBuilder[T]) Run(ctx context.Context, cache bool) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	table := b.table
	if table == nil {
		typ, _ := getModelType[T]()
		var err error
		table, err = b.db.getTableInfo(typ)
		if err != nil {
			return 0, err
		}
	}

	conn, err := b.db.pool.Take(ctx)
	if err != nil {
		return 0, lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "COUNT").
			Include("dema_expected", "connection from pool")
	}
	defer b.db.pool.Put(conn)

	return b.executeWithConn(conn, table, cache)
}

func (b CountBuilder[T]) executeWithConn(conn *sqlite.Conn, table *tableInfo, cache bool) (int64, error) {
	opts := getExecOptions()
	opts.Args = opts.Args[:0]

	querySQL := b.buildSQL(table, opts)

	var count int64
	opts.ResultFunc = func(stmt *sqlite.Stmt) error {
		count = stmt.ColumnInt64(0)
		return nil
	}

	var err error
	if cache {
		err = sqlitex.Execute(conn, querySQL, opts)
	} else {
		err = sqlitex.ExecuteTransient(conn, querySQL, opts)
	}
	putExecOptions(opts)

	if err != nil {
		return 0, lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "COUNT").
			Include("dema_expected", "successful execution")
	}

	return count, nil
}
