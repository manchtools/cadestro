package archtest

import (
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

// identName returns the most specific name an expression is known by:
// the identifier for a bare ident, or the trailing selector field
// (foo.Bar -> "Bar"). Anything else yields "".
func identName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	case *ast.StarExpr:
		return identName(x.X)
	case *ast.ParenExpr:
		return identName(x.X)
	}
	return ""
}

// isPresenceComparand reports whether an operand is a nil / empty-string
// / zero literal — i.e. the comparison is a presence/absence check, not a
// secret-value comparison. Constant-time compares only matter when both
// sides carry real secret material.
func isPresenceComparand(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == "nil"
	case *ast.BasicLit:
		switch x.Kind {
		case token.STRING:
			// `""` (or `` `` ``) — empty string literal.
			if s, err := strconv.Unquote(x.Value); err == nil {
				return s == ""
			}
		case token.INT, token.FLOAT:
			return x.Value == "0" || x.Value == "0.0"
		}
	case *ast.CallExpr:
		// []byte(nil), []byte("") — presence checks on byte slices.
		if len(x.Args) == 1 {
			return isPresenceComparand(x.Args[0])
		}
	}
	return false
}

// unquoteLit returns the string value of a STRING BasicLit, or "".
func unquoteLit(lit *ast.BasicLit) string {
	if lit == nil || lit.Kind != token.STRING {
		return ""
	}
	if s, err := strconv.Unquote(lit.Value); err == nil {
		return s
	}
	return ""
}

// secretNameRe matches identifiers that hold secret material. Bare "sig"
// and "mac" from the original sweep regex are intentionally dropped:
// "sig" collides with "assign", and "mac" is too short to be specific;
// the full "signature" / "hmac" forms are kept instead. A match is only a
// violation when it is NOT metadata about the secret (see
// secretMetaSuffixes) and NOT a presence check.
var secretNameRe = regexp.MustCompile(`(?i)(token|secret|hmac|signature|fingerprint|password|passwd|digest|apikey)`)

// secretMetaSuffixes name fields that describe a secret rather than carry
// its bytes (TokenType, SessionVersion, KeyID, ...). Comparing these with
// == is fine — they are not timing-sensitive secret material.
var secretMetaSuffixes = []string{
	"type", "kind", "id", "name", "len", "length", "count", "version",
	"expiry", "expiresat", "at", "format", "algorithm", "algo", "method",
	"status", "enabled", "disabled", "index", "idx", "field", "size",
}

// looksLikeSecretOperand reports whether an operand names secret material
// that must be compared in constant time (matches the secret regex and is
// not a metadata field).
func looksLikeSecretOperand(e ast.Expr) bool {
	name := identName(e)
	if name == "" || !secretNameRe.MatchString(name) {
		return false
	}
	lower := strings.ToLower(name)
	for _, suf := range secretMetaSuffixes {
		if strings.HasSuffix(lower, suf) {
			return false
		}
	}
	return true
}
