$ErrorActionPreference = 'Stop'

$profilePath = $PROFILE.CurrentUserAllHosts
$profileDirectory = Split-Path -Parent $profilePath
if (-not (Test-Path -LiteralPath $profileDirectory)) {
    New-Item -ItemType Directory -Path $profileDirectory -Force | Out-Null
}

$startMarker = '# >>> fnmovie release command >>>'
$endMarker = '# <<< fnmovie release command <<<'
$commandBlock = @'
# >>> fnmovie release command >>>
function fnmovie {
    $repoRoot = & git rev-parse --show-toplevel 2>$null
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace(($repoRoot -join ''))) {
        throw 'Run fnmovie from inside a fnmovie Git worktree.'
    }
    $releaseScript = Join-Path ($repoRoot -join '').Trim() 'scripts/release.ps1'
    if (-not (Test-Path -LiteralPath $releaseScript)) {
        throw "Release script not found: $releaseScript"
    }
    & $releaseScript @args
}
# <<< fnmovie release command <<<
'@

$profileContents = ''
if (Test-Path -LiteralPath $profilePath) {
    $profileContents = [System.IO.File]::ReadAllText($profilePath, [System.Text.Encoding]::UTF8)
}

$escapedStart = [regex]::Escape($startMarker)
$escapedEnd = [regex]::Escape($endMarker)
$blockPattern = "(?ms)^$escapedStart\r?\n.*?^$escapedEnd(?:\r?\n)?"
if ([regex]::IsMatch($profileContents, $blockPattern)) {
    $profileContents = [regex]::Replace($profileContents, $blockPattern, '')
}
$separator = if ($profileContents.Length -gt 0 -and -not $profileContents.EndsWith("`n")) { "`r`n`r`n" } elseif ($profileContents.Length -gt 0) { "`r`n" } else { '' }
$profileContents += $separator + $commandBlock + "`r`n"
[System.IO.File]::WriteAllText($profilePath, $profileContents, [System.Text.UTF8Encoding]::new($false))
Write-Host "Installed fnmovie in PowerShell profile: $profilePath"
Write-Host 'Open a new PowerShell session, or reload the profile, to use fnmovie patch|minor|major.'
