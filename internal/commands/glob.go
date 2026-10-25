package commands

// globMatch reports whether s matches the Redis-style glob pattern:
//
//   - "*" matches any run of bytes, including none
//   - "?" matches exactly one byte
//   - "[abc]" matches one of a, b, c; "[a-z]" a range; "[^abc]" anything else
//   - "\x" matches the byte x itself (for a literal *, ?, [ or \)
//
// It works on bytes, like Redis. A '*' is handled by remembering where it
// was and retrying from one byte further on a mismatch, so the time is at
// most len(pattern) * len(s), with no recursion.
func globMatch(pattern, s string) bool {
	p, i := 0, 0
	starP, starI := -1, 0
	for i < len(s) {
		if p < len(pattern) {
			switch c := pattern[p]; c {
			case '*':
				starP, starI = p, i
				p++
				continue
			case '?':
				p++
				i++
				continue
			case '[':
				if ok, next := matchClass(pattern, p, s[i]); ok {
					p, i = next, i+1
					continue
				}
			case '\\':
				lit := byte('\\')
				next := p + 1
				if p+1 < len(pattern) {
					lit, next = pattern[p+1], p+2
				}
				if lit == s[i] {
					p, i = next, i+1
					continue
				}
			default:
				if c == s[i] {
					p++
					i++
					continue
				}
			}
		}
		// Mismatch: let the last '*' swallow one more byte and try again.
		if starP < 0 {
			return false
		}
		starI++
		p, i = starP+1, starI
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

// matchClass matches c against the [...] class starting at pattern[start].
// It returns whether c is in the class and the index just after the
// class. Like Redis, a class with no closing ']' runs to the end of the
// pattern.
func matchClass(pattern string, start int, c byte) (bool, int) {
	p := start + 1
	negate := p < len(pattern) && pattern[p] == '^'
	if negate {
		p++
	}
	found := false
	for p < len(pattern) && pattern[p] != ']' {
		switch {
		case pattern[p] == '\\' && p+1 < len(pattern):
			if pattern[p+1] == c {
				found = true
			}
			p += 2
		case p+2 < len(pattern) && pattern[p+1] == '-' && pattern[p+2] != ']':
			lo, hi := pattern[p], pattern[p+2]
			if lo > hi {
				lo, hi = hi, lo
			}
			if lo <= c && c <= hi {
				found = true
			}
			p += 3
		default:
			if pattern[p] == c {
				found = true
			}
			p++
		}
	}
	if p < len(pattern) {
		p++ // skip ']'
	}
	return found != negate, p
}
