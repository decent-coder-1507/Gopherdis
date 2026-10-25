package resp

// Type is the first byte of a RESP message and says what kind of value follows.
type Type byte

// The RESP2 types.
const (
	SimpleString Type = '+'
	Error        Type = '-'
	Integer      Type = ':'
	BulkString   Type = '$'
	Array        Type = '*'
)

// Value is one RESP message. Which fields are used depends on Type:
// Str for simple strings and errors, Int for integers, Bulk for bulk strings
// and Array for arrays. Null marks the nil bulk string ($-1) and the nil
// array (*-1).
type Value struct {
	Type  Type
	Str   string
	Int   int64
	Bulk  []byte
	Array []Value
	Null  bool
}

// SimpleValue returns a simple string value.
func SimpleValue(s string) Value { return Value{Type: SimpleString, Str: s} }

// ErrorValue returns an error value. msg is the text after the '-'.
func ErrorValue(msg string) Value { return Value{Type: Error, Str: msg} }

// IntegerValue returns an integer value.
func IntegerValue(n int64) Value { return Value{Type: Integer, Int: n} }

// BulkValue returns a bulk string value holding s.
func BulkValue(s string) Value { return Value{Type: BulkString, Bulk: []byte(s)} }

// NullValue returns the nil bulk string ($-1).
func NullValue() Value { return Value{Type: BulkString, Null: true} }

// ArrayValue returns an array value holding vals.
func ArrayValue(vals ...Value) Value {
	if vals == nil {
		vals = []Value{}
	}
	return Value{Type: Array, Array: vals}
}

// NullArrayValue returns the nil array (*-1).
func NullArrayValue() Value { return Value{Type: Array, Null: true} }
