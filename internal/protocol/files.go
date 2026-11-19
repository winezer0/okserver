package protocol

import "time"

type FileEntry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	IsDir    bool      `json:"is_dir"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

type FileList struct {
	Total   int         `json:"total"`
	Page    int         `json:"page"`
	Limit   int         `json:"limit"`
	Files   int         `json:"files"`
	Folders int         `json:"folders"`
	Bytes   int64       `json:"bytes"`
	Path    string      `json:"path"`
	Entries []FileEntry `json:"entries"`
}
