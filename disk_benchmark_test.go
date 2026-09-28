package dema

import (
	"os"
	"path/filepath"
	"testing"

	"zombiezen.com/go/sqlite/sqlitex"
)

func setupBenchDiskDB(b *testing.B) (*DB, *sqlitex.Pool) {
	b.Helper()
	d, err := os.MkdirTemp(".", ".bench_disk_*")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { os.RemoveAll(d) })
	dbPath := filepath.Join(d, "bench.db")
	return setupBenchDBWithPath(b, dbPath)
}

// --- Disk SelectByID ---

func BenchmarkDisk_SelectByID_Cached_Raw(b *testing.B) {
	db, pool := setupBenchDiskDB(b)
	defer db.Close()
	runSelectByID_Cached_Raw(b, pool)
}

func BenchmarkDisk_SelectByID_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDiskDB(b)
	defer db.Close()
	runSelectByID_Cached_Dema(b, db)
}

func BenchmarkDisk_SelectByID_Transient_Raw(b *testing.B) {
	db, pool := setupBenchDiskDB(b)
	defer db.Close()
	runSelectByID_Transient_Raw(b, pool)
}

func BenchmarkDisk_SelectByID_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDiskDB(b)
	defer db.Close()
	runSelectByID_Transient_Dema(b, db)
}

// --- Disk SelectFilter ---

func BenchmarkDisk_SelectFilter_Cached_Raw(b *testing.B) {
	db, pool := setupBenchDiskDB(b)
	defer db.Close()
	runSelectFilter_Cached_Raw(b, pool)
}

func BenchmarkDisk_SelectFilter_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDiskDB(b)
	defer db.Close()
	runSelectFilter_Cached_Dema(b, db)
}

func BenchmarkDisk_SelectFilter_Transient_Raw(b *testing.B) {
	db, pool := setupBenchDiskDB(b)
	defer db.Close()
	runSelectFilter_Transient_Raw(b, pool)
}

func BenchmarkDisk_SelectFilter_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDiskDB(b)
	defer db.Close()
	runSelectFilter_Transient_Dema(b, db)
}

// --- Disk Insert ---

func BenchmarkDisk_Insert_Cached_Raw(b *testing.B) {
	db, pool := setupBenchDiskDB(b)
	defer db.Close()
	runInsert_Cached_Raw(b, pool)
}

func BenchmarkDisk_Insert_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDiskDB(b)
	defer db.Close()
	runInsert_Cached_Dema(b, db)
}

func BenchmarkDisk_Insert_Transient_Raw(b *testing.B) {
	db, pool := setupBenchDiskDB(b)
	defer db.Close()
	runInsert_Transient_Raw(b, pool)
}

func BenchmarkDisk_Insert_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDiskDB(b)
	defer db.Close()
	runInsert_Transient_Dema(b, db)
}

// --- Disk Update ---

func BenchmarkDisk_Update_Cached_Raw(b *testing.B) {
	db, pool := setupBenchDiskDB(b)
	defer db.Close()
	runUpdate_Cached_Raw(b, pool)
}

func BenchmarkDisk_Update_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDiskDB(b)
	defer db.Close()
	runUpdate_Cached_Dema(b, db)
}

func BenchmarkDisk_Update_Transient_Raw(b *testing.B) {
	db, pool := setupBenchDiskDB(b)
	defer db.Close()
	runUpdate_Transient_Raw(b, pool)
}

func BenchmarkDisk_Update_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDiskDB(b)
	defer db.Close()
	runUpdate_Transient_Dema(b, db)
}

// --- Disk Upsert ---

func BenchmarkDisk_Upsert_Cached_Raw(b *testing.B) {
	db, pool := setupBenchDiskDB(b)
	defer db.Close()
	runUpsert_Cached_Raw(b, pool)
}

func BenchmarkDisk_Upsert_Cached_Dema(b *testing.B) {
	db, _ := setupBenchDiskDB(b)
	defer db.Close()
	runUpsert_Cached_Dema(b, db)
}

func BenchmarkDisk_Upsert_Transient_Raw(b *testing.B) {
	db, pool := setupBenchDiskDB(b)
	defer db.Close()
	runUpsert_Transient_Raw(b, pool)
}

func BenchmarkDisk_Upsert_Transient_Dema(b *testing.B) {
	db, _ := setupBenchDiskDB(b)
	defer db.Close()
	runUpsert_Transient_Dema(b, db)
}
