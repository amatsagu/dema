package dema

import (
	"reflect"

	"zombiezen.com/go/sqlite"
)

func setScalarValue(stmt *sqlite.Stmt, colIdx int, kind reflect.Kind, target reflect.Value) {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		target.SetInt(stmt.ColumnInt64(colIdx))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		target.SetUint(uint64(stmt.ColumnInt64(colIdx)))
	case reflect.Bool:
		target.SetBool(stmt.ColumnBool(colIdx))
	case reflect.Float32, reflect.Float64:
		target.SetFloat(stmt.ColumnFloat(colIdx))
	case reflect.String:
		target.SetString(stmt.ColumnText(colIdx))
	}
}

func scanCol(stmt *sqlite.Stmt, colIdx int, col *colInfo, structVal reflect.Value) error {
	fieldVal := structVal.FieldByIndex(col.index)

	if stmt.ColumnIsNull(colIdx) {
		if col.isPtr {
			fieldVal.Set(reflect.Zero(col.typ))
		}
		return nil
	}

	if col.isDecoder {
		var raw any
		switch stmt.ColumnType(colIdx) {
		case sqlite.TypeInteger:
			raw = stmt.ColumnInt64(colIdx)
		case sqlite.TypeFloat:
			raw = stmt.ColumnFloat(colIdx)
		case sqlite.TypeText:
			raw = stmt.ColumnText(colIdx)
		case sqlite.TypeBlob:
			buf := make([]byte, stmt.ColumnLen(colIdx))
			stmt.ColumnBytes(colIdx, buf)
			raw = buf
		}

		if dec, ok := fieldVal.Addr().Interface().(FieldDecoder); ok {
			return dec.DecodeDema(raw)
		}
	}

	if col.isPtr {
		elemType := col.typ.Elem()
		elemVal := reflect.New(elemType).Elem()
		setScalarValue(stmt, colIdx, col.kind, elemVal)
		fieldVal.Set(elemVal.Addr())
		return nil
	}

	if col.isRuneSlice {
		fieldVal.Set(reflect.ValueOf([]rune(stmt.ColumnText(colIdx))))
		return nil
	}

	setScalarValue(stmt, colIdx, col.kind, fieldVal)
	return nil
}

func (t *tableInfo) scanRowDirect(stmt *sqlite.Stmt, val reflect.Value) error {
	for i, col := range t.allCols {
		if err := scanCol(stmt, i, col, val); err != nil {
			return err
		}
	}
	return nil
}

func (t *tableInfo) scanRowMapped(stmt *sqlite.Stmt, colMap []*colInfo, val reflect.Value) error {
	for i, col := range colMap {
		if col == nil {
			continue
		}
		if err := scanCol(stmt, i, col, val); err != nil {
			return err
		}
	}
	return nil
}
