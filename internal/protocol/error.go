package protocol

type Error struct {
	Error  string  `json:"error"`
	Upload *Upload `json:"upload,omitempty"`
}
