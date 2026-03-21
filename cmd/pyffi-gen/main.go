// pyffi-gen generates Go type-safe bindings from Python packages.
//
// Usage:
//
//	pyffi-gen --module json --out ./gen/jsonpkg
//	pyffi-gen --config pyffi-gen.yaml
//	pyffi-gen --module json --dry-run
//	pyffi-gen init --module numpy,pandas --python ">=3.11"
//	pyffi-gen sync
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/i2y/pyffi/internal/gen"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "init":
			cmdInit(os.Args[2:])
			return
		case "sync":
			cmdSync()
			return
		case "generate", "gen":
			cmdGenerate(os.Args[2:])
			return
		}
	}
	// Default: generate.
	cmdGenerate(os.Args[1:])
}

func cmdGenerate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	module := fs.String("module", "", "Python module to introspect")
	out := fs.String("out", "", "Output directory")
	pkg := fs.String("package", "", "Go package name (default: derived from module)")
	include := fs.String("include", "", "Comma-separated names to include")
	exclude := fs.String("exclude", "", "Comma-separated patterns to exclude")
	deps := fs.String("dependencies", "", "Comma-separated pip packages to install before introspection")
	configPath := fs.String("config", "", "Path to pyffi-gen.yaml")
	dryRun := fs.Bool("dry-run", false, "Print generated code to stdout")
	test := fs.Bool("test", false, "Generate test scaffolding")
	fs.Parse(args)

	var cfg *gen.Config
	if *configPath != "" {
		var err error
		cfg, err = gen.LoadConfig(*configPath)
		if err != nil {
			fatal(err)
		}
	} else {
		cfg = &gen.Config{Module: *module, Out: *out, Package: *pkg, Test: *test}
		if *include != "" {
			cfg.Include = strings.Split(*include, ",")
		}
		if *exclude != "" {
			cfg.Exclude = strings.Split(*exclude, ",")
		}
		if *deps != "" {
			cfg.Dependencies = strings.Split(*deps, ",")
			for i := range cfg.Dependencies {
				cfg.Dependencies[i] = strings.TrimSpace(cfg.Dependencies[i])
			}
		}
	}

	cfg.Defaults()
	if err := cfg.Validate(); err != nil {
		fatal(err)
	}

	g := gen.NewGenerator(cfg)
	var err error
	if *dryRun {
		err = g.DryRun()
	} else {
		err = g.Generate()
	}
	if err != nil {
		fatal(err)
	}
}

func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	modules := fs.String("module", "", "Comma-separated Python modules")
	pythonV := fs.String("python", ">=3.11", "Python version constraint")
	fs.Parse(args)

	if *modules == "" {
		fmt.Fprintf(os.Stderr, "error: --module is required for init\n")
		os.Exit(1)
	}

	mods := strings.Split(*modules, ",")
	for i := range mods {
		mods[i] = strings.TrimSpace(mods[i])
	}

	if err := gen.ScaffoldInit(mods, *pythonV); err != nil {
		fatal(err)
	}
}

func cmdSync() {
	if err := gen.ScaffoldSync(); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
