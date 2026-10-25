package resp

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
)

func parse(t *testing.T, input string) Value {
	t.Helper()
	v, err := NewReader(bufio.NewReader(strings.NewReader(input))).Read()
	if err != nil {
		t.Fatalf("Read(%q): %v", input, err)
	}
	return v
}

func serialize(t *testing.T, v Value) string {
	t.Helper()
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.WriteValue(v); err != nil {
		t.Fatalf("WriteValue: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	return buf.String()
}

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Value
	}{
		{"ping", "*1\r\n$4\r\nPING\r\n", ArrayValue(BulkValue("PING"))},
		{"echo hello world", "*2\r\n$4\r\nECHO\r\n$11\r\nhello world\r\n",
			ArrayValue(BulkValue("ECHO"), BulkValue("hello world"))},
		{"get key", "*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n", ArrayValue(BulkValue("GET"), BulkValue("key"))},
		{"empty bulk", "$0\r\n\r\n", BulkValue("")},
		{"null bulk", "$-1\r\n", NullValue()},
		{"null array", "*-1\r\n", NullArrayValue()},
		{"empty array", "*0\r\n", ArrayValue()},
		{"nested array", "*2\r\n*2\r\n:1\r\n:2\r\n*1\r\n+hi\r\n",
			ArrayValue(ArrayValue(IntegerValue(1), IntegerValue(2)), ArrayValue(SimpleValue("hi")))},
		{"simple string", "+OK\r\n", SimpleValue("OK")},
		{"error", "-ERR unknown command\r\n", ErrorValue("ERR unknown command")},
		{"integer", ":42\r\n", IntegerValue(42)},
		{"negative integer", ":-7\r\n", IntegerValue(-7)},
		{"bulk with CRLF inside", "$6\r\na\r\nb\r\n\r\n", BulkValue("a\r\nb\r\n")},
		{"binary bulk", "$4\r\n\x00\xff\r\x01\r\n", BulkValue("\x00\xff\r\x01")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parse(t, tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestSerialize(t *testing.T) {
	tests := []struct {
		name string
		v    Value
		want string
	}{
		{"simple", SimpleValue("OK"), "+OK\r\n"},
		{"error", ErrorValue("ERR bad"), "-ERR bad\r\n"},
		{"integer", IntegerValue(42), ":42\r\n"},
		{"negative integer", IntegerValue(-1), ":-1\r\n"},
		{"bulk", BulkValue("hello"), "$5\r\nhello\r\n"},
		{"empty bulk", BulkValue(""), "$0\r\n\r\n"},
		{"null", NullValue(), "$-1\r\n"},
		{"null array", NullArrayValue(), "*-1\r\n"},
		{"array", ArrayValue(BulkValue("ECHO"), BulkValue("hi")), "*2\r\n$4\r\nECHO\r\n$2\r\nhi\r\n"},
		{"empty array", ArrayValue(), "*0\r\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := serialize(t, tc.v); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWriterHelpers(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.WriteArray(3)
	w.WriteBulkString("a")
	w.WriteNull()
	w.WriteInteger(0)
	if buf.Len() != 0 {
		t.Fatalf("data reached the connection before Flush: %q", buf.String())
	}
	w.Flush()
	if got, want := buf.String(), "*3\r\n$1\r\na\r\n$-1\r\n:0\r\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRoundTrip(t *testing.T) {
	values := []Value{
		SimpleValue("PONG"),
		ErrorValue("WRONGTYPE Operation against a key holding the wrong kind of value"),
		IntegerValue(-9223372036854775808),
		IntegerValue(9223372036854775807),
		BulkValue("binary \x00\r\n data"),
		BulkValue(""),
		NullValue(),
		NullArrayValue(),
		ArrayValue(),
		ArrayValue(BulkValue("SET"), BulkValue("k"), BulkValue("v"), ArrayValue(NullValue(), IntegerValue(3))),
		BulkValue(strings.Repeat("x", 200_000)), // bigger than one read chunk
	}
	for _, v := range values {
		got := parse(t, serialize(t, v))
		if !reflect.DeepEqual(got, v) {
			t.Errorf("round trip changed the value:\n got  %#v\n want %#v", got, v)
		}
	}
}

func TestBadInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"unknown first char", "?foo\r\n"},
		{"lowercase type", "x\r\n"},
		{"bulk too short", "$5\r\nhi\r\n"},
		{"bulk too long", "$1\r\nhello\r\n"},
		{"bulk length not a number", "$abc\r\nhello\r\n"},
		{"bulk length empty", "$\r\n\r\n"},
		{"bulk length with plus", "$+3\r\nabc\r\n"},
		{"negative length", "$-2\r\n"},
		{"negative array length", "*-5\r\n"},
		{"missing CRLF after bulk", "$3\r\nabcXY"},
		{"line ends with LF only", "+OK\n"},
		{"integer not a number", ":12a\r\n"},
		{"integer empty", ":\r\n"},
		{"integer overflow", ":99999999999999999999\r\n"},
		{"huge length", "$9999999999999999999999\r\n"},
		{"bad item in array", "*2\r\n$3\r\nGET\r\n!x\r\n"},
		{"truncated array", "*3\r\n$3\r\nGET\r\n"},
		{"truncated bulk", "$10\r\nabc"},
		{"truncated line", "+OK"},
		{"too deeply nested", strings.Repeat("*1\r\n", maxDepth+5) + ":1\r\n"},
		{"line too long", "+" + strings.Repeat("a", 10_000) + "\r\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewReader(bufio.NewReader(strings.NewReader(tc.input))).Read()
			if err == nil {
				t.Fatalf("Read(%q) returned no error", tc.input)
			}
			if !errors.Is(err, ErrProtocol) && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Errorf("got %v, want a protocol error or unexpected EOF", err)
			}
		})
	}
}

func TestEOFBetweenMessages(t *testing.T) {
	r := NewReader(bufio.NewReader(strings.NewReader("+OK\r\n")))
	if _, err := r.Read(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Read(); err != io.EOF {
		t.Errorf("got %v, want io.EOF after the last message", err)
	}
}

func TestHugeBulkHeaderDoesNotAllocate(t *testing.T) {
	// A header that claims ~1 TB but sends almost nothing must fail
	// quickly instead of trying to allocate the whole size.
	_, err := NewReader(bufio.NewReader(strings.NewReader("$999999999999\r\nabc"))).Read()
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("got %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestSplitAcrossReads(t *testing.T) {
	input := "*3\r\n$3\r\nSET\r\n$4\r\nname\r\n$6\r\nSrijoy\r\n"
	want := ArrayValue(BulkValue("SET"), BulkValue("name"), BulkValue("Srijoy"))

	// One byte per read: every header, length and payload is split.
	r := NewReader(bufio.NewReader(iotest.OneByteReader(strings.NewReader(input))))
	got, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}

	// Through a pipe, with the writer sending odd-sized pieces.
	pr, pw := io.Pipe()
	go func() {
		data := []byte(input)
		for len(data) > 0 {
			n := min(3, len(data))
			pw.Write(data[:n])
			data = data[n:]
		}
		pw.Close()
	}()
	got, err = NewReader(bufio.NewReader(pr)).Read()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pipe: got %#v, want %#v", got, want)
	}
}

func TestSeveralMessagesInOneRead(t *testing.T) {
	r := NewReader(bufio.NewReader(strings.NewReader("*1\r\n$4\r\nPING\r\n*1\r\n$4\r\nPING\r\n+OK\r\n")))
	for i := 0; i < 2; i++ {
		v, err := r.Read()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(v, ArrayValue(BulkValue("PING"))) {
			t.Fatalf("message %d: got %#v", i, v)
		}
	}
	if v, err := r.Read(); err != nil || v.Str != "OK" {
		t.Fatalf("third message: %#v, %v", v, err)
	}
}

func FuzzReader(f *testing.F) {
	f.Add([]byte("*1\r\n$4\r\nPING\r\n"))
	f.Add([]byte("$-1\r\n"))
	f.Add([]byte(":12\r\n"))
	f.Add([]byte("*2\r\n*1\r\n+a\r\n-b\r\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		r := NewReader(bufio.NewReader(bytes.NewReader(data)))
		for {
			v, err := r.Read()
			if err != nil {
				return
			}
			// Anything we can read we must be able to write and read back.
			var buf bytes.Buffer
			w := NewWriter(&buf)
			if err := w.WriteValue(v); err != nil {
				t.Fatal(err)
			}
			w.Flush()
			back, err := NewReader(bufio.NewReader(&buf)).Read()
			if err != nil {
				t.Fatalf("re-reading %q: %v", buf.Bytes(), err)
			}
			if !reflect.DeepEqual(back, v) {
				t.Fatalf("round trip changed %#v into %#v", v, back)
			}
		}
	})
}

func TestReadCommand(t *testing.T) {
	input := "*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$0\r\n\r\n" + // empty value
		"*2\r\n$4\r\nECHO\r\n$4\r\na\r\nb\r\n" + // CRLF inside data
		"*0\r\n" +
		"*-1\r\n" +
		"*1\r\n$4\r\nPING\r\n"
	r := NewReader(bufio.NewReader(iotest.OneByteReader(strings.NewReader(input))))
	want := [][]string{{"SET", "k", ""}, {"ECHO", "a\r\nb"}, {}, {}, {"PING"}}
	for i, w := range want {
		args, err := r.ReadCommand()
		if err != nil {
			t.Fatalf("command %d: %v", i, err)
		}
		if len(args) != len(w) {
			t.Fatalf("command %d: %q, want %q", i, args, w)
		}
		for j := range w {
			if string(args[j]) != w[j] {
				t.Fatalf("command %d part %d: %q, want %q", i, j, args[j], w[j])
			}
		}
	}
	if _, err := r.ReadCommand(); err != io.EOF {
		t.Fatalf("after the last command: %v, want io.EOF", err)
	}
}

func TestReadCommandPartsDontOverlap(t *testing.T) {
	// Appending a part must not be able to overwrite the one before it.
	big := strings.Repeat("y", 3*bulkChunk+7)
	input := "*3\r\n$1\r\nx\r\n$" + strconv.Itoa(len(big)) + "\r\n" + big + "\r\n$1\r\nz\r\n"
	args, err := NewReader(bufio.NewReader(strings.NewReader(input))).ReadCommand()
	if err != nil {
		t.Fatal(err)
	}
	if string(args[0]) != "x" || string(args[1]) != big || string(args[2]) != "z" {
		t.Fatalf("parts mixed up: %q / %d bytes / %q", args[0], len(args[1]), args[2])
	}
	args[0] = append(args[0], '!') // capacity is capped, so this must copy
	if string(args[1][:1]) != "y" {
		t.Fatal("appending to one part changed the next one")
	}
}

func TestReadCommandBadInput(t *testing.T) {
	for _, input := range []string{
		"+PING\r\n",
		":1\r\n",
		"*1\r\n:1\r\n",
		"*1\r\n$-1\r\n",
		"*2\r\n*1\r\n$1\r\na\r\n",
		"*1\r\n$3\r\nabcd\r\n",
		"*2\r\n$3\r\nGET\r\n",
		"*1\r\n$99999999999\r\nabc",
		"*x\r\n",
	} {
		_, err := NewReader(bufio.NewReader(strings.NewReader(input))).ReadCommand()
		if !errors.Is(err, ErrProtocol) && !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("%q: got %v, want a protocol error or unexpected EOF", input, err)
		}
	}
}

func FuzzReadCommand(f *testing.F) {
	f.Add([]byte("*2\r\n$3\r\nGET\r\n$1\r\nk\r\n"))
	f.Add([]byte("*1\r\n$0\r\n\r\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		// ReadCommand must agree with the general Read on every valid command.
		want, werr := NewReader(bufio.NewReader(bytes.NewReader(data))).Read()
		got, gerr := NewReader(bufio.NewReader(bytes.NewReader(data))).ReadCommand()
		if gerr != nil {
			return
		}
		if werr != nil {
			t.Fatalf("ReadCommand accepted %q but Read failed: %v", data, werr)
		}
		if len(got) != len(want.Array) {
			t.Fatalf("%q: %d parts, Read saw %d", data, len(got), len(want.Array))
		}
		for i := range got {
			if !bytes.Equal(got[i], want.Array[i].Bulk) {
				t.Fatalf("%q part %d: %q vs %q", data, i, got[i], want.Array[i].Bulk)
			}
		}
	})
}
