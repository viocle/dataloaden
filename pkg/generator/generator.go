package generator

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/pkg/errors"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/imports"
)

// Config is the configuration for the generator
type Config struct {
	FileNamePrefix         string
	LoaderName             string
	KeyType                string
	ValueType              string
	WorkingDirectory       string
	DisableCacheExpiration bool
}

type templateData struct {
	Package                string
	Name                   string
	KeyType                *goType
	ValType                *goType
	DisableCacheExpiration bool
}

type goType struct {
	Modifiers  string
	ImportPath string
	ImportName string
	Name       string
}

func (t *goType) String() string {
	if t.ImportName != "" {
		return t.Modifiers + t.ImportName + "." + t.Name
	}

	return t.Modifiers + t.Name
}

func (t *goType) IsPtr() bool {
	return strings.HasPrefix(t.Modifiers, "*")
}

func (t *goType) IsSlice() bool {
	return strings.HasPrefix(t.Modifiers, "[]")
}

var partsRe = regexp.MustCompile(`^([\[\]\*]*)(.*?)(\.\w*)?$`)

func parseType(str string) (*goType, error) {
	parts := partsRe.FindStringSubmatch(str)
	if len(parts) != 4 {
		return nil, fmt.Errorf("type must be in the form []*github.com/import/path.Name")
	}

	t := &goType{
		Modifiers:  parts[1],
		ImportPath: parts[2],
		Name:       strings.TrimPrefix(parts[3], "."),
	}

	if t.Name == "" {
		t.Name = t.ImportPath
		t.ImportPath = ""
	}

	if t.ImportPath != "" {
		p, err := packages.Load(&packages.Config{Mode: packages.NeedName}, t.ImportPath)
		if err != nil {
			return nil, err
		}
		if len(p) != 1 {
			return nil, fmt.Errorf("not found")
		}

		t.ImportName = p[0].Name
	}

	return t, nil
}

// Generate dataloader file without a file name prefix
func Generate(config Config) error {
	data, err := getData(config)
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("%s%s_gen.go", config.FileNamePrefix, ToLowerCamel(data.Name))

	if err := writeTemplate(filepath.Join(config.WorkingDirectory, filename), data); err != nil {
		return err
	}

	return nil
}

func getData(config Config) (templateData, error) {
	var data templateData
	data.DisableCacheExpiration = config.DisableCacheExpiration
	genPkg := getPackage(config.WorkingDirectory)
	if genPkg == nil {
		return templateData{}, fmt.Errorf("unable to find package info for " + config.WorkingDirectory)
	}

	var err error
	data.Name = config.LoaderName
	data.Package = genPkg.Name
	data.KeyType, err = parseType(config.KeyType)
	if err != nil {
		return templateData{}, fmt.Errorf("key type: %s", err.Error())
	}
	data.ValType, err = parseType(config.ValueType)
	if err != nil {
		return templateData{}, fmt.Errorf("key type: %s", err.Error())
	}

	// if we are inside the same package as the type we don't need an import and can refer directly to the type
	if genPkg.PkgPath == data.ValType.ImportPath {
		data.ValType.ImportName = ""
		data.ValType.ImportPath = ""
	}
	if genPkg.PkgPath == data.KeyType.ImportPath {
		data.KeyType.ImportName = ""
		data.KeyType.ImportPath = ""
	}

	return data, nil
}

func getPackage(dir string) *packages.Package {
	p, _ := packages.Load(&packages.Config{
		Dir: dir,
	}, ".")

	if len(p) != 1 {
		return nil
	}

	return p[0]
}

func writeTemplate(filepath string, data templateData) error {
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return errors.Wrap(err, "generating code")
	}

	src, err := imports.Process(filepath, buf.Bytes(), nil)
	if err != nil {
		return errors.Wrap(err, "unable to gofmt")
	}

	if err := os.WriteFile(filepath, src, 0644); err != nil {
		return errors.Wrap(err, "writing output")
	}

	return nil
}

func lcFirst(s string) string {
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

func LoadThunkMissReturnType(t string) string {
	if t == "interface{}" || strings.HasPrefix(t, "*") || strings.HasPrefix(t, "[]") {
		// nullable type
		return "nil"
	}
	switch t {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64", "complex64", "complex128", "byte":
		return "0"
	case "string":
		return `""`
	case "bool":
		return "false"
	default:
		return t + "{}"
	}
}
