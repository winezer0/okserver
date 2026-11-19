package buildinfo

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Version can be set by release builds using -ldflags -X.
var Version = "dev"

func String() string {
	revision, committed, dirty := "unknown", "unknown", false
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.time":
				committed = setting.Value
			case "vcs.modified":
				dirty = setting.Value == "true"
			}
		}
	}
	return fmt.Sprintf("okserver %s commit=%s modified=%t commit_time=%s go=%s platform=%s/%s", Version, revision, dirty, committed, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
