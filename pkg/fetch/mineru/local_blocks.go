package mineru

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"websearch/pkg/i18n"
)

// LocalBlocks describes extraction coverage, never what a client has read.
type LocalBlocks struct {
	PackageID             string            `json:"package_id"`
	Format                string            `json:"format"`
	UpstreamVersion       string            `json:"upstream_version"`
	Selected              []int             `json:"selected_pages"`
	Remaining             []int             `json:"remaining_pages"`
	Requested             []int             `json:"requested_pages"`
	Covered               []int             `json:"covered_pages"`
	Total                 int               `json:"total_pages"`
	WholeDocumentComplete bool              `json:"whole_document_complete"`
	ImageContentRead      bool              `json:"image_content_read"`
	GeometryStatus        string            `json:"geometry_status"`
	Batches               []LocalBlockBatch `json:"batches"`
}

type LocalBlockBatch struct {
	Batch          int          `json:"batch"`
	Pages          []int        `json:"pages"`
	LocationStatus string       `json:"location_status"`
	Blocks         []LocalBlock `json:"blocks,omitempty"`
	Markdown       string       `json:"markdown,omitempty"`
}

type LocalBlock struct {
	ID           string                     `json:"id"`
	Order        int                        `json:"order"`
	Type         string                     `json:"type"`
	OriginalPage int                        `json:"original_page_1based,omitempty"`
	PrintedPage  *int                       `json:"printed_page"`
	Content      map[string]json.RawMessage `json:"content"`
}

// ReadLocalBlocks reads only completed, registered files. It cannot submit or resume tasks.
func ReadLocalBlocks(ctx context.Context, path string, pages []int) (*LocalBlocks, error) {
	var sourcePath string
	if strings.EqualFold(filepath.Ext(path), ".pdf") {
		sourcePath = path
		path = strings.TrimSuffix(path, filepath.Ext(path)) + ".mineru"
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	invalid := func() error { return errors.New(i18n.T("fetch.mineru.local_package_invalid")) }
	read := func(name string) ([]byte, error) {
		if !filepath.IsLocal(name) {
			return nil, invalid()
		}
		file, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > 128*1024*1024 {
			return nil, invalid()
		}
		return io.ReadAll(io.LimitReader(file, 128*1024*1024+1))
	}
	data, err := read("work.json")
	if errors.Is(err, os.ErrNotExist) {
		data, err = read("manifest.json")
	}
	if err != nil {
		return nil, err
	}
	record, err := decodeLocalPackage(data)
	if err != nil {
		return nil, err
	}
	if sourcePath == "" {
		sourcePath = filepath.Join(filepath.Dir(path), filepath.Base(record.Source))
		if _, err := os.Stat(sourcePath); errors.Is(err, os.ErrNotExist) {
			sourcePath = record.Source
		}
	}
	if source, err := os.Open(sourcePath); err == nil {
		info, statErr := source.Stat()
		if statErr != nil || !info.Mode().IsRegular() {
			_ = source.Close()
			return nil, invalid()
		}
		sum := sha256.New()
		_, copyErr := io.Copy(sum, source)
		closeErr := source.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return nil, err
		}
		if hex.EncodeToString(sum.Sum(nil)) != record.SHA256 {
			return nil, errors.New(i18n.T("fetch.mineru.local_source_changed"))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	selected, err := localPages(pages, record.TotalPages)
	if err != nil {
		return nil, err
	}
	selected = unionLocalPages(nil, selected)
	hash := sha256.New()
	hash.Write(data)
	out := &LocalBlocks{Format: record.Format, UpstreamVersion: "unknown", GeometryStatus: "unverified", Selected: selected,
		Requested: record.Requested, Total: record.TotalPages}
	for index, batch := range record.Batches {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, name := range batch.Files {
			if !filepath.IsLocal(name) {
				return nil, invalid()
			}
		}
		if batch.Markdown == "" {
			continue
		}
		if !slices.Contains(batch.Files, batch.Markdown) {
			return nil, invalid()
		}
		markdown, err := read(batch.Markdown)
		if err != nil {
			return nil, err
		}
		hash.Write([]byte(batch.Markdown))
		hash.Write(markdown)
		out.Covered = unionLocalPages(out.Covered, batch.Pages)
		var sidecar string
		ambiguous := false
		for _, name := range batch.Files {
			base := filepath.Base(name)
			if base == "content_list.json" || strings.HasSuffix(base, "_content_list.json") {
				ambiguous = ambiguous || sidecar != ""
				sidecar = name
			}
		}
		var blocks []map[string]json.RawMessage
		status := "missing_sidecar"
		if ambiguous {
			status = "ambiguous_sidecar"
		}
		if sidecar != "" && !ambiguous {
			content, err := read(sidecar)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			if err == nil {
				hash.Write([]byte(sidecar))
				hash.Write(content)
				status = "unsupported_sidecar"
				if json.Unmarshal(content, &blocks) == nil && blocks != nil {
					status = "available"
				} else {
					blocks = nil
				}
			}
		}
		wanted := false
		for _, p := range batch.Pages {
			wanted = wanted || slices.Contains(selected, p)
		}
		if !wanted {
			continue
		}
		part := LocalBlockBatch{Batch: index, Pages: batch.Pages, LocationStatus: status}
		if status != "available" {
			part.Markdown = string(markdown)
		}
		for order, raw := range blocks {
			page := -1
			var kind string
			_ = json.Unmarshal(raw["type"], &kind)
			valid := json.Unmarshal(raw["page_idx"], &page) == nil && page >= 0 && page < len(batch.Pages)
			original := 0
			if valid {
				original = batch.Pages[page]
			}
			if valid && !slices.Contains(selected, original) {
				continue
			}
			if !valid {
				part.LocationStatus = "invalid_page_index"
			}
			part.Blocks = append(part.Blocks, LocalBlock{ID: sidecar + "#" + strconv.Itoa(order), Order: order,
				Type: kind, OriginalPage: original, Content: raw})
		}
		out.Batches = append(out.Batches, part)
	}
	out.PackageID = hex.EncodeToString(hash.Sum(nil))
	for bi := range out.Batches {
		for i := range out.Batches[bi].Blocks {
			out.Batches[bi].Blocks[i].ID = out.PackageID + ":" + out.Batches[bi].Blocks[i].ID
		}
	}
	for _, page := range selected {
		if !slices.Contains(out.Covered, page) {
			out.Remaining = append(out.Remaining, page)
		}
	}
	out.WholeDocumentComplete = len(out.Covered) == out.Total
	return out, nil
}
