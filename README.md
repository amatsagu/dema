# dema

Type-safe SQLite ORM-like query builder for Go, built on [zombiezen.com/go/sqlite](https://pkg.go.dev/zombiezen.com/go/sqlite).

> [!Important]
> Pieces of this library were created with help of artificial intelligence, so the code may appear weird at times. This library is created mainly for my own silly projects but it's left public under MIT license for anyone.

```
go get github.com/amatsagu/dema
```

## Quick Start

```go
type User struct {
    ID   int    `db:"id,pk"`
    Name string `db:"name"`
    Age  int    `db:"age"`
}

var UserID   = dema.NewField[User, int]("id")
var UserName = dema.NewField[User, string]("name")
var UserAge  = dema.NewField[User, int]("age")

func main() {
    db, _ := dema.Open("app.db") // pool=10, WAL mode (by default)
    defer db.Close()
    db.Table[User]("users", nil, nil, nil) // register table (hooks are optional)
    ctx := context.Background()

    db.Insert(ctx, User{ID: 1, Name: "Alice", Age: 30})

    users, _ := db.Select[User]().Run(ctx, true) // cache=true

    found, _ := db.Select[User]().
        Where(dema.Equal(UserID, 1)).Limit(1).Run(ctx, true)

    db.Update[User]().Set(UserName, "Bob").
        Where(dema.Equal(UserID, 1)).Run(ctx, true)

    db.Delete[User]().
        Where(dema.Equal(UserID, 1)).Run(ctx, true)
}
```

## Connection

```go
db, _ := dema.Open(":memory:")                                       // in-memory
db, _ := dema.Open("app.db")                                        // file, defaults
db, _ := dema.Open("app.db", 20, sqlite.OpenReadWrite|sqlite.OpenWAL) // custom
// With connection hook (PRAGMAs, custom functions):
db, _ := dema.Open("app.db", func(conn *sqlite.Conn) error {
    return sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = ON;", nil)
})
db.Close()
```

## Model Definition

```go
type Product struct {
    ID    int     `db:"id,pk"`          // primary key
    Name  string  `db:"name"`
    Price float64 `db:"price,omitzero"` // skip on INSERT/UPDATE when zero
    SKU   string  `db:"sku,pk"`         // composite PK
}
```

**Tags:** `db:"col_name"` · `,pk` - primary key · `,omitzero` - omit when zero-valued

**Types:** `int`, `int8`–`int64`, `uint`, `uint8`–`uint64`, `float32`, `float64`, `bool`, `string`, `[]rune`, `*T` (nullable)

**Hooks:** `db.Table[T](name, onInsert, onUpdate, onDelete)` - each is `func(*T) error` or `nil`

**Custom types:** implement `FieldEncoder` (`EncodeDema() (any, error)`) and/or `FieldDecoder` (`DecodeDema(any) error`)

## Conditions

| Function | SQL |
|---|---|
| `Equal(field, val)` | `col = ?` |
| `Greater(field, val)` | `col > ?` |
| `Lesser(field, val)` | `col < ?` |
| `Within(field, vals...)` | `col IN (?, ...)` |
| `NotWithin(field, vals...)` | `col NOT IN (?, ...)` |
| `Range(field, min, max)` | `col BETWEEN ? AND ?` |
| `OutsideRange(field, min, max)` | `col NOT BETWEEN ? AND ?` |
| `And(a, b)` | `(a) AND (b)` |
| `Or(a, b)` | `(a) OR (b)` |

## Select

```go
rows, _ := db.Select[User]().Run(ctx, true)                         // all rows

rows, _ := db.Select[User]().                                       // filtered + sorted + paginated
    Where(dema.Greater(UserAge, 18)).
    OrderBy(UserName, dema.Asc).
    Page(2, 25).                                                     // page 2, 25/page (1-indexed)
    Run(ctx, false)

row, _ := db.Select[User]().                                        // single lookup
    Where(dema.Equal(UserID, 42)).Limit(1).Run(ctx, true)
```

`cache` (second arg to `Run`) toggles SQLite prepared statement caching - `true` for repeated queries, `false` for one-off.

## Count

```go
n, _ := db.Count[User]().Run(ctx, true)                              // SELECT COUNT(*) FROM "users";
n, _ := db.Count[User](nil).Run(ctx, true)                           // same: COUNT(*)
n, _ := db.Count[User](UserName).Limit(20).Run(ctx, true)           // SELECT COUNT("name") FROM "users" LIMIT 20;
n, _ := db.Count[User]().Where(dema.Greater(UserAge, 18)).Run(ctx, true)

// In transactions (no ctx parameter)
n, _ := tx.Count[User](UserName).Run(true)
```

## Insert & Upsert

```go
db.Insert(ctx, User{ID: 1, Name: "Alice", Age: 25})                 // single
db.Insert(ctx, user1, user2, user3)                                  // multiple
db.Insert(ctx, []User{{ID: 4, Name: "D"}, {ID: 5, Name: "E"}})      // slice
db.Upsert(ctx, User{ID: 1, Name: "Updated", Age: 26})               // insert or update on PK conflict
```

## Update

```go
db.Update[User]().Set(UserName, "New").Set(UserAge, 31).             // builder: set fields + WHERE
    Where(dema.Equal(UserID, 1)).Run(ctx, true)

db.Update[User](modifiedUser).Run(ctx, true)                        // full-row (requires pk)
db.UpdateRow(ctx, modifiedUser)                                      // shorthand
```

## Delete

```go
db.Delete[User]().Where(dema.Equal(UserID, 1)).Run(ctx, true)
// WHERE is always required - prevents accidental full-table deletes
```

## Transactions

All operations share a single connection; context is bound at creation.

```go
tx, _ := db.Transaction(ctx)
defer tx.Rollback() // no-op if committed

tx.Insert(User{ID: 10, Name: "TxUser"})

rows, _ := tx.Select[User]().
    Where(dema.Greater(UserAge, 18)).
    Run(true)                                                        // no ctx - uses transaction's

tx.Update[User]().Set(UserAge, 99).
    Where(dema.Equal(UserID, 10)).Run(true)

tx.Delete[User]().Where(dema.Equal(UserID, 10)).Run(false)

tx.Commit()
```

Tx mirrors DB: `Select`, `Count`, `Insert`, `Upsert`, `Update`, `UpdateRow`, `Delete`, `RawQuery`, `RawExecute`

## Raw SQL

`RawQuery[T]` supports model structs, primitive/scalar types (`uint32`, `int`, `string`, `bool`, `float64`, `[]rune`, `[]byte`), and pointers:

```go
// Scalar query
counts, _ := db.RawQuery[uint32](ctx, "SELECT COUNT(*) FROM users", false)

// Struct query
users, _ := db.RawQuery[User](ctx,
    `SELECT * FROM "users" WHERE "age" > ?`, true, 18)

db.RawExecute(ctx, `CREATE INDEX idx_age ON "users"("age")`, false)

// In transactions (no ctx parameter)
txCount, _ := tx.RawQuery[uint32](`SELECT COUNT(*) FROM "users" WHERE "age" > ?`, true, 18)
tx.RawExecute(`UPDATE "users" SET "age" = "age" + 1`, true)
```

## Error Handling

All errors use [lumo](https://github.com/amatsagu/lumo) with structured metadata:

```
dema_table     = "users"
dema_operation = "SELECT"
dema_expected  = "connection from pool"
```

Why? It's the lib I use in my silly projects so it's just nice to use it here as well for consistency. You don't like it? Too bad.