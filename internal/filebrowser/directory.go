package filebrowser

import (
	"container/heap"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/winezer0/okserver/internal/protocol"
)

var ErrInvalidQuery = errors.New("invalid directory query")

type directoryQuery struct {
	page, limit  int
	search, sort string
	descending   bool
}

func parseQuery(values url.Values) (directoryQuery, error) {
	query := directoryQuery{page: 1, limit: 100, search: strings.ToLower(values.Get("search")), sort: values.Get("sort"), descending: values.Get("direction") == "desc"}
	for name, target := range map[string]*int{"page": &query.page, "limit": &query.limit} {
		if value := values.Get(name); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 {
				return query, fmt.Errorf("%w: invalid %s", ErrInvalidQuery, name)
			}
			*target = parsed
		}
	}
	if query.limit > 500 || query.page > 1000 {
		return query, fmt.Errorf("%w: directory page exceeds limit", ErrInvalidQuery)
	}
	switch query.sort {
	case "", "name":
		query.sort = "name"
	case "size", "modified", "type":
	default:
		return query, fmt.Errorf("%w: invalid directory sort", ErrInvalidQuery)
	}
	return query, nil
}

func (query directoryQuery) less(left, right protocol.FileEntry) bool {
	if left.IsDir != right.IsDir {
		return left.IsDir
	}
	compare := 0
	switch query.sort {
	case "size":
		if left.Size < right.Size {
			compare = -1
		} else if left.Size > right.Size {
			compare = 1
		}
	case "modified":
		if left.Modified.Before(right.Modified) {
			compare = -1
		} else if left.Modified.After(right.Modified) {
			compare = 1
		}
	case "type":
		compare = strings.Compare(strings.ToLower(filepath.Ext(left.Name)), strings.ToLower(filepath.Ext(right.Name)))
	}
	if compare == 0 {
		compare = strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
	}
	if compare == 0 {
		compare = strings.Compare(left.Name, right.Name)
	}
	if query.descending {
		return compare > 0
	}
	return compare < 0
}

// The heap retains only the requested prefix while directory metadata is read in batches.
type entryHeap struct {
	entries []protocol.FileEntry
	query   directoryQuery
}

func (h entryHeap) Len() int           { return len(h.entries) }
func (h entryHeap) Less(i, j int) bool { return h.query.less(h.entries[j], h.entries[i]) }
func (h entryHeap) Swap(i, j int)      { h.entries[i], h.entries[j] = h.entries[j], h.entries[i] }
func (h *entryHeap) Push(value any)    { h.entries = append(h.entries, value.(protocol.FileEntry)) }
func (h *entryHeap) Pop() any {
	last := h.entries[len(h.entries)-1]
	h.entries = h.entries[:len(h.entries)-1]
	return last
}

func ReadPage(full, relative string, values url.Values) (protocol.FileList, error) {
	query, err := parseQuery(values)
	if err != nil {
		return protocol.FileList{}, err
	}
	directory, err := os.Open(full)
	if err != nil {
		return protocol.FileList{}, err
	}
	defer directory.Close()
	result := protocol.FileList{Path: relative, Page: query.page, Limit: query.limit, Entries: make([]protocol.FileEntry, 0)}
	selected := &entryHeap{query: query}
	maximum := query.page * query.limit
	for {
		entries, readErr := directory.ReadDir(256)
		if readErr != nil && readErr != io.EOF {
			return result, readErr
		}
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), ".okserver") || strings.EqualFold(entry.Name(), ".okserver-downloads") {
				continue
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				continue
			}
			if entry.IsDir() {
				result.Folders++
			} else {
				result.Files++
				result.Bytes += info.Size()
			}
			if !strings.Contains(strings.ToLower(entry.Name()), query.search) {
				continue
			}
			result.Total++
			item := protocol.FileEntry{Name: entry.Name(), Path: path.Join(relative, entry.Name()), IsDir: entry.IsDir(), Size: info.Size(), Modified: info.ModTime()}
			if selected.Len() < maximum {
				heap.Push(selected, item)
			} else if query.less(item, selected.entries[0]) {
				selected.entries[0] = item
				heap.Fix(selected, 0)
			}
		}
		if readErr == io.EOF {
			break
		}
	}
	sort.Slice(selected.entries, func(i, j int) bool { return query.less(selected.entries[i], selected.entries[j]) })
	start := (query.page - 1) * query.limit
	if start < len(selected.entries) {
		result.Entries = selected.entries[start:]
	}
	return result, nil
}
