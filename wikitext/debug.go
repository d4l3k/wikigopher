package wikitext

import (
	"log"
	"reflect"
	"runtime"
)

func debugRules(compute bool) {
	for _, rule := range g.rules {
		debugExpr(rule.expr, compute)
	}
}

func debugExpr(e interface{}, compute bool) {
	switch e := e.(type) {
	case *actionExpr:
		oldRun := e.run
		name := getFunctionName(e.run)
		e.run = func(p *parser) (interface{}, error) {
			log.Printf("run %q", name)
			stack := p.vstack[len(p.vstack)-1]
			r := debugRun{
				Name:  name,
				Stack: stack,
				Text:  string(p.cur.text),
			}
			if compute {
				p.vstack[len(p.vstack)-1] = shuckStack(stack)
				val, err := oldRun(p)
				if err != nil {
					return nil, err
				}
				p.vstack[len(p.vstack)-1] = stack
				r.Value = val
			}

			return r, nil
		}
		debugExpr(e.expr, compute)

	case *labeledExpr:
		debugExpr(e.expr, compute)

	case *expr:
		debugExpr(e.expr, compute)

	case *andExpr:
		debugExpr(e.expr, compute)

	case *notExpr:
		debugExpr(e.expr, compute)

	case *zeroOrOneExpr:
		debugExpr(e.expr, compute)

	case *zeroOrMoreExpr:
		debugExpr(e.expr, compute)

	case *oneOrMoreExpr:
		debugExpr(e.expr, compute)

	case *seqExpr:
		for _, e := range e.exprs {
			debugExpr(e, compute)
		}

	case *choiceExpr:
		for _, e := range e.alternatives {
			debugExpr(e, compute)
		}

	case *ruleRefExpr, *litMatcher, *andCodeExpr, *charClassMatcher, *anyMatcher, *notCodeExpr, *stateCodeExpr:

	default:
		log.Fatalf("debugExpr: unsupported type %T: %#v", e, e)
	}
}

// from https://stackoverflow.com/questions/7052693/how-to-get-the-name-of-a-function-in-go
func getFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

type debugRun struct {
	Name  string
	Stack map[string]interface{}
	Text  string
	Value interface{}
}

func shuck(v interface{}) interface{} {
	switch v := v.(type) {
	case debugRun:
		return v.Value

	case []interface{}:
		return shuckArr(v)

	default:
		return v
	}
}

func shuckArr(arr []interface{}) []interface{} {
	var out []interface{}
	for _, val := range arr {
		out = append(out, shuck(val))
	}
	return out
}

func shuckStack(stack map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range stack {
		out[k] = shuck(v)
	}
	return out
}
