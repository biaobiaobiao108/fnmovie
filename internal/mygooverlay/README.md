# MyGo playback overlay adapter

This package is the compatibility boundary for the player's per-pixel
transparent controls. MyGo exposes content attachment through
`mygo.Content`, whose method signature includes `internal/surface.Conn`. Go's
`internal` import rule means this adapter must remain under the
`github.com/egoist/mygo` module path. The Windows presenter also wraps the
internal surface so it can use `UpdateLayeredWindow` without losing per-pixel
alpha.

The root module and this nested module must require the same MyGo version. The
compile-time `mygo.Content` assertion and overlay tests should be checked on
every MyGo upgrade. If MyGo adds a public presenter or alpha-composition hook,
replace this adapter with that supported extension point.
