package dema

import (
	"strconv"
	"sync"

	"zombiezen.com/go/sqlite/sqlitex"
)

var execOptsPool = sync.Pool{
	New: func() any {
		return &sqlitex.ExecOptions{
			Args: make([]any, 0, 16),
		}
	},
}

func getExecOptions() *sqlitex.ExecOptions {
	return execOptsPool.Get().(*sqlitex.ExecOptions)
}

func putExecOptions(opts *sqlitex.ExecOptions) {
	opts.Args = opts.Args[:0]
	opts.ResultFunc = nil
	execOptsPool.Put(opts)
}

type queryBuffer struct {
	buf []byte
}

func (b *queryBuffer) WriteString(s string) {
	b.buf = append(b.buf, s...)
}

func (b *queryBuffer) WriteInt(n int) {
	b.buf = strconv.AppendInt(b.buf, int64(n), 10)
}

func (b *queryBuffer) Bytes() []byte {
	return b.buf
}

func (b *queryBuffer) Reset() {
	b.buf = b.buf[:0]
}

var qbPool = sync.Pool{
	New: func() any {
		return &queryBuffer{buf: make([]byte, 0, 512)}
	},
}

func getQueryBuffer() *queryBuffer {
	qb := qbPool.Get().(*queryBuffer)
	qb.Reset()
	return qb
}

func putQueryBuffer(qb *queryBuffer) {
	if cap(qb.buf) > 65536 {
		return
	}
	qbPool.Put(qb)
}
