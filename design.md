# Dema — Design Document

> A thin, high-performance SQLite overlay library for Go that brings native structs closer to basic CRUD operations.

---

## Core Philosophy
- **Minimal abstraction** — as close to raw SQL performance as possible
- **Low memory / low GC pressure** — pre-sized buffers, value types over pointers, zero reflection at query time
- **Type-safe** — generic `Field[T, V]` and sealed `Condition` interface for compile-time correctness
- **Tightly coupled to [zombiezen SQLite driver](https://pkg.go.dev/zombiezen.com/go/sqlite)** — no driver abstraction, direct API usage for maximum speed

---

## Package Structure

```
github.com/amatsagu/dema/
├── *.go                    # Flat package: DB, Table, Field, Condition, comparators, builders
├── go.mod
├── seed.sql                # Test/benchmark seed data (3 tables, ~500 records)
├── *_test.go               # Tests + benchmarks (raw SQL vs dema, 100% coverage target)
└── cmd/demagen/            # go generate tool (phase 2)
    └── main.go
```

---

## Connection Management

### Opening

```go
db, err := dema.Open("sqlite.db", 4)                              // path, pool size
db, err := dema.Open("sqlite.db", 4, sqlite.OpenWAL)              // with flags
db, err := dema.Open(":memory:", 1)                                // in-memory for tests
```

- **Always pool-based** — uses `sqlitex.Pool` internally
- Pool size is configurable (2nd parameter)
- Variadic `sqlite.OpenFlags` for WAL mode, sync mode, etc.
- Returns `(*DB, error)`

### Closing

```go
err := db.Close()
```

- Closes all pool connections
- All operations respect `context.Context` cancellation — on cancel/timeout, connections are properly released back to the pool

---

## Table Registration

```go
db.Table[User]("users", onInsert, onUpdate, onDelete)
```

- Registers struct type `T` with a table name
- Three optional hook functions (pass `nil` to skip):
  - `onInsert func(*T) error` — called **before** INSERT, can validate/modify
  - `onUpdate func(*T) error` — called **before** UPDATE, can validate/modify
  - `onDelete func(*T) error` — called **before** DELETE, can validate/modify
- Hooks return `error` to abort the operation
- **Reflection happens once here** — struct fields are inspected, column names resolved, field indices cached into a `tableInfo` struct
- Panics if called with a type that's already registered (programming error)

---

## Struct Mapping

### Struct Tags

```go
type User struct {
    ID   uint32 `db:"id,pk"`                // column "id", primary key
    Name string `db:"surname,omitzero"`     // column "surname", skip on zero value
}
```

**Tag format:** `db:"column_name,option1,option2"`

| Option | Behavior |
|--------|----------|
| *(name)* | Column name override (default: lowercase field name) |
| `pk` | Marks field as part of the primary key (supports composite PKs) |
| `omitzero` | Skip field in INSERT/UPDATE when value is zero value for its type |

### Supported Types

| Go Type | SQLite Type |
|---------|-------------|
| `bool` | INTEGER (0/1) |
| `int8`, `int16`, `int32`, `int64`, `int` | INTEGER |
| `uint8`, `uint16`, `uint32`, `uint64`, `uint` | INTEGER |
| `string`, `[]rune` | TEXT |
| `*T` (pointer to any above) | Same as T, nil → omitted (avoid NULL) |

### Custom Types

Types that don't directly map must implement:

```go
type FieldEncoder interface {
    EncodeDema() (any, error)
}

type FieldDecoder interface {
    DecodeDema(any) error
}
```

Example:
```go
type Snowflake uint64

func (s Snowflake) EncodeDema() (any, error) { return int64(s), nil }
func (s *Snowflake) DecodeDema(v any) error  { *s = Snowflake(v.(int64)); return nil }
```

### Field Access at Scan Time

Uses `reflect.Value.FieldByIndex()` — safe, supports embedded structs, no `unsafe`.

---

## Typed Fields

```go
var UserField = struct {
    ID   dema.Field[User, uint32]
    Name dema.Field[User, string]
}{
    ID:   dema.NewField[User, uint32]("id"),
    Name: dema.NewField[User, string]("surname"),
}
```

- `Field[T, V]` is an opaque struct holding column name + field index
- Created via `dema.NewField[T, V](columnName string)`
- Provides compile-time type safety for comparators: `dema.Equal(UserField.ID, 127)` enforces `V = uint32`
- Can be manually written or auto-generated via `go generate` (phase 2)

---

## Query API

### Select (Builder)

```go
data, err := db.Select[User]().
    Where(dema.Equal(UserField.ID, 127)).
    OrderBy(UserField.Name, dema.Asc).
    Limit(1).
    Run(ctx, true)     // returns []User, error
```

**Chain methods:**
| Method | Description |
|--------|-------------|
| `Where(Condition)` | Filter rows |
| `OrderBy(Field, Direction)` | Sort results (`dema.Asc` / `dema.Desc`) |
| `Limit(n)` | Max rows to return |
| `Page(current, size)` | Pagination helper — `Page(2, 10)` → rows 11–20 |
| `Run(ctx, cache) ([]T, error)` | Execute and return results |

**Returns `[]T`** — plain value slice, cache-friendly, pre-sized when LIMIT is known.

### Insert (Variadic)

```go
err := db.Insert(ctx, u1, u2, u3)    // variadic, infers T from args
```

- Respects `omitzero` — skips zero-value fields so SQLite uses DEFAULT/auto-increment
- Fires `onInsert` hook for each struct before SQL execution

### Update (Dual API)

**Full-row update by primary key:**
```go
err := db.Update(ctx, user)    // updates all non-PK fields WHERE pk = value
```

**Partial update via builder:**
```go
err := db.Update[User]().
    Set(UserField.Name, "newname").
    Where(dema.Equal(UserField.ID, 127)).
    Run(ctx, true)
```

- Fires `onUpdate` hook before execution

### Delete (Builder only)

```go
err := db.Delete[User]().
    Where(dema.Equal(UserField.ID, 127)).
    Run(ctx, true)
```

- **Always requires explicit WHERE** — no accidental "delete all"
- Fires `onDelete` hook before execution

### Upsert (Variadic)

```go
err := db.Upsert(ctx, user)
```

- `INSERT ... ON CONFLICT(pk_fields) DO UPDATE SET non_pk_fields = excluded.non_pk_fields`
- Conflict target = primary key field(s)
- On conflict = update all non-PK fields

### Raw Query / Execute

```go
users, err := db.RawQuery[User](ctx, "SELECT * FROM users WHERE id > ?", true, 100)
err := db.RawExecute(ctx, "CREATE INDEX idx_name ON users(surname)", true)
```

- `RawQuery[T]` scans results into structs using cached column mapping
- `RawExecute` runs a statement with no result scanning

---

## Conditions (WHERE)

Sealed `Condition` interface with unexported marker method — only dema's built-in comparators are valid.

| Comparator | SQL | Example |
|------------|-----|---------|
| `Equal(field, value)` | `col = ?` | `dema.Equal(UserField.ID, 127)` |
| `Greater(field, value)` | `col > ?` | `dema.Greater(UserField.ID, 10)` |
| `Lesser(field, value)` | `col < ?` | `dema.Lesser(UserField.ID, 100)` |
| `Within(field, ...values)` | `col IN (?, ?, ?)` | `dema.Within(UserField.ID, 10, 20, 30)` |
| `NotWithin(field, ...values)` | `col NOT IN (?, ?)` | `dema.NotWithin(UserField.ID, 5, 6)` |
| `Range(field, min, max)` | `col BETWEEN ? AND ?` | `dema.Range(UserField.ID, 1, 100)` |
| `OutsideRange(field, min, max)` | `col NOT BETWEEN ? AND ?` | `dema.OutsideRange(UserField.ID, 1, 100)` |
| `Or(a, b)` | `(a) OR (b)` | `dema.Or(condA, condB)` |
| `And(a, b)` | `(a) AND (b)` | `dema.And(condA, condB)` |

> **Intentionally limited** — max 2-level nesting via Or/And. For complex queries (JOINs, subqueries), use `RawQuery`.

---

## Transactions

```go
tx, err := db.Transaction(ctx)
if err != nil { ... }

users, err := tx.Select[User]().
    Where(dema.Equal(UserField.ID, 127)).
    Run(true)    // no ctx — bound to the transaction's context

err = tx.Insert(u1, u2)    // also no ctx on TX methods

err = tx.Commit()
// or: err = tx.Rollback()
```

- `Transaction(ctx)` borrows a connection from the pool and holds it
- All TX methods mirror DB methods but **without `ctx`** — context is bound at transaction creation
- On `Commit()` / `Rollback()`, the connection is returned to the pool
- On context cancellation, the transaction is rolled back and connection released

---

## Statement Caching

The `cache bool` parameter in `Run()` maps directly to zombiezen's API:
- `cache=true` → `sqlitex.Execute` (prepares and caches the statement for reuse)
- `cache=false` → `sqlitex.ExecuteTransient` (one-off, not cached)

---

## Error Handling

All errors use the [lumo](https://github.com/amatsagu/lumo) library:

```go
return lumo.WrapError(err).
    Include("table", "users").
    Include("operation", "INSERT").
    Include("expected", "3 rows affected")
```

- `lumo.WrapError(err)` — wraps with stack trace
- `lumo.WrapString(format, args...)` — creates new error with stack trace
- `.Include(label, value)` — adds contextual metadata

---

## SQL Generation (Internal)

- Uses `strings.Builder` with pre-sized buffers for minimal allocations
- Parallel `[]any` args accumulator for bind parameters
- Placeholder: `?` (SQLite standard)
- Designed for low GC pressure and high throughput

---

## Testing & Benchmarking Strategy

- **In-memory SQLite** (`":memory:"`) for all tests and benchmarks
- **3 test tables** with ~500 total randomized records, seeded from `seed.sql`
- **100% test coverage** target
- **Benchmarks compare raw SQL (direct zombiezen calls) vs dema** for every operation
  - Measure: `ns/op`, `B/op`, `allocs/op`
  - Goal: dema overhead as close to zero as possible

---

## Implementation Order

### Phase 1: Core Library
1. `DB` + `Open()` / `Close()` (pool management)
2. `Table[T]()` registration + struct reflection + `tableInfo` caching
3. `Field[T, V]` + `NewField`
4. `Condition` interface + all comparators
5. SQL builder internals (`strings.Builder` approach)
6. `Select` query builder + `Run()` + row scanning
7. `Insert` (variadic)
8. `Update` (both full-row and builder)
9. `Delete` (builder)
10. `Upsert` (variadic)
11. `RawQuery[T]` + `RawExecute`
12. `Transaction` with pool-aware connection management
13. Tests + benchmarks for everything

### Phase 2: Code Generation
14. `cmd/demagen` — `go generate` tool that reads struct definitions and outputs `{Model}Field` vars + hook function signatures
