package dema

import (
	"context"
	"reflect"
	"unsafe"

	"github.com/amatsagu/lumo"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func extractSingleRow(arg any) (unsafe.Pointer, reflect.Type, bool) {
	if arg == nil {
		return nil, nil, false
	}
	typ := reflect.TypeOf(arg)
	if typ == nil {
		return nil, nil, false
	}
	kind := typ.Kind()
	if kind == reflect.Pointer {
		elem := typ.Elem()
		if elem.Kind() != reflect.Struct {
			return nil, nil, false
		}
		e := (*[2]unsafe.Pointer)(unsafe.Pointer(&arg))
		if e[1] == nil {
			return nil, nil, false
		}
		return e[1], elem, true
	}
	if kind == reflect.Struct {
		e := (*[2]unsafe.Pointer)(unsafe.Pointer(&arg))
		return e[1], typ, true
	}
	return nil, nil, false
}

// Persists one or more model rows into the database.
func (db *DB) Insert(args ...any) error {
	if len(args) == 0 {
		return nil
	}
	var ctx context.Context
	cache := true
	var row any
	singleCandidate := true
	for _, arg := range args {
		if arg == nil {
			continue
		}
		switch a := arg.(type) {
		case context.Context:
			ctx = a
		case bool:
			cache = a
		default:
			if row == nil && singleCandidate {
				row = arg
			} else {
				singleCandidate = false
				row = nil
			}
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if row != nil && singleCandidate {
		if p, typ, ok := extractSingleRow(row); ok {
			table, err := db.getTableInfo(typ)
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
			conn, err := db.pool.Take(ctx)
			if err != nil {
				return lumo.WrapError(err).
					Include("dema_table", table.name).
					Include("dema_operation", "INSERT").
					Include("dema_expected", "connection from pool")
			}
			defer db.pool.Put(conn)
			return executeInsertSingle(conn, table, p, cache)
		}
	}

	parsedCtx, parsedCache, rowVals, err := parseContextCacheAndRows(args)
	if err != nil || len(rowVals) == 0 {
		return err
	}
	ctx = parsedCtx
	cache = parsedCache

	typ := rowVals[0].Type()
	table, err := db.getTableInfo(typ)
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

	conn, err := db.pool.Take(ctx)
	if err != nil {
		return lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "INSERT").
			Include("dema_expected", "connection from pool")
	}
	defer db.pool.Put(conn)

	return executeInsertRowVals(conn, table, rowVals, cache)
}

func parseContextCacheAndRows(args []any) (context.Context, bool, []reflect.Value, error) {
	ctx := context.Background()
	cache := true
	if len(args) == 0 {
		return ctx, cache, nil, nil
	}

	var rowVals []reflect.Value
	for _, arg := range args {
		if arg == nil {
			continue
		}
		switch a := arg.(type) {
		case context.Context:
			if a != nil {
				ctx = a
			}
		case bool:
			cache = a
		default:
			v := reflect.ValueOf(arg)
			if v.Kind() == reflect.Slice {
				for j := 0; j < v.Len(); j++ {
					elem := v.Index(j)
					if elem.Kind() == reflect.Pointer {
						elem = elem.Elem()
					}
					if elem.Kind() == reflect.Struct {
						rowVals = append(rowVals, elem)
					}
				}
			} else {
				if v.Kind() == reflect.Pointer {
					v = v.Elem()
				}
				if v.Kind() == reflect.Struct {
					rowVals = append(rowVals, v)
				}
			}
		}
	}

	return ctx, cache, rowVals, nil
}

func parseContextAndRows(args []any) (context.Context, []reflect.Value, error) {
	ctx, _, rows, err := parseContextCacheAndRows(args)
	return ctx, rows, err
}

func executeInsertSingle(conn *sqlite.Conn, table *tableInfo, p unsafe.Pointer, cache bool) error {
	mask := table.getNonZeroColMask(p)
	sql, cols := table.getInsertPlan(mask)

	opts := getExecOptions()
	opts.Args = opts.Args[:0]

	for _, col := range cols {
		val, _, err := col.extractValue(p)
		if err != nil {
			putExecOptions(opts)
			return lumo.WrapError(err).
				Include("dema_table", table.name).
				Include("dema_operation", "INSERT").
				Include("dema_expected", "field value extraction")
		}
		opts.Args = append(opts.Args, val)
	}

	var err error
	if cache {
		err = sqlitex.Execute(conn, sql, opts)
	} else {
		err = sqlitex.ExecuteTransient(conn, sql, opts)
	}
	putExecOptions(opts)
	if err != nil {
		return lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "INSERT").
			Include("dema_expected", "row insertion")
	}
	return nil
}

func executeInsertRowVals(conn *sqlite.Conn, table *tableInfo, rows []reflect.Value, cache bool) error {
	for i := range rows {
		rowVal := rows[i]
		if !rowVal.CanAddr() {
			addr := reflect.New(rowVal.Type()).Elem()
			addr.Set(rowVal)
			rowVal = addr
		}
		p := rowVal.Addr().UnsafePointer()
		if err := executeInsertSingle(conn, table, p, cache); err != nil {
			return err
		}
	}
	return nil
}

// Persists rows or updates non-primary-key fields on conflict with primary keys.
func (db *DB) Upsert(args ...any) error {
	if len(args) == 0 {
		return nil
	}
	var ctx context.Context
	cache := true
	var row any
	singleCandidate := true
	for _, arg := range args {
		if arg == nil {
			continue
		}
		switch a := arg.(type) {
		case context.Context:
			ctx = a
		case bool:
			cache = a
		default:
			if row == nil && singleCandidate {
				row = arg
			} else {
				singleCandidate = false
				row = nil
			}
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if row != nil && singleCandidate {
		if p, typ, ok := extractSingleRow(row); ok {
			table, err := db.getTableInfo(typ)
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
			conn, err := db.pool.Take(ctx)
			if err != nil {
				return lumo.WrapError(err).
					Include("dema_table", table.name).
					Include("dema_operation", "UPSERT").
					Include("dema_expected", "connection from pool")
			}
			defer db.pool.Put(conn)
			return executeUpsertSingle(conn, table, p, cache)
		}
	}

	parsedCtx, parsedCache, rowVals, err := parseContextCacheAndRows(args)
	if err != nil || len(rowVals) == 0 {
		return err
	}
	ctx = parsedCtx
	cache = parsedCache

	typ := rowVals[0].Type()
	table, err := db.getTableInfo(typ)
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

	conn, err := db.pool.Take(ctx)
	if err != nil {
		return lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "UPSERT").
			Include("dema_expected", "connection from pool")
	}
	defer db.pool.Put(conn)

	return executeUpsertRowVals(conn, table, rowVals, cache)
}

func executeUpsertSingle(conn *sqlite.Conn, table *tableInfo, p unsafe.Pointer, cache bool) error {
	mask := table.getNonZeroColMask(p)
	sql, cols := table.getUpsertPlan(mask)
	if len(cols) == 0 {
		return nil
	}

	opts := getExecOptions()
	opts.Args = opts.Args[:0]

	for _, col := range cols {
		val, _, err := col.extractValue(p)
		if err != nil {
			putExecOptions(opts)
			return lumo.WrapError(err).
				Include("dema_table", table.name).
				Include("dema_operation", "UPSERT").
				Include("dema_expected", "field value extraction")
		}
		opts.Args = append(opts.Args, val)
	}

	var err error
	if cache {
		err = sqlitex.Execute(conn, sql, opts)
	} else {
		err = sqlitex.ExecuteTransient(conn, sql, opts)
	}
	putExecOptions(opts)
	if err != nil {
		return lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "UPSERT").
			Include("dema_expected", "row upsert")
	}
	return nil
}

func executeUpsertRowVals(conn *sqlite.Conn, table *tableInfo, rows []reflect.Value, cache bool) error {
	for i := range rows {
		rowVal := rows[i]
		if !rowVal.CanAddr() {
			addr := reflect.New(rowVal.Type()).Elem()
			addr.Set(rowVal)
			rowVal = addr
		}
		p := rowVal.Addr().UnsafePointer()
		if err := executeUpsertSingle(conn, table, p, cache); err != nil {
			return err
		}
	}
	return nil
}
