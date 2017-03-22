package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	"golang.org/x/tools/go/vcs"
)

var (
	manifestPath = flag.String("manifest", "Godeps/Godeps.json", "Manifest file with dependencies.")
	vendorPath   = flag.String("vendor", "vendor", "Vendor directory holding dependencies.")
	cachePath    = flag.String("cache", os.TempDir(), "Temporary directory for checking out sources.")
	verbose      = flag.Bool("v", false, "Turn on verbose logging.")
)

type godepManifest struct {
	ImportPath   string
	GoVersion    string
	GodepVersion string
	Deps         []struct {
		ImportPath string
		Comment    string
		Rev        string
	}
}

func gitClone(dir, repo string) error {
	cmd := exec.Command("git", "clone", repo, dir)
	if *verbose {
		fmt.Printf("$ %s\n", strings.Join(cmd.Args, " "))
	}
	return cmd.Run()
}

func gitFetch(dir string) error {
	cmd := exec.Command("git", "fetch", "origin")
	cmd.Dir = dir
	if *verbose {
		fmt.Printf("$ cd %s; %s\n", cmd.Dir, strings.Join(cmd.Args, " "))
	}
	return cmd.Run()
}

func gitCheckout(dir, rev string) error {
	cmd := exec.Command("git", "checkout", rev)
	cmd.Dir = dir
	if *verbose {
		fmt.Printf("$ cd %s; %s\n", cmd.Dir, strings.Join(cmd.Args, " "))
	}
	return cmd.Run()
}

func gitHead(dir string) ([]byte, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	if *verbose {
		fmt.Printf("$ cd %s; %s\n", cmd.Dir, strings.Join(cmd.Args, " "))
	}
	return cmd.Output()
}

func main() {
	flag.Parse()

	manifestJSON, err := ioutil.ReadFile(*manifestPath)
	if err != nil {
		panic(err)
	}

	var manifest godepManifest
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		panic(err)
	}

	paths := make(map[string][]string)
	roots := make(map[string]*vcs.RepoRoot)
	revs := make(map[string]string)

	fmt.Printf("# Resolving package urls to repositories\n")
	for _, d := range manifest.Deps {
		rr, err := vcs.RepoRootForImportPath(d.ImportPath, *verbose)
		if err != nil {
			panic(err)
		}

		paths[rr.Root] = append(paths[rr.Root], d.ImportPath)
		roots[rr.Root] = rr
		revs[rr.Root] = d.Rev
	}

	fmt.Printf("# Checking out %d repositories locally\n", len(roots))
	for name, root := range roots {
		dir := filepath.Join(*cachePath, "vendor-verify", name)

		if *verbose {
			fmt.Printf("downloading %q rev %s to %q\n", name, revs[name], dir)
		}

		if root.VCS.Name != "Git" {
			panic(fmt.Errorf("currently we can only verify git dependencies"))
		}

		if st, err := os.Stat(dir); err != nil {
			if !os.IsNotExist(err) {
				panic(err)
			}

			if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
				panic(err)
			}

			if err := gitClone(dir, root.Repo); err != nil {
				panic(err)
			}
		} else {
			if !st.IsDir() {
				panic(fmt.Errorf("%q should be a directory", dir))
			}

			rev, err := gitHead(dir)
			if err != nil {
				panic(err)
			}

			if strings.TrimSpace(string(rev)) != revs[name] {
				if err := gitFetch(dir); err != nil {
					panic(err)
				}
			}
		}

		if err := gitCheckout(dir, revs[name]); err != nil {
			panic(err)
		}
	}

	failed := false

	fmt.Printf("# Comparing file contents\n")
	for name := range paths {
		vendorPath := filepath.Join(*vendorPath, name)
		cleanPath := filepath.Join(*cachePath, "vendor-verify", name)

		if err := filepath.Walk(vendorPath, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if fi.IsDir() {
				return nil
			}

			relativePath := strings.TrimLeft(strings.TrimPrefix(path, vendorPath), "/")

			if *verbose {
				fmt.Printf("checking %s\n", filepath.Join(name, relativePath))
			}

			d1, err := ioutil.ReadFile(filepath.Join(vendorPath, relativePath))
			if err != nil {
				return err
			}

			h1 := sha256.New()
			if _, err := io.Copy(h1, bytes.NewReader(d1)); err != nil {
				return err
			}
			sum1 := h1.Sum(nil)

			d2, err := ioutil.ReadFile(filepath.Join(cleanPath, relativePath))
			if err != nil {
				return err
			}

			h2 := sha256.New()
			if _, err := io.Copy(h2, bytes.NewReader(d2)); err != nil {
				return err
			}
			sum2 := h2.Sum(nil)

			if !bytes.Equal(sum1, sum2) {
				failed = true

				fmt.Printf("\n[!] file %s has changes\n", filepath.Join(name, relativePath))

				diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
					A:        difflib.SplitLines(string(d1)),
					B:        difflib.SplitLines(string(d2)),
					FromFile: "vendor",
					ToFile:   "original",
					Context:  3,
					Eol:      "\n",
				})

				if err == nil {
					fmt.Print(diff)
				}
			}

			return nil
		}); err != nil {
			panic(err)
		}
	}

	if failed {
		fmt.Printf("# Failures were detected\n")
		os.Exit(1)
	} else {
		fmt.Printf("# All done\n")
		os.Exit(0)
	}
}
