$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
    function Find-NsisCompiler {
        $installed = Get-Command makensis.exe -ErrorAction SilentlyContinue
        if ($installed) { return $installed.Source }
        foreach ($candidate in @(
            "${env:ProgramFiles(x86)}\NSIS\makensis.exe",
            "$env:ProgramFiles\NSIS\makensis.exe",
            "$env:LOCALAPPDATA\mygo\nsis-3.13\Bin\makensis.exe",
            "$env:LOCALAPPDATA\mygo\nsis-3.13\makensis.exe"
        )) {
            if (Test-Path -LiteralPath $candidate) { return $candidate }
        }
    }
    $compiler = Find-NsisCompiler
    if (-not $compiler) {
        # MyGo downloads its verified NSIS distribution on a fresh machine.
        go tool mygo build -platform windows/amd64 -o out\fnmovie-20261007
        if ($LASTEXITCODE -eq 0) { return }
        $compiler = Find-NsisCompiler
        if (-not $compiler) { throw 'MyGo could not prepare the NSIS compiler.' }
    }
    $shimDir = Join-Path $projectRoot 'build\nsis-utf8'
    New-Item -ItemType Directory -Force -Path $shimDir | Out-Null
    # MyGo 0.2.18 writes UTF-8 NSIS source without a BOM, while makensis defaults
    # to the Windows ANSI code page. Scope the explicit encoding to this build.
    $shimSource = @'
package main
import (
    "os"
    "os/exec"
)
func main() {
    args := append([]string{"/INPUTCHARSET", "UTF8"}, os.Args[1:]...)
    cmd := exec.Command(os.Getenv("FNMOVIE_NSIS_COMPILER"), args...)
    cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
    if err := cmd.Run(); err != nil {
        if exit, ok := err.(*exec.ExitError); ok { os.Exit(exit.ExitCode()) }
        os.Stderr.WriteString(err.Error()+"\n")
        os.Exit(1)
    }
}
'@
    [System.IO.File]::WriteAllText((Join-Path $shimDir 'main.go'), $shimSource, [System.Text.UTF8Encoding]::new($false))
    go build -o (Join-Path $shimDir 'makensis.exe') (Join-Path $shimDir 'main.go')
    if ($LASTEXITCODE -ne 0) { throw 'Could not build the UTF-8 NSIS adapter.' }
    $previousPath = $env:PATH
    $previousCompiler = $env:FNMOVIE_NSIS_COMPILER
    try {
        $env:PATH = "$shimDir;$previousPath"
        $env:FNMOVIE_NSIS_COMPILER = $compiler
        go tool mygo build -platform windows/amd64 -o out\fnmovie-20261007
        if ($LASTEXITCODE -ne 0) { throw 'Windows build failed.' }
    } finally {
        $env:PATH = $previousPath
        $env:FNMOVIE_NSIS_COMPILER = $previousCompiler
    }
} finally {
    Pop-Location
}
