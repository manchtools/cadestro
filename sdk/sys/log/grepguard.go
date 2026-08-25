package log

import (
	"strconv"
	"strings"
)

func isPathologicalGrepPattern(p string) string {

	unbounded := 0
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == '\\' && i+1 < len(p) {
			i++
			continue
		}
		switch c {
		case '*', '+':
			unbounded++
		case '{':
			if quantifierUnbounded(p[i:]) {
				unbounded++
			}
		}
	}
	if unbounded > 5 {
		return "too many unbounded quantifiers (max 5)"
	}

	depth := 0
	type groupState struct {
		start         int
		hasAlt        bool
		hasInnerQuant bool
	}
	var stack []groupState
	for i := 0; i < len(p); i++ {
		c := p[i]

		if c == '\\' && i+1 < len(p) {
			i++
			continue
		}
		switch c {
		case '(':
			stack = append(stack, groupState{start: i})
			depth++
		case ')':
			if depth == 0 {
				continue
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			depth--

			if len(stack) > 0 {
				if top.hasInnerQuant {
					stack[len(stack)-1].hasInnerQuant = true
				}
				if top.hasAlt {
					stack[len(stack)-1].hasAlt = true
				}
			}

			if i+1 < len(p) {
				next := p[i+1]
				if next == '*' || next == '+' || (next == '{' && quantifierUnbounded(p[i+1:])) {
					if top.hasInnerQuant {
						return "nested unbounded quantifier (catastrophic backtracking shape)"
					}
					if top.hasAlt {
						return "alternation under unbounded quantifier (catastrophic backtracking shape)"
					}
				}

				if next == '{' && top.hasInnerQuant {
					if _, hi, ok := boundedRepeatBounds(p[i+1:]); ok && hi >= 2 {
						return "bounded repetition of an unbounded group (catastrophic backtracking shape)"
					}
				}
			}
		case '|':
			if depth > 0 {
				stack[len(stack)-1].hasAlt = true
			}
		case '*', '+':
			if depth > 0 {
				stack[len(stack)-1].hasInnerQuant = true
			}
		case '{':
			if j := strings.IndexByte(p[i:], '}'); j > 0 && quantifierUnbounded(p[i:]) {
				if depth > 0 {
					stack[len(stack)-1].hasInnerQuant = true
				}
				i += j
			}
		}
	}
	return ""
}

func boundedRepeatBounds(p string) (lo, hi int, ok bool) {
	if len(p) == 0 || p[0] != '{' {
		return 0, 0, false
	}
	j := strings.IndexByte(p, '}')
	if j <= 0 {
		return 0, 0, false
	}
	body := p[1:j]
	if strings.HasSuffix(body, ",") {
		return 0, 0, false
	}
	k := strings.IndexByte(body, ',')
	if k < 0 {

		n, err := strconv.Atoi(strings.TrimSpace(body))
		if err != nil {
			return 0, 0, false
		}
		return n, n, true
	}

	n, err := strconv.Atoi(strings.TrimSpace(body[:k]))
	if err != nil {
		return 0, 0, false
	}
	m, err := strconv.Atoi(strings.TrimSpace(body[k+1:]))
	if err != nil {
		return 0, 0, false
	}
	return n, m, true
}

func quantifierUnbounded(p string) bool {
	if len(p) == 0 || p[0] != '{' {
		return false
	}
	j := strings.IndexByte(p, '}')
	if j <= 0 {
		return false
	}
	body := p[1:j]
	if !strings.Contains(body, ",") {
		return false
	}
	parts := strings.SplitN(body, ",", 2)
	return len(parts) == 2 && parts[1] == ""
}
