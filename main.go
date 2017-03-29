package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

var (
	fromFlag = flag.String("from", "", "identifier to be renamed")
	toFlag   = flag.String("to", "", "new name for identifier")
	debug    = flag.Bool("debug", false, "debug mode")
)

func init() {
	flag.Parse()
}

func main() {
	if *fromFlag == "" || *toFlag == "" {
		fmt.Println("-from and -to flag required")
		os.Exit(1)
	}

	pkgs, _ := dirPackageInfo("./", *fromFlag, *toFlag)
	for _, pi := range pkgs {
		err := refactImports(pi.fset, pi.f, pi.paths, pi.filePath, *debug)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}

type packageInfo struct {
	filePath string
	f        *ast.File
	fset     *token.FileSet
	paths    []*pathInfo
}

type pathInfo struct {
	old, new string
}

// refactImports fixes imports to new.
func refactImports(
	fset *token.FileSet, f *ast.File, paths []*pathInfo, filePath string, debug bool,
) error {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}
	srcDir := filepath.Dir(abs)

	if len(paths) > 0 {
		var changed bool

		for _, path := range paths {
			if !astutil.RewriteImport(fset, f, path.old, path.new) {
				continue
			}
			if debug {
				log.Printf("refactImports(newName=%q), abs=%q, srcDir=%q\n", path.new, abs, srcDir)
			}
			changed = true
		}

		if changed {
			var buf bytes.Buffer
			if err := format.Node(&buf, fset, f); err != nil {
				return fmt.Errorf("%s: couldn't format file: %v", filePath, err)
			}
			ioutil.WriteFile(filePath, buf.Bytes(), 0755)
		}

	}
	return nil
}

// dirPackageInfo find old imports
func dirPackageInfo(srcDir string, from string, to string) ([]*packageInfo, error) {
	re, _ := regexp.Compile(from)

	var fileList []string
	err := filepath.Walk(srcDir, func(path string, f os.FileInfo, err error) error {
		ignore := strings.HasPrefix(path, "vendor/") || strings.HasPrefix(path, ".git/")
		if !ignore && strings.HasSuffix(f.Name(), ".go") {
			fileList = append(fileList, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var result []*packageInfo
	for _, filePath := range fileList {
		fileSet := token.NewFileSet()
		root, err := parser.ParseFile(fileSet, filepath.Join(srcDir, filePath), nil, 0)
		if err != nil {
			continue
		}

		var paths []*pathInfo
		for _, decl := range root.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ImportSpec)
				if !ok {
					continue
				}
				if re.Match([]byte(valueSpec.Path.Value)) {
					res := re.ReplaceAll([]byte(valueSpec.Path.Value), []byte(to))
					paths = append(paths, &pathInfo{old: strings.Trim(valueSpec.Path.Value, `""`), new: strings.Trim(string(res), `""`)})
				}
			}
		}
		result = append(result, &packageInfo{filePath: filePath, fset: fileSet, f: root, paths: paths})
	}
	return result, nil
}
