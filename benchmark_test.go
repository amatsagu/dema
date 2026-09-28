package dema

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func setupBenchDB(b *testing.B) (*DB, *sqlitex.Pool) {
	b.Helper()

	seedBytes, err := os.ReadFile("seed.sql")
	if err != nil {
		b.Fatalf("failed to read seed.sql: %v", err)
	}

	db, err := Open(":memory:", 5)
	if err != nil {
		b.Fatalf("failed to open dema db: %v", err)
	}

	conn, err := db.pool.Take(context.Background())
	if err != nil {
		b.Fatalf("failed to take conn: %v", err)
	}
	defer db.pool.Put(conn)

	queries := strings.Split(string(seedBytes), ";")
	for _, q := range queries {
		q = strings.TrimSpace(q)
		if q == "" || strings.HasPrefix(q, "--") {
			continue
		}
		if err := sqlitex.ExecuteTransient(conn, q+";", nil); err != nil {
			b.Fatalf("failed executing seed query: %v", err)
		}
	}

	db.Table[User]("users", nil, nil, nil)
	db.Table[Product]("products", nil, nil, nil)
	db.Table[Order]("orders", nil, nil, nil)

	return db, db.pool
}

// --- SelectByID ---

func BenchmarkSelectByID_Cached_Raw(b *testing.B) {
	_, pool := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		var u User
		opts := &sqlitex.ExecOptions{
			Args: []any{int64(1)},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				if !stmt.ColumnIsNull(0) {
					bio := stmt.ColumnText(0)
					u.Bio = &bio
				}
				u.Name = stmt.ColumnText(1)
				u.Age = int(stmt.ColumnInt64(2))
				u.Score = stmt.ColumnFloat(3)
				u.ID = uint32(stmt.ColumnInt64(4))
				u.Active = stmt.ColumnBool(5)
				return nil
			},
		}

		if err := sqlitex.Execute(conn, "SELECT bio, surname, age, score, id, active FROM users WHERE id = ? LIMIT 1;", opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func BenchmarkSelectByID_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		res, err := db.Select[User]().
			Where(Equal(UserField.ID, 1)).
			Limit(1).
			Run(ctx, true)
		if err != nil {
			b.Fatal(err)
		}
		if len(res) == 0 {
			b.Fatal("no user returned")
		}
	}
}

func BenchmarkSelectByID_Transient_Raw(b *testing.B) {
	_, pool := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		var u User
		opts := &sqlitex.ExecOptions{
			Args: []any{int64(1)},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				if !stmt.ColumnIsNull(0) {
					bio := stmt.ColumnText(0)
					u.Bio = &bio
				}
				u.Name = stmt.ColumnText(1)
				u.Age = int(stmt.ColumnInt64(2))
				u.Score = stmt.ColumnFloat(3)
				u.ID = uint32(stmt.ColumnInt64(4))
				u.Active = stmt.ColumnBool(5)
				return nil
			},
		}

		if err := sqlitex.ExecuteTransient(conn, "SELECT bio, surname, age, score, id, active FROM users WHERE id = ? LIMIT 1;", opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func BenchmarkSelectByID_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		res, err := db.Select[User]().
			Where(Equal(UserField.ID, 1)).
			Limit(1).
			Run(ctx, false)
		if err != nil {
			b.Fatal(err)
		}
		if len(res) == 0 {
			b.Fatal("no user returned")
		}
	}
}

// --- SelectFilter ---

func BenchmarkSelectFilter_Cached_Raw(b *testing.B) {
	_, pool := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		results := make([]User, 0, 10)
		opts := &sqlitex.ExecOptions{
			Args: []any{int64(25), int64(35)},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				var u User
				if !stmt.ColumnIsNull(0) {
					bio := stmt.ColumnText(0)
					u.Bio = &bio
				}
				u.Name = stmt.ColumnText(1)
				u.Age = int(stmt.ColumnInt64(2))
				u.Score = stmt.ColumnFloat(3)
				u.ID = uint32(stmt.ColumnInt64(4))
				u.Active = stmt.ColumnBool(5)
				results = append(results, u)
				return nil
			},
		}

		if err := sqlitex.Execute(conn, "SELECT bio, surname, age, score, id, active FROM users WHERE age BETWEEN ? AND ? ORDER BY age ASC LIMIT 10;", opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func BenchmarkSelectFilter_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		res, err := db.Select[User]().
			Where(Range(UserField.Age, 25, 35)).
			OrderBy(UserField.Age, Asc).
			Limit(10).
			Run(ctx, true)
		if err != nil {
			b.Fatal(err)
		}
		if len(res) == 0 {
			b.Fatal("no rows")
		}
	}
}

func BenchmarkSelectFilter_Transient_Raw(b *testing.B) {
	_, pool := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		results := make([]User, 0, 10)
		opts := &sqlitex.ExecOptions{
			Args: []any{int64(25), int64(35)},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				var u User
				if !stmt.ColumnIsNull(0) {
					bio := stmt.ColumnText(0)
					u.Bio = &bio
				}
				u.Name = stmt.ColumnText(1)
				u.Age = int(stmt.ColumnInt64(2))
				u.Score = stmt.ColumnFloat(3)
				u.ID = uint32(stmt.ColumnInt64(4))
				u.Active = stmt.ColumnBool(5)
				results = append(results, u)
				return nil
			},
		}

		if err := sqlitex.ExecuteTransient(conn, "SELECT bio, surname, age, score, id, active FROM users WHERE age BETWEEN ? AND ? ORDER BY age ASC LIMIT 10;", opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func BenchmarkSelectFilter_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		res, err := db.Select[User]().
			Where(Range(UserField.Age, 25, 35)).
			OrderBy(UserField.Age, Asc).
			Limit(10).
			Run(ctx, false)
		if err != nil {
			b.Fatal(err)
		}
		if len(res) == 0 {
			b.Fatal("no rows")
		}
	}
}

// --- Insert ---

func BenchmarkInsert_Cached_Raw(b *testing.B) {
	_, pool := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	var i int
	for b.Loop() {
		i++
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		opts := &sqlitex.ExecOptions{
			Args: []any{nil, fmt.Sprintf("BenchRaw-%d", i), int64(30), 95.5, true},
		}
		if err := sqlitex.Execute(conn, "INSERT INTO users (bio, surname, age, score, active) VALUES (?, ?, ?, ?, ?);", opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func BenchmarkInsert_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	var i int
	for b.Loop() {
		i++
		u := User{
			Name:   fmt.Sprintf("BenchDema-%d", i),
			Age:    30,
			Active: true,
			Score:  95.5,
		}
		if err := db.Insert(ctx, u, true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInsert_Transient_Raw(b *testing.B) {
	_, pool := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	var i int
	for b.Loop() {
		i++
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		opts := &sqlitex.ExecOptions{
			Args: []any{nil, fmt.Sprintf("BenchRaw-%d", i), int64(30), 95.5, true},
		}
		if err := sqlitex.ExecuteTransient(conn, "INSERT INTO users (bio, surname, age, score, active) VALUES (?, ?, ?, ?, ?);", opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func BenchmarkInsert_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	var i int
	for b.Loop() {
		i++
		u := User{
			Name:   fmt.Sprintf("BenchDema-%d", i),
			Age:    30,
			Active: true,
			Score:  95.5,
		}
		if err := db.Insert(ctx, u, false); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Update ---

func BenchmarkUpdate_Cached_Raw(b *testing.B) {
	_, pool := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		opts := &sqlitex.ExecOptions{
			Args: []any{"UpdatedRaw", int64(33), int64(1)},
		}
		if err := sqlitex.Execute(conn, "UPDATE users SET surname = ?, age = ? WHERE id = ?;", opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func BenchmarkUpdate_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		err := db.Update[User]().
			Set(UserField.Name, "UpdatedDema").
			Set(UserField.Age, 33).
			Where(Equal(UserField.ID, 1)).
			Run(ctx, true)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUpdate_Transient_Raw(b *testing.B) {
	_, pool := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		opts := &sqlitex.ExecOptions{
			Args: []any{"UpdatedRaw", int64(33), int64(1)},
		}
		if err := sqlitex.ExecuteTransient(conn, "UPDATE users SET surname = ?, age = ? WHERE id = ?;", opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func BenchmarkUpdate_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		err := db.Update[User]().
			Set(UserField.Name, "UpdatedDema").
			Set(UserField.Age, 33).
			Where(Equal(UserField.ID, 1)).
			Run(ctx, false)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// --- Upsert ---

func BenchmarkUpsert_Cached_Raw(b *testing.B) {
	_, pool := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		opts := &sqlitex.ExecOptions{
			Args: []any{"SKU-0001", "Raw Product", int64(100), int64(50), "Electronics"},
		}
		sql := `INSERT INTO products (sku, name, price, stock, category) VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(sku) DO UPDATE SET name = excluded.name, price = excluded.price, stock = excluded.stock, category = excluded.category;`
		if err := sqlitex.Execute(conn, sql, opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func BenchmarkUpsert_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	ctx := context.Background()
	p := Product{
		SKU:      "SKU-0001",
		Name:     "Dema Product",
		Price:    100,
		Stock:    50,
		Category: "Electronics",
	}
	b.ReportAllocs()

	for b.Loop() {
		if err := db.Upsert(ctx, p, true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUpsert_Transient_Raw(b *testing.B) {
	_, pool := setupBenchDB(b)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		opts := &sqlitex.ExecOptions{
			Args: []any{"SKU-0001", "Raw Product", int64(100), int64(50), "Electronics"},
		}
		sql := `INSERT INTO products (sku, name, price, stock, category) VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(sku) DO UPDATE SET name = excluded.name, price = excluded.price, stock = excluded.stock, category = excluded.category;`
		if err := sqlitex.ExecuteTransient(conn, sql, opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func BenchmarkUpsert_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	ctx := context.Background()
	p := Product{
		SKU:      "SKU-0001",
		Name:     "Dema Product",
		Price:    100,
		Stock:    50,
		Category: "Electronics",
	}
	b.ReportAllocs()

	for b.Loop() {
		if err := db.Upsert(ctx, p, false); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Backward-Compatible Aliases ---

func BenchmarkSelectByID_Raw(b *testing.B)    { BenchmarkSelectByID_Cached_Raw(b) }
func BenchmarkSelectByID_Dema(b *testing.B)   { BenchmarkSelectByID_Cached_Dema(b) }
func BenchmarkSelectFilter_Raw(b *testing.B)  { BenchmarkSelectFilter_Cached_Raw(b) }
func BenchmarkSelectFilter_Dema(b *testing.B) { BenchmarkSelectFilter_Cached_Dema(b) }
func BenchmarkInsert_Raw(b *testing.B)        { BenchmarkInsert_Cached_Raw(b) }
func BenchmarkInsert_Dema(b *testing.B)       { BenchmarkInsert_Cached_Dema(b) }
func BenchmarkUpdate_Raw(b *testing.B)        { BenchmarkUpdate_Cached_Raw(b) }
func BenchmarkUpdate_Dema(b *testing.B)       { BenchmarkUpdate_Cached_Dema(b) }
func BenchmarkUpsert_Raw(b *testing.B)        { BenchmarkUpsert_Cached_Raw(b) }
func BenchmarkUpsert_Dema(b *testing.B)       { BenchmarkUpsert_Cached_Dema(b) }
