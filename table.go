package dema

import (
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/amatsagu/lumo"
)

var (
	encoderType   = reflect.TypeOf((*FieldEncoder)(nil)).Elem()
	decoderType   = reflect.TypeOf((*FieldDecoder)(nil)).Elem()
	runeSliceType = reflect.TypeOf([]rune(nil))
)

type colInfo struct {
	typ            reflect.Type
	name           string
	fieldName      string
	equalSQL       string
	equalLimit1SQL string
	index          []int
	offset         uintptr
	colIdx         int
	kind           reflect.Kind
	elemKind       reflect.Kind
	isPK           bool
	isOmitZero     bool
	isPtr          bool
	isRuneSlice    bool
	isEncoder      bool
	isDecoder      bool
}

func getModelType[T any]() (reflect.Type, bool) {
	typ := reflect.TypeOf((*T)(nil)).Elem()
	if typ.Kind() == reflect.Pointer {
		return typ.Elem(), true
	}
	return typ, false
}


type sqlPlan struct {
	sql  string
	cols []*colInfo
}

type tableInfo struct {
	typ               reflect.Type
	colByName         map[string]*colInfo
	insertPlans       atomic.Pointer[map[uint64]*sqlPlan]
	upsertPlans       atomic.Pointer[map[uint64]*sqlPlan]
	updateRowPlans    atomic.Pointer[map[uint64]*sqlPlan]
	selectCache       atomic.Pointer[map[string]string]
	updateSQLCache    atomic.Pointer[map[string]string]
	fastUpdatePlans   atomic.Pointer[map[uint32]string]
	onInsert          func(any) error
	onUpdate          func(any) error
	onDelete          func(any) error
	name              string
	defaultSelectSQL  string
	defaultInsertSQL  string
	defaultUpsertSQL  string
	defaultUpdateSQL  string
	allCols           []*colInfo
	pkCols            []*colInfo
	nonPkCols         []*colInfo
	defaultInsertCols []*colInfo
	defaultUpsertCols []*colInfo
	defaultUpdateCols []*colInfo
	cacheMu           sync.Mutex
	selectMu          sync.Mutex
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

func inspectStruct(t reflect.Type, indexPrefix []int, baseOffset uintptr) []*colInfo {
	var cols []*colInfo
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		currIndex := append(append([]int(nil), indexPrefix...), i)
		currOffset := baseOffset + field.Offset
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
			subCols := inspectStruct(elemType, currIndex, currOffset)
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
			offset:      currOffset,
			typ:         ft,
			kind:        elemType.Kind(),
			elemKind:    elemType.Kind(),
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
	typ, _ := getModelType[T]()
	if typ == nil {
		return nil, lumo.WrapString("model type must be a struct").
			Include("dema_table", tableName).
			Include("dema_operation", "REGISTER").
			Include("dema_expected", "valid struct type")
	}
	if typ.Kind() != reflect.Struct {
		return nil, lumo.WrapString("model type %s must be a struct", typ.String()).
			Include("dema_table", tableName).
			Include("dema_operation", "REGISTER").
			Include("dema_expected", "struct kind")
	}

	cols := inspectStruct(typ, nil, 0)
	if len(cols) == 0 {
		return nil, lumo.WrapString("model %s has no exported fields", typ.String()).
			Include("dema_table", tableName).
			Include("dema_operation", "REGISTER").
			Include("dema_expected", "at least one column")
	}

	for i, col := range cols {
		col.colIdx = i
	}

	info := &tableInfo{
		name:      tableName,
		typ:       typ,
		allCols:   cols,
		colByName: make(map[string]*colInfo, len(cols)),
	}
	selMap := make(map[string]string)
	info.selectCache.Store(&selMap)
	updMap := make(map[string]string)
	info.updateSQLCache.Store(&updMap)
	inMap := make(map[uint64]*sqlPlan)
	info.insertPlans.Store(&inMap)
	upMap := make(map[uint64]*sqlPlan)
	info.upsertPlans.Store(&upMap)
	urMap := make(map[uint64]*sqlPlan)
	info.updateRowPlans.Store(&urMap)
	fastUpMap := make(map[uint32]string)
	info.fastUpdatePlans.Store(&fastUpMap)

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

	for _, col := range cols {
		col.equalSQL = info.defaultSelectSQL + ` WHERE "` + col.name + `" = ?`
		col.equalLimit1SQL = info.defaultSelectSQL + ` WHERE "` + col.name + `" = ? LIMIT 1`
	}

	allMask := (uint64(1) << len(cols)) - 1
	info.defaultInsertSQL, info.defaultInsertCols = info.getInsertPlan(allMask)

	if len(info.pkCols) > 0 {
		info.defaultUpsertSQL, info.defaultUpsertCols = info.getUpsertPlan(allMask)
		info.defaultUpdateSQL, info.defaultUpdateCols = info.getUpdateRowPlan(allMask)
	}

	var omitMask uint64
	for i, col := range cols {
		if !col.isOmitZero {
			omitMask |= (uint64(1) << i)
		}
	}
	if omitMask != allMask {
		info.getInsertPlan(omitMask)
		if len(info.pkCols) > 0 {
			info.getUpsertPlan(omitMask)
			info.getUpdateRowPlan(omitMask)
		}
	}

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

func (t *tableInfo) getNonZeroColMask(p unsafe.Pointer) uint64 {
	var mask uint64
	for i, col := range t.allCols {
		if !col.isOmitZero {
			mask |= (uint64(1) << i)
			continue
		}
		fieldPtr := unsafe.Add(p, col.offset)
		if col.isPtr {
			if *(*unsafe.Pointer)(fieldPtr) != nil {
				mask |= (uint64(1) << i)
			}
			continue
		}
		switch col.kind {
		case reflect.Int:
			if *(*int)(fieldPtr) != 0 {
				mask |= (uint64(1) << i)
			}
		case reflect.Int64:
			if *(*int64)(fieldPtr) != 0 {
				mask |= (uint64(1) << i)
			}
		case reflect.Int32:
			if *(*int32)(fieldPtr) != 0 {
				mask |= (uint64(1) << i)
			}
		case reflect.Int16:
			if *(*int16)(fieldPtr) != 0 {
				mask |= (uint64(1) << i)
			}
		case reflect.Int8:
			if *(*int8)(fieldPtr) != 0 {
				mask |= (uint64(1) << i)
			}
		case reflect.Uint:
			if *(*uint)(fieldPtr) != 0 {
				mask |= (uint64(1) << i)
			}
		case reflect.Uint64:
			if *(*uint64)(fieldPtr) != 0 {
				mask |= (uint64(1) << i)
			}
		case reflect.Uint32:
			if *(*uint32)(fieldPtr) != 0 {
				mask |= (uint64(1) << i)
			}
		case reflect.Uint16:
			if *(*uint16)(fieldPtr) != 0 {
				mask |= (uint64(1) << i)
			}
		case reflect.Uint8:
			if *(*uint8)(fieldPtr) != 0 {
				mask |= (uint64(1) << i)
			}
		case reflect.String:
			if *(*string)(fieldPtr) != "" {
				mask |= (uint64(1) << i)
			}
		case reflect.Bool:
			if *(*bool)(fieldPtr) {
				mask |= (uint64(1) << i)
			}
		case reflect.Float64:
			if *(*float64)(fieldPtr) != 0 {
				mask |= (uint64(1) << i)
			}
		case reflect.Float32:
			if *(*float32)(fieldPtr) != 0 {
				mask |= (uint64(1) << i)
			}
		default:
			mask |= (uint64(1) << i)
		}
	}
	return mask
}

func (t *tableInfo) getInsertPlan(mask uint64) (string, []*colInfo) {
	allMask := (uint64(1) << len(t.allCols)) - 1
	if mask == allMask && t.defaultInsertSQL != "" {
		return t.defaultInsertSQL, t.defaultInsertCols
	}

	if p := t.insertPlans.Load(); p != nil {
		if plan, ok := (*p)[mask]; ok {
			return plan.sql, plan.cols
		}
	}

	t.cacheMu.Lock()
	defer t.cacheMu.Unlock()
	p := t.insertPlans.Load()
	var current map[uint64]*sqlPlan
	if p != nil {
		current = *p
		if plan, ok := current[mask]; ok {
			return plan.sql, plan.cols
		}
	}

	var activeCols []*colInfo
	for i, col := range t.allCols {
		if (mask & (uint64(1) << i)) != 0 {
			activeCols = append(activeCols, col)
		}
	}

	var sb strings.Builder
	sb.WriteString(`INSERT INTO "`)
	sb.WriteString(t.name)
	if len(activeCols) == 0 {
		sb.WriteString(`" DEFAULT VALUES;`)
	} else {
		sb.WriteString(`" (`)
		for i, col := range activeCols {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(`"`)
			sb.WriteString(col.name)
			sb.WriteString(`"`)
		}
		sb.WriteString(") VALUES (")
		for i := range activeCols {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("?")
		}
		sb.WriteString(");")
	}

	sql := sb.String()
	plan := &sqlPlan{sql: sql, cols: activeCols}
	newMap := make(map[uint64]*sqlPlan, len(current)+1)
	for k, v := range current {
		newMap[k] = v
	}
	newMap[mask] = plan
	t.insertPlans.Store(&newMap)
	return sql, activeCols
}

func (t *tableInfo) getUpsertPlan(mask uint64) (string, []*colInfo) {
	allMask := (uint64(1) << len(t.allCols)) - 1
	if mask == allMask && t.defaultUpsertSQL != "" {
		return t.defaultUpsertSQL, t.defaultUpsertCols
	}

	if p := t.upsertPlans.Load(); p != nil {
		if plan, ok := (*p)[mask]; ok {
			return plan.sql, plan.cols
		}
	}

	t.cacheMu.Lock()
	defer t.cacheMu.Unlock()
	p := t.upsertPlans.Load()
	var current map[uint64]*sqlPlan
	if p != nil {
		current = *p
		if plan, ok := current[mask]; ok {
			return plan.sql, plan.cols
		}
	}

	var activeCols []*colInfo
	for i, col := range t.allCols {
		if (mask & (uint64(1) << i)) != 0 {
			activeCols = append(activeCols, col)
		}
	}

	var sb strings.Builder
	sb.WriteString(`INSERT INTO "`)
	sb.WriteString(t.name)
	if len(activeCols) == 0 {
		sb.WriteString(`" DEFAULT VALUES;`)
	} else {
		sb.WriteString(`" (`)
		for i, col := range activeCols {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(`"`)
			sb.WriteString(col.name)
			sb.WriteString(`"`)
		}
		sb.WriteString(") VALUES (")
		for i := range activeCols {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("?")
		}
		sb.WriteString(") ON CONFLICT (")
		for i, pk := range t.pkCols {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(`"`)
			sb.WriteString(pk.name)
			sb.WriteString(`"`)
		}
		sb.WriteString(")")

		var updateCols []*colInfo
		for _, col := range activeCols {
			if !col.isPK {
				updateCols = append(updateCols, col)
			}
		}

		if len(updateCols) == 0 {
			sb.WriteString(" DO NOTHING;")
		} else {
			sb.WriteString(" DO UPDATE SET ")
			for i, col := range updateCols {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(`"`)
				sb.WriteString(col.name)
				sb.WriteString(`" = excluded."`)
				sb.WriteString(col.name)
				sb.WriteString(`"`)
			}
			sb.WriteString(";")
		}
	}

	sql := sb.String()
	plan := &sqlPlan{sql: sql, cols: activeCols}
	newMap := make(map[uint64]*sqlPlan, len(current)+1)
	for k, v := range current {
		newMap[k] = v
	}
	newMap[mask] = plan
	t.upsertPlans.Store(&newMap)
	return sql, activeCols
}

func (t *tableInfo) getUpdateRowPlan(mask uint64) (string, []*colInfo) {
	allMask := (uint64(1) << len(t.allCols)) - 1
	if mask == allMask && t.defaultUpdateSQL != "" {
		return t.defaultUpdateSQL, t.defaultUpdateCols
	}

	if p := t.updateRowPlans.Load(); p != nil {
		if plan, ok := (*p)[mask]; ok {
			return plan.sql, plan.cols
		}
	}

	t.cacheMu.Lock()
	defer t.cacheMu.Unlock()
	p := t.updateRowPlans.Load()
	var current map[uint64]*sqlPlan
	if p != nil {
		current = *p
		if plan, ok := current[mask]; ok {
			return plan.sql, plan.cols
		}
	}

	var setCols []*colInfo
	for _, col := range t.nonPkCols {
		if (mask & (uint64(1) << col.colIdx)) != 0 {
			setCols = append(setCols, col)
		}
	}
	if len(setCols) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString(`UPDATE "`)
	sb.WriteString(t.name)
	sb.WriteString(`" SET `)
	for i, col := range setCols {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(`"`)
		sb.WriteString(col.name)
		sb.WriteString(`" = ?`)
	}
	sb.WriteString(" WHERE ")
	for i, pk := range t.pkCols {
		if i > 0 {
			sb.WriteString(" AND ")
		}
		sb.WriteString(`"`)
		sb.WriteString(pk.name)
		sb.WriteString(`" = ?`)
	}
	sb.WriteString(";")

	fullCols := make([]*colInfo, len(setCols)+len(t.pkCols))
	copy(fullCols, setCols)
	copy(fullCols[len(setCols):], t.pkCols)

	sql := sb.String()
	plan := &sqlPlan{sql: sql, cols: fullCols}
	newMap := make(map[uint64]*sqlPlan, len(current)+1)
	for k, v := range current {
		newMap[k] = v
	}
	newMap[mask] = plan
	t.updateRowPlans.Store(&newMap)
	return sql, fullCols
}

func (t *tableInfo) getSelectSQL(qb *queryBuffer) string {
	b := qb.buf
	if p := t.selectCache.Load(); p != nil {
		if sql, ok := (*p)[string(b)]; ok {
			return sql
		}
	}

	key := string(b)
	t.selectMu.Lock()
	defer t.selectMu.Unlock()
	p := t.selectCache.Load()
	var current map[string]string
	if p != nil {
		current = *p
	}
	if sql, ok := current[key]; ok {
		return sql
	}
	newMap := make(map[string]string, len(current)+1)
	for k, v := range current {
		newMap[k] = v
	}
	newMap[key] = key
	t.selectCache.Store(&newMap)
	return key
}

func (t *tableInfo) getUpdateSQL(qb *queryBuffer) string {
	b := qb.buf
	if p := t.updateSQLCache.Load(); p != nil {
		if sql, ok := (*p)[string(b)]; ok {
			return sql
		}
	}

	key := string(b)
	t.cacheMu.Lock()
	defer t.cacheMu.Unlock()
	p := t.updateSQLCache.Load()
	var current map[string]string
	if p != nil {
		current = *p
	}
	if sql, ok := current[key]; ok {
		return sql
	}
	newMap := make(map[string]string, len(current)+1)
	for k, v := range current {
		newMap[k] = v
	}
	newMap[key] = key
	t.updateSQLCache.Store(&newMap)
	return key
}

func (t *tableInfo) getFastUpdateEqualSQL(numSets uint8, col0Name, col1Name, eqColName string) string {
	c0, ok0 := t.colByName[col0Name]
	if !ok0 {
		return ""
	}
	cEq, okEq := t.colByName[eqColName]
	if !okEq {
		return ""
	}
	var key uint32
	if numSets == 1 {
		key = uint32(c0.colIdx) | (uint32(cEq.colIdx) << 16) | (1 << 24)
	} else if numSets == 2 {
		c1, ok1 := t.colByName[col1Name]
		if !ok1 {
			return ""
		}
		key = uint32(c0.colIdx) | (uint32(c1.colIdx) << 8) | (uint32(cEq.colIdx) << 16) | (2 << 24)
	} else {
		return ""
	}

	if p := t.fastUpdatePlans.Load(); p != nil {
		if sql, ok := (*p)[key]; ok {
			return sql
		}
	}

	t.cacheMu.Lock()
	defer t.cacheMu.Unlock()
	p := t.fastUpdatePlans.Load()
	var current map[uint32]string
	if p != nil {
		current = *p
	}
	if sql, ok := current[key]; ok {
		return sql
	}

	var sb strings.Builder
	sb.WriteString(`UPDATE "`)
	sb.WriteString(t.name)
	sb.WriteString(`" SET "`)
	sb.WriteString(col0Name)
	sb.WriteString(`" = ?`)
	if numSets == 2 {
		sb.WriteString(`, "`)
		sb.WriteString(col1Name)
		sb.WriteString(`" = ?`)
	}
	sb.WriteString(` WHERE "`)
	sb.WriteString(eqColName)
	sb.WriteString(`" = ?;`)

	sql := sb.String()
	newMap := make(map[uint32]string, len(current)+1)
	for k, v := range current {
		newMap[k] = v
	}
	newMap[key] = sql
	t.fastUpdatePlans.Store(&newMap)
	return sql
}

func (col *colInfo) extractValue(base unsafe.Pointer) (any, bool, error) {
	fieldPtr := unsafe.Add(base, col.offset)

	if col.isPtr {
		ptr := *(*unsafe.Pointer)(fieldPtr)
		if ptr == nil {
			return nil, false, nil
		}
		if col.isEncoder {
			enc := reflect.NewAt(col.typ, fieldPtr).Elem().Interface().(FieldEncoder)
			v, err := enc.EncodeDema()
			return v, true, err
		}
		switch col.elemKind {
		case reflect.String:
			return *(*string)(ptr), true, nil
		case reflect.Int:
			return int64(*(*int)(ptr)), true, nil
		case reflect.Int64:
			return *(*int64)(ptr), true, nil
		case reflect.Int32:
			return int64(*(*int32)(ptr)), true, nil
		case reflect.Int16:
			return int64(*(*int16)(ptr)), true, nil
		case reflect.Int8:
			return int64(*(*int8)(ptr)), true, nil
		case reflect.Uint:
			return int64(*(*uint)(ptr)), true, nil
		case reflect.Uint64:
			return int64(*(*uint64)(ptr)), true, nil
		case reflect.Uint32:
			return int64(*(*uint32)(ptr)), true, nil
		case reflect.Uint16:
			return int64(*(*uint16)(ptr)), true, nil
		case reflect.Uint8:
			return int64(*(*uint8)(ptr)), true, nil
		case reflect.Bool:
			return *(*bool)(ptr), true, nil
		case reflect.Float64:
			return *(*float64)(ptr), true, nil
		case reflect.Float32:
			return float64(*(*float32)(ptr)), true, nil
		default:
			return reflect.NewAt(col.typ, fieldPtr).Elem().Interface(), true, nil
		}
	}



	if col.isEncoder {
		enc := reflect.NewAt(col.typ, fieldPtr).Interface().(FieldEncoder)
		v, err := enc.EncodeDema()
		return v, true, err
	}

	if col.isRuneSlice {
		return string(*(*[]rune)(fieldPtr)), true, nil
	}

	switch col.kind {
	case reflect.Int:
		v := *(*int)(fieldPtr)
		if uint(v) < 256 {
			return boxedInt64s[v], true, nil
		}
		return int64(v), true, nil
	case reflect.Int64:
		v := *(*int64)(fieldPtr)
		if uint64(v) < 256 {
			return boxedInt64s[v], true, nil
		}
		return v, true, nil
	case reflect.Int32:
		v := *(*int32)(fieldPtr)
		if uint32(v) < 256 {
			return boxedInt64s[v], true, nil
		}
		return int64(v), true, nil
	case reflect.Int16:
		v := *(*int16)(fieldPtr)
		if uint16(v) < 256 {
			return boxedInt64s[v], true, nil
		}
		return int64(v), true, nil
	case reflect.Int8:
		v := *(*int8)(fieldPtr)
		if v >= 0 {
			return boxedInt64s[v], true, nil
		}
		return int64(v), true, nil
	case reflect.Uint:
		v := *(*uint)(fieldPtr)
		if v < 256 {
			return boxedInt64s[v], true, nil
		}
		return int64(v), true, nil
	case reflect.Uint64:
		v := *(*uint64)(fieldPtr)
		if v < 256 {
			return boxedInt64s[v], true, nil
		}
		return int64(v), true, nil
	case reflect.Uint32:
		v := *(*uint32)(fieldPtr)
		if v < 256 {
			return boxedInt64s[v], true, nil
		}
		return int64(v), true, nil
	case reflect.Uint16:
		v := *(*uint16)(fieldPtr)
		if v < 256 {
			return boxedInt64s[v], true, nil
		}
		return int64(v), true, nil
	case reflect.Uint8:
		v := *(*uint8)(fieldPtr)
		return boxedInt64s[v], true, nil
	case reflect.Bool:
		if *(*bool)(fieldPtr) {
			return boxedTrue, true, nil
		}
		return boxedFalse, true, nil
	case reflect.Float64:
		return *(*float64)(fieldPtr), true, nil
	case reflect.Float32:
		return float64(*(*float32)(fieldPtr)), true, nil
	case reflect.String:
		return *(*string)(fieldPtr), true, nil
	default:
		return reflect.NewAt(col.typ, fieldPtr).Elem().Interface(), true, nil
	}
}
