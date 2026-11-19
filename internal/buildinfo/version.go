package buildinfo

import (
	"fmt"
	"runtime/debug"
)

func String(versions ...string) string {
	version := "dev"
	if len(versions) > 0 {
		version = versions[0]
	}
	committed := "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.time" && setting.Value != "" {
				committed = setting.Value
			}
		}
	}
	return Format(version, committed)
}

func Format(version, committed string) string {
	if committed == "" {
		committed = "unknown"
	}
	return fmt.Sprintf("%s (commit %s)", version, committed)
}
