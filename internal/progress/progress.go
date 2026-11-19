package progress

import (
	"fmt"
	"io"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"github.com/winezer0/okserver/internal/transfer"
)

func NewProgress() *mpb.Progress {
	return mpb.New(
		mpb.WithWidth(30),
		mpb.PopCompletedMode(),
		mpb.WithAutoRefresh(),
	)
}

func NewProgressBar(bar *mpb.Progress, mode, path string, size int64) *mpb.Bar {
	return bar.AddBar(size, mpb.AppendDecorators(
		decor.Any(func(st decor.Statistics) string {
			var percent int64
			switch {
			case st.Total <= 0:
			case st.Current >= st.Total:
				percent = 100
			default:
				percent = 100 * st.Current / st.Total
			}

			current := transfer.ConvertByte(st.Current, true)
			if st.Completed {
				return fmt.Sprintf("done %s %s %s", current, mode, path)
			}
			return fmt.Sprintf("%3d%% %s %s %s", percent, current, mode, path)
		}, decor.WCSyncWidth),
	))
}

type Writer struct {
	writer io.Writer
	bar    *mpb.Bar
}

func NewProgressWriter(writer io.Writer, bar *mpb.Bar) *Writer {
	return &Writer{writer: writer, bar: bar}
}
func (w *Writer) Write(buffer []byte) (int, error) {
	n, err := w.writer.Write(buffer)
	w.bar.IncrBy(n)
	return n, err
}
func (w *Writer) Close()                   { w.bar.Abort(false); w.bar.EnableTriggerComplete() }
func (w *Writer) SetCurrent(current int64) { w.bar.SetCurrent(current) }
