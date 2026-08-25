package archtest

import (
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

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

func isPresenceComparand(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == "nil"
	case *ast.BasicLit:
		switch x.Kind {
		case token.STRING:

			if s, err := strconv.Unquote(x.Value); err == nil {
				return s == ""
			}
		case token.INT, token.FLOAT:
			return x.Value == "0" || x.Value == "0.0"
		}
	case *ast.CallExpr:

		if len(x.Args) == 1 {
			return isPresenceComparand(x.Args[0])
		}
	}
	return false
}

func unquoteLit(lit *ast.BasicLit) string {
	if lit == nil || lit.Kind != token.STRING {
		return ""
	}
	if s, err := strconv.Unquote(lit.Value); err == nil {
		return s
	}
	return ""
}

var secretNameRe = regexp.MustCompile(`(?i)(token|secret|hmac|signature|fingerprint|password|passwd|digest|apikey)`)

var secretMetaSuffixes = []string{
	"type", "kind", "id", "name", "len", "length", "count", "version",
	"expiry", "expiresat", "at", "format", "algorithm", "algo", "method",
	"status", "enabled", "disabled", "index", "idx", "field", "size",
}

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
