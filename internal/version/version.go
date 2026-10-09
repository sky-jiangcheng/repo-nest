// app via `-ldflags "-X main.version=$(VERSION)"`. When built without
// ldflags the constant below is used as the default.
package version

// Version is the current RepoNest version. The canonical value lives in
// wails.json (info.productVersion); scripts/bump-version.sh propagates it
// here. CI builds override this via ldflags at link time.
const Version = "1.16.1"
