package resp

import "io"

// maxKeptBuffer is the largest command buffer a Reader keeps between
// commands. After one huge command the memory is given back instead of
// staying with the connection for good.
const maxKeptBuffer = 1 << 20

// ReadCommand reads one client command, an array of bulk strings, and
// returns its parts. It is the fast path the server uses: the returned
// slices point into a buffer the Reader reuses, so they are only valid until
// the next call. A null or empty array returns no parts.
func (r *Reader) ReadCommand() ([][]byte, error) {
	b, err := r.r.ReadByte()
	if err != nil {
		return nil, err
	}
	if Type(b) != Array {
		return nil, protoErr("expected '*', got %q", b)
	}
	n, err := r.readLength()
	if err != nil {
		return nil, err
	}

	if cap(r.buf) > maxKeptBuffer {
		r.buf = nil
	}
	r.buf = r.buf[:0]
	r.ends = r.ends[:0]
	for i := int64(0); i < n; i++ {
		if err := r.readCommandPart(); err != nil {
			return nil, unexpectedEOF(err)
		}
	}

	// Slice the parts only now: appending to buf may have moved it.
	r.args = r.args[:0]
	start := 0
	for _, end := range r.ends {
		r.args = append(r.args, r.buf[start:end:end])
		start = end
	}
	return r.args, nil
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
