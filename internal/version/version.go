// Package version holds aiinventory's build-time version string.
package version

// Version is overridden at build time via:
//
//	go build -ldflags "-X github.com/pborges/aiinventory/internal/version.Version=v1.2.3"
//
// The release Docker image sets this from the pushed git tag (see
// .github/workflows/docker.yml); a plain `go build` leaves it as "dev".
var Version = "dev"
