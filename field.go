package dema

type OrderDirection string

const (
	Asc  OrderDirection = "ASC"
	Desc OrderDirection = "DESC"
)

type Field[T any, V any] struct {
	Name string
}

func NewField[T any, V any](columnName string) Field[T, V] {
	return Field[T, V]{Name: columnName}
}

func (f Field[T, V]) ColumnName() string {
	return f.Name
}

func (f Field[T, V]) String() string {
	return f.Name
}
