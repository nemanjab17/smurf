package version

// Version is set at build time via -ldflags "-X ...version.Version=v0.3.0".
// Falls back to "dev" for local builds.
var Version = "dev"
