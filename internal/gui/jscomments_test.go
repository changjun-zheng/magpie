package gui

import "testing"

// jsCodeBytes returns the number of bytes in src that are code. Comments and
// the whitespace around them are left out, so a comment costs nothing, and
// neither does formatting; the bytes inside a string, template literal or
// regex literal do count, spaces included, since they are the program's data.
// A comment is found the way a browser finds one: a // or /* inside a string,
// a template literal (its ${...} expressions included) or a regex literal is
// not a comment.
func jsCodeBytes(src []byte) int {
	_, n := stripJSComments(src)
	return n
}

// stripJSComments copies src with every comment byte replaced by a space,
// keeping a comment's newlines, and reports how many bytes are code.
func stripJSComments(src []byte) ([]byte, int) {
	out := make([]byte, len(src))
	copy(out, src)
	code := 0
	i, n := 0, len(src)
	// last is the last significant token, empty when an expression may
	// start. It tells a division from a regex literal's opening slash.
	last := ""
	// exprs is the brace depth of each open ${...} in a template literal.
	var exprs []int
	inTemplate := false

	for i < n {
		c := src[i]

		if inTemplate {
			switch {
			case c == '\\':
				code++
				i++
				if i < n {
					code++
					i++
				}
			case c == '`':
				code++
				inTemplate = false
				last = "literal"
				i++
			case c == '$' && i+1 < n && src[i+1] == '{':
				code += 2
				exprs = append(exprs, 0)
				inTemplate = false
				last = "("
				i += 2
			default:
				code++
				i++
			}
			continue
		}

		if c == '/' && i+1 < n {
			switch src[i+1] {
			case '/':
				j := i + 2
				for j < n && src[j] != '\n' && src[j] != '\r' {
					j++
				}
				blankJSComment(out[i:j])
				i = j
				continue
			case '*':
				j := i + 2
				for j+1 < n && !(src[j] == '*' && src[j+1] == '/') {
					j++
				}
				if j+1 < n {
					j += 2
				} else {
					j = n
				}
				blankJSComment(out[i:j])
				i = j
				continue
			}
		}

		switch {
		case c == '"' || c == '\'':
			j := scanJSString(src, i)
			code += j - i
			last = "literal"
			i = j
		case c == '`':
			code++
			inTemplate = true
			i++
		case c == '/':
			if jsRegexAllowed(last) {
				j := scanJSRegex(src, i)
				code += j - i
				last = "literal"
				i = j
			} else {
				code++
				last = "/"
				i++
			}
		case c == '{':
			code++
			if len(exprs) > 0 {
				exprs[len(exprs)-1]++
			}
			last = "{"
			i++
		case c == '}':
			code++
			if len(exprs) > 0 && exprs[len(exprs)-1] == 0 {
				exprs = exprs[:len(exprs)-1]
				inTemplate = true
				last = "("
			} else {
				if len(exprs) > 0 {
					exprs[len(exprs)-1]--
				}
				last = "}"
			}
			i++
		case c == '+' || c == '-':
			code++
			if i+1 < n && src[i+1] == c {
				code++
				i += 2
				last = "expr" // x++ / y is a division
			} else {
				i++
				last = string(c)
			}
		case c == '(' || c == '[' || c == ',' || c == ';' || c == ':' || c == '?' ||
			c == '=' || c == '!' || c == '&' || c == '|' || c == '*' || c == '%' ||
			c == '^' || c == '~' || c == '<' || c == '>':
			code++
			last = string(c)
			i++
		case c == ')' || c == ']':
			code++
			last = string(c)
			i++
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
			i++ // formatting is not code
		default:
			if isJSIdentByte(c) {
				j := i + 1
				for j < n && isJSIdentByte(src[j]) {
					j++
				}
				code += j - i
				last = string(src[i:j])
				i = j
			} else {
				code++
				i++
			}
		}
	}
	return out, code
}

// blankJSComment blanks a comment's bytes, keeping its newlines so the copy
// keeps the file's line numbers.
func blankJSComment(b []byte) {
	for i, c := range b {
		if c != '\n' && c != '\r' {
			b[i] = ' '
		}
	}
}

func scanJSString(src []byte, i int) int {
	q := src[i]
	j := i + 1
	for j < len(src) {
		switch src[j] {
		case '\\':
			j++
			if j < len(src) {
				j++
			}
		case q:
			return j + 1
		case '\n', '\r':
			return j // unterminated: stop at the line's end
		default:
			j++
		}
	}
	return j
}

func scanJSRegex(src []byte, i int) int {
	j := i + 1
	inClass := false
	for j < len(src) {
		c := src[j]
		if c == '\\' {
			j++
			if j < len(src) {
				j++
			}
			continue
		}
		if c == '\n' || c == '\r' {
			return j // unterminated: stop at the line's end
		}
		if inClass {
			if c == ']' {
				inClass = false
			}
			j++
			continue
		}
		switch c {
		case '[':
			inClass = true
			j++
		case '/':
			j++
			for j < len(src) && isJSIdentByte(src[j]) {
				j++
			}
			return j
		default:
			j++
		}
	}
	return j
}

// jsRegexAllowed says whether a slash after last starts a regex literal. A
// slash after a value (an identifier that is not a keyword, a literal, a
// number, or a closing bracket) is a division. Where the two cannot be told
// apart without parsing, this says division.
func jsRegexAllowed(last string) bool {
	switch last {
	case "",
		"(", "[", "{", ",", ";", ":", "?", "=", "!", "&", "|",
		"+", "-", "*", "%", "^", "~", "<", ">", "/",
		"return", "typeof", "instanceof", "in", "of", "new",
		"delete", "void", "case", "do", "else", "yield", "await",
		"throw", "default":
		return true
	}
	return false
}

func isJSIdentByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c >= '0' && c <= '9' || c == '_' || c == '$' || c >= 0x80
}

func TestStripJSComments(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string // comment bytes blanked, a block comment's newlines kept
		code int    // bytes of code: comments and formatting left out
	}{
		{
			name: "a line comment",
			src:  "a // b\nc",
			want: "a     \nc",
			code: 2,
		},
		{
			name: "a block comment",
			src:  "a/**/b",
			want: "a    b",
			code: 2,
		},
		{
			name: "a block comment keeps its newlines",
			src:  "a/*\n*/b",
			want: "a  \n  b",
			code: 2, // the newline inside the comment is the comment's
		},
		{
			name: "a string's // is not a comment",
			src:  `x = "a // b"`,
			want: `x = "a // b"`,
			code: 10,
		},
		{
			name: "a string's /* is not a comment",
			src:  `x = 'a /* b'`,
			want: `x = 'a /* b'`,
			code: 10,
		},
		{
			name: "a template's // is not a comment",
			src:  "x = `a // b`",
			want: "x = `a // b`",
			code: 10,
		},
		{
			name: "a comment in a template's expression is a comment",
			src:  "x = `a ${b /* c */} d`",
			want: "x = `a ${b        } d`",
			code: 12,
		},
		{
			name: "a regex's escaped slashes are not comments",
			src:  `x = /a\/\/b/`,
			want: `x = /a\/\/b/`,
			code: 10,
		},
		{
			name: "a regex's character class may hold /*",
			src:  `x = /[/*]/`,
			want: `x = /[/*]/`,
			code: 8,
		},
		{
			name: "a division then a comment",
			src:  "x = a / b // c",
			want: "x = a / b     ",
			code: 5,
		},
		{
			name: "a regex after return",
			src:  `return /a\/\/b/ // c`,
			want: `return /a\/\/b/     `,
			code: 14,
		},
		{
			name: "nested templates and their expressions",
			src:  "x = `a${`b${c /* d */}`}`",
			want: "x = `a${`b${c        }`}`",
			code: 15,
		},
		{
			name: "a comment-only line",
			src:  "// everything\n",
			want: "             \n",
			code: 0,
		},
		{
			name: "a whole comment line costs nothing",
			src:  "x = 1;\n// why it is one\n",
			want: "x = 1;\n                \n",
			code: 4,
		},
		{
			name: "an unterminated block comment",
			src:  "x = 1; /* no end",
			want: "x = 1;          ",
			code: 4,
		},
		{
			name: "a division after ++",
			src:  "a++ / b // c",
			want: "a++ / b     ",
			code: 5,
		},
		{
			name: "a regex after an operator",
			src:  `x = (/a\/b/).test(z)`,
			want: `x = (/a\/b/).test(z)`,
			code: 18,
		},
		{
			name: "a string in a template expression keeps its //",
			src:  "x = `${'a // b' /* c */}`",
			want: "x = `${'a // b'        }`",
			code: 15,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, code := stripJSComments([]byte(tc.src))
			if string(out) != tc.want {
				t.Errorf("stripped source:\n got %q\nwant %q", out, tc.want)
			}
			want := tc.code
			if code != want {
				t.Errorf("code bytes = %d, want %d", code, want)
			}
			if got := jsCodeBytes([]byte(tc.src)); got != want {
				t.Errorf("jsCodeBytes = %d, want %d", got, want)
			}
		})
	}
}
