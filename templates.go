package main

import (
	"bufio"
	"log"
	"path"
	"strconv"
	"strings"

	lua "github.com/Shopify/go-lua"
	"github.com/d4l3k/wikigopher/wikitext"
	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
)

var templateFuncs = map[string]func(attrs []wikitext.Attribute) (interface{}, error){
	"ifeq": func(attrs []wikitext.Attribute) (interface{}, error) {
		if len(attrs) < 3 || len(attrs) > 4 {
			return nil, errors.Errorf("must have 3 or 4 arguments to #ifeq, got %d", len(attrs))
		}

		var trueVal interface{}
		var falseVal interface{}
		if len(attrs) >= 3 {
			trueVal = attrs[2].Key
		}
		if len(attrs) == 4 {
			falseVal = attrs[3].Key
		}

		a := wikitext.Concat(attrs[0].Key)
		b := wikitext.Concat(attrs[1].Key)
		aVal, err := strconv.ParseFloat(a, 64)
		if err == nil {
			bVal, err := strconv.ParseFloat(b, 64)
			if err == nil {
				if aVal == bVal {
					return trueVal, nil
				}
				return falseVal, nil
			}
		}

		if a == b {
			return trueVal, nil
		}
		return falseVal, nil
	},

	"if": func(attrs []wikitext.Attribute) (interface{}, error) {
		if len(attrs) < 2 || len(attrs) > 3 {
			return nil, errors.Errorf("must have 2 or 3 arguments to #if, got %d", len(attrs))
		}

		a := strings.TrimSpace(wikitext.Concat(attrs[0].Key))
		if len(a) > 0 {
			return attrs[1].Key, nil
		}
		if len(attrs) > 2 {
			return attrs[2].Key, nil
		}
		return nil, nil
	},

	"invoke": func(attrs []wikitext.Attribute) (interface{}, error) {
		if len(attrs) < 1 {
			return nil, errors.Errorf("must have at least one attribute")
		}

		module, err := loadModule(wikitext.Concat(attrs[0]))
		if err != nil {
			return nil, err
		}
		methodName := wikitext.Concat(attrs[1])

		l := lua.NewState()

		lua.OpenLibraries(l)
		/*
			lua.BaseOpen(l)
			lua.StringOpen(l)
			lua.MathOpen(l)
			lua.TableOpen(l)
			lua.Bit32Open(l)
		*/

		l.Global("require")
		l.SetGlobal("oldRequire")

		l.Register("require", func(l *lua.State) int {
			moduleName := lua.CheckString(l, 0)
			log.Printf("require called! %q", moduleName)

			if moduleName == "libraryUtil" {
				if err := lua.DoFile(l, path.Join("lua", moduleName+".lua")); err != nil {
					lua.Errorf(l, errors.Wrapf(err, "executing module %q", moduleName).Error())
					return 0
				}
				return lua.MultipleReturns
			} else if strings.HasPrefix(moduleName, "Module:") {
				body, err := articleBody(moduleName)
				if err != nil {
					lua.Errorf(l, errors.Wrapf(err, "loading module %q", moduleName).Error())
				}
				if err := lua.DoString(l, body); err != nil {
					lua.Errorf(l, errors.Wrapf(err, "executing module %q", moduleName).Error())
					return 0
				}
				spew.Dump(l.ToValue(0))
				spew.Dump(l.ToValue(-1))
				return lua.MultipleReturns
			}

			l.Global("oldRequire")
			l.PushString(moduleName)
			l.Call(1, 1)
			return 1
		})
		if err := lua.DoString(l, module); err != nil {
			return nil, errors.Wrapf(err, "DoString")
		}
		log.Printf("module loaded")
		l.Field(-1, methodName)
		l.PushString("args")
		if err := l.ProtectedCall(1, 1, 0); err != nil {
			return nil, errors.Wrapf(err, "calling %q", methodName)
		}
		return lua.CheckString(l, 0), nil
	},
}

func loadModule(name string) (string, error) {
	name = "Module:" + name
	return articleBody(name)
}

func stripComments(code string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(code))
	var b strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "--") {
			continue
		}
		b.WriteString(line)
		b.WriteRune('\n')
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return b.String(), nil
}

func articleBody(name string) (string, error) {
	articleMeta, err := fetchArticle(name)
	if err != nil {
		return "", err
	}
	p, err := readArticle(articleMeta)
	if err != nil {
		return "", err
	}
	return p.Text, nil
}

func templateFuncHandler(name string, attrs []wikitext.Attribute) (interface{}, error) {
	f, ok := templateFuncs[name]
	if ok {
		v, err := f(attrs)
		if err != nil {
			log.Printf("Error executing func %q: %+v", name, err)
			return nil, err
		}
		return v, nil
	}
	return nil, errors.Errorf("unknown func: %q, args: %v", name, attrs)
}

func (p page) templateHandler(name string, attrs []wikitext.Attribute) (interface{}, error) {
	if name == "NAMESPACE" {
		parts := strings.Split(p.Title, ":")
		if len(parts) > 1 {
			return parts[0], nil
		}
		return nil, nil

	} else if name == "NUMBEROFARTICLES" {
		mu.Lock()
		defer mu.Unlock()

		return len(mu.offsets), nil

	} else if strings.HasPrefix(name, "#") {
		parts := strings.SplitN(name, ":", 2)
		if len(parts) > 1 {
			attrs = append([]wikitext.Attribute{
				{Key: parts[1]},
			}, attrs...)
		}
		return templateFuncHandler(parts[0][1:], attrs)
	}

	/*
		templateBody, err := articleBody("Template:" + name)
		if err != nil {
			return nil, errors.Wrapf(err, "unknown template: %q, args: %v", name, attrs)
		}

		body, err := wikitext.Convert(
			[]byte(templateBody),
			wikitext.TemplateHandler(p.templateHandler),
		)
		if err != nil {
			return nil, err
		}
		doc, err := html.Parse(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}

		return doc, nil
	*/

	return nil, errors.Errorf("unknown template: %q, args: %v", name, attrs)
}
