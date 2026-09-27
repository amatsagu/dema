package dema

import (
	"context"
	"reflect"
	"strings"

	"github.com/amatsagu/lumo"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Insert inserts one or more model rows into the database.
func (db *DB) Insert(args ...any) error {
	ctx, rowVals, err := parseContextAndRows(args)
	if err != nil || len(rowVals) == 0 {
		return err
	}

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

	return executeInsertRowVals(conn, table, rowVals)
}

func parseContextAndRows(args []any) (context.Context, []reflect.Value, error) {
	ctx := context.Background()
	if len(args) > 0 {
		if c, ok := args[0].(context.Context); ok && c != nil {
			ctx = c
			args = args[1:]
		}
	}

	if len(args) == 0 {
		return ctx, nil, nil
	}

	var rowVals []reflect.Value
	for _, arg := range args {
		if arg == nil {
			continue
		}
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

	return ctx, rowVals, nil
}

func executeInsertRowVals(conn *sqlite.Conn, table *tableInfo, rows []reflect.Value) error {
	for i := range rows {
		rowVal := rows[i]
		if !rowVal.CanAddr() {
			addr := reflect.New(rowVal.Type()).Elem()
			addr.Set(rowVal)
			rowVal = addr
		}

		var colNames []string
		var args []any

		for _, col := range table.allCols {
			val, include, err := table.extractColValue(col, rowVal)
			if err != nil {
				return lumo.WrapError(err).
					Include("dema_table", table.name).
					Include("dema_operation", "INSERT").
					Include("dema_expected", "field value extraction")
			}
			if !include {
				continue
			}
			colNames = append(colNames, col.name)
			args = append(args, encodeValue(val))
		}

		if len(colNames) == 0 {
			sql := `INSERT INTO "` + table.name + `" DEFAULT VALUES;`
			opts := &sqlitex.ExecOptions{}
			if err := sqlitex.Execute(conn, sql, opts); err != nil {
				return lumo.WrapError(err).
					Include("dema_table", table.name).
					Include("dema_operation", "INSERT").
					Include("dema_expected", "default values execution")
			}
			continue
		}

		var sb strings.Builder
		sb.WriteString(`INSERT INTO "`)
		sb.WriteString(table.name)
		sb.WriteString(`" (`)
		for j, name := range colNames {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(`"`)
			sb.WriteString(name)
			sb.WriteString(`"`)
		}
		sb.WriteString(`) VALUES (`)
		for j := range colNames {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(`?`)
		}
		sb.WriteString(`);`)

		opts := &sqlitex.ExecOptions{Args: args}
		if err := sqlitex.Execute(conn, sb.String(), opts); err != nil {
			return lumo.WrapError(err).
				Include("dema_table", table.name).
				Include("dema_operation", "INSERT").
				Include("dema_expected", "row insertion")
		}
	}
	return nil
}

// Upsert inserts rows or updates non-primary-key fields on conflict with primary keys.
func (db *DB) Upsert(args ...any) error {
	ctx, rowVals, err := parseContextAndRows(args)
	if err != nil || len(rowVals) == 0 {
		return err
	}

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

	return executeUpsertRowVals(conn, table, rowVals)
}

func executeUpsertRowVals(conn *sqlite.Conn, table *tableInfo, rows []reflect.Value) error {
	for i := range rows {
		rowVal := rows[i]
		if !rowVal.CanAddr() {
			addr := reflect.New(rowVal.Type()).Elem()
			addr.Set(rowVal)
			rowVal = addr
		}

		var colNames []string
		var args []any

		for _, col := range table.allCols {
			val, include, err := table.extractColValue(col, rowVal)
			if err != nil {
				return lumo.WrapError(err).
					Include("dema_table", table.name).
					Include("dema_operation", "UPSERT").
					Include("dema_expected", "field value extraction")
			}
			if !include {
				continue
			}
			colNames = append(colNames, col.name)
			args = append(args, encodeValue(val))
		}

		if len(colNames) == 0 {
			continue
		}

		var sb strings.Builder
		sb.WriteString(`INSERT INTO "`)
		sb.WriteString(table.name)
		sb.WriteString(`" (`)
		for j, name := range colNames {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(`"`)
			sb.WriteString(name)
			sb.WriteString(`"`)
		}
		sb.WriteString(`) VALUES (`)
		for j := range colNames {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(`?`)
		}
		sb.WriteString(`) ON CONFLICT (`)
		for j, pk := range table.pkCols {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(`"`)
			sb.WriteString(pk.name)
			sb.WriteString(`"`)
		}
		sb.WriteString(`) `)

		if len(table.nonPkCols) == 0 {
			sb.WriteString(`DO NOTHING;`)
		} else {
			sb.WriteString(`DO UPDATE SET `)
			for j, nonPk := range table.nonPkCols {
				if j > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(`"`)
				sb.WriteString(nonPk.name)
				sb.WriteString(`" = excluded."`)
				sb.WriteString(nonPk.name)
				sb.WriteString(`"`)
			}
			sb.WriteString(`;`)
		}

		opts := &sqlitex.ExecOptions{Args: args}
		if err := sqlitex.Execute(conn, sb.String(), opts); err != nil {
			return lumo.WrapError(err).
				Include("dema_table", table.name).
				Include("dema_operation", "UPSERT").
				Include("dema_expected", "row upsert")
		}
	}
	return nil
}
