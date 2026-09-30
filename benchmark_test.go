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

func setupBenchDBWithPath(b *testing.B, path string) (*DB, *sqlitex.Pool) {
	b.Helper()

	seedBytes, err := os.ReadFile("seed.sql")
	if err != nil {
		b.Fatalf("failed to read seed.sql: %v", err)
	}

	db, err := Open(path, 5)
	if err != nil {
		b.Fatalf("failed to open dema db: %v", err)
	}

	conn, err := db.pool.Take(context.Background())
	if err != nil {
		b.Fatalf("failed to take conn: %v", err)
	}
	defer db.pool.Put(conn)

	_ = sqlitex.ExecuteTransient(conn, "BEGIN;", nil)
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
	_ = sqlitex.ExecuteTransient(conn, "COMMIT;", nil)

	db.Table[User]("users", nil, nil, nil)
	db.Table[Product]("products", nil, nil, nil)
	db.Table[Order]("orders", nil, nil, nil)

	return db, db.pool
}

func setupBenchDB(b *testing.B) (*DB, *sqlitex.Pool) {
	b.Helper()
	return setupBenchDBWithPath(b, ":memory:")
}

// --- SelectByID Runners ---

func runSelectByID_Cached_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
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

func runSelectByID_Cached_Dema(b *testing.B, db *DB) {
	b.Helper()
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

func runSelectByID_Transient_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
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

func runSelectByID_Transient_Dema(b *testing.B, db *DB) {
	b.Helper()
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

// --- SelectFilter Runners ---

func runSelectFilter_Cached_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
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

func runSelectFilter_Cached_Dema(b *testing.B, db *DB) {
	b.Helper()
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

func runSelectFilter_Transient_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
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

func runSelectFilter_Transient_Dema(b *testing.B, db *DB) {
	b.Helper()
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

// --- Insert Runners ---

func runInsert_Cached_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
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

func runInsert_Cached_Dema(b *testing.B, db *DB) {
	b.Helper()
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

func runInsert_Transient_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
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

func runInsert_Transient_Dema(b *testing.B, db *DB) {
	b.Helper()
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

// --- Update Runners ---

func runUpdate_Cached_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
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

func runUpdate_Cached_Dema(b *testing.B, db *DB) {
	b.Helper()
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

func runUpdate_Transient_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
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

func runUpdate_Transient_Dema(b *testing.B, db *DB) {
	b.Helper()
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

// --- Upsert Runners ---

func runUpsert_Cached_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
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

func runUpsert_Cached_Dema(b *testing.B, db *DB) {
	b.Helper()
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

func runUpsert_Transient_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
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

func runUpsert_Transient_Dema(b *testing.B, db *DB) {
	b.Helper()
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

// --- Benchmark Definitions (In-Memory) ---

func BenchmarkSelectByID_Cached_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runSelectByID_Cached_Raw(b, pool)
}

func BenchmarkSelectByID_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runSelectByID_Cached_Dema(b, db)
}

func BenchmarkSelectByID_Transient_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runSelectByID_Transient_Raw(b, pool)
}

func BenchmarkSelectByID_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runSelectByID_Transient_Dema(b, db)
}

func BenchmarkSelectFilter_Cached_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runSelectFilter_Cached_Raw(b, pool)
}

func BenchmarkSelectFilter_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runSelectFilter_Cached_Dema(b, db)
}

func BenchmarkSelectFilter_Transient_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runSelectFilter_Transient_Raw(b, pool)
}

func BenchmarkSelectFilter_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runSelectFilter_Transient_Dema(b, db)
}

func BenchmarkInsert_Cached_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runInsert_Cached_Raw(b, pool)
}

func BenchmarkInsert_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runInsert_Cached_Dema(b, db)
}

func BenchmarkInsert_Transient_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runInsert_Transient_Raw(b, pool)
}

func BenchmarkInsert_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runInsert_Transient_Dema(b, db)
}

func BenchmarkUpdate_Cached_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runUpdate_Cached_Raw(b, pool)
}

func BenchmarkUpdate_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runUpdate_Cached_Dema(b, db)
}

func BenchmarkUpdate_Transient_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runUpdate_Transient_Raw(b, pool)
}

func BenchmarkUpdate_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runUpdate_Transient_Dema(b, db)
}

func BenchmarkUpsert_Cached_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runUpsert_Cached_Raw(b, pool)
}

func BenchmarkUpsert_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runUpsert_Cached_Dema(b, db)
}

func BenchmarkUpsert_Transient_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runUpsert_Transient_Raw(b, pool)
}

func BenchmarkUpsert_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runUpsert_Transient_Dema(b, db)
}

// --- Count Runners ---

func runCount_Cached_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		var count int64
		opts := &sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				count = stmt.ColumnInt64(0)
				return nil
			},
		}

		if err := sqlitex.Execute(conn, `SELECT COUNT(*) FROM "users";`, opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
		if count == 0 {
			b.Fatal("unexpected 0 count")
		}
	}
}

func runCount_Cached_Dema(b *testing.B, db *DB) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		count, err := db.Count[User](nil).Run(ctx, true)
		if err != nil {
			b.Fatal(err)
		}
		if count == 0 {
			b.Fatal("unexpected 0 count")
		}
	}
}

func runCount_Transient_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		var count int64
		opts := &sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				count = stmt.ColumnInt64(0)
				return nil
			},
		}

		if err := sqlitex.ExecuteTransient(conn, `SELECT COUNT(*) FROM "users";`, opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
		if count == 0 {
			b.Fatal("unexpected 0 count")
		}
	}
}

func runCount_Transient_Dema(b *testing.B, db *DB) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		count, err := db.Count[User](nil).Run(ctx, false)
		if err != nil {
			b.Fatal(err)
		}
		if count == 0 {
			b.Fatal("unexpected 0 count")
		}
	}
}

func BenchmarkCount_Cached_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runCount_Cached_Raw(b, pool)
}

func BenchmarkCount_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runCount_Cached_Dema(b, db)
}

func BenchmarkCount_Transient_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runCount_Transient_Raw(b, pool)
}

func BenchmarkCount_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runCount_Transient_Dema(b, db)
}

// --- Delete Runners ---

func runDelete_Cached_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		opts := &sqlitex.ExecOptions{
			Args: []any{int64(999999)},
		}
		if err := sqlitex.Execute(conn, `DELETE FROM "users" WHERE id = ?;`, opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func runDelete_Cached_Dema(b *testing.B, db *DB) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		err := db.Delete[User]().
			Where(Equal(UserField.ID, 999999)).
			Run(ctx, true)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func runDelete_Transient_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		opts := &sqlitex.ExecOptions{
			Args: []any{int64(999999)},
		}
		if err := sqlitex.ExecuteTransient(conn, `DELETE FROM "users" WHERE id = ?;`, opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func runDelete_Transient_Dema(b *testing.B, db *DB) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		err := db.Delete[User]().
			Where(Equal(UserField.ID, 999999)).
			Run(ctx, false)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDelete_Cached_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runDelete_Cached_Raw(b, pool)
}

func BenchmarkDelete_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runDelete_Cached_Dema(b, db)
}

func BenchmarkDelete_Transient_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runDelete_Transient_Raw(b, pool)
}

func BenchmarkDelete_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runDelete_Transient_Dema(b, db)
}

// --- Returning Runners ---

func runInsert_Returning_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	var i int
	for b.Loop() {
		i++
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		var u User
		opts := &sqlitex.ExecOptions{
			Args: []any{nil, fmt.Sprintf("BenchRaw-%d", i), int64(30), 95.5, true},
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
		if err := sqlitex.Execute(conn, `INSERT INTO users (bio, surname, age, score, active) VALUES (?, ?, ?, ?, ?) RETURNING bio, surname, age, score, id, active;`, opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func runInsert_Returning_Dema(b *testing.B, db *DB) {
	b.Helper()
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
		res, err := db.InsertInto[User](u).Returning().Run(ctx, true)
		if err != nil || len(res) == 0 {
			b.Fatalf("Insert Returning failed: %v", err)
		}
	}
}

func runInsert_Then_Select_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	var i int
	for b.Loop() {
		i++
		name := fmt.Sprintf("BenchRaw-%d", i)
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		insertOpts := &sqlitex.ExecOptions{
			Args: []any{nil, name, int64(30), 95.5, true},
		}
		if err := sqlitex.Execute(conn, "INSERT INTO users (bio, surname, age, score, active) VALUES (?, ?, ?, ?, ?);", insertOpts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)

		conn, err = pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}
		var u User
		selectOpts := &sqlitex.ExecOptions{
			Args: []any{name},
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
		if err := sqlitex.Execute(conn, "SELECT bio, surname, age, score, id, active FROM users WHERE surname = ? LIMIT 1;", selectOpts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func runInsert_Then_Select_Dema(b *testing.B, db *DB) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	var i int
	for b.Loop() {
		i++
		name := fmt.Sprintf("BenchDema-%d", i)
		u := User{
			Name:   name,
			Age:    30,
			Active: true,
			Score:  95.5,
		}
		if err := db.Insert(ctx, u, true); err != nil {
			b.Fatal(err)
		}
		res, err := db.Select[User]().
			Where(Equal(UserField.Name, name)).
			Limit(1).
			Run(ctx, true)
		if err != nil || len(res) == 0 {
			b.Fatalf("Select after insert failed: %v", err)
		}
	}
}

func runUpdate_Returning_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		var u User
		opts := &sqlitex.ExecOptions{
			Args: []any{"UpdatedRaw", int64(33), int64(1)},
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
		if err := sqlitex.Execute(conn, `UPDATE users SET surname = ?, age = ? WHERE id = ? RETURNING bio, surname, age, score, id, active;`, opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func runUpdate_Returning_Dema(b *testing.B, db *DB) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		res, err := db.Update[User]().
			Set(UserField.Name, "UpdatedDema").
			Set(UserField.Age, 33).
			Where(Equal(UserField.ID, 1)).
			Returning().
			Run(ctx, true)
		if err != nil || len(res) == 0 {
			b.Fatalf("Update Returning failed: %v", err)
		}
	}
}

func runDelete_Returning_Raw(b *testing.B, pool *sqlitex.Pool) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := pool.Take(ctx)
		if err != nil {
			b.Fatal(err)
		}

		var u User
		opts := &sqlitex.ExecOptions{
			Args: []any{int64(999999)},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				u.ID = uint32(stmt.ColumnInt64(0))
				return nil
			},
		}
		if err := sqlitex.Execute(conn, `DELETE FROM "users" WHERE id = ? RETURNING id;`, opts); err != nil {
			pool.Put(conn)
			b.Fatal(err)
		}
		pool.Put(conn)
	}
}

func runDelete_Returning_Dema(b *testing.B, db *DB) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		_, err := db.Delete[User]().
			Where(Equal(UserField.ID, 999999)).
			Returning(UserField.ID).
			Run(ctx, true)
		if err != nil {
			b.Fatalf("Delete Returning failed: %v", err)
		}
	}
}

func BenchmarkInsert_Returning_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runInsert_Returning_Raw(b, pool)
}

func BenchmarkInsert_Returning_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runInsert_Returning_Dema(b, db)
}

func BenchmarkInsert_Then_Select_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runInsert_Then_Select_Raw(b, pool)
}

func BenchmarkInsert_Then_Select_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runInsert_Then_Select_Dema(b, db)
}

func BenchmarkUpdate_Returning_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runUpdate_Returning_Raw(b, pool)
}

func BenchmarkUpdate_Returning_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runUpdate_Returning_Dema(b, db)
}

func BenchmarkDelete_Returning_Raw(b *testing.B) {
	db, pool := setupBenchDB(b)
	defer db.Close()
	runDelete_Returning_Raw(b, pool)
}

func BenchmarkDelete_Returning_Dema(b *testing.B) {
	db, _ := setupBenchDB(b)
	defer db.Close()
	runDelete_Returning_Dema(b, db)
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
func BenchmarkCount_Raw(b *testing.B)         { BenchmarkCount_Cached_Raw(b) }
func BenchmarkCount_Dema(b *testing.B)        { BenchmarkCount_Cached_Dema(b) }
func BenchmarkDelete_Raw(b *testing.B)        { BenchmarkDelete_Cached_Raw(b) }
func BenchmarkDelete_Dema(b *testing.B)       { BenchmarkDelete_Cached_Dema(b) }
