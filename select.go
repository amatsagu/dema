package dema

import (
	"context"
	"reflect"
	"strconv"
	"strings"

	"github.com/amatsagu/lumo"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type orderClause struct {
	col string
	dir OrderDirection
}

type SelectBuilder[T any] struct {
	db     *DB
	conn   *sqlite.Conn
	cond   Condition
	orders []orderClause
	limit  int
	offset int
	hasLim bool
	hasOff bool
}

func (db *DB) Select[T any]() *SelectBuilder[T] {
	return &SelectBuilder[T]{
		db: db,
	}
}

func (b *SelectBuilder[T]) Where(cond Condition) *SelectBuilder[T] {
	b.cond = cond
	return b
}

func (b *SelectBuilder[T]) OrderBy[V any](field Field[T, V], dir OrderDirection) *SelectBuilder[T] {
	b.orders = append(b.orders, orderClause{
		col: field.Name,
		dir: dir,
	})
	return b
}

func (b *SelectBuilder[T]) Limit(amount int) *SelectBuilder[T] {
	if amount < 0 {
		amount = 0
	}
	b.limit = amount
	b.hasLim = true
	return b
}

// Page applies 1-indexed pagination (current page, page size).
func (b *SelectBuilder[T]) Page(current, size int) *SelectBuilder[T] {
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

func (b *SelectBuilder[T]) buildSQL(table *tableInfo) (string, []any) {
	if b.cond == nil && len(b.orders) == 0 && !b.hasLim && !b.hasOff {
		return table.defaultSelectSQL, nil
	}

	var sb strings.Builder
	sb.Grow(len(table.defaultSelectSQL) + 64)
	sb.WriteString(table.defaultSelectSQL)

	var args []any
	if b.cond != nil {
		sb.WriteString(" WHERE ")
		b.cond.toSQL(&sb, &args)
	}

	if len(b.orders) > 0 {
		sb.WriteString(" ORDER BY ")
		for i, ord := range b.orders {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(`"`)
			sb.WriteString(ord.col)
			sb.WriteString(`" `)
			sb.WriteString(string(ord.dir))
		}
	}

	if b.hasLim {
		sb.WriteString(" LIMIT ")
		sb.WriteString(strconv.Itoa(b.limit))
	}
	if b.hasOff {
		sb.WriteString(" OFFSET ")
		sb.WriteString(strconv.Itoa(b.offset))
	}

	return sb.String(), args
}

func (b *SelectBuilder[T]) Run(args ...any) ([]T, error) {
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

	var zero T
	typ := reflect.TypeOf(zero)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	table, err := b.db.getTableInfo(typ)
	if err != nil {
		return nil, err
	}

	querySQL, args := b.buildSQL(table)

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

	opts := &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			var row T
			val := reflect.ValueOf(&row).Elem()
			scanTarget := val
			if scanTarget.Kind() == reflect.Pointer {
				if scanTarget.IsNil() {
					scanTarget.Set(reflect.New(scanTarget.Type().Elem()))
				}
				scanTarget = scanTarget.Elem()
			}
			if err := table.scanRowDirect(stmt, scanTarget); err != nil {
				return err
			}
			results = append(results, row)
			return nil
		},
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
