package dema

import (
	"context"
	"reflect"
	"sync"
	"unsafe"

	"github.com/amatsagu/lumo"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type Tx struct {
	db   *DB
	conn *sqlite.Conn
	ctx  context.Context
	mu   sync.Mutex
	done bool
}

func (db *DB) Transaction(ctx context.Context) (*Tx, error) {
	conn, err := db.pool.Take(ctx)
	if err != nil {
		return nil, lumo.WrapError(err).
			Include("dema_table", "").
			Include("dema_operation", "TRANSACTION").
			Include("dema_expected", "connection from pool")
	}

	if err := sqlitex.Execute(conn, "BEGIN;", nil); err != nil {
		db.pool.Put(conn)
		return nil, lumo.WrapError(err).
			Include("dema_table", "").
			Include("dema_operation", "BEGIN").
			Include("dema_expected", "successful transaction start")
	}

	return &Tx{
		db:   db,
		conn: conn,
		ctx:  ctx,
	}, nil
}

func (tx *Tx) Commit() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.done {
		return lumo.WrapString("transaction is already closed").
			Include("dema_table", "").
			Include("dema_operation", "COMMIT").
			Include("dema_expected", "open transaction")
	}
	tx.done = true
	defer tx.db.pool.Put(tx.conn)

	if err := sqlitex.Execute(tx.conn, "COMMIT;", nil); err != nil {
		return lumo.WrapError(err).
			Include("dema_table", "").
			Include("dema_operation", "COMMIT").
			Include("dema_expected", "successful transaction commit")
	}
	return nil
}

func (tx *Tx) Rollback() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.done {
		return nil
	}
	tx.done = true
	defer tx.db.pool.Put(tx.conn)

	oldCh := tx.conn.SetInterrupt(nil)
	defer tx.conn.SetInterrupt(oldCh)

	if !tx.conn.AutocommitEnabled() {
		if err := sqlitex.Execute(tx.conn, "ROLLBACK;", nil); err != nil {
			return lumo.WrapError(err).
				Include("dema_table", "").
				Include("dema_operation", "ROLLBACK").
				Include("dema_expected", "successful transaction rollback")
		}
	}
	return nil
}

func (tx *Tx) checkActive() error {
	if tx.done {
		return lumo.WrapString("transaction is already closed").
			Include("dema_table", "").
			Include("dema_operation", "EXEC").
			Include("dema_expected", "open transaction")
	}
	if err := tx.ctx.Err(); err != nil {
		return lumo.WrapError(err).
			Include("dema_table", "").
			Include("dema_operation", "EXEC").
			Include("dema_expected", "active context")
	}
	return nil
}

type TxSelectBuilder[T any] struct {
	tx      *Tx
	builder SelectBuilder[T]
}

func (tx *Tx) Select[T any]() *TxSelectBuilder[T] {
	return &TxSelectBuilder[T]{
		tx:      tx,
		builder: tx.db.Select[T](),
	}
}

func (b *TxSelectBuilder[T]) Where(cond Condition) *TxSelectBuilder[T] {
	b.builder = b.builder.Where(cond)
	return b
}

func (b *TxSelectBuilder[T]) OrderBy[V any](field Field[T, V], dir OrderDirection) *TxSelectBuilder[T] {
	b.builder = b.builder.OrderBy(field, dir)
	return b
}

func (b *TxSelectBuilder[T]) Limit(amount int) *TxSelectBuilder[T] {
	b.builder = b.builder.Limit(amount)
	return b
}

func (b *TxSelectBuilder[T]) Page(current, size int) *TxSelectBuilder[T] {
	b.builder = b.builder.Page(current, size)
	return b
}

func (b *TxSelectBuilder[T]) Run(cache bool) ([]T, error) {
	if err := b.tx.checkActive(); err != nil {
		return nil, err
	}

	table := b.builder.table
	isPtrModel := b.builder.isPtrModel
	if table == nil {
		typ, isPtr := getModelType[T]()
		isPtrModel = isPtr
		var err error
		table, err = b.tx.db.getTableInfo(typ)
		if err != nil {
			return nil, err
		}
	}

	opts := getExecOptions()
	opts.Args = opts.Args[:0]

	querySQL := b.builder.buildSQL(table, opts)

	if b.builder.hasLim && b.builder.limit == 1 && !isPtrModel {
		results := make([]T, 1)
		rowPtr := unsafe.Pointer(&results[0])
		initialArgsLen := len(opts.Args)
		opts.ResultFunc = func(stmt *sqlite.Stmt) error {
			opts.Args = append(opts.Args, nil)
			return table.scanRowDirectPtr(stmt, rowPtr)
		}

		var err error
		if cache {
			err = sqlitex.Execute(b.tx.conn, querySQL, opts)
		} else {
			err = sqlitex.ExecuteTransient(b.tx.conn, querySQL, opts)
		}
		found := len(opts.Args) > initialArgsLen
		putExecOptions(opts)

		if err != nil {
			return nil, lumo.WrapError(err).
				Include("dema_table", table.name).
				Include("dema_operation", "SELECT").
				Include("dema_expected", "successful execution")
		}

		if !found {
			return results[:0], nil
		}
		return results, nil
	}

	capHint := 16
	if b.builder.hasLim && b.builder.limit > 0 {
		capHint = b.builder.limit
	}
	results := make([]T, 0, capHint)

	if isPtrModel {
		opts.ResultFunc = func(stmt *sqlite.Stmt) error {
			elem := reflect.New(table.typ)
			if err := table.scanRowDirectPtr(stmt, elem.UnsafePointer()); err != nil {
				return err
			}
			results = append(results, elem.Interface().(T))
			return nil
		}
	} else {
		opts.ResultFunc = func(stmt *sqlite.Stmt) error {
			idx := len(results)
			if idx == cap(results) {
				newCap := cap(results) * 2
				if newCap == 0 {
					newCap = 4
				}
				newRes := make([]T, len(results), newCap)
				copy(newRes, results)
				results = newRes
			}
			results = results[:idx+1]
			rowPtr := unsafe.Pointer(&results[idx])
			if err := table.scanRowDirectPtr(stmt, rowPtr); err != nil {
				results = results[:idx]
				return err
			}
			return nil
		}
	}

	var err error
	if cache {
		err = sqlitex.Execute(b.tx.conn, querySQL, opts)
	} else {
		err = sqlitex.ExecuteTransient(b.tx.conn, querySQL, opts)
	}
	putExecOptions(opts)

	if err != nil {
		return nil, lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "SELECT").
			Include("dema_expected", "successful execution")
	}

	return results, nil
}

func (tx *Tx) Insert(args ...any) error {
	if err := tx.checkActive(); err != nil {
		return err
	}
	if len(args) == 0 {
		return nil
	}

	var row any
	if len(args) == 1 {
		row = args[0]
	} else if len(args) == 2 {
		if _, ok := args[0].(context.Context); ok {
			row = args[1]
		}
	}

	if row != nil {
		if p, typ, ok := extractSingleRow(row); ok {
			table, err := tx.db.getTableInfo(typ)
			if err != nil {
				return err
			}
			if table.onInsert != nil {
				rowPtr := reflect.New(table.typ)
				copy(unsafe.Slice((*byte)(rowPtr.UnsafePointer()), table.typ.Size()), unsafe.Slice((*byte)(p), table.typ.Size()))
				if err := table.onInsert(rowPtr.Interface()); err != nil {
					return lumo.WrapError(err).
						Include("dema_table", table.name).
						Include("dema_operation", "INSERT").
						Include("dema_expected", "onInsert hook validation")
				}
				p = rowPtr.UnsafePointer()
			}
			return executeInsertSingle(tx.conn, table, p, true)
		}
	}

	_, rowVals, err := parseContextAndRows(args)
	if err != nil || len(rowVals) == 0 {
		return err
	}

	typ := rowVals[0].Type()
	table, err := tx.db.getTableInfo(typ)
	if err != nil {
		return err
	}

	for i := range rowVals {
		if table.onInsert != nil {
			rowPtr := reflect.New(typ)
			rowPtr.Elem().Set(rowVals[i])
			if err := table.onInsert(rowPtr.Interface()); err != nil {
				return lumo.WrapError(err).
					Include("dema_table", table.name).
					Include("dema_operation", "INSERT").
					Include("dema_expected", "onInsert hook validation")
			}
			rowVals[i] = rowPtr.Elem()
		}
	}

	return executeInsertRowVals(tx.conn, table, rowVals, true)
}

func (tx *Tx) Upsert(args ...any) error {
	if err := tx.checkActive(); err != nil {
		return err
	}
	if len(args) == 0 {
		return nil
	}

	var row any
	if len(args) == 1 {
		row = args[0]
	} else if len(args) == 2 {
		if _, ok := args[0].(context.Context); ok {
			row = args[1]
		}
	}

	if row != nil {
		if p, typ, ok := extractSingleRow(row); ok {
			table, err := tx.db.getTableInfo(typ)
			if err != nil {
				return err
			}
			if len(table.pkCols) == 0 {
				return lumo.WrapString("upsert requires at least one primary key field marked with 'pk' tag").
					Include("dema_table", table.name).
					Include("dema_operation", "UPSERT").
					Include("dema_expected", "primary key definition")
			}
			if table.onInsert != nil {
				rowPtr := reflect.New(table.typ)
				copy(unsafe.Slice((*byte)(rowPtr.UnsafePointer()), table.typ.Size()), unsafe.Slice((*byte)(p), table.typ.Size()))
				if err := table.onInsert(rowPtr.Interface()); err != nil {
					return lumo.WrapError(err).
						Include("dema_table", table.name).
						Include("dema_operation", "UPSERT").
						Include("dema_expected", "onInsert hook validation")
				}
				p = rowPtr.UnsafePointer()
			}
			return executeUpsertSingle(tx.conn, table, p, true)
		}
	}

	_, rowVals, err := parseContextAndRows(args)
	if err != nil || len(rowVals) == 0 {
		return err
	}

	typ := rowVals[0].Type()
	table, err := tx.db.getTableInfo(typ)
	if err != nil {
		return err
	}

	if len(table.pkCols) == 0 {
		return lumo.WrapString("upsert requires at least one primary key field marked with 'pk' tag").
			Include("dema_table", table.name).
			Include("dema_operation", "UPSERT").
			Include("dema_expected", "primary key definition")
	}

	for i := range rowVals {
		if table.onInsert != nil {
			rowPtr := reflect.New(typ)
			rowPtr.Elem().Set(rowVals[i])
			if err := table.onInsert(rowPtr.Interface()); err != nil {
				return lumo.WrapError(err).
					Include("dema_table", table.name).
					Include("dema_operation", "UPSERT").
					Include("dema_expected", "onInsert hook validation")
			}
			rowVals[i] = rowPtr.Elem()
		}
	}

	return executeUpsertRowVals(tx.conn, table, rowVals, true)
}

type TxUpdateBuilder[T any] struct {
	tx      *Tx
	builder UpdateBuilder[T]
}

func (tx *Tx) Update[T any](rows ...T) *TxUpdateBuilder[T] {
	return &TxUpdateBuilder[T]{
		tx:      tx,
		builder: tx.db.Update(rows...),
	}
}

func (b *TxUpdateBuilder[T]) Set[V any](field Field[T, V], val V) *TxUpdateBuilder[T] {
	b.builder = b.builder.Set(field, val)
	return b
}

func (b *TxUpdateBuilder[T]) Where(cond Condition) *TxUpdateBuilder[T] {
	b.builder = b.builder.Where(cond)
	return b
}

func (b *TxUpdateBuilder[T]) Row(row T) *TxUpdateBuilder[T] {
	b.builder = b.builder.Row(row)
	return b
}

func (b *TxUpdateBuilder[T]) Run(cache bool) error {
	if err := b.tx.checkActive(); err != nil {
		return err
	}

	table := b.builder.table
	if table == nil {
		typ, _ := getModelType[T]()
		var err error
		table, err = b.tx.db.getTableInfo(typ)
		if err != nil {
			return err
		}
	}

	return b.builder.executeWithConn(b.tx.conn, table, cache)
}

func (tx *Tx) UpdateRow(args ...any) error {
	if err := tx.checkActive(); err != nil {
		return err
	}
	if len(args) == 0 {
		return lumo.WrapString("no struct row provided for update").
			Include("dema_table", "").
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "model struct")
	}

	var row any
	if len(args) == 1 {
		row = args[0]
	} else if len(args) == 2 {
		if _, ok := args[0].(context.Context); ok {
			row = args[1]
		}
	}

	if row != nil {
		if p, typ, ok := extractSingleRow(row); ok {
			table, err := tx.db.getTableInfo(typ)
			if err != nil {
				return err
			}
			if len(table.pkCols) == 0 {
				return lumo.WrapString("update requires model with primary key field marked with 'pk' tag").
					Include("dema_table", table.name).
					Include("dema_operation", "UPDATE").
					Include("dema_expected", "primary key definition")
			}
			if table.onUpdate != nil {
				rowPtr := reflect.New(table.typ)
				copy(unsafe.Slice((*byte)(rowPtr.UnsafePointer()), table.typ.Size()), unsafe.Slice((*byte)(p), table.typ.Size()))
				if err := table.onUpdate(rowPtr.Interface()); err != nil {
					return lumo.WrapError(err).
						Include("dema_table", table.name).
						Include("dema_operation", "UPDATE").
						Include("dema_expected", "onUpdate hook validation")
				}
				p = rowPtr.UnsafePointer()
			}
			return executeUpdateRowSingle(tx.conn, table, p, true)
		}
	}

	_, rowVals, err := parseContextAndRows(args)
	if err != nil || len(rowVals) == 0 {
		return lumo.WrapString("no struct row provided for update").
			Include("dema_table", "").
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "model struct")
	}

	rowVal := rowVals[0]
	typ := rowVal.Type()
	table, err := tx.db.getTableInfo(typ)
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

	return executeUpdateRowVal(tx.conn, table, rowVal, true)
}

type TxDeleteBuilder[T any] struct {
	tx      *Tx
	builder DeleteBuilder[T]
}

func (tx *Tx) Delete[T any]() *TxDeleteBuilder[T] {
	return &TxDeleteBuilder[T]{
		tx:      tx,
		builder: tx.db.Delete[T](),
	}
}

func (b *TxDeleteBuilder[T]) Where(cond Condition) *TxDeleteBuilder[T] {
	b.builder = b.builder.Where(cond)
	return b
}

func (b *TxDeleteBuilder[T]) Run(cache bool) error {
	if err := b.tx.checkActive(); err != nil {
		return err
	}

	typ, _ := getModelType[T]()
	table, err := b.tx.db.getTableInfo(typ)
	if err != nil {
		return err
	}

	if b.builder.cond == nil {
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

	return b.builder.executeWithConn(b.tx.conn, table, cache)
}

func (tx *Tx) RawQuery[T any](sql string, cache bool, args ...any) ([]T, error) {
	if err := tx.checkActive(); err != nil {
		return nil, err
	}

	typ, _ := getModelType[T]()
	if isScalarType(typ) {
		return executeRawQueryScalar[T](tx.conn, sql, cache, args...)
	}

	table, err := tx.db.getTableInfo(typ)
	if err != nil {
		return nil, err
	}

	return executeRawQuery[T](tx.conn, table, sql, cache, args...)
}

func (tx *Tx) RawExecute(sql string, cache bool, args ...any) error {
	if err := tx.checkActive(); err != nil {
		return err
	}

	return executeRawExecute(tx.conn, sql, cache, args...)
}

type TxCountBuilder[T any] struct {
	tx      *Tx
	builder CountBuilder[T]
}

func (tx *Tx) Count[T any](field ...any) *TxCountBuilder[T] {
	return &TxCountBuilder[T]{
		tx:      tx,
		builder: tx.db.Count[T](field...),
	}
}

func (b *TxCountBuilder[T]) Where(cond Condition) *TxCountBuilder[T] {
	b.builder = b.builder.Where(cond)
	return b
}

func (b *TxCountBuilder[T]) Limit(amount int) *TxCountBuilder[T] {
	b.builder = b.builder.Limit(amount)
	return b
}

func (b *TxCountBuilder[T]) Offset(amount int) *TxCountBuilder[T] {
	b.builder = b.builder.Offset(amount)
	return b
}

func (b *TxCountBuilder[T]) Page(current, size int) *TxCountBuilder[T] {
	b.builder = b.builder.Page(current, size)
	return b
}

func (b *TxCountBuilder[T]) SQL() string {
	return b.builder.SQL()
}

func (b *TxCountBuilder[T]) Run(cache bool) (int64, error) {
	if err := b.tx.checkActive(); err != nil {
		return 0, err
	}

	table := b.builder.table
	if table == nil {
		typ, _ := getModelType[T]()
		var err error
		table, err = b.tx.db.getTableInfo(typ)
		if err != nil {
			return 0, err
		}
	}

	return b.builder.executeWithConn(b.tx.conn, table, cache)
}
