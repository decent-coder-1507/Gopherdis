package resp

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

// maxKeptBuffer is the largest command buffer a Reader keeps between
// commands. After one huge command the memory is given back instead of
// staying with the connection for good.
const maxKeptBuffer = 1 << 20

// ReadCommand reads one client command and returns its parts. It is the
// fast path the server uses: the returned slices point into a buffer the
// Reader reuses, so they are only valid until the next call.
//
// A command is normally an array of bulk strings. Anything that doesn't
// start with '*' is read as an inline command: one line of words separated
// by spaces, which is what someone typing into telnet or nc sends. A null,
// empty or blank command returns no parts.
func (r *Reader) ReadCommand() ([][]byte, error) {
	b, err := r.r.ReadByte()
	if err != nil {
		return nil, err
	}
	if cap(r.buf) > maxKeptBuffer {
		r.buf = nil
	}
	r.buf = r.buf[:0]
	r.ends = r.ends[:0]

	if Type(b) != Array {
		r.r.UnreadByte()
		if err := r.readInline(); err != nil {
			return nil, unexpectedEOF(err)
		}
		return r.parts(), nil
	}

	n, err := r.readLength()
	if err != nil {
		return nil, err
	}
	if n > r.maxArray() {
		return nil, protoErr("invalid multibulk length")
	}
	for i := int64(0); i < n; i++ {
		if err := r.readCommandPart(); err != nil {
			return nil, unexpectedEOF(err)
		}
	}
	return r.parts(), nil
}

// parts slices r.buf into the command's parts. It runs only once the whole
// command has been read, because appending to buf may have moved it.
func (r *Reader) parts() [][]byte {
	r.args = r.args[:0]
	start := 0
	for _, end := range r.ends {
		r.args = append(r.args, r.buf[start:end:end])
		start = end
	}
	return r.args
}

// readCommandPart reads one bulk string of a command and appends its bytes to r.buf.
func (r *Reader) readCommandPart() error {
	b, err := r.r.ReadByte()
	if err != nil {
		return err
	}
	if Type(b) != BulkString {
		return protoErr("expected '$', got %q", b)
	}
	n, err := r.readLength()
	if err != nil {
		return err
	}
	if n < 0 {
		return protoErr("null bulk string in a command")
	}
	if n > r.maxBulk() {
		return protoErr("invalid bulk length")
	}

	// Grow buf in steps of at most bulkChunk, so a header that claims a huge
	// length can't make us allocate more than the client actually sends.
	for remaining := n; remaining > 0; {
		step := int(min(remaining, bulkChunk))
		start := len(r.buf)
		r.buf = append(r.buf, make([]byte, step)...)
		if _, err := io.ReadFull(r.r, r.buf[start:]); err != nil {
			return err
		}
		remaining -= int64(step)
	}
	r.ends = append(r.ends, len(r.buf))
	return r.expectCRLF()
}

// readInline reads one line ending in "\n" (a "\r" before it is dropped)
// and stores its space-separated words as the command's parts.
func (r *Reader) readInline() error {
	var line []byte
	for {
		chunk, err := r.r.ReadSlice('\n')
		if len(line)+len(chunk) > maxInlineLen {
			return protoErr("too big inline request")
		}
		line = append(line, chunk...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil {
			return err
		}
		break
	}
	line = bytes.TrimSuffix(line[:len(line)-1], []byte("\r"))
	for _, word := range bytes.Fields(line) {
		r.buf = append(r.buf, word...)
		r.ends = append(r.ends, len(r.buf))
	}
	return nil
}
