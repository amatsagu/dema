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
	cond        Condition
	table       *tableInfo
	db          *DB
	conn        *sqlite.Conn
	order0      orderClause
	order1      orderClause
	extraOrders []orderClause
	limit       int
	offset      int
	numOrders   uint8
	hasLim      bool
	hasOff      bool
	isPtrModel  bool
}

func (db *DB) Select[T any]() SelectBuilder[T] {
	typ, isPtr := getModelType[T]()
	var table *tableInfo
	if typ != nil {
		table = db.tableInfo(typ)
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
	o := orderClause{col: field.Name, dir: dir}
	switch b.numOrders {
	case 0:
		b.order0 = o
	case 1:
		b.order1 = o
	default:
		b.extraOrders = append(b.extraOrders, o)
	}
	b.numOrders++
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
	if b.cond == nil && b.numOrders == 0 && !b.hasLim && !b.hasOff {
		return table.defaultSelectSQL
	}

	if b.numOrders == 0 && !b.hasOff {
		if eq, ok := b.cond.(fastEqualCondition); ok {
			if col, ok := table.colByName[eq.equalCol()]; ok {
				if b.hasLim && b.limit == 1 {
					opts.Args = append(opts.Args, eq.equalArg())
					return col.equalLimit1SQL
				}
				if !b.hasLim {
					opts.Args = append(opts.Args, eq.equalArg())
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

	if b.numOrders > 0 {
		qb.WriteString(" ORDER BY ")
		for i := uint8(0); i < b.numOrders; i++ {
			if i > 0 {
				qb.WriteString(", ")
			}
			var ord orderClause
			switch i {
			case 0:
				ord = b.order0
			case 1:
				ord = b.order1
			default:
				ord = b.extraOrders[i-2]
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

	return table.getSelectSQL(qb.buf)
}

func (b SelectBuilder[T]) Run(args ...any) ([]T, error) {
	return b.run(args)
}

func (b SelectBuilder[T]) run(args []any) ([]T, error) {
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
	opts.Args = opts.Args[:0]

	querySQL := b.buildSQL(table, opts)

	conn, err := b.db.pool.Take(ctx)
	if err != nil {
		putExecOptions(opts)
		return nil, lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "SELECT").
			Include("dema_expected", "connection from pool")
	}

	if b.hasLim && b.limit == 1 && !isPtrModel {
		results := make([]T, 1)
		rowPtr := unsafe.Pointer(&results[0])
		initialArgsLen := len(opts.Args)
		opts.ResultFunc = func(stmt *sqlite.Stmt) error {
			opts.Args = append(opts.Args, nil)
			return table.scanRowDirectPtr(stmt, rowPtr)
		}

		var err error
		if cache {
			err = sqlitex.Execute(conn, querySQL, opts)
		} else {
			err = sqlitex.ExecuteTransient(conn, querySQL, opts)
		}
		found := len(opts.Args) > initialArgsLen
		putExecOptions(opts)
		b.db.pool.Put(conn)

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
	putExecOptions(opts)
	b.db.pool.Put(conn)

	if err != nil {
		return nil, lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "SELECT").
			Include("dema_expected", "successful execution")
	}

	return results, nil
}
