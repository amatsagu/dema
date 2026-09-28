package dema

import (
	"context"
	"reflect"
	"unsafe"

	"github.com/amatsagu/lumo"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type orderClause struct {
	col string
	dir OrderDirection
}

type SelectBuilder[T any] struct {
	cond       Condition
	table      *tableInfo
	db         *DB
	conn       *sqlite.Conn
	ordersBuf  [2]orderClause
	orders     []orderClause
	limit      int
	offset     int
	hasLim     bool
	hasOff     bool
	isPtrModel bool
}

func (db *DB) Select[T any]() SelectBuilder[T] {
	typ, isPtr := getModelType[T]()
	var table *tableInfo
	if typ != nil {
		table, _ = db.getTableInfo(typ)
	}
	return SelectBuilder[T]{
		db:         db,
		table:      table,
		isPtrModel: isPtr,
	}
}

func (b SelectBuilder[T]) Where(cond Condition) SelectBuilder[T] {
	b.cond = cond
	return b
}

func (b SelectBuilder[T]) OrderBy[V any](field Field[T, V], dir OrderDirection) SelectBuilder[T] {
	if b.orders == nil {
		b.orders = b.ordersBuf[:0]
	}
	b.orders = append(b.orders, orderClause{
		col: field.Name,
		dir: dir,
	})
	return b
}

func (b SelectBuilder[T]) Limit(amount int) SelectBuilder[T] {
	if amount < 0 {
		amount = 0
	}
	b.limit = amount
	b.hasLim = true
	return b
}

// Page applies 1-indexed pagination (current page, page size).
func (b SelectBuilder[T]) Page(current, size int) SelectBuilder[T] {
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

func (b SelectBuilder[T]) buildSQL(table *tableInfo, opts *sqlitex.ExecOptions) string {
	if b.cond == nil && len(b.orders) == 0 && !b.hasLim && !b.hasOff {
		return table.defaultSelectSQL
	}

	if len(b.orders) == 0 && !b.hasOff {
		if eq, ok := b.cond.(equalCond); ok {
			if col, ok := table.colByName[eq.col]; ok {
				if b.hasLim && b.limit == 1 {
					opts.Args = append(opts.Args, encodeValue(eq.val))
					return col.equalLimit1SQL
				}
				if !b.hasLim {
					opts.Args = append(opts.Args, encodeValue(eq.val))
					return col.equalSQL
				}
			}
		}
	}

	qb := getQueryBuffer()
	defer putQueryBuffer(qb)

	qb.WriteString(table.defaultSelectSQL)

	if b.cond != nil {
		qb.WriteString(" WHERE ")
		b.cond.writeSQL(qb)
		opts.Args = b.cond.appendArgs(opts.Args)
	}

	if len(b.orders) > 0 {
		qb.WriteString(" ORDER BY ")
		for i, ord := range b.orders {
			if i > 0 {
				qb.WriteString(", ")
			}
			qb.WriteString(`"`)
			qb.WriteString(ord.col)
			qb.WriteString(`" `)
			qb.WriteString(string(ord.dir))
		}
	}

	if b.hasLim {
		qb.WriteString(" LIMIT ")
		qb.WriteInt(b.limit)
	}
	if b.hasOff {
		qb.WriteString(" OFFSET ")
		qb.WriteInt(b.offset)
	}

	return table.getSelectSQL(qb)
}

func (b SelectBuilder[T]) Run(args ...any) ([]T, error) {
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

	table := b.table
	isPtrModel := b.isPtrModel
	if table == nil {
		var err error
		var typ reflect.Type
		typ, isPtrModel = getModelType[T]()
		table, err = b.db.getTableInfo(typ)
		if err != nil {
			return nil, err
		}
	}

	opts := getExecOptions()
	defer putExecOptions(opts)
	opts.Args = opts.Args[:0]

	querySQL := b.buildSQL(table, opts)

	conn, err := b.db.pool.Take(ctx)
	if err != nil {
		return nil, lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "SELECT").
			Include("dema_expected", "connection from pool")
	}
	defer b.db.pool.Put(conn)

	capHint := 16
	if b.hasLim && b.limit > 0 {
		capHint = b.limit
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

	if cache {
		err = sqlitex.Execute(conn, querySQL, opts)
	} else {
		err = sqlitex.ExecuteTransient(conn, querySQL, opts)
	}

	if err != nil {
		return nil, lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "SELECT").
			Include("dema_expected", "successful execution")
	}

	return results, nil
}
