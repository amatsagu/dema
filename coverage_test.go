package dema

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"zombiezen.com/go/sqlite/sqlitex"
)

type PtrReceiverEnc struct {
	val string
}

func (p *PtrReceiverEnc) EncodeDema() (any, error) {
	if p == nil {
		return nil, nil
	}
	return "ENC:" + p.val, nil
}

type FailEnc struct {
	val string
}

func (f FailEnc) EncodeDema() (any, error) {
	return nil, errors.New("encode error simulated")
}

type FailDec struct {
	val string
}

func (f *FailDec) DecodeDema(v any) error {
	return errors.New("decode error simulated")
}

type ValueDec struct {
	val string
}

func (v ValueDec) DecodeDema(raw any) error {
	return nil
}

type PtrEncModel struct {
	Data PtrReceiverEnc `db:"data"`
	ID   int            `db:"id,pk"`
}

type FailEncModel struct {
	Data FailEnc `db:"data"`
	ID   int     `db:"id,pk"`
}

type FailEncPKModel struct {
	ID   FailEnc `db:"id,pk"`
	Data string  `db:"data"`
}

type FailDecModel struct {
	Data FailDec `db:"data"`
	ID   int     `db:"id,pk"`
}

type ValueDecModel struct {
	Data ValueDec `db:"data"`
	ID   int      `db:"id,pk"`
}

type HookTxModel struct {
	Name string `db:"name"`
	ID   int    `db:"id,pk"`
}

func TestConditionEdgeCoverage(t *testing.T) {
	if v := encodeValue(nil); v != nil {
		t.Fatalf("expected nil from encodeValue(nil), got %v", v)
	}

	runes := []rune("hello")
	if v := encodeValue(runes); v != "hello" {
		t.Fatalf("expected 'hello' from encodeValue([]rune), got %v", v)
	}

	fe := FailEnc{val: "boom"}
	if v := encodeValue(fe); v != fe {
		t.Fatalf("expected original FailEnc on encode failure, got %v", v)
	}
}

func TestOpenAndTableErrorCoverage(t *testing.T) {
	dbURI, err := Open("file:test_file_uri?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("Open with file: prefix failed: %v", err)
	}
	_ = dbURI.Close()

	_, err = Open("/dev/null/cannot/create/db/here/file.db")
	if err == nil {
		t.Fatalf("expected error opening invalid path")
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on Table[any]")
		}
	}()
	db, err := Open(":memory:", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Table[any]("nil_table", nil, nil, nil)
}

func TestNewTableInfoErrors(t *testing.T) {
	_, err := newTableInfo[any]("nil_table", nil, nil, nil)
	if err == nil {
		t.Fatalf("expected error from newTableInfo[any]")
	}

	_, err = newTableInfo[int]("int_table", nil, nil, nil)
	if err == nil {
		t.Fatalf("expected error from newTableInfo[int]")
	}
}

func TestPointerModelsAndCacheFlag(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	usersPtr, err := db.Select[*User]().Run(ctx, false)
	if err != nil || len(usersPtr) == 0 {
		t.Fatalf("Select[*User] failed: %v", err)
	}

	ordered, err := db.Select[User]().
		OrderBy(UserField.Age, Desc).
		OrderBy(UserField.Name, Asc).
		Run(ctx, true)
	if err != nil || len(ordered) == 0 {
		t.Fatalf("multiple OrderBy failed: %v", err)
	}

	err = db.Update[*User]().
		Set(NewField[*User, string]("surname"), "PtrUpdated").
		Where(Equal(NewField[*User, uint32]("id"), 1)).
		Run(ctx, false)
	if err != nil {
		t.Fatalf("Update[*User] with cache=false failed: %v", err)
	}

	err = db.Delete[*User]().
		Where(Equal(NewField[*User, uint32]("id"), 99999)).
		Run(ctx, false)
	if err != nil {
		t.Fatalf("Delete[*User] with cache=false failed: %v", err)
	}

	rawPtr, err := db.RawQuery[*User](ctx, "SELECT id, surname FROM users LIMIT 1;", false)
	if err != nil || len(rawPtr) == 0 {
		t.Fatalf("RawQuery[*User] failed: %v", err)
	}

	rawBgCtx, err := db.RawQuery[User](context.Background(), "SELECT id, surname FROM users LIMIT 1;", true)
	if err != nil || len(rawBgCtx) == 0 {
		t.Fatalf("RawQuery with context.Background() failed: %v", err)
	}

	if err := db.RawExecute(context.Background(), "SELECT 1;", false); err != nil {
		t.Fatalf("RawExecute with context.Background() and cache=false failed: %v", err)
	}

	rawUnknownCol, err := db.RawQuery[User](ctx, "SELECT id, surname, 42 AS non_existent_column FROM users LIMIT 1;", true)
	if err != nil || len(rawUnknownCol) == 0 {
		t.Fatalf("RawQuery with unknown column failed: %v", err)
	}
}

func TestCancelledContextOnPoolOperations(t *testing.T) {
	db, err := Open(":memory:", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	_ = db.RawExecute(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, surname TEXT, age INT, active INT, bio TEXT, score REAL);", false)
	db.Table[User]("users", nil, nil, nil)

	heldConn, err := db.pool.Take(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.pool.Put(heldConn)

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := db.Select[User]().Run(cancelledCtx); err == nil {
		t.Fatalf("expected error on cancelled ctx in Select")
	}
	if err := db.Insert(cancelledCtx, User{ID: 1}); err == nil {
		t.Fatalf("expected error on cancelled ctx in Insert")
	}
	if err := db.Upsert(cancelledCtx, User{ID: 1}); err == nil {
		t.Fatalf("expected error on cancelled ctx in Upsert")
	}
	if err := db.Update[User]().Set(UserField.Name, "X").Where(Equal(UserField.ID, 1)).Run(cancelledCtx); err == nil {
		t.Fatalf("expected error on cancelled ctx in Update.Run")
	}
	if err := db.UpdateRow(cancelledCtx, User{ID: 1, Name: "X"}); err == nil {
		t.Fatalf("expected error on cancelled ctx in UpdateRow")
	}
	if err := db.Delete[User]().Where(Equal(UserField.ID, 1)).Run(cancelledCtx); err == nil {
		t.Fatalf("expected error on cancelled ctx in Delete.Run")
	}
	if _, err := db.RawQuery[User](cancelledCtx, "SELECT 1;", false); err == nil {
		t.Fatalf("expected error on cancelled ctx in RawQuery")
	}
	if err := db.RawExecute(cancelledCtx, "SELECT 1;", false); err == nil {
		t.Fatalf("expected error on cancelled ctx in RawExecute")
	}
	if _, err := db.Transaction(cancelledCtx); err == nil {
		t.Fatalf("expected error on cancelled ctx in Transaction")
	}
}

func TestInsertArgsVariations(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	_ = db.RawExecute(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, surname TEXT, age INT, active INT, bio TEXT, score REAL);", false)
	db.Table[User]("users", nil, nil, nil)

	u1 := User{ID: 10, Name: "Ten"}
	u2 := User{ID: 11, Name: "Eleven"}

	if err := db.Insert([]User{u1}); err != nil {
		t.Fatalf("Insert with []User failed: %v", err)
	}
	if err := db.Insert([]*User{&u2}); err != nil {
		t.Fatalf("Insert with []*User failed: %v", err)
	}

	u3 := User{ID: 12, Name: "Twelve"}
	if err := db.Insert(&u3); err != nil {
		t.Fatalf("Insert with *User failed: %v", err)
	}

	u4 := User{ID: 13, Name: "Thirteen"}
	if err := db.Insert(nil, u4); err != nil {
		t.Fatalf("Insert with nil arg failed: %v", err)
	}

	if err := db.Upsert([]User{{ID: 10, Name: "TenUpdated"}}); err != nil {
		t.Fatalf("Upsert with []User failed: %v", err)
	}
	if err := db.Upsert([]*User{{ID: 11, Name: "ElevenUpdated"}}); err != nil {
		t.Fatalf("Upsert with []*User failed: %v", err)
	}

	u10 := User{ID: 10, Name: "TenRowUpdated"}
	if err := db.UpdateRow(&u10); err != nil {
		t.Fatalf("UpdateRow with pointer failed: %v", err)
	}
}

func TestCustomEncodersAndDecoders(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	_ = db.RawExecute(ctx, "CREATE TABLE ptr_enc (id INTEGER PRIMARY KEY, data TEXT);", false)
	db.Table[PtrEncModel]("ptr_enc", nil, nil, nil)

	pModel := PtrEncModel{ID: 1, Data: PtrReceiverEnc{val: "custom"}}
	if err := db.Insert(pModel); err != nil {
		t.Fatalf("inserting PtrEncModel failed: %v", err)
	}

	_ = db.RawExecute(ctx, "CREATE TABLE fail_enc (id INTEGER PRIMARY KEY, data TEXT);", false)
	db.Table[FailEncModel]("fail_enc", nil, nil, nil)

	fModel := FailEncModel{ID: 1, Data: FailEnc{val: "boom"}}
	if err := db.Insert(fModel); err == nil {
		t.Fatalf("expected Insert to fail with failing encoder")
	}
	if err := db.Upsert(fModel); err == nil {
		t.Fatalf("expected Upsert to fail with failing encoder")
	}
	if err := db.UpdateRow(fModel); err == nil {
		t.Fatalf("expected UpdateRow to fail with failing encoder")
	}

	_ = db.RawExecute(ctx, "CREATE TABLE fail_enc_pk (id TEXT PRIMARY KEY, data TEXT);", false)
	db.Table[FailEncPKModel]("fail_enc_pk", nil, nil, nil)
	fPKModel := FailEncPKModel{ID: FailEnc{val: "pk"}, Data: "hello"}
	if err := db.UpdateRow(fPKModel); err == nil {
		t.Fatalf("expected UpdateRow to fail with failing encoder on PK")
	}

	_ = db.RawExecute(ctx, "CREATE TABLE fail_dec (id INTEGER PRIMARY KEY, data TEXT);", false)
	db.Table[FailDecModel]("fail_dec", nil, nil, nil)
	_ = db.RawExecute(ctx, "INSERT INTO fail_dec (id, data) VALUES (1, 'test');", false)

	if _, err := db.Select[FailDecModel]().Run(ctx); err == nil {
		t.Fatalf("expected Select to fail with failing decoder")
	}
	if _, err := db.RawQuery[FailDecModel](ctx, "SELECT id, data FROM fail_dec;", false); err == nil {
		t.Fatalf("expected RawQuery to fail with failing decoder")
	}

	_ = db.RawExecute(ctx, "CREATE TABLE val_dec (id INTEGER PRIMARY KEY, data TEXT);", false)
	db.Table[ValueDecModel]("val_dec", nil, nil, nil)
	_ = db.RawExecute(ctx, "INSERT INTO val_dec (id, data) VALUES (1, 'val');", false)

	vRows, err := db.Select[ValueDecModel]().Run(ctx)
	if err != nil || len(vRows) != 1 {
		t.Fatalf("Select with ValueDec failed: %v", err)
	}
}

func TestSQLExecutionErrorsOnDroppedTable(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	_ = db.RawExecute(ctx, "CREATE TABLE drop_me (id INTEGER PRIMARY KEY, name TEXT);", false)

	type DropMe struct {
		Name string `db:"name"`
		ID   int    `db:"id,pk"`
	}
	db.Table[DropMe]("drop_me", nil, nil, nil)

	_ = db.RawExecute(ctx, "DROP TABLE drop_me;", false)

	if _, err := db.Select[DropMe]().Run(ctx, false); err == nil {
		t.Fatalf("expected error executing Select on dropped table")
	}
	if err := db.Insert(DropMe{ID: 1, Name: "A"}); err == nil {
		t.Fatalf("expected error executing Insert on dropped table")
	}
	if err := db.Upsert(DropMe{ID: 1, Name: "A"}); err == nil {
		t.Fatalf("expected error executing Upsert on dropped table")
	}
	if err := db.Update[DropMe]().Set(NewField[DropMe, string]("name"), "B").Where(Equal(NewField[DropMe, int]("id"), 1)).Run(ctx, false); err == nil {
		t.Fatalf("expected error executing Update on dropped table")
	}
	if err := db.UpdateRow(DropMe{ID: 1, Name: "B"}); err == nil {
		t.Fatalf("expected error executing UpdateRow on dropped table")
	}
	if err := db.Delete[DropMe]().Where(Equal(NewField[DropMe, int]("id"), 1)).Run(ctx, false); err == nil {
		t.Fatalf("expected error executing Delete on dropped table")
	}
	if _, err := db.RawQuery[DropMe](ctx, "SELECT FROM WHERE MALFORMED;", false); err == nil {
		t.Fatalf("expected error executing malformed RawQuery")
	}
	if err := db.RawExecute(ctx, "SELECT FROM WHERE MALFORMED;", false); err == nil {
		t.Fatalf("expected error executing malformed RawExecute")
	}
}

func TestTransactionComprehensiveBranches(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	_ = db.RawExecute(ctx, "CREATE TABLE hook_tx (id INTEGER PRIMARY KEY, name TEXT);", false)

	onInsert := func(m *HookTxModel) error {
		if m != nil && m.ID < 0 {
			return errors.New("negative ID rejected in insert")
		}
		if m != nil && m.Name == "modify" {
			m.Name = "modified_by_hook"
		}
		return nil
	}
	onUpdate := func(m *HookTxModel) error {
		if m != nil && m.ID < 0 {
			return errors.New("negative ID rejected in update")
		}
		if m != nil && m.Name == "modify" {
			m.Name = "modified_by_hook"
		}
		return nil
	}
	onDelete := func(m *HookTxModel) error {
		return nil
	}

	db.Table("hook_tx", onInsert, onUpdate, onDelete)
	_ = db.RawExecute(ctx, "INSERT INTO hook_tx (id, name) VALUES (1, 'initial');", false)

	tx, err := db.Transaction(context.Background())
	if err != nil {
		t.Fatalf("Transaction with context.Background() failed: %v", err)
	}

	txPtrRows, err := tx.Select[*HookTxModel]().Run(false)
	if err != nil || len(txPtrRows) == 0 || txPtrRows[0] == nil {
		t.Fatalf("tx.Select[*HookTxModel] failed: %v", err)
	}

	if err := tx.RawExecute("SELECT 1;", false); err != nil {
		t.Fatalf("active tx.RawExecute with cache=false failed: %v", err)
	}

	err = tx.Update[*HookTxModel]().
		Set(NewField[*HookTxModel, string]("name"), "PtrTx").
		Where(Equal(NewField[*HookTxModel, int]("id"), 1)).
		Run(false)
	if err != nil {
		t.Fatalf("tx.Update[*HookTxModel] failed: %v", err)
	}

	err = tx.Delete[*HookTxModel]().
		Where(Equal(NewField[*HookTxModel, int]("id"), 999)).
		Run(false)
	if err != nil {
		t.Fatalf("tx.Delete[*HookTxModel] failed: %v", err)
	}

	_, err = tx.RawQuery[*HookTxModel]("SELECT id, name FROM hook_tx;", false)
	if err != nil {
		t.Fatalf("tx.RawQuery[*HookTxModel] failed: %v", err)
	}

	type UnregisteredTx struct{ ID int }
	if _, err := tx.Select[UnregisteredTx]().Run(); err == nil {
		t.Fatalf("expected error on tx.Select unregistered")
	}
	if err := tx.Insert(UnregisteredTx{ID: 1}); err == nil {
		t.Fatalf("expected error on tx.Insert unregistered")
	}
	if err := tx.Upsert(UnregisteredTx{ID: 1}); err == nil {
		t.Fatalf("expected error on tx.Upsert unregistered")
	}
	if err := tx.Update[UnregisteredTx]().Set(NewField[UnregisteredTx, int]("id"), 1).Where(Equal(NewField[UnregisteredTx, int]("id"), 1)).Run(); err == nil {
		t.Fatalf("expected error on tx.Update unregistered")
	}
	if err := tx.UpdateRow(UnregisteredTx{ID: 1}); err == nil {
		t.Fatalf("expected error on tx.UpdateRow unregistered")
	}
	if err := tx.Delete[UnregisteredTx]().Where(Equal(NewField[UnregisteredTx, int]("id"), 1)).Run(); err == nil {
		t.Fatalf("expected error on tx.Delete unregistered")
	}
	if _, err := tx.RawQuery[UnregisteredTx]("SELECT 1;", false); err == nil {
		t.Fatalf("expected error on tx.RawQuery unregistered")
	}

	if err := tx.Insert(HookTxModel{ID: -10, Name: "Bad"}); err == nil {
		t.Fatalf("expected tx.Insert to fail on hook validation")
	}
	if err := tx.Upsert(HookTxModel{ID: -10, Name: "Bad"}); err == nil {
		t.Fatalf("expected tx.Upsert to fail on hook validation")
	}
	if err := tx.UpdateRow(HookTxModel{ID: -10, Name: "Bad"}); err == nil {
		t.Fatalf("expected tx.UpdateRow to fail on hook validation")
	}

	if err := tx.Insert(HookTxModel{ID: 100, Name: "modify"}); err != nil {
		t.Fatalf("tx.Insert modify failed: %v", err)
	}
	if err := tx.Upsert(HookTxModel{ID: 100, Name: "modify"}); err != nil {
		t.Fatalf("tx.Upsert modify failed: %v", err)
	}
	if err := tx.UpdateRow(HookTxModel{ID: 100, Name: "modify"}); err != nil {
		t.Fatalf("tx.UpdateRow modify failed: %v", err)
	}

	txRows, err := tx.Select[HookTxModel]().Where(Equal(NewField[HookTxModel, int]("id"), 100)).Run()
	if err != nil || len(txRows) != 1 || txRows[0].Name != "modified_by_hook" {
		t.Fatalf("expected row name modified_by_hook, got %+v", txRows)
	}

	_ = tx.Commit()

	txCancelCtx, cancel := context.WithCancel(context.Background())
	tx2, err := db.Transaction(txCancelCtx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	if _, err := tx2.Select[HookTxModel]().Run(); err == nil {
		t.Fatalf("expected error on cancelled tx2.Select")
	}
	if err := tx2.Insert(HookTxModel{ID: 1}); err == nil {
		t.Fatalf("expected error on cancelled tx2.Insert")
	}
	if err := tx2.Upsert(HookTxModel{ID: 1}); err == nil {
		t.Fatalf("expected error on cancelled tx2.Upsert")
	}
	if err := tx2.Update[HookTxModel]().Set(NewField[HookTxModel, string]("name"), "x").Where(Equal(NewField[HookTxModel, int]("id"), 1)).Run(); err == nil {
		t.Fatalf("expected error on cancelled tx2.Update")
	}
	if err := tx2.UpdateRow(HookTxModel{ID: 1, Name: "x"}); err == nil {
		t.Fatalf("expected error on cancelled tx2.UpdateRow")
	}
	if err := tx2.Delete[HookTxModel]().Where(Equal(NewField[HookTxModel, int]("id"), 1)).Run(); err == nil {
		t.Fatalf("expected error on cancelled tx2.Delete")
	}
	if _, err := tx2.RawQuery[HookTxModel]("SELECT 1;", false); err == nil {
		t.Fatalf("expected error on cancelled tx2.RawQuery")
	}
	if err := tx2.RawExecute("SELECT 1;", false); err == nil {
		t.Fatalf("expected error on cancelled tx2.RawExecute")
	}

	_ = tx2.Rollback()
}

func TestBuilderHookErrors(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	_ = db.RawExecute(ctx, "CREATE TABLE hook_builder (id INTEGER PRIMARY KEY, name TEXT);", false)

	type HookBuilderModel struct {
		Name string `db:"name"`
		ID   int    `db:"id,pk"`
	}

	onUpdateFail := func(m *HookBuilderModel) error {
		return errors.New("onUpdate fail")
	}
	onDeleteFail := func(m *HookBuilderModel) error {
		return errors.New("onDelete fail")
	}

	db.Table("hook_builder", nil, onUpdateFail, onDeleteFail)

	ub := db.Update[HookBuilderModel]().Row(HookBuilderModel{ID: 1, Name: "A"})
	if err := ub.Run(); err == nil {
		t.Fatalf("expected UpdateBuilder.Row to fail on onUpdate hook")
	}

	ubSet := db.Update[HookBuilderModel]().
		Set(NewField[HookBuilderModel, string]("name"), "B").
		Where(Equal(NewField[HookBuilderModel, int]("id"), 1))
	if err := ubSet.Run(); err == nil {
		t.Fatalf("expected UpdateBuilder.Set to fail on onUpdate hook")
	}

	delB := db.Delete[HookBuilderModel]().Where(Equal(NewField[HookBuilderModel, int]("id"), 1))
	if err := delB.Run(); err == nil {
		t.Fatalf("expected DeleteBuilder to fail on onDelete hook")
	}

	tx, err := db.Transaction(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	if err := tx.Update[HookBuilderModel]().Row(HookBuilderModel{ID: 1, Name: "A"}).Run(); err == nil {
		t.Fatalf("expected tx.Update.Row to fail on onUpdate hook")
	}
	if err := tx.Delete[HookBuilderModel]().Where(Equal(NewField[HookBuilderModel, int]("id"), 1)).Run(); err == nil {
		t.Fatalf("expected tx.Delete to fail on onDelete hook")
	}
}

func TestMoreRemainingBranches(t *testing.T) {
	db, err := Open(":memory:", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	_ = db.RawExecute(ctx, "CREATE TABLE not_null_table (id INT NOT NULL);", false)
	type NotNullOmit struct {
		ID int `db:"id,omitzero"`
	}
	db.Table[NotNullOmit]("not_null_table", nil, nil, nil)
	if err := db.Insert(NotNullOmit{ID: 0}); err == nil {
		t.Fatalf("expected error inserting DEFAULT VALUES on NOT NULL table")
	}

	_ = db.RawExecute(ctx, "CREATE TABLE upsert_hook (id INT PRIMARY KEY, name TEXT);", false)
	type UpsertHookModel struct {
		Name string `db:"name"`
		ID   int    `db:"id,pk"`
	}
	onInsert := func(m *UpsertHookModel) error {
		if m.ID < 0 {
			return errors.New("negative ID in upsert")
		}
		if m.Name == "orig" {
			m.Name = "mutated_upsert"
		}
		return nil
	}
	db.Table("upsert_hook", onInsert, nil, nil)

	if err := db.Upsert(UpsertHookModel{ID: -1, Name: "fail"}); err == nil {
		t.Fatalf("expected Upsert to fail on onInsert hook")
	}
	if err := db.Upsert(UpsertHookModel{ID: 10, Name: "orig"}); err != nil {
		t.Fatalf("Upsert with hook failed: %v", err)
	}
	gotUpsert, err := db.Select[UpsertHookModel]().Where(Equal(NewField[UpsertHookModel, int]("id"), 10)).Run()
	if err != nil || len(gotUpsert) != 1 || gotUpsert[0].Name != "mutated_upsert" {
		t.Fatalf("expected mutated name, got %+v", gotUpsert)
	}

	_ = db.RawExecute(ctx, "CREATE TABLE upsert_all_omit (id INT PRIMARY KEY);", false)
	type UpsertAllOmit struct {
		ID int `db:"id,pk,omitzero"`
	}
	db.Table[UpsertAllOmit]("upsert_all_omit", nil, nil, nil)
	if err := db.Upsert(UpsertAllOmit{ID: 0}); err != nil {
		t.Fatalf("Upsert with all omitted columns should succeed: %v", err)
	}

	_ = db.RawExecute(ctx, "CREATE TABLE update_row_arg (id INT PRIMARY KEY, name TEXT);", false)
	type UpdateRowArgModel struct {
		Name string `db:"name"`
		ID   int    `db:"id,pk"`
	}
	db.Table[UpdateRowArgModel]("update_row_arg", nil, nil, nil)
	_ = db.Insert(UpdateRowArgModel{ID: 1, Name: "Initial"})

	uArgBuilder := db.Update(UpdateRowArgModel{ID: 1, Name: "UpdatedViaArg"})
	if err := uArgBuilder.Run(); err != nil {
		t.Fatalf("Update(row).Run() failed: %v", err)
	}

	_ = db.RawExecute(ctx, "CREATE TABLE update_mutate (id INT PRIMARY KEY, name TEXT);", false)
	type UpdateMutateModel struct {
		Name string `db:"name"`
		ID   int    `db:"id,pk"`
	}
	onUpdateMutate := func(m *UpdateMutateModel) error {
		if m != nil && m.Name == "orig" {
			m.Name = "mutated_update"
		}
		return nil
	}
	db.Table("update_mutate", nil, onUpdateMutate, nil)
	_ = db.Insert(UpdateMutateModel{ID: 1, Name: "Initial"})

	if err := db.UpdateRow(UpdateMutateModel{ID: 1, Name: "orig"}); err != nil {
		t.Fatalf("UpdateRow with mutating hook failed: %v", err)
	}
	gotMutated, err := db.Select[UpdateMutateModel]().Where(Equal(NewField[UpdateMutateModel, int]("id"), 1)).Run()
	if err != nil || len(gotMutated) != 1 || gotMutated[0].Name != "mutated_update" {
		t.Fatalf("expected mutated update, got %+v", gotMutated)
	}

	if err := db.UpdateRow(); err == nil {
		t.Fatalf("expected error on db.UpdateRow() without arguments")
	}

	_ = db.RawExecute(ctx, "CREATE TABLE raw_case (col_name TEXT);", false)
	type RawCaseModel struct {
		ColName string `db:"col_name"`
	}
	db.Table[RawCaseModel]("raw_case", nil, nil, nil)
	_ = db.Insert(RawCaseModel{ColName: "hello"})

	rawCaseRows, err := db.RawQuery[RawCaseModel](ctx, `SELECT col_name AS "COL_NAME" FROM raw_case;`, false)
	if err != nil || len(rawCaseRows) != 1 || rawCaseRows[0].ColName != "hello" {
		t.Fatalf("RawQuery case-insensitive column match failed: %v, rows: %+v", err, rawCaseRows)
	}

	tx, err := db.Transaction(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = sqlitex.Execute(tx.conn, "ROLLBACK;", nil)
	if err := tx.Commit(); err == nil {
		t.Fatalf("expected Commit to fail after manual rollback")
	}

	_ = db.RawExecute(ctx, "CREATE TABLE tx_drop (id INT PRIMARY KEY);", false)
	type TxDropModel struct {
		ID int `db:"id,pk"`
	}
	db.Table[TxDropModel]("tx_drop", nil, nil, nil)

	tx2, err := db.Transaction(ctx)
	if err != nil {
		t.Fatal(err)
	}

	_ = sqlitex.Execute(tx2.conn, "DROP TABLE tx_drop;", nil)
	if _, err := tx2.Select[TxDropModel]().Run(false); err == nil {
		t.Fatalf("expected tx2.Select to fail on dropped table")
	}
	_ = tx2.Rollback()

	db.Table("hook_tx_model_test", nil, nil, func(m *HookTxModel) error { return nil })
	info, err := db.getTableInfo(reflect.TypeOf(HookTxModel{}))
	if err != nil {
		t.Fatal(err)
	}
	if info.onDelete != nil {
		dummy := &HookTxModel{ID: 1}
		if err := info.onDelete(dummy); err != nil {
			t.Fatalf("onDelete with non-nil row failed: %v", err)
		}
	}

	if err := db.RawExecute(ctx, "SELECT ?;", false, 42); err != nil {
		t.Fatalf("RawExecute with args and cache=false failed: %v", err)
	}

	_ = db.RawExecute(ctx, "CREATE TABLE tx_fail_dec (id INT PRIMARY KEY, data TEXT);", false)
	db.Table[FailDecModel]("tx_fail_dec", nil, nil, nil)
	_ = db.RawExecute(ctx, "INSERT INTO tx_fail_dec (id, data) VALUES (1, 'data');", false)

	tx3, err := db.Transaction(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx3.Rollback()

	if _, err := tx3.Select[FailDecModel]().Run(false); err == nil {
		t.Fatalf("expected tx3.Select to fail on failing decoder")
	}

	dirtyConn, err := db.pool.Take(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = sqlitex.Execute(dirtyConn, "BEGIN;", nil)
	db.pool.Put(dirtyConn)

	_, err = db.Transaction(ctx)
	if err == nil {
		t.Fatalf("expected Transaction to fail when connection already in BEGIN")
	}

	cleanConn, err := db.pool.Take(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = sqlitex.Execute(cleanConn, "ROLLBACK;", nil)
	db.pool.Put(cleanConn)
}

type AllTypesModel struct {
	PInt      *int     `db:"p_int,omitzero"`
	PInt8     *int8    `db:"p_int8,omitzero"`
	PInt16    *int16   `db:"p_int16,omitzero"`
	PInt32    *int32   `db:"p_int32,omitzero"`
	PInt64    *int64   `db:"p_int64,omitzero"`
	PUint     *uint    `db:"p_uint,omitzero"`
	PUint8    *uint8   `db:"p_uint8,omitzero"`
	PUint16   *uint16  `db:"p_uint16,omitzero"`
	PUint32   *uint32  `db:"p_uint32,omitzero"`
	PUint64   *uint64  `db:"p_uint64,omitzero"`
	PFloat32  *float32 `db:"p_float32,omitzero"`
	PFloat64  *float64 `db:"p_float64,omitzero"`
	PBool     *bool    `db:"p_bool,omitzero"`
	PStr      *string  `db:"p_str,omitzero"`
	Str       string   `db:"str,omitzero"`
	ID        int      `db:"id,pk"`
	F64       float64  `db:"f64,omitzero"`
	I64       int64    `db:"i64,omitzero"`
	U64       uint64   `db:"u64,omitzero"`
	I         int      `db:"i,omitzero"`
	U         uint     `db:"u,omitzero"`
	I32       int32    `db:"i32,omitzero"`
	U32       uint32   `db:"u32,omitzero"`
	F32       float32  `db:"f32,omitzero"`
	I16       int16    `db:"i16,omitzero"`
	U16       uint16   `db:"u16,omitzero"`
	I8        int8     `db:"i8,omitzero"`
	U8        uint8    `db:"u8,omitzero"`
	B         bool     `db:"b,omitzero"`
}

func TestAllTypeScanningAndHooks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	col := NewField[User, int]("age")
	strCol := NewField[User, string]("surname")
	ConditionSQL(Equal(col, 20))
	ConditionSQL(Greater(col, 20))
	ConditionSQL(Lesser(col, 20))
	ConditionSQL(Within(col, 20, 30))
	ConditionSQL(Within(col))
	ConditionSQL(NotWithin(col, 20, 30))
	ConditionSQL(NotWithin(col))
	ConditionSQL(Range(col, 20, 30))
	ConditionSQL(OutsideRange(col, 20, 30))
	ConditionSQL(And(Equal(col, 20), Greater(strCol, "a")))
	ConditionSQL(Or(Equal(col, 20), Lesser(strCol, "z")))

	qbHuge := &queryBuffer{buf: make([]byte, 70000)}
	putQueryBuffer(qbHuge)

	_ = db.RawExecute(ctx, `CREATE TABLE all_types (
		id INTEGER PRIMARY KEY,
		p_int INTEGER, p_int8 INTEGER, p_int16 INTEGER, p_int32 INTEGER, p_int64 INTEGER,
		p_uint INTEGER, p_uint8 INTEGER, p_uint16 INTEGER, p_uint32 INTEGER, p_uint64 INTEGER,
		p_float32 REAL, p_float64 REAL, p_bool INTEGER, p_str TEXT,
		str TEXT, f64 REAL, i64 INTEGER, u64 INTEGER, i INTEGER, u INTEGER,
		i32 INTEGER, u32 INTEGER, f32 REAL, i16 INTEGER, u16 INTEGER,
		i8 INTEGER, u8 INTEGER, b INTEGER
	);`, false)

	db.Table[AllTypesModel]("all_types", nil, nil, nil)

	// Insert zero values
	z := AllTypesModel{ID: 1}
	if err := db.Insert(ctx, z); err != nil {
		t.Fatal(err)
	}

	// Insert non-zero values
	pi := 1; pi8 := int8(2); pi16 := int16(3); pi32 := int32(4); pi64 := int64(5)
	pu := uint(6); pu8 := uint8(7); pu16 := uint16(8); pu32 := uint32(9); pu64 := uint64(10)
	pf32 := float32(11.5); pf64 := 12.5; pb := true; ps := "ptrstr"

	nz := AllTypesModel{
		ID: 2, PInt: &pi, PInt8: &pi8, PInt16: &pi16, PInt32: &pi32, PInt64: &pi64,
		PUint: &pu, PUint8: &pu8, PUint16: &pu16, PUint32: &pu32, PUint64: &pu64,
		PFloat32: &pf32, PFloat64: &pf64, PBool: &pb, PStr: &ps,
		Str: "hello", F64: 99.9, I64: 88, U64: 77, I: 66, U: 55,
		I32: 44, U32: 33, F32: 22.5, I16: 11, U16: 10, I8: 9, U8: 8, B: true,
	}
	if err := db.Insert(ctx, nz); err != nil {
		t.Fatal(err)
	}

	// Select back
	rows, err := db.Select[AllTypesModel]().Run(ctx)
	if err != nil || len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %v (%d)", err, len(rows))
	}

	// Update row
	nz.Str = "updated"
	if err := db.UpdateRow(ctx, nz); err != nil {
		t.Fatal(err)
	}

	// Upsert
	if err := db.Upsert(ctx, nz); err != nil {
		t.Fatal(err)
	}

	// Multi-row and slice insert in transaction
	tx, err := db.Transaction(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	nz3 := AllTypesModel{ID: 3, Str: "three"}
	nz4 := AllTypesModel{ID: 4, Str: "four"}
	if err := tx.Insert(nz3, nz4); err != nil {
		t.Fatal(err)
	}
	if err := tx.Upsert(nz3, nz4); err != nil {
		t.Fatal(err)
	}

	slice := []AllTypesModel{{ID: 5, Str: "five"}, {ID: 6, Str: "six"}}
	if err := tx.Insert(slice); err != nil {
		t.Fatal(err)
	}

	if err := tx.UpdateRow(nz3); err != nil {
		t.Fatal(err)
	}
	if err := tx.UpdateRow(slice[0], slice[1]); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateRow(ctx, nz3, nz4); err != nil {
		t.Fatal(err)
	}
}

