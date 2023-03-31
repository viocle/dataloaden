package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/viocle/dataloaden/pkg/generator"
	"golang.org/x/tools/go/packages"
)

// This is an example custom tool that uses dataloaden to generate a dataloader with a custom prefix. There are more ways than this, but this is one way to do it if you want to customize things.
// Example of what to place at the top of your go file to call this tool if used in pkhname/user.go:
// //go:generate go run ../custom_tool/tool.go -name UserLoader -type string -return *github.com/viocle/dataloaden/example.User

func main() {
	loaderName := flag.String("name", "", "Name of loader")
	dataType := flag.String("type", "", "Key data type")
	returnType := flag.String("return", "", "Return data type")

	flag.Parse()

	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(1)
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Getwd Error:", err.Error())
		os.Exit(2)
	}

	p, err := packages.Load(&packages.Config{
		Dir: wd,
	}, ".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Packages Load Error:", err.Error())
		os.Exit(3)
	}
	fmt.Println(p)

	if err := generator.GenerateWithPrefix("dataLoader-", *loaderName, *dataType, *returnType, wd); err != nil {
		fmt.Fprintln(os.Stderr, "Generate Error:", err.Error())
		os.Exit(4)
	}
}
