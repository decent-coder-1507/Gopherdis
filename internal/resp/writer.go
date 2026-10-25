package resp

import (
	"bufio"
	"io"
	"strconv"
)

// Writer writes RESP values to a buffered stream. Nothing reaches the
// underlying writer until Flush is called (or the buffer fills up).
type Writer struct {
	w *bufio.Writer
	// num is scratch space for formatting numbers without allocating.
	num []byte
}

// NewWriter returns a Writer that buffers output for w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: bufio.NewWriter(w), num: make([]byte, 0, 24)}
}

// Flush sends everything buffered so far.
func (w *Writer) Flush() error {
	return w.w.Flush()
}

// Buffered returns the number of bytes written but not yet flushed.
func (w *Writer) Buffered() int {
	return w.w.Buffered()
}

func (w *Writer) line(t Type, s string) error {
	w.w.WriteByte(byte(t))
	w.w.WriteString(s)
	_, err := w.w.WriteString("\r\n")
	return err
}

func (w *Writer) number(t Type, n int64) error {
	w.w.WriteByte(byte(t))
	w.num = strconv.AppendInt(w.num[:0], n, 10)
	w.w.Write(w.num)
	_, err := w.w.WriteString("\r\n")
	return err
}

// WriteSimple writes a simple string, e.g. +OK.
func (w *Writer) WriteSimple(s string) error { return w.line(SimpleString, s) }

// WriteError writes an error. msg is the text without the leading '-',
// e.g. "ERR unknown command 'FOO'".
func (w *Writer) WriteError(msg string) error { return w.line(Error, msg) }

// WriteInteger writes an integer, e.g. :42.
func (w *Writer) WriteInteger(n int64) error { return w.number(Integer, n) }

// WriteBulk writes b as a bulk string.
func (w *Writer) WriteBulk(b []byte) error {
	w.number(BulkString, int64(len(b)))
	w.w.Write(b)
	_, err := w.w.WriteString("\r\n")
	return err
}

// WriteBulkString writes s as a bulk string.
func (w *Writer) WriteBulkString(s string) error {
	w.number(BulkString, int64(len(s)))
	w.w.WriteString(s)
	_, err := w.w.WriteString("\r\n")
	return err
}

// WriteNull writes the nil bulk string, $-1.
func (w *Writer) WriteNull() error {
	_, err := w.w.WriteString("$-1\r\n")
	return err
}

// WriteNullArray writes the nil array, *-1.
func (w *Writer) WriteNullArray() error {
	_, err := w.w.WriteString("*-1\r\n")
	return err
}

// WriteArray writes the header of an array of n items. The caller then
// writes the n items with the other Write methods.
func (w *Writer) WriteArray(n int) error { return w.number(Array, int64(n)) }

// WriteValue writes any Value, including nested arrays.
func (w *Writer) WriteValue(v Value) error {
	switch v.Type {
	case SimpleString:
		return w.WriteSimple(v.Str)
	case Error:
		return w.WriteError(v.Str)
	case Integer:
		return w.WriteInteger(v.Int)
	case BulkString:
		if v.Null {
			return w.WriteNull()
		}
		return w.WriteBulk(v.Bulk)
	case Array:
		if v.Null {
			return w.WriteNullArray()
		}
		if err := w.WriteArray(len(v.Array)); err != nil {
			return err
		}
		for _, item := range v.Array {
			if err := w.WriteValue(item); err != nil {
				return err
			}
		}
		return nil
	default:
		return protoErr("cannot write unknown type %q", byte(v.Type))
	}
}
