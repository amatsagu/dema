package dema

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/amatsagu/lumo"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type CustomID uint64

func (c CustomID) EncodeDema() (any, error) {
	return int64(c), nil
}

func (c *CustomID) DecodeDema(v any) error {
	switch val := v.(type) {
	case int64:
		*c = CustomID(val)
	case int:
		*c = CustomID(val)
	case uint64:
		*c = CustomID(val)
	}
	return nil
}

type User struct {
	Bio    *string `db:"bio"`
	Name   string  `db:"surname"`
	Age    int     `db:"age"`
	Score  float64 `db:"score"`
	ID     uint32  `db:"id,pk,omitzero"`
	Active bool    `db:"active"`
}

type Product struct {
	SKU      string `db:"sku,pk"`
	Name     string `db:"name"`
	Category string `db:"category"`
	Price    int    `db:"price"`
	Stock    int    `db:"stock"`
}

type Order struct {
	Status   string  `db:"status"`
	OrderID  int     `db:"order_id,pk"`
	ItemID   int     `db:"item_id,pk"`
	Quantity int     `db:"quantity"`
	Total    float64 `db:"total"`
}

type CustomModel struct {
	Code string   `db:"code"`
	ID   CustomID `db:"id,pk"`
}

type RuneModel struct {
	Chars []rune `db:"chars"`
	ID    int    `db:"id,pk"`
}

type BaseAudit struct {
	CreatedBy string `db:"created_by"`
}

type EmbeddedModel struct {
	BaseAudit
	Title string `db:"title"`
	ID    int    `db:"id,pk"`
}

type IgnoredFieldModel struct {
	Name    string `db:"name"`
	Ignored string `db:"-"`
	private string
	ID      int `db:"id,pk"`
}

type PKOnlyModel struct {
	ID1 int `db:"id1,pk"`
	ID2 int `db:"id2,pk"`
}

var UserField = struct {
	ID     Field[User, uint32]
	Name   Field[User, string]
	Age    Field[User, int]
	Active Field[User, bool]
	Score  Field[User, float64]
}{
	ID:     NewField[User, uint32]("id"),
	Name:   NewField[User, string]("surname"),
	Age:    NewField[User, int]("age"),
	Active: NewField[User, bool]("active"),
	Score:  NewField[User, float64]("score"),
}

var ProductField = struct {
	SKU      Field[Product, string]
	Name     Field[Product, string]
	Price    Field[Product, int]
	Stock    Field[Product, int]
	Category Field[Product, string]
}{
	SKU:      NewField[Product, string]("sku"),
	Name:     NewField[Product, string]("name"),
	Price:    NewField[Product, int]("price"),
	Stock:    NewField[Product, int]("stock"),
	Category: NewField[Product, string]("category"),
}

var OrderField = struct {
	OrderID  Field[Order, int]
	ItemID   Field[Order, int]
	Quantity Field[Order, int]
	Total    Field[Order, float64]
	Status   Field[Order, string]
}{
	OrderID:  NewField[Order, int]("order_id"),
	ItemID:   NewField[Order, int]("item_id"),
	Quantity: NewField[Order, int]("quantity"),
	Total:    NewField[Order, float64]("total"),
	Status:   NewField[Order, string]("status"),
}

func setupTestDB(t *testing.T) *DB {
	t.Helper()

	seedBytes, err := os.ReadFile("seed.sql")
	if err != nil {
		t.Fatalf("failed to read seed.sql: %v", err)
	}

	db, err := Open(":memory:", 5)
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	conn, err := db.pool.Take(context.Background())
	if err != nil {
		t.Fatalf("failed to get connection for seed: %v", err)
	}
	defer db.pool.Put(conn)

	queries := strings.Split(string(seedBytes), ";")
	for _, q := range queries {
		q = strings.TrimSpace(q)
		if q == "" || strings.HasPrefix(q, "--") {
			continue
		}
		if err := sqlitex.ExecuteTransient(conn, q+";", nil); err != nil {
			t.Fatalf("failed executing seed query %q: %v", q, err)
		}
	}

	db.Table[User]("users", nil, nil, nil)
	db.Table[Product]("products", nil, nil, nil)
	db.Table[Order]("orders", nil, nil, nil)

	return db
}

func TestOpenAndClose(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("second close should be nil: %v", err)
	}

	db2, err := Open(":memory:", 0, sqlite.OpenReadWrite|sqlite.OpenCreate)
	if err != nil {
		t.Fatalf("open with flags error: %v", err)
	}
	_ = db2.Close()
}

func TestTableRegistration(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Table[User]("users", nil, nil, nil)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic on duplicate table registration")
		}
	}()
	db.Table[User]("users_dup", nil, nil, nil)
}

func TestTableRegistrationNonStruct(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic on non-struct table registration")
		}
	}()
	db.Table[int]("ints", nil, nil, nil)
}

func TestTableRegistrationEmptyStruct(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	type Empty struct{}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic on empty struct table registration")
		}
	}()
	db.Table[Empty]("empty", nil, nil, nil)
}

func TestTableTagsAndEmbedded(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Table[IgnoredFieldModel]("ignored", nil, nil, nil)
	info, err := db.getTableInfo(infoType[IgnoredFieldModel]())
	if err != nil {
		t.Fatal(err)
	}
	if len(info.allCols) != 2 {
		t.Fatalf("expected 2 cols (id, name), got %d", len(info.allCols))
	}

	db.Table[EmbeddedModel]("embedded", nil, nil, nil)
	embInfo, err := db.getTableInfo(infoType[EmbeddedModel]())
	if err != nil {
		t.Fatal(err)
	}
	if len(embInfo.allCols) != 3 {
		t.Fatalf("expected 3 cols (id, created_by, title), got %d", len(embInfo.allCols))
	}
}

func infoType[T any]() reflectType {
	var zero T
	return reflect.TypeOf(zero)
}

type reflectType = reflect.Type

func TestSelectAllAndLimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	products, err := db.Select[Product]().Run(ctx, true)
	if err != nil {
		t.Fatalf("select products error: %v", err)
	}
	if len(products) != 200 {
		t.Fatalf("expected 200 products, got %d", len(products))
	}

	limUsers, err := db.Select[User]().Limit(5).Run(ctx, false)
	if err != nil {
		t.Fatalf("select with limit error: %v", err)
	}
	if len(limUsers) != 5 {
		t.Fatalf("expected 5 users, got %d", len(limUsers))
	}
}

func TestSelectWhereComparators(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	users, err := db.Select[User]().Where(Equal(UserField.ID, 1)).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].ID != 1 {
		t.Fatalf("unexpected user: %+v", users)
	}

	uRange, err := db.Select[User]().
		Where(And(Greater(UserField.ID, 10), Lesser(UserField.ID, 15))).
		OrderBy(UserField.ID, Asc).
		Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(uRange) != 4 {
		t.Fatalf("expected 4 users, got %d", len(uRange))
	}

	uWithin, err := db.Select[User]().Where(Within(UserField.ID, 20, 21, 22)).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(uWithin) != 3 {
		t.Fatalf("expected 3 users, got %d", len(uWithin))
	}

	uEmptyWithin, err := db.Select[User]().Where(Within(UserField.ID)).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(uEmptyWithin) != 0 {
		t.Fatalf("expected 0 users, got %d", len(uEmptyWithin))
	}

	uNotWithin, err := db.Select[User]().
		Where(And(Lesser(UserField.ID, 5), NotWithin(UserField.ID, 1, 2))).
		Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(uNotWithin) != 2 {
		t.Fatalf("expected 2 users, got %d", len(uNotWithin))
	}

	uAll, err := db.Select[User]().Where(NotWithin(UserField.ID)).Limit(3).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(uAll) != 3 {
		t.Fatalf("expected 3 users, got %d", len(uAll))
	}

	uBetween, err := db.Select[User]().Where(Range(UserField.ID, 50, 55)).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(uBetween) != 6 {
		t.Fatalf("expected 6 users, got %d", len(uBetween))
	}

	uOutside, err := db.Select[User]().
		Where(And(Lesser(UserField.ID, 5), OutsideRange(UserField.ID, 2, 4))).
		Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(uOutside) != 1 || uOutside[0].ID != 1 {
		t.Fatalf("expected user 1, got %+v", uOutside)
	}

	uOr, err := db.Select[User]().Where(Or(Equal(UserField.ID, 1), Equal(UserField.ID, 2))).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(uOr) != 2 {
		t.Fatalf("expected 2 users, got %d", len(uOr))
	}
}

func TestSelectPagination(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	page1, err := db.Select[User]().OrderBy(UserField.ID, Asc).Page(1, 10).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 10 || page1[0].ID != 1 || page1[9].ID != 10 {
		t.Fatalf("page 1 mismatch: first=%d, last=%d", page1[0].ID, page1[9].ID)
	}

	page2, err := db.Select[User]().OrderBy(UserField.ID, Asc).Page(2, 10).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 10 || page2[0].ID != 11 || page2[9].ID != 20 {
		t.Fatalf("page 2 mismatch: first=%d, last=%d", page2[0].ID, page2[9].ID)
	}

	pZero, err := db.Select[User]().Page(0, -5).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(pZero) != 0 {
		t.Fatalf("expected 0 rows for negative size, got %d", len(pZero))
	}
}

func TestInsertSingleAndMultiple(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	if err := db.Insert(); err != nil {
		t.Fatalf("empty insert should return nil: %v", err)
	}

	bio1 := "Developer"
	u1 := User{ID: 201, Name: "Amatsagu", Age: 30, Active: true, Bio: &bio1, Score: 99.5}
	if err := db.Insert(ctx, u1); err != nil {
		t.Fatalf("insert u1 failed: %v", err)
	}

	fetched, err := db.Select[User]().Where(Equal(UserField.ID, 201)).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(fetched) != 1 || fetched[0].Name != "Amatsagu" || fetched[0].Bio == nil || *fetched[0].Bio != "Developer" {
		t.Fatalf("unexpected fetched user: %+v", fetched)
	}

	u2 := User{Name: "Betta", Age: 25, Active: false}
	u3 := User{Name: "Kiri", Age: 28, Active: true}
	if err := db.Insert(ctx, u2, u3); err != nil {
		t.Fatalf("insert u2, u3 failed: %v", err)
	}

	uAuto, err := db.Select[User]().Where(Within(UserField.ID, 202, 203)).OrderBy(UserField.ID, Asc).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(uAuto) != 2 || uAuto[0].Name != "Betta" || uAuto[1].Name != "Kiri" {
		t.Fatalf("unexpected autoincrement users: %+v", uAuto)
	}
}

func TestUpdateRowAndBuilder(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	u := User{ID: 1, Name: "Updated Name", Age: 40, Active: true, Score: 88.0}
	if err := db.UpdateRow(ctx, u); err != nil {
		t.Fatalf("UpdateRow failed: %v", err)
	}

	check, err := db.Select[User]().Where(Equal(UserField.ID, 1)).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(check) != 1 || check[0].Name != "Updated Name" || check[0].Age != 40 {
		t.Fatalf("updated user mismatch: %+v", check)
	}

	err = db.Update[User]().
		Set(UserField.Name, "Builder Name").
		Set(UserField.Age, 42).
		Where(Equal(UserField.ID, 1)).
		Run(ctx, true)
	if err != nil {
		t.Fatalf("Update builder failed: %v", err)
	}

	check2, err := db.Select[User]().Where(Equal(UserField.ID, 1)).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(check2) != 1 || check2[0].Name != "Builder Name" || check2[0].Age != 42 {
		t.Fatalf("builder updated user mismatch: %+v", check2)
	}

	u2 := User{ID: 1, Name: "Row In Builder", Age: 45}
	err = db.Update(u2).Run(ctx, true)
	if err != nil {
		t.Fatalf("Update builder Row failed: %v", err)
	}

	u3 := User{ID: 1, Name: "Helper Function", Age: 46}
	if err := Update(ctx, db, u3); err != nil {
		t.Fatalf("Update helper function failed: %v", err)
	}

	ord := Order{OrderID: 1, ItemID: 1, Quantity: 99, Total: 123.45, Status: "shipped"}
	if err := db.UpdateRow(ctx, ord); err != nil {
		t.Fatalf("Order composite PK update failed: %v", err)
	}
	checkOrd, err := db.Select[Order]().Where(And(Equal(OrderField.OrderID, 1), Equal(OrderField.ItemID, 1))).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(checkOrd) != 1 || checkOrd[0].Quantity != 99 || checkOrd[0].Status != "shipped" {
		t.Fatalf("order mismatch: %+v", checkOrd)
	}

	errNoWhere := db.Update[User]().Set(UserField.Name, "fail").Run(ctx, true)
	if errNoWhere == nil {
		t.Fatalf("expected error updating without WHERE")
	}

	errNoSet := db.Update[User]().Where(Equal(UserField.ID, 1)).Run(ctx, true)
	if errNoSet == nil {
		t.Fatalf("expected error updating without Set")
	}
}

func TestDeleteBuilder(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	err := db.Delete[User]().Run(ctx, true)
	if err == nil {
		t.Fatalf("expected error deleting without WHERE")
	}

	err = db.Delete[User]().Where(Equal(UserField.ID, 1)).Run(ctx, true)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	users, err := db.Select[User]().Where(Equal(UserField.ID, 1)).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 0 {
		t.Fatalf("expected 0 users after delete, got %d", len(users))
	}
}

func TestUpsert(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	if err := db.Upsert(); err != nil {
		t.Fatalf("empty upsert should return nil: %v", err)
	}

	newP := Product{SKU: "SKU-9999", Name: "Brand New Product", Price: 199, Stock: 50, Category: "Gadgets"}
	if err := db.Upsert(ctx, newP); err != nil {
		t.Fatalf("upsert insert failed: %v", err)
	}

	p1, err := db.Select[Product]().Where(Equal(ProductField.SKU, "SKU-9999")).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(p1) != 1 || p1[0].Price != 199 {
		t.Fatalf("unexpected upserted product: %+v", p1)
	}

	updatedP := Product{SKU: "SKU-9999", Name: "Brand New Product", Price: 250, Stock: 40, Category: "Gadgets"}
	if err := db.Upsert(ctx, updatedP); err != nil {
		t.Fatalf("upsert update failed: %v", err)
	}

	p2, err := db.Select[Product]().Where(Equal(ProductField.SKU, "SKU-9999")).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(p2) != 1 || p2[0].Price != 250 || p2[0].Stock != 40 {
		t.Fatalf("unexpected updated product on conflict: %+v", p2)
	}

	ord := Order{OrderID: 100, ItemID: 1, Quantity: 5, Total: 50.0, Status: "new"}
	if err := db.Upsert(ctx, ord); err != nil {
		t.Fatalf("upsert order insert failed: %v", err)
	}
	ordUp := Order{OrderID: 100, ItemID: 1, Quantity: 10, Total: 100.0, Status: "paid"}
	if err := db.Upsert(ctx, ordUp); err != nil {
		t.Fatalf("upsert order update failed: %v", err)
	}

	ordCheck, err := db.Select[Order]().Where(And(Equal(OrderField.OrderID, 100), Equal(OrderField.ItemID, 1))).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ordCheck) != 1 || ordCheck[0].Quantity != 10 || ordCheck[0].Status != "paid" {
		t.Fatalf("unexpected order after upsert: %+v", ordCheck)
	}
}

func TestRawQueryAndExecute(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	err := db.RawExecute(ctx, "CREATE INDEX idx_user_age ON users(age);", true)
	if err != nil {
		t.Fatalf("RawExecute index creation failed: %v", err)
	}

	users, err := db.RawQuery[User](ctx, "SELECT id, surname, age, active, bio, score FROM users WHERE age > ? ORDER BY age ASC LIMIT 3;", true, 50)
	if err != nil {
		t.Fatalf("RawQuery failed: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("expected 3 users from raw query, got %d", len(users))
	}
	for _, u := range users {
		if u.Age <= 50 {
			t.Fatalf("expected user age > 50, got %d", u.Age)
		}
	}

	prods, err := db.RawQuery[Product](ctx, "SELECT * FROM products LIMIT 2;", false)
	if err != nil {
		t.Fatalf("RawQuery SELECT * failed: %v", err)
	}
	if len(prods) != 2 || prods[0].SKU == "" {
		t.Fatalf("unexpected prods: %+v", prods)
	}
}

func TestTransactions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	tx, err := db.Transaction(ctx)
	if err != nil {
		t.Fatalf("failed to start transaction: %v", err)
	}

	u := User{ID: 301, Name: "TxUser", Age: 35}
	if err := tx.Insert(u); err != nil {
		t.Fatalf("tx insert failed: %v", err)
	}

	txUsers, err := tx.Select[User]().Where(Equal(UserField.ID, 301)).Run(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(txUsers) != 1 {
		t.Fatalf("expected 1 user inside tx, got %d", len(txUsers))
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	dbUsers, err := db.Select[User]().Where(Equal(UserField.ID, 301)).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(dbUsers) != 1 {
		t.Fatalf("expected committed user in db, got %d", len(dbUsers))
	}

	tx2, err := db.Transaction(ctx)
	if err != nil {
		t.Fatal(err)
	}

	uRollback := User{ID: 302, Name: "RollbackUser", Age: 35}
	if err := tx2.Insert(uRollback); err != nil {
		t.Fatal(err)
	}

	if err := tx2.Rollback(); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	afterRollback, err := db.Select[User]().Where(Equal(UserField.ID, 302)).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterRollback) != 0 {
		t.Fatalf("expected 0 users after rollback, got %d", len(afterRollback))
	}

	if err := tx2.Rollback(); err != nil {
		t.Fatalf("second rollback should be nil: %v", err)
	}

	if err := tx2.Insert(u); err == nil {
		t.Fatalf("expected error on closed tx")
	}

	tx3, err := db.Transaction(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx3.Rollback()

	if err := tx3.Upsert(User{ID: 303, Name: "TxUpsert", Age: 20}); err != nil {
		t.Fatal(err)
	}
	if err := tx3.UpdateRow(User{ID: 303, Name: "TxUpdatedRow", Age: 22}); err != nil {
		t.Fatal(err)
	}
	if err := tx3.Update[User]().Set(UserField.Name, "TxUpdatedBuilder").Where(Equal(UserField.ID, 303)).Run(true); err != nil {
		t.Fatal(err)
	}
	rawTx, err := tx3.RawQuery[User]("SELECT * FROM users WHERE id = ?", true, 303)
	if err != nil || len(rawTx) != 1 || rawTx[0].Name != "TxUpdatedBuilder" {
		t.Fatalf("unexpected raw query in tx: %+v", rawTx)
	}
	if err := tx3.RawExecute("UPDATE users SET age = 99 WHERE id = 303", true); err != nil {
		t.Fatal(err)
	}
	if err := tx3.Delete[User]().Where(Equal(UserField.ID, 303)).Run(true); err != nil {
		t.Fatal(err)
	}
}

func TestHooksValidationAndAbort(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	onInsert := func(u *User) error {
		if u.Age < 18 {
			return fmt.Errorf("user must be at least 18 years old")
		}
		return nil
	}
	onUpdate := func(u *User) error {
		if u != nil && u.Score < 0 {
			return fmt.Errorf("score cannot be negative")
		}
		if u == nil {
			return fmt.Errorf("builder update not allowed")
		}
		return nil
	}
	onDelete := func(u *User) error {
		return fmt.Errorf("deletions are disabled")
	}

	db.Table("users", onInsert, onUpdate, onDelete)
	ctx := context.Background()

	_ = db.RawExecute(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, surname TEXT, age INTEGER, active INTEGER, bio TEXT, score REAL);", false)

	underage := User{ID: 1, Name: "Kid", Age: 16}
	if err := db.Insert(ctx, underage); err == nil {
		t.Fatalf("expected onInsert validation error")
	}

	adult := User{ID: 1, Name: "Adult", Age: 21, Score: 10}
	if err := db.Insert(ctx, adult); err != nil {
		t.Fatalf("insert adult failed: %v", err)
	}

	badUpdate := User{ID: 1, Name: "Adult", Age: 21, Score: -5}
	if err := db.UpdateRow(ctx, badUpdate); err == nil {
		t.Fatalf("expected onUpdate validation error")
	}

	if err := db.Update[User]().Set(UserField.Name, "New").Where(Equal(UserField.ID, 1)).Run(ctx, true); err == nil {
		t.Fatalf("expected onUpdate nil error")
	}

	if err := db.Delete[User]().Where(Equal(UserField.ID, 1)).Run(ctx, true); err == nil {
		t.Fatalf("expected onDelete error")
	}
}

func TestCustomTypesAndRunes(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Table[CustomModel]("customs", nil, nil, nil)
	db.Table[RuneModel]("runes", nil, nil, nil)
	ctx := context.Background()

	_ = db.RawExecute(ctx, "CREATE TABLE customs (id INTEGER PRIMARY KEY, code TEXT);", false)
	_ = db.RawExecute(ctx, "CREATE TABLE runes (id INTEGER PRIMARY KEY, chars TEXT);", false)

	c1 := CustomModel{ID: CustomID(123456789), Code: "ALPHA"}
	if err := db.Insert(ctx, c1); err != nil {
		t.Fatalf("custom model insert error: %v", err)
	}

	cGet, err := db.Select[CustomModel]().Where(Equal(NewField[CustomModel, CustomID]("id"), CustomID(123456789))).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(cGet) != 1 || cGet[0].ID != CustomID(123456789) || cGet[0].Code != "ALPHA" {
		t.Fatalf("unexpected custom model: %+v", cGet)
	}

	r1 := RuneModel{ID: 1, Chars: []rune("Hello 世界 🚀")}
	if err := db.Insert(ctx, r1); err != nil {
		t.Fatalf("rune model insert error: %v", err)
	}

	rGet, err := db.Select[RuneModel]().Where(Equal(NewField[RuneModel, int]("id"), 1)).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rGet) != 1 || string(rGet[0].Chars) != "Hello 世界 🚀" {
		t.Fatalf("unexpected rune model: %s", string(rGet[0].Chars))
	}
}

func TestLumoErrorMetadata(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	type Unregistered struct{ ID int }
	_, err = db.Select[Unregistered]().Run(context.Background(), true)
	if err == nil {
		t.Fatalf("expected error for unregistered table")
	}

	le, ok := err.(*lumo.LumoError)
	if !ok {
		t.Fatalf("expected *lumo.LumoError, got %T", err)
	}
	if !strings.Contains(le.Error(), "not registered") {
		t.Fatalf("unexpected error message: %s", le.Error())
	}

	var buf strings.Builder
	lumo.ChangeOutput(&buf)
	lumo.Error("error details: %v", le)
	lumo.Close()
	lumo.ChangeOutput(os.Stdout)

	logged := buf.String()
	if !strings.Contains(logged, "dema_table") {
		t.Errorf("expected dema_table in lumo log, got: %s", logged)
	}
	if !strings.Contains(logged, "dema_operation") {
		t.Errorf("expected dema_operation in lumo log, got: %s", logged)
	}
	if !strings.Contains(logged, "dema_expected") {
		t.Errorf("expected dema_expected in lumo log, got: %s", logged)
	}
}

func TestContextCancellation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	time.Sleep(5 * time.Millisecond)

	_, err := db.Select[User]().Run(ctx, true)
	if err == nil {
		t.Fatalf("expected error on cancelled context")
	}
}

func TestConditionSQLHelper(t *testing.T) {
	sql, args := ConditionSQL(Equal(UserField.ID, 127))
	if sql != "id = ?" || len(args) != 1 || args[0] != uint32(127) {
		t.Fatalf("unexpected sql: %s, args: %+v", sql, args)
	}

	sqlNil, argsNil := ConditionSQL(nil)
	if sqlNil != "" || len(argsNil) != 0 {
		t.Fatalf("expected empty for nil condition")
	}
}

func TestTxQueryBuilders(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	tx, err := db.Transaction(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	res, err := tx.Select[User]().
		OrderBy(UserField.ID, Desc).
		Limit(5).
		Page(1, 5).
		Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 5 {
		t.Fatalf("expected 5 users, got %d", len(res))
	}

	u := User{ID: 1, Name: "TxRowUpdated", Age: 50}
	if err := tx.Update(u).Run(false); err != nil {
		t.Fatalf("tx update builder Row failed: %v", err)
	}
}

func TestConditionEdgeCases(t *testing.T) {
	c1 := Equal(UserField.ID, 1)
	c2 := Greater(UserField.ID, 2)
	c3 := Lesser(UserField.ID, 3)
	c4 := Within(UserField.ID, 4)
	c5 := NotWithin(UserField.ID, 5)
	c6 := Range(UserField.ID, 6, 7)
	c7 := OutsideRange(UserField.ID, 8, 9)

	c1.isCondition()
	c2.isCondition()
	c3.isCondition()
	c4.isCondition()
	c5.isCondition()
	c6.isCondition()
	c7.isCondition()

	if Or(nil, nil) != nil {
		t.Fatalf("Or(nil, nil) should be nil")
	}
	if Or(c1, nil) != c1 {
		t.Fatalf("Or(c1, nil) should be c1")
	}
	if Or(nil, c1) != c1 {
		t.Fatalf("Or(nil, c1) should be c1")
	}
	cOr := Or(c1, c2)
	cOr.isCondition()

	if And(nil, nil) != nil {
		t.Fatalf("And(nil, nil) should be nil")
	}
	if And(c1, nil) != c1 {
		t.Fatalf("And(c1, nil) should be c1")
	}
	if And(nil, c1) != c1 {
		t.Fatalf("And(nil, c1) should be c1")
	}
	cAnd := And(c1, c2)
	cAnd.isCondition()
}

func TestInsertDefaultValues(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	type AllOmit struct {
		ID int `db:"id,pk,omitzero"`
	}

	db.Table[AllOmit]("all_omit", nil, nil, nil)
	ctx := context.Background()

	_ = db.RawExecute(ctx, "CREATE TABLE all_omit (id INTEGER PRIMARY KEY AUTOINCREMENT);", false)

	if err := db.Insert(ctx, AllOmit{ID: 0}); err != nil {
		t.Fatalf("insert default values failed: %v", err)
	}

	rows, err := db.Select[AllOmit]().Run(ctx, false)
	if err != nil || len(rows) != 1 || rows[0].ID != 1 {
		t.Fatalf("unexpected rows after default values insert: %+v, err: %v", rows, err)
	}
}

func TestUpsertAndPKEdgeCases(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	type NoPK struct {
		Name string `db:"name"`
	}
	db.Table[NoPK]("no_pk", nil, nil, nil)
	_ = db.RawExecute(ctx, "CREATE TABLE no_pk (name TEXT);", false)

	if err := db.Upsert(ctx, NoPK{Name: "test"}); err == nil {
		t.Fatalf("expected error upserting into table without PK")
	}

	if err := db.UpdateRow(ctx, NoPK{Name: "test"}); err == nil {
		t.Fatalf("expected error updating table without PK")
	}

	db.Table[PKOnlyModel]("pk_only", nil, nil, nil)
	_ = db.RawExecute(ctx, "CREATE TABLE pk_only (id1 INTEGER, id2 INTEGER, PRIMARY KEY (id1, id2));", false)

	pkItem := PKOnlyModel{ID1: 1, ID2: 2}
	if err := db.Upsert(ctx, pkItem); err != nil {
		t.Fatalf("first upsert into pk_only failed: %v", err)
	}
	if err := db.Upsert(ctx, pkItem); err != nil {
		t.Fatalf("conflict upsert into pk_only failed: %v", err)
	}

	type OmitUpdateModel struct {
		Extra string `db:"extra,omitzero"`
		ID    int    `db:"id,pk"`
	}
	db.Table[OmitUpdateModel]("omit_update", nil, nil, nil)
	_ = db.RawExecute(ctx, "CREATE TABLE omit_update (id INTEGER PRIMARY KEY, extra TEXT);", false)

	if err := db.UpdateRow(ctx, OmitUpdateModel{ID: 1, Extra: ""}); err != nil {
		t.Fatalf("update with all non-PK fields omitted should succeed: %v", err)
	}
}

func TestErrorBranchesAndUnregistered(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	type NotRegistered struct {
		ID int `db:"id,pk"`
	}

	if _, err := db.Select[NotRegistered]().Run(ctx, false); err == nil {
		t.Fatalf("expected error on select unregistered")
	}
	if err := db.Insert(ctx, NotRegistered{ID: 1}); err == nil {
		t.Fatalf("expected error on insert unregistered")
	}
	if err := db.Upsert(ctx, NotRegistered{ID: 1}); err == nil {
		t.Fatalf("expected error on upsert unregistered")
	}
	if err := db.UpdateRow(ctx, NotRegistered{ID: 1}); err == nil {
		t.Fatalf("expected error on update unregistered")
	}
	if err := db.Update[NotRegistered]().Set(NewField[NotRegistered, int]("id"), 1).Where(Equal(NewField[NotRegistered, int]("id"), 1)).Run(ctx, false); err == nil {
		t.Fatalf("expected error on update builder unregistered")
	}
	if err := db.Delete[NotRegistered]().Where(Equal(NewField[NotRegistered, int]("id"), 1)).Run(ctx, false); err == nil {
		t.Fatalf("expected error on delete unregistered")
	}
	if _, err := db.RawQuery[NotRegistered](ctx, "SELECT 1;", false); err == nil {
		t.Fatalf("expected error on raw query unregistered")
	}

	b := db.Select[NotRegistered]().Limit(-5)
	if b.limit != 0 {
		t.Fatalf("expected limit to be clamped to 0")
	}

	if err := db.RawExecute(ctx, "INVALID SQL SYNTAX HERE;", false); err == nil {
		t.Fatalf("expected error on invalid RawExecute")
	}

	type PtrModel struct {
		ID int `db:"id,pk"`
	}
	db.Table[*PtrModel]("ptr_model", nil, nil, nil)

	tx, err := db.Transaction(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err == nil {
		t.Fatalf("expected error on double commit")
	}

	if _, err := tx.Select[PtrModel]().Run(false); err == nil {
		t.Fatalf("expected error on closed tx Select")
	}
	if err := tx.Insert(PtrModel{ID: 1}); err == nil {
		t.Fatalf("expected error on closed tx Insert")
	}
	if err := tx.Upsert(PtrModel{ID: 1}); err == nil {
		t.Fatalf("expected error on closed tx Upsert")
	}
	if err := tx.UpdateRow(PtrModel{ID: 1}); err == nil {
		t.Fatalf("expected error on closed tx UpdateRow")
	}
	if err := tx.Update[PtrModel]().Set(NewField[PtrModel, int]("id"), 1).Where(Equal(NewField[PtrModel, int]("id"), 1)).Run(false); err == nil {
		t.Fatalf("expected error on closed tx Update builder")
	}
	if err := tx.Delete[PtrModel]().Where(Equal(NewField[PtrModel, int]("id"), 1)).Run(false); err == nil {
		t.Fatalf("expected error on closed tx Delete")
	}
	if _, err := tx.RawQuery[PtrModel]("SELECT 1;", false); err == nil {
		t.Fatalf("expected error on closed tx RawQuery")
	}
	if err := tx.RawExecute("SELECT 1;", false); err == nil {
		t.Fatalf("expected error on closed tx RawExecute")
	}
}

type ErrEncoderType int

func (e ErrEncoderType) EncodeDema() (any, error) {
	return nil, fmt.Errorf("forced encode error")
}

type CustomDecoderType struct {
	TextVal  string
	BlobVal  []byte
	FloatVal float64
}

func (c *CustomDecoderType) DecodeDema(v any) error {
	switch val := v.(type) {
	case string:
		c.TextVal = val
	case float64:
		c.FloatVal = val
	case []byte:
		c.BlobVal = val
	}
	return nil
}

func TestCoverageBoost(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	type ErrModel struct {
		ID  int            `db:"id,pk"`
		Val ErrEncoderType `db:"val"`
	}
	db.Table[ErrModel]("err_model", nil, nil, nil)
	_ = db.RawExecute(ctx, "CREATE TABLE err_model (id INTEGER PRIMARY KEY, val INTEGER);", false)

	if err := db.Insert(ctx, ErrModel{ID: 1, Val: 10}); err == nil {
		t.Fatalf("expected error inserting with ErrEncoder")
	}
	if err := db.Upsert(ctx, ErrModel{ID: 1, Val: 10}); err == nil {
		t.Fatalf("expected error upserting with ErrEncoder")
	}
	if err := db.UpdateRow(ctx, ErrModel{ID: 1, Val: 10}); err == nil {
		t.Fatalf("expected error updating row with ErrEncoder")
	}

	type DecModel struct {
		DecF CustomDecoderType `db:"decf"`
		DecT CustomDecoderType `db:"dect"`
		DecB CustomDecoderType `db:"decb"`
		ID   int               `db:"id,pk"`
	}
	db.Table[DecModel]("dec_model", nil, nil, nil)
	_ = db.RawExecute(ctx, "CREATE TABLE dec_model (id INTEGER PRIMARY KEY, decf REAL, dect TEXT, decb BLOB);", false)
	_ = db.RawExecute(ctx, "INSERT INTO dec_model VALUES (1, 3.14, 'hello', X'DEADBEEF');", false)

	decs, err := db.Select[DecModel]().Run(ctx, false)
	if err != nil || len(decs) != 1 {
		t.Fatalf("failed to select DecModel: %v", err)
	}
	if decs[0].DecF.FloatVal != 3.14 || decs[0].DecT.TextVal != "hello" || len(decs[0].DecB.BlobVal) == 0 {
		t.Fatalf("unexpected DecModel values: %+v", decs[0])
	}

	type SimpleUser struct {
		ID int `db:"id,pk"`
	}
	db.Table[SimpleUser]("simple_user", nil, nil, nil)
	_ = db.RawExecute(ctx, "CREATE TABLE simple_user (id INTEGER PRIMARY KEY, extra TEXT);", false)
	_ = db.RawExecute(ctx, "INSERT INTO simple_user VALUES (1, 'ignored');", false)

	simples, err := db.RawQuery[SimpleUser](ctx, "SELECT extra, id, 123 AS other FROM simple_user;", false)
	if err != nil || len(simples) != 1 || simples[0].ID != 1 {
		t.Fatalf("failed RawQuery with extra columns: %v", err)
	}

	var sb strings.Builder
	var args []any
	bcNil := binaryCond{op: "AND", a: nil, b: nil}
	bcNil.toSQL(&sb, &args)

	bcLeftNil := binaryCond{op: "AND", a: nil, b: Equal(UserField.ID, 1)}
	bcLeftNil.toSQL(&sb, &args)

	bcRightNil := binaryCond{op: "AND", a: Equal(UserField.ID, 1), b: nil}
	bcRightNil.toSQL(&sb, &args)

	cancCtx, cancel := context.WithCancel(context.Background())
	tx, err := db.Transaction(cancCtx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	cancel()
	if err := tx.checkActive(); err == nil {
		t.Fatalf("expected error on cancelled tx context")
	}

	conds := []Condition{
		Equal(UserField.ID, 1),
		Greater(UserField.ID, 1),
		Lesser(UserField.ID, 1),
		Within(UserField.ID, 1),
		NotWithin(UserField.ID, 1),
		Range(UserField.ID, 1, 2),
		OutsideRange(UserField.ID, 1, 2),
		Or(Equal(UserField.ID, 1), Equal(UserField.ID, 2)),
		And(Equal(UserField.ID, 1), Equal(UserField.ID, 2)),
	}
	for _, c := range conds {
		c.isCondition()
	}
}

func TestExampleMDSnippet(t *testing.T) {
	type ExampleUser struct {
		Name string `db:"surname"`
		ID   uint32 `db:"id,pk,omitzero"`
	}

	var ExampleUserField = struct {
		ID   Field[ExampleUser, uint32]
		Name Field[ExampleUser, string]
	}{
		ID:   Field[ExampleUser, uint32]{Name: "id"},
		Name: Field[ExampleUser, string]{Name: "surname"},
	}

	onUpdate := func(u *ExampleUser) error {
		if u == nil {
			return fmt.Errorf("user was already deleted")
		}
		return nil
	}

	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	_ = db.RawExecute(ctx, "CREATE TABLE users_example (id INTEGER PRIMARY KEY AUTOINCREMENT, surname TEXT);", false)

	db.Table("users_example", nil, onUpdate, nil)

	u1 := ExampleUser{ID: 127, Name: "Amatsagu"}
	u2 := ExampleUser{Name: "Betta"}
	u3 := ExampleUser{Name: "Kiri"}
	err = db.Insert(u1, u2, u3)
	if err != nil {
		t.Fatal(err)
	}

	data, err := db.Select[ExampleUser]().Where(Equal(ExampleUserField.ID, 127)).Limit(1).Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("no user with that ID")
	}

	uNew := data[0]
	if uNew.ID != 127 || uNew.Name != "Amatsagu" {
		t.Fatalf("unexpected user: %+v", uNew)
	}

	data2, err := db.Select[ExampleUser]().Where(Equal(ExampleUserField.Name, "Betta")).Run(context.Background(), true)
	if err != nil || len(data2) == 0 {
		t.Fatalf("failed to fetch Betta: %v", err)
	}
	if data2[0].ID == 0 {
		t.Fatalf("expected autoincrement ID for Betta, got 0")
	}
}

func TestPKWithoutOmitZeroAndMissingPK(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	type UserNoOmit struct {
		Name string `db:"name"`
		ID   int    `db:"id,pk"`
	}
	_ = db.RawExecute(ctx, "CREATE TABLE users_no_omit (id INTEGER PRIMARY KEY, name TEXT);", false)
	db.Table[UserNoOmit]("users_no_omit", nil, nil, nil)

	if err := db.Insert(UserNoOmit{ID: 0, Name: "ZeroOne"}); err != nil {
		t.Fatalf("inserting ID 0 should succeed: %v", err)
	}

	rows, err := db.Select[UserNoOmit]().Run(ctx, true)
	if err != nil || len(rows) != 1 || rows[0].ID != 0 {
		t.Fatalf("expected row with ID 0, got %+v", rows)
	}

	if err := db.Insert(UserNoOmit{ID: 0, Name: "ZeroTwo"}); err == nil {
		t.Fatalf("expected UNIQUE constraint error when inserting second ID=0 without omitzero")
	}

	type UserWithOmit struct {
		Name string `db:"name"`
		ID   int    `db:"id,pk,omitzero"`
	}
	_ = db.RawExecute(ctx, "CREATE TABLE users_with_omit (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT);", false)
	db.Table[UserWithOmit]("users_with_omit", nil, nil, nil)

	if err := db.Insert(UserWithOmit{ID: 0, Name: "One"}); err != nil {
		t.Fatalf("inserting with omitzero should succeed: %v", err)
	}
	if err := db.Insert(UserWithOmit{ID: 0, Name: "Two"}); err != nil {
		t.Fatalf("inserting second with omitzero should succeed: %v", err)
	}

	rowsOmit, err := db.Select[UserWithOmit]().Run(ctx, true)
	if err != nil || len(rowsOmit) != 2 || rowsOmit[0].ID == 0 || rowsOmit[1].ID == 0 {
		t.Fatalf("expected autoincremented IDs, got %+v", rowsOmit)
	}

	type ItemNoPK struct {
		Name string `db:"name"`
	}
	_ = db.RawExecute(ctx, "CREATE TABLE items_no_pk (name TEXT);", false)
	db.Table[ItemNoPK]("items_no_pk", nil, nil, nil)

	if err := db.UpdateRow(ItemNoPK{Name: "abc"}); err == nil {
		t.Fatalf("expected UpdateRow to fail early on model without PK")
	} else if le, ok := err.(*lumo.LumoError); !ok || !strings.Contains(le.Error(), "primary key") {
		t.Fatalf("expected lumo error mentioning primary key, got %v", err)
	}

	if err := db.Upsert(ItemNoPK{Name: "abc"}); err == nil {
		t.Fatalf("expected Upsert to fail early on model without PK")
	} else if le, ok := err.(*lumo.LumoError); !ok || !strings.Contains(le.Error(), "primary key") {
		t.Fatalf("expected lumo error mentioning primary key, got %v", err)
	}

	tx, err := db.Transaction(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	if err := tx.UpdateRow(ItemNoPK{Name: "abc"}); err == nil {
		t.Fatalf("expected tx.UpdateRow to fail on model without PK")
	}
	if err := tx.Upsert(ItemNoPK{Name: "abc"}); err == nil {
		t.Fatalf("expected tx.Upsert to fail on model without PK")
	}
}

func TestTxAndBuilderExtendedCoverage(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	if UserField.ID.ColumnName() != "id" {
		t.Fatalf("unexpected ColumnName: %s", UserField.ID.ColumnName())
	}
	if UserField.ID.String() != "id" {
		t.Fatalf("unexpected String: %s", UserField.ID.String())
	}

	if err := Update(User{ID: 1, Name: "test"}); err == nil {
		t.Fatalf("expected error when no DB passed to Update")
	}
	if err := Update(db, User{ID: 1, Name: "AdminUpdated"}); err != nil {
		t.Fatalf("package level Update failed: %v", err)
	}

	ub := db.Update(User{ID: 1, Name: "AdminRowUpdated"})
	if err := ub.Run(ctx, true); err != nil {
		t.Fatalf("UpdateBuilder.Row failed: %v", err)
	}

	tx, err := db.Transaction(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := tx.Insert(); err != nil {
		t.Fatalf("empty tx.Insert should be no-op: %v", err)
	}
	if err := tx.Upsert(); err != nil {
		t.Fatalf("empty tx.Upsert should be no-op: %v", err)
	}
	if err := tx.UpdateRow(); err == nil {
		t.Fatalf("expected error on empty tx.UpdateRow")
	}

	if err := tx.Delete[User]().Run(true); err == nil {
		t.Fatalf("expected error on tx.Delete without WHERE")
	}

	txUb := tx.Update(User{ID: 1, Name: "TxRowUpdated"})
	if err := txUb.Run(true); err != nil {
		t.Fatalf("tx.Update with row failed: %v", err)
	}

	if err := tx.RawExecute("CREATE TEMP TABLE temp_tx (val TEXT);", false); err != nil {
		t.Fatalf("tx.RawExecute failed: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("tx.Commit failed: %v", err)
	}

	if err := tx.Commit(); err == nil {
		t.Fatalf("expected error committing closed tx")
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("expected safe rollback on closed tx, got: %v", err)
	}
}

type customDecoderVal struct {
	v string
}

func (c *customDecoderVal) DecodeDema(raw any) error {
	if s, ok := raw.(string); ok {
		c.v = "decoded:" + s
	}
	return nil
}

type customNamedID uint32
type customNamedName string

func TestRawQueryPrimitives(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Setup table for test
	err := db.RawExecute(ctx, `CREATE TABLE colors (
		id INTEGER PRIMARY KEY,
		name TEXT,
		code TEXT,
		score REAL,
		is_warm BOOLEAN,
		extra BLOB,
		nullable_val INTEGER
	);`, false)
	if err != nil {
		t.Fatalf("failed to create colors table: %v", err)
	}
	err = db.RawExecute(ctx, `INSERT INTO colors (id, name, code, score, is_warm, extra, nullable_val) VALUES (1, 'red', 'RED01', 9.5, 1, X'CAFE', NULL);`, false)
	if err != nil {
		t.Fatalf("failed to insert color 1: %v", err)
	}
	err = db.RawExecute(ctx, `INSERT INTO colors (id, name, code, score, is_warm, extra, nullable_val) VALUES (2, 'blue', 'BLU02', 8.2, 0, X'BEEF', 42);`, false)
	if err != nil {
		t.Fatalf("failed to insert color 2: %v", err)
	}

	// 1. Exact user example: RawQuery[uint32]
	counts, err := db.RawQuery[uint32](ctx, "SELECT COUNT(*) FROM colors;", false)
	if err != nil {
		t.Fatalf("RawQuery[uint32] failed: %v", err)
	}
	if len(counts) != 1 || counts[0] != 2 {
		t.Fatalf("expected [2], got %+v", counts)
	}

	// 2. All integer types
	u64s, err := db.RawQuery[uint64](ctx, "SELECT id FROM colors ORDER BY id;", false)
	if err != nil || len(u64s) != 2 || u64s[0] != 1 || u64s[1] != 2 {
		t.Fatalf("uint64 failed: %v, %+v", err, u64s)
	}
	i32s, err := db.RawQuery[int32](ctx, "SELECT id FROM colors ORDER BY id;", false)
	if err != nil || len(i32s) != 2 || i32s[0] != 1 || i32s[1] != 2 {
		t.Fatalf("int32 failed: %v, %+v", err, i32s)
	}
	i64s, err := db.RawQuery[int64](ctx, "SELECT id FROM colors ORDER BY id;", false)
	if err != nil || len(i64s) != 2 || i64s[0] != 1 || i64s[1] != 2 {
		t.Fatalf("int64 failed: %v, %+v", err, i64s)
	}
	ints, err := db.RawQuery[int](ctx, "SELECT id FROM colors ORDER BY id;", false)
	if err != nil || len(ints) != 2 || ints[0] != 1 || ints[1] != 2 {
		t.Fatalf("int failed: %v, %+v", err, ints)
	}

	// 3. String & []rune
	names, err := db.RawQuery[string](ctx, "SELECT name FROM colors ORDER BY id;", false)
	if err != nil || len(names) != 2 || names[0] != "red" || names[1] != "blue" {
		t.Fatalf("string failed: %v, %+v", err, names)
	}
	runes, err := db.RawQuery[[]rune](ctx, "SELECT code FROM colors WHERE id = 1;", false)
	if err != nil || len(runes) != 1 || string(runes[0]) != "RED01" {
		t.Fatalf("[]rune failed: %v, %+v", err, runes)
	}

	// 4. Floats & bools & bytes
	scores, err := db.RawQuery[float64](ctx, "SELECT score FROM colors WHERE id = 1;", false)
	if err != nil || len(scores) != 1 || scores[0] != 9.5 {
		t.Fatalf("float64 failed: %v, %+v", err, scores)
	}
	f32s, err := db.RawQuery[float32](ctx, "SELECT score FROM colors WHERE id = 1;", false)
	if err != nil || len(f32s) != 1 || f32s[0] != 9.5 {
		t.Fatalf("float32 failed: %v, %+v", err, f32s)
	}
	bools, err := db.RawQuery[bool](ctx, "SELECT is_warm FROM colors ORDER BY id;", false)
	if err != nil || len(bools) != 2 || !bools[0] || bools[1] {
		t.Fatalf("bool failed: %v, %+v", err, bools)
	}
	blobs, err := db.RawQuery[[]byte](ctx, "SELECT extra FROM colors WHERE id = 1;", false)
	if err != nil || len(blobs) != 1 || len(blobs[0]) != 2 || blobs[0][0] != 0xCA || blobs[0][1] != 0xFE {
		t.Fatalf("[]byte failed: %v, %+v", err, blobs)
	}

	// 5. Pointers (null and non-null)
	nullPtrs, err := db.RawQuery[*uint32](ctx, "SELECT nullable_val FROM colors ORDER BY id;", false)
	if err != nil || len(nullPtrs) != 2 {
		t.Fatalf("*uint32 failed: %v, %+v", err, nullPtrs)
	}
	if nullPtrs[0] != nil {
		t.Fatalf("expected nil for null column, got %v", *nullPtrs[0])
	}
	if nullPtrs[1] == nil || *nullPtrs[1] != 42 {
		t.Fatalf("expected 42 for non-null column, got %v", nullPtrs[1])
	}

	strPtrs, err := db.RawQuery[*string](ctx, "SELECT name FROM colors ORDER BY id;", false)
	if err != nil || len(strPtrs) != 2 || strPtrs[0] == nil || *strPtrs[0] != "red" {
		t.Fatalf("*string failed: %v, %+v", err, strPtrs)
	}

	// 6. Custom named types
	namedIDs, err := db.RawQuery[customNamedID](ctx, "SELECT id FROM colors WHERE id = 1;", false)
	if err != nil || len(namedIDs) != 1 || namedIDs[0] != 1 {
		t.Fatalf("customNamedID failed: %v, %+v", err, namedIDs)
	}
	namedNames, err := db.RawQuery[customNamedName](ctx, "SELECT name FROM colors WHERE id = 1;", false)
	if err != nil || len(namedNames) != 1 || namedNames[0] != "red" {
		t.Fatalf("customNamedName failed: %v, %+v", err, namedNames)
	}

	// 7. Custom FieldDecoder
	decValues, err := db.RawQuery[customDecoderVal](ctx, "SELECT name FROM colors WHERE id = 1;", false)
	if err != nil || len(decValues) != 1 || decValues[0].v != "decoded:red" {
		t.Fatalf("customDecoderVal failed: %v, %+v", err, decValues)
	}

	// 8. In transaction
	tx, err := db.Transaction(ctx)
	if err != nil {
		t.Fatalf("Transaction failed: %v", err)
	}
	defer tx.Rollback()

	txCounts, err := tx.RawQuery[uint32]("SELECT COUNT(*) FROM colors;", false)
	if err != nil || len(txCounts) != 1 || txCounts[0] != 2 {
		t.Fatalf("tx.RawQuery[uint32] failed: %v, %+v", err, txCounts)
	}
	txNames, err := tx.RawQuery[string]("SELECT name FROM colors WHERE id = ?;", false, 2)
	if err != nil || len(txNames) != 1 || txNames[0] != "blue" {
		t.Fatalf("tx.RawQuery[string] with arg failed: %v, %+v", err, txNames)
	}

	// 9. Passing pointer args (both nil and non-nil)
	var nilPtr *int
	nonNilInt := 2
	nonNilPtr := &nonNilInt
	resPtrArg, err := db.RawQuery[string](ctx, "SELECT name FROM colors WHERE id = ?;", false, nonNilPtr)
	if err != nil || len(resPtrArg) != 1 || resPtrArg[0] != "blue" {
		t.Fatalf("RawQuery with pointer arg failed: %v, %+v", err, resPtrArg)
	}
	resNilArg, err := db.RawQuery[uint32](ctx, "SELECT COUNT(*) FROM colors WHERE nullable_val IS ?;", false, nilPtr)
	if err != nil || len(resNilArg) != 1 || resNilArg[0] != 1 {
		t.Fatalf("RawQuery with nil pointer arg failed: %v, %+v", err, resNilArg)
	}
}

func TestOnConnectionHook(t *testing.T) {
	ctx := context.Background()

	// 1. Direct func(*sqlite.Conn) error
	var called1 int
	hook1 := func(conn *sqlite.Conn) error {
		called1++
		return sqlitex.ExecuteTransient(conn, "PRAGMA application_id = 12345;", nil)
	}

	db1, err := Open(":memory:", 1, hook1)
	if err != nil {
		t.Fatalf("Open with hook1 failed: %v", err)
	}
	defer db1.Close()

	appID, err := db1.RawQuery[int](ctx, "PRAGMA application_id;", false)
	if err != nil || len(appID) != 1 || appID[0] != 12345 {
		t.Fatalf("expected application_id 12345, got %v (%+v)", err, appID)
	}
	if called1 == 0 {
		t.Fatalf("expected hook1 to be called, got %d", called1)
	}

	// 2. dema.OnConnection wrapper
	var called2 int
	hook2 := OnConnection(func(conn *sqlite.Conn) error {
		called2++
		return sqlitex.ExecuteTransient(conn, "PRAGMA user_version = 42;", nil)
	})

	db2, err := Open(":memory:", 1, hook2)
	if err != nil {
		t.Fatalf("Open with OnConnection failed: %v", err)
	}
	defer db2.Close()

	userVer, err := db2.RawQuery[int](ctx, "PRAGMA user_version;", false)
	if err != nil || len(userVer) != 1 || userVer[0] != 42 {
		t.Fatalf("expected user_version 42, got %v (%+v)", err, userVer)
	}
	if called2 == 0 {
		t.Fatalf("expected hook2 to be called, got %d", called2)
	}

	// 3. Chained hooks
	var chainA, chainB bool
	db3, err := Open(":memory:", 1,
		func(conn *sqlite.Conn) error {
			chainA = true
			return nil
		},
		OnConnection(func(conn *sqlite.Conn) error {
			chainB = true
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("Open with chained hooks failed: %v", err)
	}
	defer db3.Close()

	_, err = db3.RawQuery[int](ctx, "SELECT 1;", false)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !chainA || !chainB {
		t.Fatalf("expected both chained hooks to be called, got A=%v, B=%v", chainA, chainB)
	}
}

type Server struct {
	ID    int    `db:"id,pk"`
	Title string `db:"title"`
}

var ServerField = struct {
	ID    Field[Server, int]
	Title Field[Server, string]
}{
	ID:    NewField[Server, int]("id"),
	Title: NewField[Server, string]("title"),
}

func TestCount(t *testing.T) {
	db, err := Open(":memory:", 5)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Table[Server]("servers", nil, nil, nil)
	ctx := context.Background()

	// 1. Verify exact SQL generation as requested by user
	sql1 := db.Count[Server](nil).SQL()
	if sql1 != `SELECT COUNT(*) FROM "servers";` {
		t.Fatalf("expected %q, got %q", `SELECT COUNT(*) FROM "servers";`, sql1)
	}

	sql2 := db.Count[Server](ServerField.Title).Limit(20).SQL()
	if sql2 != `SELECT COUNT("title") FROM "servers" LIMIT 20;` {
		t.Fatalf("expected %q, got %q", `SELECT COUNT("title") FROM "servers" LIMIT 20;`, sql2)
	}

	// Also verify package-level Count
	sqlPkg1 := Count[Server](nil).SQL()
	if sqlPkg1 != `SELECT COUNT(*) FROM "servers";` {
		t.Fatalf("expected %q, got %q", `SELECT COUNT(*) FROM "servers";`, sqlPkg1)
	}

	sqlPkg2 := Count[Server](ServerField.Title).Limit(20).SQL()
	if sqlPkg2 != `SELECT COUNT("title") FROM "servers" LIMIT 20;` {
		t.Fatalf("expected %q, got %q", `SELECT COUNT("title") FROM "servers" LIMIT 20;`, sqlPkg2)
	}

	// Verify no-args Count()
	sqlNoArgs := db.Count[Server]().SQL()
	if sqlNoArgs != `SELECT COUNT(*) FROM "servers";` {
		t.Fatalf("expected %q, got %q", `SELECT COUNT(*) FROM "servers";`, sqlNoArgs)
	}

	// Verify string column name
	sqlStrCol := db.Count[Server]("title").SQL()
	if sqlStrCol != `SELECT COUNT("title") FROM "servers";` {
		t.Fatalf("expected %q, got %q", `SELECT COUNT("title") FROM "servers";`, sqlStrCol)
	}

	// 2. Setup table in sqlite and insert test rows
	err = db.RawExecute(ctx, `CREATE TABLE servers (id INTEGER PRIMARY KEY, title TEXT);`, false)
	if err != nil {
		t.Fatal(err)
	}

	// Empty table count
	cnt0, err := db.Count[Server](nil).Run(ctx, true)
	if err != nil || cnt0 != 0 {
		t.Fatalf("expected 0, got %v (%d)", err, cnt0)
	}

	// Insert 3 servers
	err = db.Insert(ctx,
		Server{ID: 1, Title: "prod-web-01"},
		Server{ID: 2, Title: "prod-web-02"},
		Server{ID: 3, Title: "staging-db-01"},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Total count
	cntTotal, err := db.Count[Server](nil).Run(ctx, true)
	if err != nil || cntTotal != 3 {
		t.Fatalf("expected 3, got %v (%d)", err, cntTotal)
	}

	// Count by field
	cntTitle, err := db.Count[Server](ServerField.Title).Run(ctx, true)
	if err != nil || cntTitle != 3 {
		t.Fatalf("expected 3, got %v (%d)", err, cntTitle)
	}

	// Count with WHERE
	cntWhere, err := db.Count[Server](nil).
		Where(Equal(ServerField.ID, 2)).
		Run(ctx, true)
	if err != nil || cntWhere != 1 {
		t.Fatalf("expected 1, got %v (%d)", err, cntWhere)
	}

	// Count with LIMIT
	cntLimit, err := db.Count[Server](ServerField.Title).
		Limit(20).
		Run(ctx, true)
	if err != nil || cntLimit != 3 {
		t.Fatalf("expected 3, got %v (%d)", err, cntLimit)
	}

	// Count with Page
	cntPage, err := db.Count[Server](ServerField.Title).
		Page(1, 2).
		Run(ctx, false)
	if err != nil || cntPage != 3 {
		t.Fatalf("expected 3, got %v (%d)", err, cntPage)
	}

	// Count on transaction
	tx, err := db.Transaction(ctx)
	if err != nil {
		t.Fatal(err)
	}

	txCount, err := tx.Count[Server](nil).Run(true)
	if err != nil || txCount != 3 {
		t.Fatalf("expected txCount 3, got %v (%d)", err, txCount)
	}

	txCountFiltered, err := tx.Count[Server](ServerField.Title).
		Where(Equal(ServerField.Title, "prod-web-01")).
		Run(false)
	if err != nil || txCountFiltered != 1 {
		t.Fatalf("expected txCountFiltered 1, got %v (%d)", err, txCountFiltered)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	// Package-level Count with db
	cntPkg, err := Count[Server](db, nil).Run(ctx, true)
	if err != nil || cntPkg != 3 {
		t.Fatalf("expected cntPkg 3, got %v (%d)", err, cntPkg)
	}

	// Error case: unregistered table
	type UnregisteredServer struct {
		ID int
	}
	_, err = db.Count[UnregisteredServer]().Run(ctx, true)
	if err == nil {
		t.Fatalf("expected error on unregistered count")
	}
}



