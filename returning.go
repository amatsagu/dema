package dema

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"unsafe"

	"github.com/amatsagu/lumo"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func extractReturningColName(f any) string {
	if f == nil {
		return ""
	}
	switch v := f.(type) {
	case string:
		return v
	case interface{ ColumnName() string }:
		return v.ColumnName()
	case fmt.Stringer:
		return v.String()
	default:
		rv := reflect.ValueOf(f)
		if rv.Kind() == reflect.Struct {
			if fName := rv.FieldByName("Name"); fName.IsValid() && fName.Kind() == reflect.String {
				return fName.String()
			}
		}
		return ""
	}
}

func parseReturningCols(table *tableInfo, fields []any) ([]*colInfo, string, error) {
	if len(fields) == 0 || (len(fields) == 1 && fields[0] == nil) {
		return nil, table.returningAllColsSQL, nil
	}

	if len(fields) == 1 {
		name := extractReturningColName(fields[0])
		if name == "" || name == "*" {
			return nil, table.returningAllColsSQL, nil
		}
		col := table.colByName[name]
		if col == nil {
			return nil, "", lumo.WrapString("unknown column in RETURNING: %s", name).
				Include("dema_table", table.name).
				Include("dema_operation", "RETURNING").
				Include("dema_expected", "valid table column")
		}
		return col.singleColMap, col.quotedName, nil
	}

	colMap := make([]*colInfo, 0, len(fields))
	var sb strings.Builder
	for _, f := range fields {
		if f == nil {
			continue
		}
		name := extractReturningColName(f)
		if name == "" || name == "*" {
			continue
		}

		col := table.colByName[name]
		if col == nil {
			return nil, "", lumo.WrapString("unknown column in RETURNING: %s", name).
				Include("dema_table", table.name).
				Include("dema_operation", "RETURNING").
				Include("dema_expected", "valid table column")
		}

		colMap = append(colMap, col)
		if len(colMap) > 1 {
			sb.WriteString(", ")
		}
		sb.WriteString(col.quotedName)
	}

	if len(colMap) == 0 {
		return nil, table.returningAllColsSQL, nil
	}
	return colMap, sb.String(), nil
}

func resolveReturningCols(table *tableInfo, numFields uint8, field0 any, fields []any) ([]*colInfo, string, error) {
	if numFields == 0 || (numFields == 1 && field0 == nil) {
		return nil, table.returningAllColsSQL, nil
	}
	if numFields == 1 {
		name := extractReturningColName(field0)
		if name == "" || name == "*" {
			return nil, table.returningAllColsSQL, nil
		}
		col := table.colByName[name]
		if col == nil {
			return nil, "", lumo.WrapString("unknown column in RETURNING: %s", name).
				Include("dema_table", table.name).
				Include("dema_operation", "RETURNING").
				Include("dema_expected", "valid table column")
		}
		return col.singleColMap, col.quotedName, nil
	}
	return parseReturningCols(table, fields)
}

type UpdateReturningBuilder[T any] struct {
	builder   UpdateBuilder[T]
	field0    any
	numFields uint8
	fields    []any
}

func (b UpdateReturningBuilder[T]) Set[V any](field Field[T, V], val V) UpdateReturningBuilder[T] {
	b.builder = b.builder.Set(field, val)
	return b
}

func (b UpdateReturningBuilder[T]) Where(cond Condition) UpdateReturningBuilder[T] {
	b.builder = b.builder.Where(cond)
	return b
}

func (b UpdateReturningBuilder[T]) Returning(fields ...any) UpdateReturningBuilder[T] {
	if len(fields) == 0 {
		b.field0 = nil
		b.fields = nil
		b.numFields = 0
		return b
	}
	if len(fields) == 1 {
		b.field0 = fields[0]
		b.fields = nil
		b.numFields = 1
		return b
	}
	b.fields = fields
	b.numFields = 2
	return b
}

func (b UpdateReturningBuilder[T]) SQL() (string, error) {
	table := b.builder.table
	if table == nil {
		typ, _ := getModelType[T]()
		var err error
		table, err = b.builder.db.getTableInfo(typ)
		if err != nil {
			return "", err
		}
	}
	opts := getExecOptions()
	opts.Args = opts.Args[:0]
	sql, _, err := b.buildSQL(table, opts)
	putExecOptions(opts)
	return sql, err
}

func (b UpdateReturningBuilder[T]) buildSQL(table *tableInfo, opts *sqlitex.ExecOptions) (string, []*colInfo, error) {
	if b.builder.numSets == 0 && !b.builder.hasRow {
		return "", nil, lumo.WrapString("no fields specified for update").
			Include("dema_table", table.name).
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "at least one field to update")
	}

	if b.builder.cond == nil {
		return "", nil, lumo.WrapString("update requires a WHERE condition to prevent accidental full-table updates").
			Include("dema_table", table.name).
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "explicit WHERE condition")
	}

	colMap, retSQL, err := resolveReturningCols(table, b.numFields, b.field0, b.fields)
	if err != nil {
		return "", nil, err
	}

	qb := getQueryBuffer()
	qb.WriteString(`UPDATE "`)
	qb.WriteString(table.name)
	qb.WriteString(`" SET `)

	if b.builder.hasRow {
		var p unsafe.Pointer
		if rowVal := reflect.ValueOf(b.builder.row); rowVal.Kind() == reflect.Pointer {
			p = rowVal.UnsafePointer()
		} else {
			p = unsafe.Pointer(&b.builder.row)
		}
		for i, col := range table.nonPkCols {
			if i > 0 {
				qb.WriteString(", ")
			}
			qb.WriteString(`"`)
			qb.WriteString(col.name)
			qb.WriteString(`" = ?`)
			val, _, err := col.extractValue(p)
			if err != nil {
				putQueryBuffer(qb)
				return "", nil, lumo.WrapError(err).
					Include("dema_table", table.name).
					Include("dema_operation", "UPDATE").
					Include("dema_expected", "field value extraction")
			}
			opts.Args = append(opts.Args, val)
		}
	} else {
		for i := uint8(0); i < b.builder.numSets; i++ {
			if i > 0 {
				qb.WriteString(", ")
			}
			var col string
			var val any
			switch i {
			case 0:
				col, val = b.builder.set0.col, b.builder.set0.val
			case 1:
				col, val = b.builder.set1.col, b.builder.set1.val
			case 2:
				col, val = b.builder.set2.col, b.builder.set2.val
			case 3:
				col, val = b.builder.set3.col, b.builder.set3.val
			default:
				s := b.builder.extra[i-4]
				col, val = s.col, s.val
			}
			qb.WriteString(`"`)
			qb.WriteString(col)
			qb.WriteString(`" = ?`)
			opts.Args = append(opts.Args, encodeValue(val))
		}
	}

	qb.WriteString(" WHERE ")
	b.builder.cond.writeSQL(qb)
	opts.Args = b.builder.cond.appendArgs(opts.Args)

	qb.WriteString(" RETURNING ")
	qb.WriteString(retSQL)
	qb.WriteString(";")

	querySQL := table.getUpdateSQL(qb)
	putQueryBuffer(qb)
	return querySQL, colMap, nil
}

func (b UpdateReturningBuilder[T]) Run(ctx context.Context, cache bool) ([]T, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	table := b.builder.table
	typ, isPtrModel := getModelType[T]()
	if table == nil {
		var err error
		table, err = b.builder.db.getTableInfo(typ)
		if err != nil {
			return nil, err
		}
	}

	if table.onUpdate != nil {
		if err := table.onUpdate(nil); err != nil {
			return nil, lumo.WrapError(err).
				Include("dema_table", table.name).
				Include("dema_operation", "UPDATE").
				Include("dema_expected", "onUpdate hook validation")
		}
	}

	conn, err := b.builder.db.pool.Take(ctx)
	if err != nil {
		return nil, lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "connection from pool")
	}
	defer b.builder.db.pool.Put(conn)

	return b.executeWithConn(conn, table, isPtrModel, cache)
}

func (b UpdateReturningBuilder[T]) executeWithConn(conn *sqlite.Conn, table *tableInfo, isPtrModel bool, cache bool) ([]T, error) {
	opts := getExecOptions()
	opts.Args = opts.Args[:0]

	var querySQL string
	var colMap []*colInfo

	if eq, ok := b.builder.cond.(fastEqualCondition); ok && b.builder.numSets <= 2 && !b.builder.hasRow && (b.numFields == 0 || (b.numFields == 1 && b.field0 == nil)) {
		querySQL = table.getFastUpdateReturningEqualSQL(b.builder.numSets, b.builder.set0.col, b.builder.set1.col, eq.equalCol())
		if querySQL != "" {
			opts.Args = append(opts.Args, encodeValue(b.builder.set0.val))
			if b.builder.numSets == 2 {
				opts.Args = append(opts.Args, encodeValue(b.builder.set1.val))
			}
			opts.Args = append(opts.Args, eq.equalArg())
			colMap = nil
		}
	}

	if querySQL == "" {
		var err error
		querySQL, colMap, err = b.buildSQL(table, opts)
		if err != nil {
			putExecOptions(opts)
			return nil, err
		}
	}

	results := make([]T, 0, 1)
	if isPtrModel {
		opts.ResultFunc = func(stmt *sqlite.Stmt) error {
			elem := reflect.New(table.typ)
			p := elem.UnsafePointer()
			var err error
			if len(colMap) == 0 {
				err = table.scanRowDirectPtr(stmt, p)
			} else {
				err = table.scanRowMappedPtr(stmt, colMap, p)
			}
			if err != nil {
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
					newCap = 1
				}
				newRes := make([]T, len(results), newCap)
				copy(newRes, results)
				results = newRes
			}
			results = results[:idx+1]
			rowPtr := unsafe.Pointer(&results[idx])
			var err error
			if len(colMap) == 0 {
				err = table.scanRowDirectPtr(stmt, rowPtr)
			} else {
				err = table.scanRowMappedPtr(stmt, colMap, rowPtr)
			}
			if err != nil {
				results = results[:idx]
				return err
			}
			return nil
		}
	}

	var err error
	if cache {
		err = sqlitex.Execute(conn, querySQL, opts)
	} else {
		err = sqlitex.ExecuteTransient(conn, querySQL, opts)
	}
	putExecOptions(opts)

	if err != nil {
		return nil, lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "UPDATE").
			Include("dema_expected", "successful execution")
	}
	return results, nil
}

type DeleteReturningBuilder[T any] struct {
	builder   DeleteBuilder[T]
	field0    any
	numFields uint8
	fields    []any
}

func (b DeleteReturningBuilder[T]) Where(cond Condition) DeleteReturningBuilder[T] {
	b.builder = b.builder.Where(cond)
	return b
}

func (b DeleteReturningBuilder[T]) Returning(fields ...any) DeleteReturningBuilder[T] {
	if len(fields) == 0 {
		b.field0 = nil
		b.fields = nil
		b.numFields = 0
		return b
	}
	if len(fields) == 1 {
		b.field0 = fields[0]
		b.fields = nil
		b.numFields = 1
		return b
	}
	b.fields = fields
	b.numFields = 2
	return b
}

func (b DeleteReturningBuilder[T]) SQL() (string, error) {
	table := b.builder.table
	if table == nil {
		typ, _ := getModelType[T]()
		var err error
		table, err = b.builder.db.getTableInfo(typ)
		if err != nil {
			return "", err
		}
	}
	opts := getExecOptions()
	opts.Args = opts.Args[:0]
	sql, _, err := b.buildSQL(table, opts)
	putExecOptions(opts)
	return sql, err
}

func (b DeleteReturningBuilder[T]) buildSQL(table *tableInfo, opts *sqlitex.ExecOptions) (string, []*colInfo, error) {
	if b.builder.cond == nil {
		return "", nil, lumo.WrapString("WHERE condition required for delete").
			Include("dema_table", table.name).
			Include("dema_operation", "DELETE").
			Include("dema_expected", "explicit WHERE condition")
	}

	colMap, retSQL, err := resolveReturningCols(table, b.numFields, b.field0, b.fields)
	if err != nil {
		return "", nil, err
	}

	qb := getQueryBuffer()
	qb.WriteString(`DELETE FROM "`)
	qb.WriteString(table.name)
	qb.WriteString(`" WHERE `)
	b.builder.cond.writeSQL(qb)
	opts.Args = b.builder.cond.appendArgs(opts.Args)
	qb.WriteString(` RETURNING `)
	qb.WriteString(retSQL)
	qb.WriteString(`;`)

	querySQL := table.getUpdateSQL(qb)
	putQueryBuffer(qb)
	return querySQL, colMap, nil
}

func (b DeleteReturningBuilder[T]) Run(ctx context.Context, cache bool) ([]T, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	table := b.builder.table
	typ, isPtrModel := getModelType[T]()
	if table == nil {
		var err error
		table, err = b.builder.db.getTableInfo(typ)
		if err != nil {
			return nil, err
		}
	}

	if table.onDelete != nil {
		if err := table.onDelete(nil); err != nil {
			return nil, lumo.WrapError(err).
				Include("dema_table", table.name).
				Include("dema_operation", "DELETE").
				Include("dema_expected", "onDelete hook validation")
		}
	}

	conn, err := b.builder.db.pool.Take(ctx)
	if err != nil {
		return nil, lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "DELETE").
			Include("dema_expected", "connection from pool")
	}
	defer b.builder.db.pool.Put(conn)

	return b.executeWithConn(conn, table, isPtrModel, cache)
}

func (b DeleteReturningBuilder[T]) executeWithConn(conn *sqlite.Conn, table *tableInfo, isPtrModel bool, cache bool) ([]T, error) {
	opts := getExecOptions()
	opts.Args = opts.Args[:0]

	var querySQL string
	var colMap []*colInfo

	if eq, ok := b.builder.cond.(fastEqualCondition); ok {
		if col := table.colByName[eq.equalCol()]; col != nil {
			if b.numFields == 0 || (b.numFields == 1 && b.field0 == nil) {
				querySQL = col.deleteReturningAllSQL
				colMap = nil
				opts.Args = append(opts.Args, eq.equalArg())
			} else if b.numFields == 1 {
				retName := extractReturningColName(b.field0)
				if retName == "" || retName == "*" {
					querySQL = col.deleteReturningAllSQL
					colMap = nil
					opts.Args = append(opts.Args, eq.equalArg())
				} else if retName == col.name {
					querySQL = col.deleteReturningSelfSQL
					colMap = col.singleColMap
					opts.Args = append(opts.Args, eq.equalArg())
				} else if retCol := table.colByName[retName]; retCol != nil {
					querySQL = table.getFastDeleteReturningSQL(col, retCol)
					colMap = retCol.singleColMap
					opts.Args = append(opts.Args, eq.equalArg())
				}
			}
		}
	}

	if querySQL == "" {
		var err error
		querySQL, colMap, err = b.buildSQL(table, opts)
		if err != nil {
			putExecOptions(opts)
			return nil, err
		}
	}

	var results []T
	if isPtrModel {
		opts.ResultFunc = func(stmt *sqlite.Stmt) error {
			elem := reflect.New(table.typ)
			p := elem.UnsafePointer()
			var err error
			if len(colMap) == 0 {
				err = table.scanRowDirectPtr(stmt, p)
			} else {
				err = table.scanRowMappedPtr(stmt, colMap, p)
			}
			if err != nil {
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
					newCap = 1
				}
				newRes := make([]T, len(results), newCap)
				copy(newRes, results)
				results = newRes
			}
			results = results[:idx+1]
			rowPtr := unsafe.Pointer(&results[idx])
			var err error
			if len(colMap) == 0 {
				err = table.scanRowDirectPtr(stmt, rowPtr)
			} else {
				err = table.scanRowMappedPtr(stmt, colMap, rowPtr)
			}
			if err != nil {
				results = results[:idx]
				return err
			}
			return nil
		}
	}

	var err error
	if cache {
		err = sqlitex.Execute(conn, querySQL, opts)
	} else {
		err = sqlitex.ExecuteTransient(conn, querySQL, opts)
	}
	putExecOptions(opts)

	if err != nil {
		return nil, lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "DELETE").
			Include("dema_expected", "successful execution")
	}
	return results, nil
}

type InsertBuilder[T any] struct {
	db        *DB
	table     *tableInfo
	single    T
	hasSingle bool
	rows      []T
}

func (db *DB) InsertInto[T any](rows ...T) InsertBuilder[T] {
	typ, _ := getModelType[T]()
	var table *tableInfo
	if typ != nil {
		table = db.tableInfo(typ)
	}
	if len(rows) == 1 {
		return InsertBuilder[T]{
			db:        db,
			table:     table,
			single:    rows[0],
			hasSingle: true,
		}
	}
	var r []T
	if len(rows) > 1 {
		r = make([]T, len(rows))
		copy(r, rows)
	}
	return InsertBuilder[T]{
		db:    db,
		table: table,
		rows:  r,
	}
}

func (b InsertBuilder[T]) Values(rows ...T) InsertBuilder[T] {
	if len(rows) == 0 {
		return b
	}
	if b.hasSingle {
		b.rows = []T{b.single}
		b.hasSingle = false
		var zero T
		b.single = zero
	}
	b.rows = append(b.rows, rows...)
	return b
}

func (b InsertBuilder[T]) Returning(fields ...any) InsertReturningBuilder[T] {
	if len(fields) == 0 {
		return InsertReturningBuilder[T]{
			builder:   b,
			numFields: 0,
		}
	}
	if len(fields) == 1 {
		return InsertReturningBuilder[T]{
			builder:   b,
			field0:    fields[0],
			numFields: 1,
		}
	}
	return InsertReturningBuilder[T]{
		builder:   b,
		fields:    fields,
		numFields: 2,
	}
}

func (b InsertBuilder[T]) Run(ctx context.Context, cache bool) error {
	if b.hasSingle {
		return b.db.Insert(ctx, b.single, cache)
	}
	if len(b.rows) == 0 {
		return nil
	}
	return b.db.Insert(ctx, b.rows, cache)
}

type InsertReturningBuilder[T any] struct {
	builder   InsertBuilder[T]
	field0    any
	numFields uint8
	fields    []any
}

func (b InsertReturningBuilder[T]) Values(rows ...T) InsertReturningBuilder[T] {
	b.builder = b.builder.Values(rows...)
	return b
}

func (b InsertReturningBuilder[T]) Returning(fields ...any) InsertReturningBuilder[T] {
	if len(fields) == 0 {
		b.field0 = nil
		b.fields = nil
		b.numFields = 0
		return b
	}
	if len(fields) == 1 {
		b.field0 = fields[0]
		b.fields = nil
		b.numFields = 1
		return b
	}
	b.fields = fields
	b.numFields = 2
	return b
}

func (b InsertReturningBuilder[T]) SQL() (string, error) {
	table := b.builder.table
	if table == nil {
		typ, _ := getModelType[T]()
		var err error
		table, err = b.builder.db.getTableInfo(typ)
		if err != nil {
			return "", err
		}
	}
	if !b.builder.hasSingle && len(b.builder.rows) == 0 {
		return "", lumo.WrapString("no rows provided for INSERT RETURNING").
			Include("dema_table", table.name).
			Include("dema_operation", "INSERT").
			Include("dema_expected", "at least one row")
	}
	var row T
	if b.builder.hasSingle {
		row = b.builder.single
	} else {
		row = b.builder.rows[0]
	}
	opts := getExecOptions()
	opts.Args = opts.Args[:0]
	sql, _, err := b.buildSingleRowSQL(table, row, opts)
	putExecOptions(opts)
	return sql, err
}

func (b InsertReturningBuilder[T]) buildSingleRowSQL(table *tableInfo, row T, opts *sqlitex.ExecOptions) (string, []*colInfo, error) {
	colMap, retSQL, err := resolveReturningCols(table, b.numFields, b.field0, b.fields)
	if err != nil {
		return "", nil, err
	}

	var p unsafe.Pointer
	if isPtrModel := table.typ != reflect.TypeOf(row); isPtrModel {
		p = *(*unsafe.Pointer)(unsafe.Pointer(&row))
	} else {
		p = unsafe.Pointer(&row)
	}

	mask := table.getNonZeroColMask(p)
	var activeCols []*colInfo
	for i, col := range table.allCols {
		if (mask & (uint64(1) << i)) != 0 {
			activeCols = append(activeCols, col)
		}
	}

	qb := getQueryBuffer()
	qb.WriteString(`INSERT INTO "`)
	qb.WriteString(table.name)
	if len(activeCols) == 0 {
		qb.WriteString(`" DEFAULT VALUES RETURNING `)
	} else {
		qb.WriteString(`" (`)
		for i, col := range activeCols {
			if i > 0 {
				qb.WriteString(", ")
			}
			qb.WriteString(`"`)
			qb.WriteString(col.name)
			qb.WriteString(`"`)
		}
		qb.WriteString(") VALUES (")
		for i, col := range activeCols {
			if i > 0 {
				qb.WriteString(", ")
			}
			qb.WriteString("?")
			val, _, err := col.extractValue(p)
			if err != nil {
				putQueryBuffer(qb)
				return "", nil, lumo.WrapError(err).
					Include("dema_table", table.name).
					Include("dema_operation", "INSERT").
					Include("dema_expected", "field value extraction")
			}
			opts.Args = append(opts.Args, val)
		}
		qb.WriteString(") RETURNING ")
	}
	qb.WriteString(retSQL)
	qb.WriteString(";")

	sql := table.getSelectSQL(qb.buf)
	putQueryBuffer(qb)
	return sql, colMap, nil
}

func (b InsertReturningBuilder[T]) Run(ctx context.Context, cache bool) ([]T, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !b.builder.hasSingle && len(b.builder.rows) == 0 {
		return nil, nil
	}

	table := b.builder.table
	typ, isPtrModel := getModelType[T]()
	if table == nil {
		var err error
		table, err = b.builder.db.getTableInfo(typ)
		if err != nil {
			return nil, err
		}
	}

	conn, err := b.builder.db.pool.Take(ctx)
	if err != nil {
		return nil, lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "INSERT").
			Include("dema_expected", "connection from pool")
	}
	defer b.builder.db.pool.Put(conn)

	return b.executeWithConn(conn, table, isPtrModel, cache)
}

func (b InsertReturningBuilder[T]) executeWithConn(conn *sqlite.Conn, table *tableInfo, isPtrModel bool, cache bool) ([]T, error) {
	n := 1
	if !b.builder.hasSingle {
		n = len(b.builder.rows)
	}
	results := make([]T, 0, n)

	opts := getExecOptions()
	opts.Args = opts.Args[:0]

	var colMap []*colInfo
	var useFastInsertPlan bool

	if b.numFields == 0 || (b.numFields == 1 && b.field0 == nil) {
		useFastInsertPlan = true
		colMap = nil
	} else if b.numFields == 1 {
		retName := extractReturningColName(b.field0)
		if retName == "" || retName == "*" {
			useFastInsertPlan = true
			colMap = nil
		} else {
			col := table.colByName[retName]
			if col == nil {
				putExecOptions(opts)
				return nil, lumo.WrapString("unknown column in RETURNING: %s", retName).
					Include("dema_table", table.name).
					Include("dema_operation", "RETURNING").
					Include("dema_expected", "valid table column")
			}
			colMap = col.singleColMap
		}
	} else {
		var err error
		colMap, _, err = parseReturningCols(table, b.fields)
		if err != nil {
			putExecOptions(opts)
			return nil, err
		}
	}

	if isPtrModel {
		opts.ResultFunc = func(stmt *sqlite.Stmt) error {
			elem := reflect.New(table.typ)
			p := elem.UnsafePointer()
			var err error
			if len(colMap) == 0 {
				err = table.scanRowDirectPtr(stmt, p)
			} else {
				err = table.scanRowMappedPtr(stmt, colMap, p)
			}
			if err != nil {
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
					newCap = 1
				}
				newRes := make([]T, len(results), newCap)
				copy(newRes, results)
				results = newRes
			}
			results = results[:idx+1]
			rowPtr := unsafe.Pointer(&results[idx])
			var err error
			if len(colMap) == 0 {
				err = table.scanRowDirectPtr(stmt, rowPtr)
			} else {
				err = table.scanRowMappedPtr(stmt, colMap, rowPtr)
			}
			if err != nil {
				results = results[:idx]
				return err
			}
			return nil
		}
	}

	if b.builder.hasSingle {
		if err := executeInsertReturningRow(conn, table, isPtrModel, cache, useFastInsertPlan, b, opts, b.builder.single); err != nil {
			putExecOptions(opts)
			return nil, err
		}
	} else {
		for _, row := range b.builder.rows {
			if err := executeInsertReturningRow(conn, table, isPtrModel, cache, useFastInsertPlan, b, opts, row); err != nil {
				putExecOptions(opts)
				return nil, err
			}
		}
	}

	putExecOptions(opts)
	return results, nil
}

func executeInsertReturningRow[T any](conn *sqlite.Conn, table *tableInfo, isPtrModel bool, cache bool, useFastInsertPlan bool, b InsertReturningBuilder[T], opts *sqlitex.ExecOptions, row T) error {
	if table.onInsert != nil {
		rowPtr := reflect.New(table.typ)
		var p unsafe.Pointer
		if isPtrModel {
			p = *(*unsafe.Pointer)(unsafe.Pointer(&row))
		} else {
			p = unsafe.Pointer(&row)
		}
		copy(unsafe.Slice((*byte)(rowPtr.UnsafePointer()), table.typ.Size()), unsafe.Slice((*byte)(p), table.typ.Size()))
		if err := table.onInsert(rowPtr.Interface()); err != nil {
			return lumo.WrapError(err).
				Include("dema_table", table.name).
				Include("dema_operation", "INSERT").
				Include("dema_expected", "onInsert hook validation")
		}
		if isPtrModel {
			row = rowPtr.Interface().(T)
		} else {
			row = rowPtr.Elem().Interface().(T)
		}
	}

	opts.Args = opts.Args[:0]
	var sql string

	var p unsafe.Pointer
	if isPtrModel {
		p = *(*unsafe.Pointer)(unsafe.Pointer(&row))
	} else {
		p = unsafe.Pointer(&row)
	}

	if useFastInsertPlan {
		mask := table.getNonZeroColMask(p)
		var cols []*colInfo
		sql, cols = table.getInsertReturningPlan(mask)
		for _, col := range cols {
			val, _, err := col.extractValue(p)
			if err != nil {
				return lumo.WrapError(err).
					Include("dema_table", table.name).
					Include("dema_operation", "INSERT").
					Include("dema_expected", "field value extraction")
			}
			opts.Args = append(opts.Args, val)
		}
	} else {
		var err error
		sql, _, err = b.buildSingleRowSQL(table, row, opts)
		if err != nil {
			return err
		}
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
			Include("dema_operation", "INSERT").
			Include("dema_expected", "row insertion")
	}
	return nil
}

func Insert[T any](args ...any) InsertBuilder[T] {
	var db *DB
	var rows []T
	var single T
	var hasSingle bool
	firstRow := true
	for _, arg := range args {
		if arg == nil {
			continue
		}
		switch a := arg.(type) {
		case *DB:
			db = a
		case T:
			if firstRow {
				single = a
				hasSingle = true
				firstRow = false
			} else {
				if hasSingle {
					rows = []T{single}
					hasSingle = false
					var zero T
					single = zero
				}
				rows = append(rows, a)
			}
		case []T:
			if hasSingle {
				rows = []T{single}
				hasSingle = false
				var zero T
				single = zero
			}
			rows = append(rows, a...)
		}
	}

	var table *tableInfo
	if db != nil {
		typ, _ := getModelType[T]()
		if typ != nil {
			table = db.tableInfo(typ)
		}
	}

	return InsertBuilder[T]{
		db:        db,
		table:     table,
		single:    single,
		hasSingle: hasSingle,
		rows:      rows,
	}
}
