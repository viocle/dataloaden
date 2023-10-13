package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/viocle/dataloaden/pkg/generator"
)

func main() {
	if len(os.Args) < 4 || len(os.Args) > 5 {
		fmt.Println("usage: name keyType valueType")
		fmt.Println(" example:")
		fmt.Println(" dataloaden 'UserLoader int []*github.com/my/package.User'")
		os.Exit(1)
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	disableCacheExpiration := false
	if len(os.Args) >= 5 {
		if strings.ToLower(os.Args[4]) == "true" {
			disableCacheExpiration = true
		}
	}

	if err := generator.Generate(generator.Config{LoaderName: os.Args[1], KeyType: os.Args[2], ValueType: os.Args[3], WorkingDirectory: wd, DisableCacheExpiration: disableCacheExpiration}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
}
