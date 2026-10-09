module fnmovie

go 1.27.1

require (
	github.com/ebitengine/purego v0.11.1
	github.com/egoist/mygo v0.3.6
	github.com/egoist/mygo/fnmovieoverlay v0.0.0
	golang.org/x/image v0.46.0
)

replace github.com/egoist/mygo/fnmovieoverlay => ./internal/mygooverlay

require github.com/go-text/typesetting v0.3.5 // indirect

tool github.com/egoist/mygo/cmd/mygo
