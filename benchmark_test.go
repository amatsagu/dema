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

func BenchmarkSelectByID_Raw(b *testing.B) {
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
				u.ID = uint32(stmt.ColumnInt64(0))
				u.Name = stmt.ColumnText(1)
				u.Age = int(stmt.ColumnInt64(2))
				u.Active = stmt.ColumnBool(3)
				if !stmt.ColumnIsNull(4) {
					bio := stmt.ColumnText(4)
					u.Bio = &bio
				}
				u.Score = stmt.ColumnFloat(5)
				return nil
			},
		}

		if err := sqlitex.Execute(conn, "SELECT id, surname, age, active, bio, score FROM users WHERE id = ? LIMIT 1;", opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func BenchmarkSelectByID_Dema(b *testing.B) {
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

func BenchmarkSelectFilter_Raw(b *testing.B) {
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
				u.ID = uint32(stmt.ColumnInt64(0))
				u.Name = stmt.ColumnText(1)
				u.Age = int(stmt.ColumnInt64(2))
				u.Active = stmt.ColumnBool(3)
				if !stmt.ColumnIsNull(4) {
					bio := stmt.ColumnText(4)
					u.Bio = &bio
				}
				u.Score = stmt.ColumnFloat(5)
				results = append(results, u)
				return nil
			},
		}

		if err := sqlitex.Execute(conn, "SELECT id, surname, age, active, bio, score FROM users WHERE age >= ? AND age <= ? ORDER BY age ASC LIMIT 10;", opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func BenchmarkSelectFilter_Dema(b *testing.B) {
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

func BenchmarkInsert_Raw(b *testing.B) {
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
			Args: []any{fmt.Sprintf("BenchRaw-%d", i), int64(30), true, 95.5},
		}
		if err := sqlitex.Execute(conn, "INSERT INTO users (surname, age, active, score) VALUES (?, ?, ?, ?);", opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func BenchmarkInsert_Dema(b *testing.B) {
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
		if err := db.Insert(ctx, u); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUpdate_Raw(b *testing.B) {
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

func BenchmarkUpdate_Dema(b *testing.B) {
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

func BenchmarkUpsert_Raw(b *testing.B) {
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

func BenchmarkUpsert_Dema(b *testing.B) {
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
		if err := db.Upsert(ctx, p); err != nil {
			b.Fatal(err)
		}
	}
}
