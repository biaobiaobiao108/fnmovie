"""Copy the arm64 libmpv dependency graph and make its paths bundle-relative."""

import re
import shutil
import subprocess
import sys
from pathlib import Path


def output(*args):
    return subprocess.check_output(args, text=True).strip()


def macho(file, flag):
    return output("otool", "-arch", "arm64", flag, file).splitlines()[1:]


def rpaths(file):
    return re.findall(
        r"cmd LC_RPATH\s+cmdsize \d+\s+path (.*?) \(offset",
        "\n".join(macho(file, "-l")),
    )


def system_library(name):
    return name.startswith(("/usr/lib/", "/System/Library/"))


def resolve(name, file, search):
    if name.startswith("@loader_path/"):
        return file.parent / name.removeprefix("@loader_path/")
    if name.startswith("@rpath/"):
        relative = name.removeprefix("@rpath/")
        for directory in search:
            candidate = Path(directory) / relative
            if candidate.is_file():
                return candidate
    elif name.startswith("/"):
        return Path(name)
    raise ValueError(f"cannot resolve {name} required by {file}")


def bundle(destination):
    prefix = Path(output("brew", "--prefix"))
    cellar = Path(output("brew", "--cellar")).resolve()
    mpv = Path(output("brew", "--prefix", "mpv")) / "lib/libmpv.dylib"
    destination.mkdir()
    sources = {}
    packages = set()

    def copy(source, name, inherited_search=()):
        source = source.resolve(strict=True)
        if name in sources:
            if sources[name] != source:
                raise ValueError(f"two libraries have the same bundle name: {name}")
            return
        package, version, *_ = source.relative_to(cellar).parts
        packages.add(cellar / package / version)
        sources[name] = source
        target = destination / name
        subprocess.run(["lipo", source, "-verify_arch", "arm64"], check=True)
        if output("lipo", "-archs", source) == "arm64":
            shutil.copyfile(source, target)
        else:
            subprocess.run(["lipo", source, "-thin", "arm64", "-output", target], check=True)

        native_rpaths = rpaths(source)
        search = tuple(value.replace("@loader_path", str(source.parent)) for value in native_rpaths)
        search += inherited_search + (str(prefix / "lib"),)
        ids = {line.strip() for line in macho(source, "-D")}
        edits = ["-id", f"@loader_path/{name}"]
        for line in macho(source, "-L"):
            dependency = line.strip().split(" (compatibility version ", 1)[0]
            if dependency in ids or system_library(dependency):
                continue
            path = resolve(dependency, source, search)
            if system_library(str(path)):
                edits.extend(("-change", dependency, str(path)))
                continue
            copy(path, path.name, search)
            edits.extend(("-change", dependency, f"@loader_path/{path.name}"))
        for value in native_rpaths:
            edits.extend(("-delete_rpath", value))
        subprocess.run(["install_name_tool", *edits, target], check=True)
        subprocess.run(["codesign", "--force", "--sign", "-", target], check=True)

    copy(mpv, "libmpv.dylib")

    notices = destination / "notices"
    notices.mkdir()
    for package in sorted(packages):
        notice_dir = notices / package.parent.name
        notice_dir.mkdir()
        for path in package.iterdir():
            if path.is_file() and path.name.upper().startswith(("LICENSE", "COPYING", "COPYRIGHT", "NOTICE", "AUTHORS")):
                shutil.copyfile(path, notice_dir / path.name)
        for path in (package / ".brew").glob("*.rb"):
            shutil.copyfile(path, notice_dir / path.name)
    (notices / "components.txt").write_text(
        "Homebrew libraries bundled with the macOS player\n\n"
        + "".join(f"{package.relative_to(cellar)} https://formulae.brew.sh/formula/{package.parent.name}\n" for package in sorted(packages)),
        encoding="utf-8",
    )

    for name in sources:
        for line in macho(destination / name, "-L"):
            dep = line.strip().split(" (compatibility version ", 1)[0]
            if not system_library(dep) and (
                not dep.startswith("@loader_path/") or dep.removeprefix("@loader_path/") not in sources
            ):
                raise ValueError(f"unbundled dependency in {name}: {dep}")


if __name__ == "__main__":
    bundle(Path(sys.argv[1]))
