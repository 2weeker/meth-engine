package stylesheets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
)

type Sheet struct {
	Body []byte
	ETag string
}

var importRe = regexp.MustCompile(`(?m)^[ \t]*@import[ \t]+(?:url\(\s*)?["']([^"']+)["']\s*\)?[ \t]*;[ \t]*\r?\n?`)

var commentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)

func Build(fsys fs.FS, dir string) (map[string]Sheet, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	sheets := map[string]Sheet{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".css") {
			continue
		}
		body, err := combine(fsys, dir, path.Join(dir, e.Name()), nil)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(body)
		sheets[e.Name()] = Sheet{Body: body, ETag: `"` + hex.EncodeToString(sum[:8]) + `"`}
	}
	return sheets, nil
}

func combine(fsys fs.FS, root, file string, stack []string) ([]byte, error) {
	if slices.Contains(stack, file) {
		return nil, fmt.Errorf("stylesheets: import cycle: %s -> %s", strings.Join(stack, " -> "), file)
	}
	src, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, fmt.Errorf("stylesheets: %w", err)
	}
	stack = append(stack, file)

	var out strings.Builder
	last := 0
	for _, m := range importRe.FindAllSubmatchIndex(src, -1) {
		out.Write(src[last:m[0]])
		last = m[1]
		ref := string(src[m[2]:m[3]])
		if strings.Contains(ref, ":") || strings.HasPrefix(ref, "/") {
			return nil, fmt.Errorf("stylesheets: %s: only relative local imports are combined, not %q", file, ref)
		}
		target := path.Join(path.Dir(file), ref)
		if !strings.HasPrefix(target, root+"/") {
			return nil, fmt.Errorf("stylesheets: %s: %q is outside %s", file, ref, root)
		}
		body, err := combine(fsys, root, target, stack)
		if err != nil {
			return nil, err
		}
		out.Write(body)
	}
	out.Write(src[last:])

	if rest := commentRe.ReplaceAllString(out.String(), ""); strings.Contains(rest, "@import") {
		return nil, fmt.Errorf("stylesheets: %s: unsupported @import (only @import \"./x.css\"; is combined)", file)
	}
	return []byte(out.String()), nil
}
