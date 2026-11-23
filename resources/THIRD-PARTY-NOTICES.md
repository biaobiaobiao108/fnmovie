# Third-party notices

## mpv

This application bundles `libmpv-2.dll`, the Windows x86_64 client library from
[shinchiro's Windows build releases](https://github.com/shinchiro/mpv-winbuild-cmake/releases/tag/20260928).
The included build was published on 2026-09-28. mpv is free software under the
GNU General Public License, version 2 or later. The full license text is in
`licenses/mpv-GPL-2.0.txt`. The corresponding mpv source revision is
[eb0ee10315](https://github.com/mpv-player/mpv/tree/eb0ee10315); the Windows
build project and build instructions are available at the release link above.

哞哩影院 uses the libmpv client API directly and does not bundle or launch
`mpv.exe`. Playback controls are drawn by the MyGo native UI.

The macOS bundle includes Homebrew's arm64 libmpv and the libraries linked by
that build. Their versions, available license files and Homebrew build recipes
with upstream source URLs are in `player/notices` in the application resources.

The bundled builds include FFmpeg and other third-party components.
Their respective copyright and license terms apply. See the source and build
project above for the component configuration and notices.
