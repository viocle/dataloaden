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

// LoadThunkMissReturnType returns the default value for the given type
func LoadThunkMissReturnType(t string) string {
	if t == "interface{}" || strings.HasPrefix(t, "*") || strings.HasPrefix(t, "[]") || strings.HasPrefix(t, "map[") {
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

// LoadThunkMarshalType returns the code to handle marshaling the given type from a string in LoadThunk
func LoadThunkMarshalType(t string) string {
	if strings.HasPrefix(t, "*") {
		// pointer type
		return fmt.Sprintf("if v == \"\" || v == \"null\" {\n// key found, empty value, return nil\nreturn nil, nil\n}\nret := &%s{}\nif err := l.redisConfig.ObjUnmarshal([]byte(v), ret); err == nil {\nreturn ret, nil\n}", t[1:])
	} else if strings.HasPrefix(t, "[]") || strings.HasPrefix(t, "map[") {
		// slice/map type
		return fmt.Sprintf("if v == \"\" || v == \"null\" {\n// key found, empty value, return nil\nreturn nil, nil\n}\nvar ret %s\nif err := l.redisConfig.ObjUnmarshal([]byte(v), &ret); err == nil {\nreturn ret, nil\n}", t)
	}
	switch t {
	case "string":
		return "return v, nil"
	case "int", "int8", "int16", "int32", "rune", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64", "complex64", "complex128", "byte":
		return fmt.Sprintf("ret, err := strconv.Parse%s(v, 10, 64)\nif err == nil {\n\treturn %s(ret), nil\n}", strconvParseType(t), t)
	case "bool":
		return "ret, err := strconv.ParseBool(v)\nif err == nil {\n\treturn ret, nil\n}"
	default:
		// probably a struct by value. Try to unmarshal from json
		return fmt.Sprintf("if v == \"\" || v == \"null\" {\n// key found, empty value, return empty value\nreturn %s{}, nil\n}\nret := %s{}\nif err := l.redisConfig.ObjUnmarshal([]byte(v), &ret); err == nil {\nreturn ret, nil\n}", t, t)
	}
}

// LoadAllMarshalType returns the code to handle marshaling the given type from a string in LoadAll
func LoadAllMarshalType(t string) string {
	if strings.HasPrefix(t, "*") {
		// pointer type
		return fmt.Sprintf("if v == \"\" || v == \"null\" {\n// key found, empty value, return nil\nretVals[i] = nil\n}\nret := &%s{}\nif err := l.redisConfig.ObjUnmarshal([]byte(v), ret); err == nil {\nretVals[i] = ret\n}", t[1:])
	} else if strings.HasPrefix(t, "[]") || strings.HasPrefix(t, "map[") {
		// slice/map type
		return fmt.Sprintf("if v == \"\" || v == \"null\" {\n// key found, empty value, return nil\nretVals[i] = nil\n} else {\nvar ret %s\nif err := l.redisConfig.ObjUnmarshal([]byte(v), &ret); err == nil {\nretVals[i] = nil\n}\n}", t)
	}
	switch t {
	case "string":
		return "retVals[i] = v"
	case "int", "int8", "int16", "int32", "rune", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64", "complex64", "complex128", "byte":
		return fmt.Sprintf("ret, err := strconv.Parse%s(v, 10, 64)\nif err == nil {\n\tretVals[i] = %s(ret)\n}", strconvParseType(t), t)
	case "bool":
		return "ret, err := strconv.ParseBool(v)\nif err == nil {\n\tretVals[i] = ret\n}"
	default:
		// probably a struct by value. Try to unmarshal from json
		return fmt.Sprintf("if v == \"\" || v == \"null\" {\n// key found, empty value, set empty value\nretVals[i] = %s{}\n}\nret := %s{}\nif err := l.redisConfig.ObjUnmarshal([]byte(v), &ret); err == nil {\nretVals[i] = ret\n}", t, t)
	}
}

// strconvParseType returns the type to use in strconv.Parse* for the given type
func strconvParseType(t string) string {
	switch t {
	case "byte", "int8", "int16", "int32", "int64", "rune", "int":
		return "Int"
	case "uint", "uint8", "uint16", "uint32", "uint64":
		return "Uint"
	case "float32", "float64":
		return "Float"
	case "bool":
		return "Bool"
	case "complex64", "complex128":
		return "Complex"
	default:
		return ""
	}
}

func IsStructType(t string) bool {
	switch t {
	case "byte", "int8", "int16", "int32", "rune", "int", "int64", "uint", "uint8", "uint16", "uint32", "uintptr", "uint64", "float32", "float64", "complex64", "complex128", "bool", "string", "[]string":
		return false
	default:
		if strings.HasPrefix(t, "[]") || strings.HasPrefix(t, "map[") {
			return false
		}
		return true
	}
}

// ToRedisKey returns the code to convert the given key to a string for use as a redis key
func ToRedisKey(t, name string, keyType interface{}) string {
	if t == "string" {
		// no conversion needed, use directly
		return "key"
	}
	// we'll need to convert the type to a string
	switch t {
	case "byte", "int8", "int16", "int32", "rune", "int":
		return "strconv.FormatInt(int64(key), 10)"
	case "int64":
		// dont include unnecessary type conversion
		return "strconv.FormatInt(key, 10)"
	case "uint", "uint8", "uint16", "uint32", "uintptr":
		return "strconv.FormatUint(uint64(key), 10)"
	case "uint64":
		// dont include unnecessary type conversion
		return "strconv.FormatUint(key, 10)"
	case "bool":
		return "strconv.FormatBool(key)"
	case "float32":
		return "strconv.FormatFloat(float64(key), 'f', -1, 64)"
	case "float64":
		// dont include unnecessary type conversion
		return "strconv.FormatFloat(key, 'f', -1, 64)"
	case "complex64":
		return "strconv.FormatComplex(complex128(key), 'f', -1, 64)"
	case "complex128":
		// dont include unnecessary type conversion
		return "strconv.FormatComplex(key, 'f', -1, 64)"
	case "[]string":
		return "strings.Join(key, \":\")"
	default:
		// serialize to json and use as key. Redis key length limit is 512MB, but if you're using a key that large you're doing it wrong
		// to reduce the size of your keys, try setting json tags on your struct fields to reduce the field name length and ommit empty fields
		// see type UserByIDAndOrg in example/user.go for an example. oid will be in the json result instead of OrgID
		return "l.redisConfig.KeyToStringFunc(key)"
	}
}
