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
	pool   *sqlitex.Pool
	mu     sync.RWMutex
	tables map[reflect.Type]*tableInfo
	closed bool
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

	return &DB{
		pool:   pool,
		tables: make(map[reflect.Type]*tableInfo),
	}, nil
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
}

func (db *DB) getTableInfo(typ reflect.Type) (*tableInfo, error) {
	db.mu.RLock()
	info, ok := db.tables[typ]
	db.mu.RUnlock()

	if !ok {
		return nil, lumo.WrapString("table for type %s is not registered", typ.String()).
			Include("dema_table", "").
			Include("dema_operation", "LOOKUP").
			Include("dema_expected", "registered table")
	}
	return info, nil
}
