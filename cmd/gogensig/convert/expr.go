package convert

import (
	"fmt"
	"strconv"

	"github.com/goplus/llcppg/ast"
	"github.com/goplus/llcppg/cmd/gogensig/errs"
)

type ExprWrap struct {
	e ast.Expr
}

func Expr(e ast.Expr) *ExprWrap {
	return &ExprWrap{e: e}
}

func (p *ExprWrap) ToInt() (int, error) {
	v, ok := p.e.(*ast.BasicLit)
	if ok && v.Kind == ast.IntLit {
		v, err := litToInt(v.Value)
		if err != nil {
			return 0, err
		}
		return int(v), nil
	}
	return 0, errs.NewCantConvertError(p.e, "int")
}

func (p *ExprWrap) ToFloat(bitSize int) (float64, error) {
	v, ok := p.e.(*ast.BasicLit)
	if ok && v.Kind == ast.FloatLit {
		return litToFloat(v.Value, bitSize)
	}
	return 0, errs.NewCantConvertError(v, "float")
}

func (p *ExprWrap) ToString() (string, error) {
	v, ok := p.e.(*ast.BasicLit)
	if ok && v.Kind == ast.StringLit {
		return litToString(v.Value)
	}
	return "", errs.NewCantConvertError(v, "string")
}

func (p *ExprWrap) ToChar() (int8, error) {
	v, ok := p.e.(*ast.BasicLit)
	if ok && v.Kind == ast.CharLit {
		var iV int8
		_, err := fmt.Sscan(v.Value, &iV)
		if err == nil {
			return iV, nil
		}
	}
	return 0, errs.NewCantConvertError(p.e, "char")
}

func (p *ExprWrap) IsVoid() bool {
	retType, ok := p.e.(*ast.BuiltinType)
	if ok && retType.Kind == ast.Void {
		return true
	}
	return false
}

// litToInt parses a string literal into an int64 value.
// It returns the value and the smallest IntType that can hold the value.
func litToInt(lit string) (int64, error) {
	return strconv.ParseInt(lit, 0, 64)
}

func litToUint(lit string) (uint64, error) {
	return strconv.ParseUint(lit, 0, 64)
}

func litToFloat(lit string, bitSize int) (float64, error) {
	return strconv.ParseFloat(lit, bitSize)
}

func litToString(lit string) (string, error) {
	return strconv.Unquote(lit)
}
