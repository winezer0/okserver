package observability

import (
	"context"

	"github.com/winezer0/okserver/internal/upload"
)

func UploadObserver(logger RequestLogger) func(upload.Event) {
	return func(event upload.Event) {
		if logger == nil {
			return
		}
		fields := []any{"task_id", event.TaskID, "offset", event.Offset}
		if event.Err != nil {
			logger.ErrorContext(context.Background(), event.Name, append(fields, "error", event.Err.Error())...)
		} else {
			logger.InfoContext(context.Background(), event.Name, fields...)
		}
	}
}
