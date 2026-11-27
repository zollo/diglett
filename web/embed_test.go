package web

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// Every local asset the UI references (icons, manifest, CSS, JS) must be
// embedded, or the page ships with broken images and a missing favicon.
func TestIndexAssetsAreEmbedded(t *testing.T) {
	static := Static()
	index, err := fs.ReadFile(static, "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	refs := regexp.MustCompile(`(?:href|src)="([^"#]+)"`).FindAllStringSubmatch(string(index), -1)
	if len(refs) == 0 {
		t.Fatal("no asset references found in index.html")
	}
	for _, m := range refs {
		ref := m[1]
		if strings.Contains(ref, "://") || ref == "./" {
			continue
		}
		if _, err := fs.Stat(static, strings.TrimPrefix(ref, "./")); err != nil {
			t.Errorf("index.html references %q, which is not embedded: %v", ref, err)
		}
	}
}

func TestManifestIconsAreEmbedded(t *testing.T) {
	static := Static()
	raw, err := fs.ReadFile(static, "site.webmanifest")
	if err != nil {
		t.Fatalf("read site.webmanifest: %v", err)
	}
	var manifest struct {
		Name  string `json:"name"`
		Icons []struct {
			Src string `json:"src"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("site.webmanifest is not valid JSON: %v", err)
	}
	if manifest.Name != "Diglett" || len(manifest.Icons) == 0 {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	for _, icon := range manifest.Icons {
		if _, err := fs.Stat(static, icon.Src); err != nil {
			t.Errorf("manifest icon %q is not embedded: %v", icon.Src, err)
		}
	}
	if _, err := fs.Stat(static, "favicon.ico"); err != nil {
		t.Errorf("favicon.ico is not embedded: %v", err)
	}
}
