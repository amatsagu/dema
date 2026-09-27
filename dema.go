package dema

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/amatsagu/lumo"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

var memCounter uint64

type DB struct {
	tablesValue atomic.Pointer[map[reflect.Type]*tableInfo]
	pool        *sqlitex.Pool
	tables      map[reflect.Type]*tableInfo
	mu          sync.RWMutex
	closed      bool
}

func Open(path string, opts ...any) (*DB, error) {
	poolSize := 10
	var openFlags sqlite.OpenFlags

	for _, opt := range opts {
		switch o := opt.(type) {
		case int:
			if o > 0 {
				poolSize = o
			}
		case sqlite.OpenFlags:
			openFlags |= o
		}
	}

	if openFlags == 0 {
		openFlags = sqlite.OpenReadWrite | sqlite.OpenCreate | sqlite.OpenWAL | sqlite.OpenURI
	}

	if path == ":memory:" {
		id := atomic.AddUint64(&memCounter, 1)
		path = fmt.Sprintf("file:memdb_%d?mode=memory&cache=shared", id)
		openFlags |= sqlite.OpenURI
	} else if strings.HasPrefix(path, "file:") {
		openFlags |= sqlite.OpenURI
	}

	pool, err := sqlitex.NewPool(path, sqlitex.PoolOptions{
		Flags:    openFlags,
		PoolSize: poolSize,
	})
	if err != nil {
		return nil, lumo.WrapError(err).
			Include("dema_table", "").
			Include("dema_operation", "OPEN").
			Include("dema_expected", "valid sqlite pool")
	}

	tablesMap := make(map[reflect.Type]*tableInfo)
	db := &DB{
		pool:   pool,
		tables: tablesMap,
	}
	db.tablesValue.Store(&tablesMap)
	return db, nil
}

func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return nil
	}
	db.closed = true

	if err := db.pool.Close(); err != nil {
		return lumo.WrapError(err).
			Include("dema_table", "").
			Include("dema_operation", "CLOSE").
			Include("dema_expected", "successful pool closure")
	}
	return nil
}

func (db *DB) Table[T any](name string, onInsert, onUpdate, onDelete func(*T) error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	var zero T
	typ := reflect.TypeOf(zero)
	if typ == nil {
		panic("dema: model type cannot be nil")
	}
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		panic(fmt.Sprintf("dema: model type %s must be a struct", typ.String()))
	}

	if _, exists := db.tables[typ]; exists {
		panic(fmt.Sprintf("dema: table for type %s is already registered", typ.String()))
	}

	info, err := newTableInfo(name, onInsert, onUpdate, onDelete)
	if err != nil {
		panic(err)
	}

	db.tables[typ] = info
	newMap := make(map[reflect.Type]*tableInfo, len(db.tables))
	for k, v := range db.tables {
		newMap[k] = v
	}
	db.tablesValue.Store(&newMap)
}

func (db *DB) getTableInfo(typ reflect.Type) (*tableInfo, error) {
	if p := db.tablesValue.Load(); p != nil {
		if info, ok := (*p)[typ]; ok {
			return info, nil
		}
	}

	return nil, lumo.WrapString("table for type %s is not registered", typ.String()).
		Include("dema_table", "").
		Include("dema_operation", "LOOKUP").
		Include("dema_expected", "registered table")
}
