package protocol

type Error struct {
	Error  string  `json:"error"`
	Code   string  `json:"code,omitempty"`
	Upload *Upload `json:"upload,omitempty"`
}
