package upload

// Event reports lifecycle changes without tying persistence to a logging library.
// Observers may run concurrently and must not call back into the manager.
type Event struct {
	Name   string
	TaskID string
	Offset int64
	Err    error
}

func (m *Manager) emit(name, id string, offset int64, err error) {
	if m.observe != nil {
		m.observe(Event{Name: name, TaskID: id, Offset: offset, Err: err})
	}
}
