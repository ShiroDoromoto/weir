// Package version carries the version of this build.
package version

// Version is the version of this build. A build made from a release tag
// overwrites it with:
//
//	-ldflags "-X github.com/ShiroDoromoto/weir/internal/version.Version=<tag>"
//
// A build made any other way says so by leaving it at "dev".
var Version = "dev"
