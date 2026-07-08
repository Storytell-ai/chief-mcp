package mcp

import "runtime/debug"

// Version is the release version, injected at build time by the command's main
// package via -ldflags "-X main.version=..." assigning into it. Local builds
// leave it empty and fall back to the VCS revision the Go toolchain embeds.
var Version = ""

// buildVersion reports the injected release version, or the VCS revision of a
// local build, or "dev" when neither is available.
func buildVersion() string {
	if Version != "" {
		return Version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}

	var revision string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}

	if revision == "" {
		return "dev"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if dirty {
		return revision + "-dirty"
	}
	return revision
}
