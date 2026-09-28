package dema

import (
	"context"
	"reflect"
	"unsafe"

	"github.com/amatsagu/lumo"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type setClause struct {
	val any
	col string
}

type UpdateBuilder[T any] struct {
	cond    Condition
	row     T
	db      *DB
	table   *tableInfo
	set0    setClause
	set1    setClause
	set2    setClause
	set3    setClause
	extra   []setClause
	numSets uint8
	hasRow  bool
}

func (db *DB) Update[T any](rows ...T) UpdateBuilder[T] {
	typ, _ := getModelType[T]()
	var table *tableInfo
	if typ != nil {
		table, _ = db.getTableInfo(typ)
	}
	b := UpdateBuilder[T]{
		db:    db,
		table: table,
	}
	if len(rows) > 0 {
		b.hasRow = true
		b.row = rows[0]
	}
	return b
}

func (b UpdateBuilder[T]) Set[V any](field Field[T, V], val V) UpdateBuilder[T] {
	s := setClause{col: field.Name, val: val}
	switch b.numSets {
	case 0:
		b.set0 = s
	case 1:
		b.set1 = s
	case 2:
		b.set2 = s
	case 3:
		b.set3 = s
	default:
		b.extra = append(b.extra, s)
	}
	b.numSets++
	return b
}

func (b UpdateBuilder[T]) Where(cond Condition) UpdateBuilder[T] {
	b.cond = cond
	return b
}

func (b UpdateBuilder[T]) Row(row T) UpdateBuilder[T] {
	b.hasRow = true
	b.row = row
	return b
}

func (b UpdateBuilder[T]) Run(args ...any) error {
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
	if table == nil {
		typ, _ := getModelType[T]()
		var err error
		table, err = b.db.getTableInfo(typ)
		if err != nil {
			return err
		}
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

func (b UpdateBuilder[T]) executeWithConn(conn *sqlite.Conn, table *tableInfo, cache bool) error {
	if b.hasRow {
		if table.onUpdate != nil {
			rowCopy := b.row
			if err := table.onUpdate(&rowCopy); err != nil {
				return lumo.WrapError(err).
					Include("dema_table", table.name).
					Include("dema_operation", "UPDATE").
					Include("dema_expected", "onUpdate hook validation")
			}
			b.row = rowCopy
		}
		return executeUpdateRow(conn, table, b.row, cache)
	}

	if b.numSets == 0 {
		return lumo.WrapString("no fields specified for update (call Set or pass row)").
			Include("dema_table", table.name).
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "at least one field to update")
	}

	if b.cond == nil {
		return lumo.WrapString("update requires a WHERE condition to prevent accidental full-table updates").
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

	opts := getExecOptions()
	defer putExecOptions(opts)
	opts.Args = opts.Args[:0]

	qb := getQueryBuffer()
	defer putQueryBuffer(qb)

	qb.WriteString(`UPDATE "`)
	qb.WriteString(table.name)
	qb.WriteString(`" SET `)
	for i := uint8(0); i < b.numSets; i++ {
		if i > 0 {
			qb.WriteString(", ")
		}
		var col string
		var val any
		switch i {
		case 0:
			col, val = b.set0.col, b.set0.val
		case 1:
			col, val = b.set1.col, b.set1.val
		case 2:
			col, val = b.set2.col, b.set2.val
		case 3:
			col, val = b.set3.col, b.set3.val
		default:
			s := b.extra[i-4]
			col, val = s.col, s.val
		}
		qb.WriteString(`"`)
		qb.WriteString(col)
		qb.WriteString(`" = ?`)
		opts.Args = append(opts.Args, encodeValue(val))
	}

	qb.WriteString(" WHERE ")
	b.cond.writeSQL(qb)
	opts.Args = b.cond.appendArgs(opts.Args)
	qb.WriteString(";")

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
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "successful execution")
	}
	return nil
}

func (db *DB) UpdateRow(args ...any) error {
	if len(args) == 0 {
		return lumo.WrapString("no struct row provided to UpdateRow").
			Include("dema_table", "").
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "model struct instance")
	}

	ctx := context.Background()
	var row any
	if len(args) == 1 {
		row = args[0]
	} else if len(args) == 2 {
		if c, ok := args[0].(context.Context); ok {
			if c != nil {
				ctx = c
			}
			row = args[1]
		}
	}

	if row != nil {
		if p, typ, ok := extractSingleRow(row); ok {
			table, err := db.getTableInfo(typ)
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
			conn, err := db.pool.Take(ctx)
			if err != nil {
				return lumo.WrapError(err).
					Include("dema_table", table.name).
					Include("dema_operation", "UPDATE").
					Include("dema_expected", "connection from pool")
			}
			defer db.pool.Put(conn)
			return executeUpdateRowSingle(conn, table, p, true)
		}
	}

	var rowVal reflect.Value
	var foundRow bool
	for _, arg := range args {
		if arg == nil {
			continue
		}
		if c, ok := arg.(context.Context); ok && c != nil {
			ctx = c
			continue
		}
		v := reflect.ValueOf(arg)
		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}
		if v.Kind() == reflect.Struct {
			rowVal = v
			foundRow = true
		}
	}

	if !foundRow {
		return lumo.WrapString("no struct row provided to UpdateRow").
			Include("dema_table", "").
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "model struct instance")
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
	return executeUpdateRowSingle(conn, table, unsafe.Pointer(&row), cache)
}

func executeUpdateRowVal(conn *sqlite.Conn, table *tableInfo, rowVal reflect.Value, cache bool) error {
	if !rowVal.CanAddr() {
		addr := reflect.New(rowVal.Type()).Elem()
		addr.Set(rowVal)
		rowVal = addr
	}
	return executeUpdateRowSingle(conn, table, rowVal.Addr().UnsafePointer(), cache)
}

func executeUpdateRowSingle(conn *sqlite.Conn, table *tableInfo, p unsafe.Pointer, cache bool) error {
	if len(table.pkCols) == 0 {
		return lumo.WrapString("update requires model with primary key field marked with 'pk' tag").
			Include("dema_table", table.name).
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "primary key definition")
	}

	mask := table.getNonZeroColMask(p)
	sql, cols := table.getUpdateRowPlan(mask)
	if sql == "" || len(cols) == 0 {
		return nil
	}

	opts := getExecOptions()
	defer putExecOptions(opts)
	opts.Args = opts.Args[:0]

	for _, col := range cols {
		val, _, err := col.extractValue(p)
		if err != nil {
			return lumo.WrapError(err).
				Include("dema_table", table.name).
				Include("dema_operation", "UPDATE").
				Include("dema_expected", "field value extraction")
		}
		opts.Args = append(opts.Args, encodeValue(val))
	}

	var err error
	if cache {
		err = sqlitex.Execute(conn, sql, opts)
	} else {
		err = sqlitex.ExecuteTransient(conn, sql, opts)
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
