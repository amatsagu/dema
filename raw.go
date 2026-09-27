package dema

import (
	"context"
	"reflect"
	"strings"

	"github.com/amatsagu/lumo"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// RawQuery executes a raw SQL query and scans results into a slice of model struct T.
func (db *DB) RawQuery[T any](ctx context.Context, sql string, cache bool, args ...any) ([]T, error) {
	var zero T
	typ := reflect.TypeOf(zero)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	table, err := db.getTableInfo(typ)
	if err != nil {
		return nil, err
	}

	conn, err := db.pool.Take(ctx)
	if err != nil {
		return nil, lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "RAW_QUERY").
			Include("dema_expected", "connection from pool")
	}
	defer db.pool.Put(conn)

	return executeRawQuery[T](conn, table, sql, cache, args...)
}

func executeRawQuery[T any](conn *sqlite.Conn, table *tableInfo, sql string, cache bool, args ...any) ([]T, error) {
	encodedArgs := make([]any, len(args))
	for i, arg := range args {
		encodedArgs[i] = encodeValue(arg)
	}

	results := make([]T, 0, 16)
	var colMap []*colInfo
	var mapInited bool

	opts := &sqlitex.ExecOptions{
		Args: encodedArgs,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if !mapInited {
				colCount := stmt.ColumnCount()
				colMap = make([]*colInfo, colCount)
				for i := 0; i < colCount; i++ {
					colName := stmt.ColumnName(i)
					if col, ok := table.colByName[colName]; ok {
						colMap[i] = col
					} else if col, ok := table.colByName[strings.ToLower(colName)]; ok {
						colMap[i] = col
					}
				}
				mapInited = true
			}

			var row T
			val := reflect.ValueOf(&row).Elem()
			scanTarget := val
			if scanTarget.Kind() == reflect.Pointer {
				if scanTarget.IsNil() {
					scanTarget.Set(reflect.New(scanTarget.Type().Elem()))
				}
				scanTarget = scanTarget.Elem()
			}
			if err := table.scanRowMapped(stmt, colMap, scanTarget); err != nil {
				return err
			}
			results = append(results, row)
			return nil
		},
	}

	var err error
	if cache {
		err = sqlitex.Execute(conn, sql, opts)
	} else {
		err = sqlitex.ExecuteTransient(conn, sql, opts)
	}

	if err != nil {
		return nil, lumo.WrapError(err).
			Include("dema_table", table.name).
			Include("dema_operation", "RAW_QUERY").
			Include("dema_expected", "successful execution")
	}
	return results, nil
}

// RawExecute executes a raw SQL statement without result scanning.
func (db *DB) RawExecute(ctx context.Context, sql string, cache bool, args ...any) error {
	conn, err := db.pool.Take(ctx)
	if err != nil {
		return lumo.WrapError(err).
			Include("dema_table", "").
			Include("dema_operation", "RAW_EXECUTE").
			Include("dema_expected", "connection from pool")
	}
	defer db.pool.Put(conn)

	return executeRawExecute(conn, sql, cache, args...)
}

func executeRawExecute(conn *sqlite.Conn, sql string, cache bool, args ...any) error {
	encodedArgs := make([]any, len(args))
	for i, arg := range args {
		encodedArgs[i] = encodeValue(arg)
	}

	opts := &sqlitex.ExecOptions{Args: encodedArgs}
	var err error
	if cache {
		err = sqlitex.Execute(conn, sql, opts)
	} else {
		err = sqlitex.ExecuteTransient(conn, sql, opts)
	}

	if err != nil {
		return lumo.WrapError(err).
			Include("dema_table", "").
			Include("dema_operation", "RAW_EXECUTE").
			Include("dema_expected", "successful execution")
	}
	return nil
}
