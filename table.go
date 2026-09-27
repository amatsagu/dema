package dema

import (
	"reflect"
	"strings"

	"github.com/amatsagu/lumo"
)

var (
	encoderType   = reflect.TypeOf((*FieldEncoder)(nil)).Elem()
	decoderType   = reflect.TypeOf((*FieldDecoder)(nil)).Elem()
	runeSliceType = reflect.TypeOf([]rune(nil))
)

type colInfo struct {
	typ         reflect.Type
	name        string
	fieldName   string
	index       []int
	kind        reflect.Kind
	isPK        bool
	isOmitZero  bool
	isPtr       bool
	isRuneSlice bool
	isEncoder   bool
	isDecoder   bool
}

type tableInfo struct {
	typ              reflect.Type
	colByName        map[string]*colInfo
	onInsert         func(any) error
	onUpdate         func(any) error
	onDelete         func(any) error
	name             string
	defaultSelectSQL string
	allCols          []*colInfo
	pkCols           []*colInfo
	nonPkCols        []*colInfo
}

func parseTag(tag string, defaultName string) (name string, isPK bool, isOmitZero bool, ignore bool) {
	if tag == "-" {
		return "", false, false, true
	}
	name = defaultName
	if tag == "" {
		return name, false, false, false
	}
	parts := strings.Split(tag, ",")
	first := strings.TrimSpace(parts[0])
	if first != "" {
		name = first
	}
	for _, opt := range parts[1:] {
		opt = strings.TrimSpace(opt)
		switch opt {
		case "pk":
			isPK = true
		case "omitzero", "omitempty":
			isOmitZero = true
		}
	}
	return name, isPK, isOmitZero, false
}

func inspectStruct(t reflect.Type, indexPrefix []int) []*colInfo {
	var cols []*colInfo
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		currIndex := append(append([]int(nil), indexPrefix...), i)
		tag := field.Tag.Get("db")
		defaultColName := strings.ToLower(field.Name)
		colName, isPK, isOmitZero, ignore := parseTag(tag, defaultColName)
		if ignore {
			continue
		}

		ft := field.Type
		isPtr := ft.Kind() == reflect.Pointer
		elemType := ft
		if isPtr {
			elemType = ft.Elem()
		}

		if field.Anonymous && elemType.Kind() == reflect.Struct &&
			!ft.Implements(encoderType) && !reflect.PointerTo(ft).Implements(encoderType) {
			subCols := inspectStruct(elemType, currIndex)
			cols = append(cols, subCols...)
			continue
		}

		isEnc := ft.Implements(encoderType) || reflect.PointerTo(ft).Implements(encoderType)
		isDec := ft.Implements(decoderType) || reflect.PointerTo(ft).Implements(decoderType)
		isRunes := ft == runeSliceType || (isPtr && elemType == runeSliceType)

		col := &colInfo{
			name:        colName,
			fieldName:   field.Name,
			index:       currIndex,
			typ:         ft,
			kind:        elemType.Kind(),
			isPK:        isPK,
			isOmitZero:  isOmitZero,
			isPtr:       isPtr,
			isRuneSlice: isRunes,
			isEncoder:   isEnc,
			isDecoder:   isDec,
		}
		cols = append(cols, col)
	}
	return cols
}

func newTableInfo[T any](tableName string, onInsert, onUpdate, onDelete func(*T) error) (*tableInfo, error) {
	var zero T
	typ := reflect.TypeOf(zero)
	if typ == nil {
		return nil, lumo.WrapString("model type must be a struct").
			Include("dema_table", tableName).
			Include("dema_operation", "REGISTER").
			Include("dema_expected", "valid struct type")
	}
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil, lumo.WrapString("model type %s must be a struct", typ.String()).
			Include("dema_table", tableName).
			Include("dema_operation", "REGISTER").
			Include("dema_expected", "struct kind")
	}

	cols := inspectStruct(typ, nil)
	if len(cols) == 0 {
		return nil, lumo.WrapString("model %s has no exported fields", typ.String()).
			Include("dema_table", tableName).
			Include("dema_operation", "REGISTER").
			Include("dema_expected", "at least one column")
	}

	info := &tableInfo{
		name:      tableName,
		typ:       typ,
		allCols:   cols,
		colByName: make(map[string]*colInfo, len(cols)),
	}

	for _, col := range cols {
		info.colByName[col.name] = col
		if col.isPK {
			info.pkCols = append(info.pkCols, col)
		} else {
			info.nonPkCols = append(info.nonPkCols, col)
		}
	}

	var sb strings.Builder
	sb.WriteString("SELECT ")
	for i, col := range cols {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(`"`)
		sb.WriteString(col.name)
		sb.WriteString(`"`)
	}
	sb.WriteString(` FROM "`)
	sb.WriteString(tableName)
	sb.WriteString(`"`)
	info.defaultSelectSQL = sb.String()

	if onInsert != nil {
		info.onInsert = func(v any) error {
			return onInsert(v.(*T))
		}
	}
	if onUpdate != nil {
		info.onUpdate = func(v any) error {
			if v == nil {
				return onUpdate(nil)
			}
			return onUpdate(v.(*T))
		}
	}
	if onDelete != nil {
		info.onDelete = func(v any) error {
			if v == nil {
				return onDelete(nil)
			}
			return onDelete(v.(*T))
		}
	}

	return info, nil
}

func (t *tableInfo) extractColValue(col *colInfo, structVal reflect.Value) (any, bool, error) {
	fieldVal := structVal.FieldByIndex(col.index)

	if col.isOmitZero && fieldVal.IsZero() {
		return nil, false, nil
	}

	if col.isPtr {
		if fieldVal.IsNil() {
			return nil, false, nil
		}
		fieldVal = fieldVal.Elem()
	}

	if col.isEncoder {
		if enc, ok := fieldVal.Addr().Interface().(FieldEncoder); ok {
			v, err := enc.EncodeDema()
			return v, true, err
		}
	}

	if col.isRuneSlice {
		return string(fieldVal.Interface().([]rune)), true, nil
	}

	return fieldVal.Interface(), true, nil
}
