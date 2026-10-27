// Package buildinfo identifies the running build.
package buildinfo

// Version is the source commit of this build. Release images set it at link
// time:
//
//	go build -ldflags "-X github.com/gym-pulse/gym-pulse-api/internal/buildinfo.Version=<sha>"
//
// It is never modified at run time.
var Version = "dev"
