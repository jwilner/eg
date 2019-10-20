// The eg command performs example-based refactoring.
// For documentation, run the command, or see Help in
// golang.org/x/tools/refactor/eg.
package main // import "golang.org/x/tools/cmd/eg"

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/token"
	"golang.org/x/tools/go/buildutil"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/refactor/eg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	helpFlag       = flag.Bool("help", false, "show detailed help message")
	templateFlag   = flag.String("t", "", "template.go file specifying the refactoring")
	writeFlag      = flag.Bool("w", false, "rewrite input files in place (by default, the results are printed to standard output)")
	verboseFlag    = flag.Bool("v", false, "show verbose matcher diagnostics")

	beforeEditFlags arrayFlags
	afterEditFlags  arrayFlags
)

func init() {
	flag.Var((*buildutil.TagsFlag)(&build.Default.BuildTags), "tags", buildutil.TagsFlagDoc)
	flag.Var(
		&beforeEditFlags,
		"beforeedit",
		"A command to exec before each file is edited (e.g. chmod, checkout).  Whitespace delimits argument words.  "+
			"The string '{}' is replaced by the file name.",
	)
	flag.Var(
		&afterEditFlags,
		"afteredit",
		"A command to exec after each file is edited (e.g sed).  Whitespace delimits argument words.  The string "+
			"'{}' is replaced by the file name.",
	)
}

const usage = `eg: an example-based refactoring tool.

Usage: eg -t template.go [-w] <args>...

-help            show detailed help message
-t template_file specifies the template file (use -help to see explanation)
-w          	 causes files to be re-written in place.
-v               show verbose matcher diagnostics
-beforeedit cmd  a command to exec before each file is modified.
                 "{}" represents the name of the file.
-afteredit  cmd  a command to exec after each file is edited (e.g sed).
                 "{}" represents the name of the file.
`

func main() {
	if err := doMain(); err != nil {
		fmt.Fprintf(os.Stderr, "eg: %s\n", err)
		os.Exit(1)
	}
}

// finds the transformer and removes the template package from pkgs
func buildTransformer(tmplPath string, fSet *token.FileSet, pkgs *[]*packages.Package) (*eg.Transformer, error) {
	// find the template package in the processed packages according to the absolute file path
	var tmplPkg *packages.Package
	for i := 0; tmplPkg == nil && i < len(*pkgs); i++ {
		pkg := (*pkgs)[i]
		for _, f := range pkg.GoFiles {
			if f == tmplPath {
				tmplPkg = pkg
				*pkgs = append((*pkgs)[:i], (*pkgs)[i+1:]...)
				break
			}
		}
	}
	if tmplPkg == nil {
		return nil, errors.New("didn't find template in module path")
	}

	var tmplFile *ast.File
	for _, f := range tmplPkg.Syntax {
		if tmplPath == fSet.File(f.Pos()).Name() {
			tmplFile = f
			break
		}
	}
	if tmplFile == nil {
		panic("didn't find template in template package somehow")
	}

	return eg.NewTransformer(fSet, tmplPkg.Types, tmplFile, tmplPkg.TypesInfo, *verboseFlag)
}

func doMain() error {
	flag.Parse()
	args := flag.Args()

	if *helpFlag {
		fmt.Fprint(os.Stderr, eg.Help)
		os.Exit(2)
	}

	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	if *templateFlag == "" {
		return fmt.Errorf("no -t template.go file specified")
	}
	tmplPath, err := filepath.Abs(*templateFlag)
	if err != nil {
		return fmt.Errorf("unable to resolve tmpl flag: %v", templateFlag)
	}

	fSet := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedFiles |
			packages.NeedImports |
			packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedTypesInfo,
		Fset: fSet,
	}

	pkgs, err := packages.Load(cfg, append([]string{"file=" + tmplPath}, flag.Args()...)...) // forward CLI args
	if err != nil {
		return fmt.Errorf("load: %v\n", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return errors.New("error loading packages")
	}

	xform, err := buildTransformer(tmplPath, fSet, &pkgs)

	fmt.Fprintf(os.Stderr, "visiting %v packages", len(pkgs))

	var hadErrors bool
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			n := xform.Transform(pkg.TypesInfo, pkg.Types, file)
			if n == 0 {
				continue
			}
			filename := fSet.File(file.Pos()).Name()
			fmt.Fprintf(os.Stderr, "=== %s (%d matches)\n", filename, n)
			if *writeFlag {
				// Run the before-edit command (e.g. "chmod +w",  "checkout") if any.
				for _, f := range beforeEditFlags {
					if err := runCmdOnFile(f, filename); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: before edit hook %q failed (%s)\n", f, err)
					}
				}
				if err := eg.WriteAST(fSet, filename, file); err != nil {
					fmt.Fprintf(os.Stderr, "eg: %s\n", err)
					hadErrors = true
				}
				for _, f := range afterEditFlags {
					if err := runCmdOnFile(f, filename); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: before edit hook %q failed (%s)\n", f, err)
					}
				}
			} else {
				format.Node(os.Stdout, fSet, file)
			}
		}
	}
	if hadErrors {
		os.Exit(1)
	}
	return nil
}

func runCmdOnFile(flag string, filename string) error {
	args := strings.Fields(flag)
	if len(args) == 0 {
		return nil
	}
	// Replace "{}" with the filename, like find(1).
	for i := range args {
		if i > 0 {
			args[i] = strings.Replace(args[i], "{}", filename, -1)
		}
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%q failed: %v", args, err)
	}
	return nil
}

type arrayFlags []string

func (i *arrayFlags) String() string {
	return fmt.Sprintf("%s", *i)
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}
