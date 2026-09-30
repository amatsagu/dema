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

func (db *DB) Delete[T any]() DeleteBuilder[T] {
	typ, _ := getModelType[T]()
	var table *tableInfo
	if typ != nil {
		table = db.tableInfo(typ)
	}
	return DeleteBuilder[T]{
		db:    db,
		table: table,
	}
}

func (b DeleteBuilder[T]) Where(cond Condition) DeleteBuilder[T] {
	b.cond = cond
	return b
}

func (b DeleteBuilder[T]) Returning(fields ...any) DeleteReturningBuilder[T] {
	if len(fields) == 0 {
		return DeleteReturningBuilder[T]{
			builder:   b,
			numFields: 0,
		}
	}
	if len(fields) == 1 {
		return DeleteReturningBuilder[T]{
			builder:   b,
			field0:    fields[0],
			numFields: 1,
		}
	}
	return DeleteReturningBuilder[T]{
		builder:   b,
		fields:    fields,
		numFields: 2,
	}
}

func (b DeleteBuilder[T]) Run(ctx context.Context, cache bool) error {
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

func (b DeleteBuilder[T]) executeWithConn(conn *sqlite.Conn, table *tableInfo, cache bool) error {
	opts := getExecOptions()
	opts.Args = opts.Args[:0]

	if eq, ok := b.cond.(fastEqualCondition); ok {
		if col := table.colByName[eq.equalCol()]; col != nil && col.deleteSQL != "" {
			opts.Args = append(opts.Args, eq.equalArg())
			var err error
			if cache {
				err = sqlitex.Execute(conn, col.deleteSQL, opts)
			} else {
				err = sqlitex.ExecuteTransient(conn, col.deleteSQL, opts)
			}
			putExecOptions(opts)
			if err != nil {
				return lumo.WrapError(err).
					Include("dema_table", table.name).
					Include("dema_operation", "DELETE").
					Include("dema_expected", "successful execution")
			}
			return nil
		}
	}

	qb := getQueryBuffer()
	qb.WriteString(`DELETE FROM "`)
	qb.WriteString(table.name)
	qb.WriteString(`" WHERE `)
	b.cond.writeSQL(qb)
	opts.Args = b.cond.appendArgs(opts.Args)
	qb.WriteString(`;`)

	querySQL := table.getUpdateSQL(qb)
	putQueryBuffer(qb)

	var err error
	if cache {
		err = sqlitex.Execute(conn, querySQL, opts)
	} else {
		err = sqlitex.ExecuteTransient(conn, querySQL, opts)
	}
	putExecOptions(opts)

	if err != nil {
		return lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "DELETE").
			Include("dema_expected", "successful execution")
	}
	return nil
}
