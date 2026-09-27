package dema

type FieldEncoder interface {
	EncodeDema() (any, error)
}

type FieldDecoder interface {
	DecodeDema(any) error
}
