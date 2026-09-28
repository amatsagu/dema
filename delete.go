package dema

import (
	"context"

	"github.com/amatsagu/lumo"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type DeleteBuilder[T any] struct {
	db    *DB
	table *tableInfo
	cond  Condition
}

func (db *DB) Delete[T any]() *DeleteBuilder[T] {
	typ, _ := getModelType[T]()
	var table *tableInfo
	if typ != nil {
		table, _ = db.getTableInfo(typ)
	}
	return &DeleteBuilder[T]{
		db:    db,
		table: table,
	}
}

func (b *DeleteBuilder[T]) Where(cond Condition) *DeleteBuilder[T] {
	b.cond = cond
	return b
}

func (b *DeleteBuilder[T]) Run(args ...any) error {
	var ctx context.Context
	cache := true

	for _, arg := range args {
		switch a := arg.(type) {
		case context.Context:
			ctx = a
		case bool:
			cache = a
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	table := b.table
	if table == nil {
		typ, _ := getModelType[T]()
		var err error
		table, err = b.db.getTableInfo(typ)
		if err != nil {
			return err
		}
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
