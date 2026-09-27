package dema

import (
	"reflect"
	"unsafe"

	"zombiezen.com/go/sqlite"
)

func scanColPtr(stmt *sqlite.Stmt, colIdx int, col *colInfo, base unsafe.Pointer) error {
	fieldPtr := unsafe.Add(base, col.offset)

	if stmt.ColumnIsNull(colIdx) {
		if col.isPtr {
			*(*uintptr)(fieldPtr) = 0
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
		dec := reflect.NewAt(col.typ, fieldPtr).Interface().(FieldDecoder)
		return dec.DecodeDema(raw)
	}

	if col.isPtr {
		switch col.elemKind {
		case reflect.String:
			s := stmt.ColumnText(colIdx)
			*(**string)(fieldPtr) = &s
		case reflect.Int:
			v := int(stmt.ColumnInt64(colIdx))
			*(**int)(fieldPtr) = &v
		case reflect.Int64:
			v := stmt.ColumnInt64(colIdx)
			*(**int64)(fieldPtr) = &v
		case reflect.Int32:
			v := int32(stmt.ColumnInt64(colIdx))
			*(**int32)(fieldPtr) = &v
		case reflect.Int16:
			v := int16(stmt.ColumnInt64(colIdx))
			*(**int16)(fieldPtr) = &v
		case reflect.Int8:
			v := int8(stmt.ColumnInt64(colIdx))
			*(**int8)(fieldPtr) = &v
		case reflect.Uint:
			v := uint(stmt.ColumnInt64(colIdx))
			*(**uint)(fieldPtr) = &v
		case reflect.Uint64:
			v := uint64(stmt.ColumnInt64(colIdx))
			*(**uint64)(fieldPtr) = &v
		case reflect.Uint32:
			v := uint32(stmt.ColumnInt64(colIdx))
			*(**uint32)(fieldPtr) = &v
		case reflect.Uint16:
			v := uint16(stmt.ColumnInt64(colIdx))
			*(**uint16)(fieldPtr) = &v
		case reflect.Uint8:
			v := uint8(stmt.ColumnInt64(colIdx))
			*(**uint8)(fieldPtr) = &v
		case reflect.Bool:
			v := stmt.ColumnBool(colIdx)
			*(**bool)(fieldPtr) = &v
		case reflect.Float64:
			v := stmt.ColumnFloat(colIdx)
			*(**float64)(fieldPtr) = &v
		case reflect.Float32:
			v := float32(stmt.ColumnFloat(colIdx))
			*(**float32)(fieldPtr) = &v
		default:
		}
		return nil
	}

	if col.isRuneSlice {
		*(*[]rune)(fieldPtr) = []rune(stmt.ColumnText(colIdx))
		return nil
	}

	switch col.kind {
	case reflect.Int:
		*(*int)(fieldPtr) = int(stmt.ColumnInt64(colIdx))
	case reflect.Int64:
		*(*int64)(fieldPtr) = stmt.ColumnInt64(colIdx)
	case reflect.Int32:
		*(*int32)(fieldPtr) = int32(stmt.ColumnInt64(colIdx))
	case reflect.Int16:
		*(*int16)(fieldPtr) = int16(stmt.ColumnInt64(colIdx))
	case reflect.Int8:
		*(*int8)(fieldPtr) = int8(stmt.ColumnInt64(colIdx))
	case reflect.Uint:
		*(*uint)(fieldPtr) = uint(stmt.ColumnInt64(colIdx))
	case reflect.Uint64:
		*(*uint64)(fieldPtr) = uint64(stmt.ColumnInt64(colIdx))
	case reflect.Uint32:
		*(*uint32)(fieldPtr) = uint32(stmt.ColumnInt64(colIdx))
	case reflect.Uint16:
		*(*uint16)(fieldPtr) = uint16(stmt.ColumnInt64(colIdx))
	case reflect.Uint8:
		*(*uint8)(fieldPtr) = uint8(stmt.ColumnInt64(colIdx))
	case reflect.Bool:
		*(*bool)(fieldPtr) = stmt.ColumnBool(colIdx)
	case reflect.Float64:
		*(*float64)(fieldPtr) = stmt.ColumnFloat(colIdx)
	case reflect.Float32:
		*(*float32)(fieldPtr) = float32(stmt.ColumnFloat(colIdx))
	case reflect.String:
		*(*string)(fieldPtr) = stmt.ColumnText(colIdx)
	default:
	}
	return nil
}

func (t *tableInfo) scanRowDirectPtr(stmt *sqlite.Stmt, base unsafe.Pointer) error {
	for i, col := range t.allCols {
		if err := scanColPtr(stmt, i, col, base); err != nil {
			return err
		}
	}
	return nil
}

func (t *tableInfo) scanRowMappedPtr(stmt *sqlite.Stmt, colMap []*colInfo, base unsafe.Pointer) error {
	for i, col := range colMap {
		if col == nil {
			continue
		}
		if err := scanColPtr(stmt, i, col, base); err != nil {
			return err
		}
	}
	return nil
}
