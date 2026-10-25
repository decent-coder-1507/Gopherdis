package store

// List is a list of values that grows cheaply at both ends.
//
// A plain slice would make every LPUSH copy the whole list to make room at
// the front. Instead, items pushed to the front go into head in reverse
// order and items pushed to the back go into tail, so both pushes are
// appends. Index 0 is the last element of head (or tail[0] if head is empty).
type List struct {
	head [][]byte
	tail [][]byte
}

// NewList returns a list holding items, in order. It keeps the slice.
func NewList(items [][]byte) *List {
	return &List{tail: items}
}

// Len returns the number of items.
func (l *List) Len() int {
	return len(l.head) + len(l.tail)
}

// PushFront adds v at the start.
func (l *List) PushFront(v []byte) {
	l.head = append(l.head, v)
}

// PushBack adds v at the end.
func (l *List) PushBack(v []byte) {
	l.tail = append(l.tail, v)
}

// at returns item i, with 0 <= i < Len().
func (l *List) at(i int) []byte {
	if i < len(l.head) {
		return l.head[len(l.head)-1-i]
	}
	return l.tail[i-len(l.head)]
}

// Slice returns a new slice holding items start through stop, both
// included, with 0 <= start <= stop < Len(). The item bytes are shared
// (they are never changed in place) but the returned slice is the
// caller's, so it stays valid after the list changes.
func (l *List) Slice(start, stop int) [][]byte {
	out := make([][]byte, 0, stop-start+1)
	for i := start; i <= stop; i++ {
		out = append(out, l.at(i))
	}
	return out
}

// Items returns all items in order, as a new slice.
func (l *List) Items() [][]byte {
	if l.Len() == 0 {
		return [][]byte{}
	}
	return l.Slice(0, l.Len()-1)
}
