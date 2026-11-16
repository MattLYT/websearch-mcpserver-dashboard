package mineru

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func blockFixture(t *testing.T, pages []int, sidecar string, work bool) string {
	t.Helper()
	dir := t.TempDir()
	md := "generated/batch/full.md"
	name := "generated/batch/document_content_list.json"
	if err := os.MkdirAll(filepath.Join(dir, "generated/batch"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, md), []byte("# Original\n\nTable caption: 速度 (m/s)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	files := []string{md}
	if sidecar != "" {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(sidecar), 0600); err != nil {
			t.Fatal(err)
		}
		files = append(files, name)
	}
	record := localPackage{Format: localPackageFormat, Source: filepath.Join(dir, "missing.pdf"), SHA256: "old",
		TotalPages: 10, Requested: []int{4, 5, 8}, Batches: []localBatch{{Pages: pages, Markdown: md, Files: files}}}
	manifest := "manifest.json"
	if work {
		manifest = "work.json"
		record.Batches = append(record.Batches, localBatch{Pages: []int{9}, TaskRef: "pending"})
	}
	if err := writeLocalJSON(filepath.Join(dir, manifest), record); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReadLocalBlocksOriginalPagesAndPartialWork(t *testing.T) {
	for _, pages := range [][]int{{5}, {8}, {4, 5}} {
		t.Run(LocalPageRanges(pages), func(t *testing.T) {
			content := `[{"type":"table","page_idx":0,"table_body":"<table><tr><td>2 m/s</td></tr></table>","table_caption":["Speed"],"table_footnote":["At 20 C"]}]`
			if len(pages) == 2 {
				content = strings.TrimSuffix(content, "]") + `,{"type":"text","page_idx":1,"text":"Original next page"}]`
			}
			dir := blockFixture(t, pages, content, true)
			out, err := ReadLocalBlocks(context.Background(), dir, pages)
			if err != nil {
				t.Fatal(err)
			}
			if out.WholeDocumentComplete || out.ImageContentRead || !slices.Equal(out.Covered, pages) || len(out.Remaining) != 0 || len(out.Batches) != 1 {
				t.Fatalf("bad coverage %+v", out)
			}
			blocks := out.Batches[0].Blocks
			for i, p := range pages {
				if blocks[i].OriginalPage != p || blocks[i].Order != i || !strings.HasPrefix(blocks[i].ID, out.PackageID+":") {
					t.Fatalf("bad mapping %+v", blocks)
				}
			}
			if string(blocks[0].Content["table_footnote"]) != `["At 20 C"]` {
				t.Fatal("lost table context")
			}
		})
	}
}

func TestReadLocalBlocksFallbackAndInvalidIndex(t *testing.T) {
	for _, content := range []string{"", `{"unknown":[]}`, `[{"type":"text","text":"Unknown page"}]`, `[{"type":"text","page_idx":8,"text":"No guess"}]`} {
		dir := blockFixture(t, []int{8}, content, false)
		out, err := ReadLocalBlocks(context.Background(), dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		part := out.Batches[0]
		if part.LocationStatus == "available" {
			t.Fatalf("claimed location: %+v", part)
		}
		for _, b := range part.Blocks {
			if b.OriginalPage != 0 {
				t.Fatal("guessed original page")
			}
		}
		if len(part.Blocks) == 0 && !strings.Contains(part.Markdown, "m/s") {
			t.Fatal("missing Markdown fallback")
		}
	}
}

func TestReadLocalBlocksRegisteredPathsAndVersion(t *testing.T) {
	dir := blockFixture(t, []int{5}, `[{"type":"text","page_idx":0,"text":"Old"}]`, false)
	old, err := ReadLocalBlocks(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "generated/batch/document_content_list.json")
	if err := os.WriteFile(path, []byte(`[{"type":"text","page_idx":0,"text":"New"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	current, err := ReadLocalBlocks(context.Background(), dir, nil)
	if err != nil || old.PackageID == current.PackageID || old.Batches[0].Blocks[0].ID == current.Batches[0].Blocks[0].ID {
		t.Fatalf("identity not changed: %v", err)
	}
	manifest := filepath.Join(dir, "manifest.json")
	data, _ := os.ReadFile(manifest)
	var record localPackage
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	record.Batches[0].Markdown = "../outside.md"
	record.Batches[0].Files = append(record.Batches[0].Files, "../outside.md")
	if err := writeLocalJSON(manifest, record); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLocalBlocks(context.Background(), dir, nil); err == nil {
		t.Fatal("accepted traversal")
	}
	record.Batches[0].Markdown = "unregistered.md"
	record.Batches[0].Files = nil
	if err := writeLocalJSON(manifest, record); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLocalBlocks(context.Background(), dir, nil); err == nil {
		t.Fatal("accepted unregistered file")
	}
}

func TestReadLocalBlocksWorkPrecedenceAndSourceChange(t *testing.T) {
	dir := blockFixture(t, []int{5}, `[{"type":"text","page_idx":0,"text":"Current"}]`, true)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"invalid":"old"}`), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := ReadLocalBlocks(context.Background(), dir, []int{5})
	if err != nil || len(out.Batches) != 1 {
		t.Fatalf("work did not take precedence: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "missing.pdf"), []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLocalBlocks(context.Background(), dir, nil); err == nil {
		t.Fatal("changed source accepted")
	}
}
