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
	"path/filepath"
	"regexp"
	"strings"

	"os"

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
		log.Println("-from and -to flag required")
		os.Exit(1)
	}

	pkgs, _ := dirPackageInfo("./", *fromFlag, *toFlag)
	for _, pi := range pkgs {
		err := refactImports(pi.fset, pi.f, pi.paths, pi.filename, *debug)
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
	}
}

type packageInfo struct {
	filename string
	f        *ast.File
	fset     *token.FileSet
	paths    []*pathInfo
}

type pathInfo struct {
	old, new string
}

// refactImports fixes imports to new.
func refactImports(
	fset *token.FileSet, f *ast.File, paths []*pathInfo, filename string, debug bool,
) error {
	abs, err := filepath.Abs(filename)
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
				return fmt.Errorf("%s: couldn't format file: %v", filename, err)
			}
			ioutil.WriteFile(filename, buf.Bytes(), 0755)
		}

	}
	return nil
}

// dirPackageInfo find old imports
func dirPackageInfo(srcDir string, from string, to string) ([]*packageInfo, error) {
	re, _ := regexp.Compile(from)

	packageFileInfos, err := ioutil.ReadDir(srcDir)
	if err != nil {
		return nil, err
	}

	var result []*packageInfo
	for _, fi := range packageFileInfos {
		if !strings.HasSuffix(fi.Name(), ".go") {
			continue
		}

		fileSet := token.NewFileSet()
		root, err := parser.ParseFile(fileSet, filepath.Join(srcDir, fi.Name()), nil, 0)
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
		result = append(result, &packageInfo{filename: fi.Name(), fset: fileSet, f: root, paths: paths})
	}
	return result, nil
}
