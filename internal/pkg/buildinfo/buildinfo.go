// Package buildinfo reports the version of the running binary.
package buildinfo

import (
	"runtime/debug"
	"strings"
)

// Version returns the module version stamped into the binary in SemVer form
// without the leading "v": "X.Y.Z" when built at a vX.Y.Z tag, a pre-release
// such as "X.Y.Z-0.20261109060017-7897bbd61e4c" after it, and "(devel)" when
// no version is stamped.
func Version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Format("")
	}

	return Format(info.Main.Version)
}

// Format turns a Go module version into the displayed version: the leading
// "v" is dropped and an empty version becomes "(devel)".
func Format(moduleVersion string) string {
	if moduleVersion == "" {
		return "(devel)"
	}

	return strings.TrimPrefix(moduleVersion, "v")
}
