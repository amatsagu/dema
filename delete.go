package dema

import (
	"context"
	"reflect"

	"github.com/amatsagu/lumo"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type DeleteBuilder[T any] struct {
	db   *DB
	cond Condition
}

func (db *DB) Delete[T any]() *DeleteBuilder[T] {
	return &DeleteBuilder[T]{
		db: db,
	}
}

func (b *DeleteBuilder[T]) Where(cond Condition) *DeleteBuilder[T] {
	b.cond = cond
	return b
}

func (b *DeleteBuilder[T]) Run(args ...any) error {
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

	if b.cond == nil {
		return lumo.WrapString("WHERE condition required for delete").
			Include("dema_table", table.name).
			Include("dema_operation", "DELETE").
			Include("dema_expected", "explicit WHERE condition")
	}

	if table.onDelete != nil {
		if err := table.onDelete(nil); err != nil {
			return lumo.WrapError(err).
				Include("dema_table", table.name).
				Include("dema_operation", "DELETE").
				Include("dema_expected", "onDelete hook validation")
		}
	}

	conn, err := b.db.pool.Take(ctx)
	if err != nil {
		return lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "DELETE").
			Include("dema_expected", "connection from pool")
	}
	defer b.db.pool.Put(conn)

	return b.executeWithConn(conn, table, cache)
}

func (b *DeleteBuilder[T]) executeWithConn(conn *sqlite.Conn, table *tableInfo, cache bool) error {
	opts := getExecOptions()
	defer putExecOptions(opts)
	opts.Args = opts.Args[:0]

	qb := getQueryBuffer()
	defer putQueryBuffer(qb)

	qb.WriteString(`DELETE FROM "`)
	qb.WriteString(table.name)
	qb.WriteString(`" WHERE `)
	b.cond.writeSQL(qb)
	opts.Args = b.cond.appendArgs(opts.Args)
	qb.WriteString(`;`)

	querySQL := table.getUpdateSQL(qb)

	var err error
	if cache {
		err = sqlitex.Execute(conn, querySQL, opts)
	} else {
		err = sqlitex.ExecuteTransient(conn, querySQL, opts)
	}

	if err != nil {
		return lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "DELETE").
			Include("dema_expected", "successful execution")
	}
	return nil
}
