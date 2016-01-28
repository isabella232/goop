package goop

import (
	"go/build"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var goGetDownloadRe = regexp.MustCompile(`(?m)^(\S+)\s+\(download\)$`)

type DownloadRecorder struct {
	downloads map[string]struct{}
	writer    io.Writer
}

func NewDownloadRecorder(writer io.Writer) *DownloadRecorder {
	return &DownloadRecorder{downloads: map[string]struct{}{}, writer: writer}
}

func (d *DownloadRecorder) Write(p []byte) (n int, err error) {
	s := string(p)
	matches := goGetDownloadRe.FindAllStringSubmatch(s, -1)
	if matches != nil {
		for _, m := range matches {
			d.downloads[m[1]] = struct{}{}
		}
	}
	return d.writer.Write(p)
}

func (d *DownloadRecorder) Downloads() []string {
	s := make([]string, 0, len(d.downloads))
	for k, _ := range d.downloads {
		s = append(s, k)
	}
	return s
}

func (g *Goop) goGet(pkgpath string, gopath string) ([]string, error) {
	pkgs := map[string]bool{}

	sourceDirs, err := sourceDirs(pkgpath)
	if err != nil {
		return nil, err
	}

	for _, sourceDir := range sourceDirs {
		if sourceDir == pkgpath {
			sourceDir = ""
		}
		downloads, err := g.goGetSingle(pkgpath, gopath, sourceDir)
		if err != nil {
			return nil, err
		}

		for _, pkg := range downloads {
			pkgs[pkg] = true
		}
	}

	ret := make([]string, 0, len(pkgs))

	for pkg, _ := range pkgs {
		ret = append(ret, pkg)
	}

	return ret, nil
}

func (g *Goop) goGetSingle(pkgpath string, gopath string, subdir string) ([]string, error) {
	path := "."
	if subdir != "" {
		path = "./" + subdir
	}
	cmd := exec.Command("go", "get", "-d", "-v", path)
	env := g.patchedEnv(true)
	env["GOPATH"] = gopath
	cmd.Dir = pkgpath
	cmd.Env = env.Strings()
	cmd.Stdin = g.stdin
	cmd.Stdout = g.stdout
	dlRec := NewDownloadRecorder(g.stderr)
	cmd.Stderr = dlRec
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	return dlRec.Downloads(), nil
}

func sourceDirs(root string) ([]string, error) {
	folders := map[string]bool{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			parent, folder := filepath.Split(path)
			if folder == "testdata" || strings.HasPrefix(folder, "_") || strings.HasPrefix(folder, ".") {
				return filepath.SkipDir
			}

			if folders[parent] {
				return filepath.SkipDir
			}

			_, err := build.ImportDir(path, 0)
			if _, noGoError := err.(*build.NoGoError); noGoError {
				return nil
			}

			folders[path] = false
		} else {
			if len(path) > 3 && path[len(path)-3:] == ".go" && (len(path) < 8 || path[len(path)-8:] != "_test.go") {
				root := filepath.Dir(path)
				if _, ok := folders[root]; ok {
					folders[root] = true
				}
			}
		}

		return nil
	})

	ret := []string{}

	for folder, isSource := range folders {
		if isSource && !strings.Contains(folder, "/internal/") {
			if folder == root {
				folder = "."
			}
			ret = append(ret, strings.TrimPrefix(folder, root+"/"))
		}
	}

	if err != nil {
		return nil, err
	}

	sort.Strings(ret)

	return ret, nil
}
