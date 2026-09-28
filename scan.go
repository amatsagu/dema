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
			if col.isRuneSlice {
				v := []rune(stmt.ColumnText(colIdx))
				*(**[]rune)(fieldPtr) = &v
			} else if col.elemKind == reflect.Slice && col.typ.Elem().Elem().Kind() == reflect.Uint8 {
				buf := make([]byte, stmt.ColumnLen(colIdx))
				stmt.ColumnBytes(colIdx, buf)
				*(**[]byte)(fieldPtr) = &buf
			}
		}
		return nil
	}

	if col.isRuneSlice {
		*(*[]rune)(fieldPtr) = []rune(stmt.ColumnText(colIdx))
		return nil
	}
	if col.kind == reflect.Slice && col.typ.Elem().Kind() == reflect.Uint8 {
		buf := make([]byte, stmt.ColumnLen(colIdx))
		stmt.ColumnBytes(colIdx, buf)
		*(*[]byte)(fieldPtr) = buf
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
		fieldPtr := unsafe.Add(base, col.offset)

		if stmt.ColumnIsNull(i) {
			if col.isPtr {
				*(*uintptr)(fieldPtr) = 0
			}
			continue
		}

		if col.isPtr {
			switch col.elemKind {
			case reflect.String:
				s := stmt.ColumnText(i)
				*(**string)(fieldPtr) = &s
			case reflect.Int:
				v := int(stmt.ColumnInt64(i))
				*(**int)(fieldPtr) = &v
			case reflect.Int64:
				v := stmt.ColumnInt64(i)
				*(**int64)(fieldPtr) = &v
			case reflect.Uint32:
				v := uint32(stmt.ColumnInt64(i))
				*(**uint32)(fieldPtr) = &v
			case reflect.Bool:
				v := stmt.ColumnBool(i)
				*(**bool)(fieldPtr) = &v
			case reflect.Float64:
				v := stmt.ColumnFloat(i)
				*(**float64)(fieldPtr) = &v
			default:
				if err := scanColPtr(stmt, i, col, base); err != nil {
					return err
				}
			}
			continue
		}

		switch col.kind {
		case reflect.Int:
			*(*int)(fieldPtr) = int(stmt.ColumnInt64(i))
		case reflect.Int64:
			*(*int64)(fieldPtr) = stmt.ColumnInt64(i)
		case reflect.Uint32:
			*(*uint32)(fieldPtr) = uint32(stmt.ColumnInt64(i))
		case reflect.String:
			*(*string)(fieldPtr) = stmt.ColumnText(i)
		case reflect.Bool:
			*(*bool)(fieldPtr) = stmt.ColumnBool(i)
		case reflect.Float64:
			*(*float64)(fieldPtr) = stmt.ColumnFloat(i)
		case reflect.Int32:
			*(*int32)(fieldPtr) = int32(stmt.ColumnInt64(i))
		case reflect.Uint64:
			*(*uint64)(fieldPtr) = uint64(stmt.ColumnInt64(i))
		case reflect.Uint:
			*(*uint)(fieldPtr) = uint(stmt.ColumnInt64(i))
		default:
			if err := scanColPtr(stmt, i, col, base); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *tableInfo) scanRowMappedPtr(stmt *sqlite.Stmt, colMap []*colInfo, base unsafe.Pointer) error {
	for i, col := range colMap {
		if col == nil {
			continue
		}
		fieldPtr := unsafe.Add(base, col.offset)

		if stmt.ColumnIsNull(i) {
			if col.isPtr {
				*(*uintptr)(fieldPtr) = 0
			}
			continue
		}

		if col.isPtr {
			switch col.elemKind {
			case reflect.String:
				s := stmt.ColumnText(i)
				*(**string)(fieldPtr) = &s
			case reflect.Int:
				v := int(stmt.ColumnInt64(i))
				*(**int)(fieldPtr) = &v
			case reflect.Int64:
				v := stmt.ColumnInt64(i)
				*(**int64)(fieldPtr) = &v
			case reflect.Uint32:
				v := uint32(stmt.ColumnInt64(i))
				*(**uint32)(fieldPtr) = &v
			case reflect.Bool:
				v := stmt.ColumnBool(i)
				*(**bool)(fieldPtr) = &v
			case reflect.Float64:
				v := stmt.ColumnFloat(i)
				*(**float64)(fieldPtr) = &v
			default:
				if err := scanColPtr(stmt, i, col, base); err != nil {
					return err
				}
			}
			continue
		}

		switch col.kind {
		case reflect.Int:
			*(*int)(fieldPtr) = int(stmt.ColumnInt64(i))
		case reflect.Int64:
			*(*int64)(fieldPtr) = stmt.ColumnInt64(i)
		case reflect.Uint32:
			*(*uint32)(fieldPtr) = uint32(stmt.ColumnInt64(i))
		case reflect.String:
			*(*string)(fieldPtr) = stmt.ColumnText(i)
		case reflect.Bool:
			*(*bool)(fieldPtr) = stmt.ColumnBool(i)
		case reflect.Float64:
			*(*float64)(fieldPtr) = stmt.ColumnFloat(i)
		case reflect.Int32:
			*(*int32)(fieldPtr) = int32(stmt.ColumnInt64(i))
		case reflect.Uint64:
			*(*uint64)(fieldPtr) = uint64(stmt.ColumnInt64(i))
		case reflect.Uint:
			*(*uint)(fieldPtr) = uint(stmt.ColumnInt64(i))
		default:
			if err := scanColPtr(stmt, i, col, base); err != nil {
				return err
			}
		}
	}
	return nil
}

func isScalarType(typ reflect.Type) bool {
	if typ == nil || typ.Kind() != reflect.Struct {
		return true
	}
	return typ.Implements(decoderType) || reflect.PointerTo(typ).Implements(decoderType)
}

func scanScalar[T any](stmt *sqlite.Stmt, colIdx int) (T, error) {
	var res T
	if stmt.ColumnIsNull(colIdx) {
		return res, nil
	}

	switch p := any(&res).(type) {
	case *int:
		*p = int(stmt.ColumnInt64(colIdx))
	case *int64:
		*p = stmt.ColumnInt64(colIdx)
	case *int32:
		*p = int32(stmt.ColumnInt64(colIdx))
	case *int16:
		*p = int16(stmt.ColumnInt64(colIdx))
	case *int8:
		*p = int8(stmt.ColumnInt64(colIdx))
	case *uint:
		*p = uint(stmt.ColumnInt64(colIdx))
	case *uint64:
		*p = uint64(stmt.ColumnInt64(colIdx))
	case *uint32:
		*p = uint32(stmt.ColumnInt64(colIdx))
	case *uint16:
		*p = uint16(stmt.ColumnInt64(colIdx))
	case *uint8:
		*p = uint8(stmt.ColumnInt64(colIdx))
	case *uintptr:
		*p = uintptr(stmt.ColumnInt64(colIdx))
	case *bool:
		*p = stmt.ColumnBool(colIdx)
	case *float64:
		*p = stmt.ColumnFloat(colIdx)
	case *float32:
		*p = float32(stmt.ColumnFloat(colIdx))
	case *string:
		*p = stmt.ColumnText(colIdx)
	case *[]rune:
		*p = []rune(stmt.ColumnText(colIdx))
	case *[]byte:
		buf := make([]byte, stmt.ColumnLen(colIdx))
		stmt.ColumnBytes(colIdx, buf)
		*p = buf

	case **int:
		v := int(stmt.ColumnInt64(colIdx))
		*p = &v
	case **int64:
		v := stmt.ColumnInt64(colIdx)
		*p = &v
	case **int32:
		v := int32(stmt.ColumnInt64(colIdx))
		*p = &v
	case **int16:
		v := int16(stmt.ColumnInt64(colIdx))
		*p = &v
	case **int8:
		v := int8(stmt.ColumnInt64(colIdx))
		*p = &v
	case **uint:
		v := uint(stmt.ColumnInt64(colIdx))
		*p = &v
	case **uint64:
		v := uint64(stmt.ColumnInt64(colIdx))
		*p = &v
	case **uint32:
		v := uint32(stmt.ColumnInt64(colIdx))
		*p = &v
	case **uint16:
		v := uint16(stmt.ColumnInt64(colIdx))
		*p = &v
	case **uint8:
		v := uint8(stmt.ColumnInt64(colIdx))
		*p = &v
	case **uintptr:
		v := uintptr(stmt.ColumnInt64(colIdx))
		*p = &v
	case **bool:
		v := stmt.ColumnBool(colIdx)
		*p = &v
	case **float64:
		v := stmt.ColumnFloat(colIdx)
		*p = &v
	case **float32:
		v := float32(stmt.ColumnFloat(colIdx))
		*p = &v
	case **string:
		v := stmt.ColumnText(colIdx)
		*p = &v
	case **[]rune:
		v := []rune(stmt.ColumnText(colIdx))
		*p = &v
	case **[]byte:
		buf := make([]byte, stmt.ColumnLen(colIdx))
		stmt.ColumnBytes(colIdx, buf)
		*p = &buf

	case *any:
		switch stmt.ColumnType(colIdx) {
		case sqlite.TypeInteger:
			*p = stmt.ColumnInt64(colIdx)
		case sqlite.TypeFloat:
			*p = stmt.ColumnFloat(colIdx)
		case sqlite.TypeText:
			*p = stmt.ColumnText(colIdx)
		case sqlite.TypeBlob:
			buf := make([]byte, stmt.ColumnLen(colIdx))
			stmt.ColumnBytes(colIdx, buf)
			*p = buf
		case sqlite.TypeNull:
			*p = nil
		}

	default:
		return scanScalarSlow(stmt, colIdx, res)
	}
	return res, nil
}

//go:noinline
func scanScalarSlow[T any](stmt *sqlite.Stmt, colIdx int, res T) (T, error) {
	if dec, ok := any(&res).(FieldDecoder); ok {
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
		case sqlite.TypeNull:
			raw = nil
		}
		if err := dec.DecodeDema(raw); err != nil {
			return res, err
		}
		return res, nil
	}

	val := reflect.ValueOf(&res).Elem()
	if val.Kind() == reflect.Pointer {
		if stmt.ColumnIsNull(colIdx) {
			return res, nil
		}
		elem := reflect.New(val.Type().Elem())
		if err := scanScalarReflect(stmt, colIdx, elem.Elem()); err != nil {
			return res, err
		}
		val.Set(elem)
		return res, nil
	}

	if err := scanScalarReflect(stmt, colIdx, val); err != nil {
		return res, err
	}
	return res, nil
}

func scanScalarReflect(stmt *sqlite.Stmt, colIdx int, val reflect.Value) error {
	if val.CanAddr() {
		if dec, ok := val.Addr().Interface().(FieldDecoder); ok {
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
			case sqlite.TypeNull:
				raw = nil
			}
			return dec.DecodeDema(raw)
		}
	}

	switch val.Kind() {
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		val.SetInt(stmt.ColumnInt64(colIdx))
	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8, reflect.Uintptr:
		val.SetUint(uint64(stmt.ColumnInt64(colIdx)))
	case reflect.Float64, reflect.Float32:
		val.SetFloat(stmt.ColumnFloat(colIdx))
	case reflect.String:
		val.SetString(stmt.ColumnText(colIdx))
	case reflect.Bool:
		val.SetBool(stmt.ColumnBool(colIdx))
	case reflect.Slice:
		elemKind := val.Type().Elem().Kind()
		if elemKind == reflect.Int32 {
			val.Set(reflect.ValueOf([]rune(stmt.ColumnText(colIdx))))
		} else if elemKind == reflect.Uint8 {
			buf := make([]byte, stmt.ColumnLen(colIdx))
			stmt.ColumnBytes(colIdx, buf)
			val.SetBytes(buf)
		}
	}
	return nil
}
