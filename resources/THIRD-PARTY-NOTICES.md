# Third-party notices

## mpv

This application bundles the Windows x86_64 build of [mpv](https://mpv.io/),
downloaded from [shinchiro's Windows build releases](https://github.com/shinchiro/mpv-winbuild-cmake/releases).
The included build was published on 2026-10-07. mpv is free software under the
GNU General Public License, version 2 or later. The full license text is in
`licenses/mpv-GPL-2.0.txt`. The corresponding mpv source revision is
[eb0ee10315](https://github.com/mpv-player/mpv/tree/eb0ee10315); the Windows
build project and build instructions are available at the release link above.

The bundled `windows-amd64/player/scripts/osc.lua` is mpv's upstream OSC script
from the same source revision and is loaded explicitly because the app disables
user mpv configuration files.

The bundled Windows build includes FFmpeg and other third-party components.
Their respective copyright and license terms apply. See the source and build
project above for the component configuration and notices.
