package resp

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// ErrProtocol is wrapped by every error caused by bytes that are not valid RESP.
// Errors from the underlying reader (io.EOF, a closed connection) are returned as they are.
var ErrProtocol = errors.New("protocol error")

// maxDepth limits how deeply arrays may nest. Clients only ever send flat
// arrays; the limit stops a stream like "*1\r\n*1\r\n*1\r\n..." from growing
// the stack until the process dies.
const maxDepth = 32

// bulkChunk is the largest bulk string we allocate up front. Bigger ones grow
// as their bytes actually arrive, so a client that only sends a header like
// "$999999999999\r\n" cannot make us allocate memory it never fills.
const bulkChunk = 64 * 1024

// Reader reads RESP values from a buffered stream.
type Reader struct {
	r *bufio.Reader
}

// NewReader returns a Reader that reads from r.
func NewReader(r *bufio.Reader) *Reader {
	return &Reader{r: r}
}

func protoErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrProtocol, fmt.Sprintf(format, args...))
}

// Read reads exactly one complete value. TCP delivers a byte stream, so the
// value may arrive over many reads; Read keeps reading until it has all of it.
func (r *Reader) Read() (Value, error) {
	return r.read(0)
}

func (r *Reader) read(depth int) (Value, error) {
	if depth > maxDepth {
		return Value{}, protoErr("arrays nested deeper than %d", maxDepth)
	}
	b, err := r.r.ReadByte()
	if err != nil {
		return Value{}, err
	}
	switch Type(b) {
	case SimpleString, Error:
		line, err := r.readLine()
		if err != nil {
			return Value{}, err
		}
		return Value{Type: Type(b), Str: string(line)}, nil
	case Integer:
		line, err := r.readLine()
		if err != nil {
			return Value{}, err
		}
		n, err := strconv.ParseInt(string(line), 10, 64)
		if err != nil {
			return Value{}, protoErr("invalid integer %q", line)
		}
		return IntegerValue(n), nil
	case BulkString:
		return r.readBulk()
	case Array:
		return r.readArray(depth)
	default:
		return Value{}, protoErr("unknown type byte %q", b)
	}
}

// readLine returns the bytes up to "\r\n", without them. The slice is only
// valid until the next read.
func (r *Reader) readLine() ([]byte, error) {
	line, err := r.r.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return nil, protoErr("line too long")
	}
	if err != nil {
		// The type byte was already read, so any EOF here is mid-message.
		return nil, unexpectedEOF(err)
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return nil, protoErr("line not terminated by CRLF")
	}
	return line[:len(line)-2], nil
}

// readLength reads the length line of a bulk string or array. It returns -1
// for a null value and rejects any other negative number.
func (r *Reader) readLength() (int64, error) {
	line, err := r.readLine()
	if err != nil {
		return 0, err
	}
	if len(line) == 2 && line[0] == '-' && line[1] == '1' {
		return -1, nil
	}
	if len(line) == 0 || len(line) > 18 {
		return 0, protoErr("invalid length %q", line)
	}
	var n int64
	for _, c := range line {
		if c < '0' || c > '9' {
			return 0, protoErr("invalid length %q", line)
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

func (r *Reader) readBulk() (Value, error) {
	n, err := r.readLength()
	if err != nil {
		return Value{}, err
	}
	if n == -1 {
		return NullValue(), nil
	}

	// Read exactly n bytes using the length. The data may itself contain
	// "\r\n", so we never scan it for a terminator.
	var data []byte
	if n <= bulkChunk {
		data = make([]byte, n)
		if _, err := io.ReadFull(r.r, data); err != nil {
			return Value{}, unexpectedEOF(err)
		}
	} else {
		var buf bytes.Buffer
		buf.Grow(bulkChunk)
		if _, err := io.CopyN(&buf, r.r, n); err != nil {
			return Value{}, unexpectedEOF(err)
		}
		data = buf.Bytes()
	}

	if err := r.expectCRLF(); err != nil {
		return Value{}, err
	}
	return Value{Type: BulkString, Bulk: data}, nil
}

func (r *Reader) readArray(depth int) (Value, error) {
	n, err := r.readLength()
	if err != nil {
		return Value{}, err
	}
	if n == -1 {
		return NullArrayValue(), nil
	}
	// Like bulk strings, don't trust the header for the allocation size.
	vals := make([]Value, 0, min(n, 1024))
	for i := int64(0); i < n; i++ {
		v, err := r.read(depth + 1)
		if err != nil {
			return Value{}, unexpectedEOF(err)
		}
		vals = append(vals, v)
	}
	return Value{Type: Array, Array: vals}, nil
}

func (r *Reader) expectCRLF() error {
	cr, err := r.r.ReadByte()
	if err != nil {
		return unexpectedEOF(err)
	}
	lf, err := r.r.ReadByte()
	if err != nil {
		return unexpectedEOF(err)
	}
	if cr != '\r' || lf != '\n' {
		return protoErr("bulk string not terminated by CRLF")
	}
	return nil
}

// unexpectedEOF turns io.EOF in the middle of a value into io.ErrUnexpectedEOF,
// so callers can tell "client left between commands" from "client left mid-command".
func unexpectedEOF(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}
