package client

import (
	"net/url"
	"strings"
)

// CLI URLs identify a remote file; all wire requests use the versioned API.
func downloadURL(value string) string {
	remote, err := url.Parse(value)
	if err != nil {
		return value
	}
	path := remote.Path
	if strings.HasPrefix(path, "/download/") {
		return remote.String()
	}
	remote.Path = "/download/" + strings.TrimPrefix(path, "/")
	remote.RawPath = ""
	query := remote.Query()
	remote.RawQuery = query.Encode()
	return remote.String()
}
