[CmdletBinding()]
param(
    [Parameter(Mandatory, Position = 0)]
    [ValidateSet('patch', 'minor', 'major')]
    [string]$Level,

    [switch]$WhatIf
)

$ErrorActionPreference = 'Stop'
if (Get-Variable -Name PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue) {
    $PSNativeCommandUseErrorActionPreference = $false
}

function Invoke-GitResult {
    param([Parameter(Mandatory)][string[]]$GitArgs)

    $output = & git @GitArgs 2>&1
    $exitCode = $LASTEXITCODE
    [pscustomobject]@{
        ExitCode = $exitCode
        Text     = (($output | ForEach-Object { [string]$_ }) -join [Environment]::NewLine).Trim()
    }
}

function Invoke-Git {
    param([Parameter(Mandatory)][string[]]$GitArgs)

    $result = Invoke-GitResult -GitArgs $GitArgs
    if ($result.ExitCode -ne 0) {
        throw "git $($GitArgs -join ' ') failed (exit $($result.ExitCode)): $($result.Text)"
    }
    $result.Text
}

$repoRoot = Invoke-Git -GitArgs @('-C', $PSScriptRoot, 'rev-parse', '--show-toplevel')
Push-Location -LiteralPath $repoRoot
try {
    $branch = Invoke-Git -GitArgs @('branch', '--show-current')
    if ([string]::IsNullOrWhiteSpace($branch)) {
        throw 'Release must run from a named branch, not detached HEAD.'
    }

    $upstream = Invoke-Git -GitArgs @('rev-parse', '--abbrev-ref', '--symbolic-full-name', '@{upstream}')
    $separator = $upstream.IndexOf('/')
    if ($separator -lt 1 -or $separator -eq ($upstream.Length - 1)) {
        throw "Could not determine the remote branch from upstream '$upstream'."
    }
    $remote = $upstream.Substring(0, $separator)
    $remoteBranch = $upstream.Substring($separator + 1)

    $workingTree = Invoke-Git -GitArgs @('status', '--porcelain', '--untracked-files=all')
    if (-not [string]::IsNullOrWhiteSpace($workingTree)) {
        throw 'Release requires a clean worktree. Commit, stash, or discard the current changes first.'
    }

    Invoke-Git -GitArgs @('fetch', $remote, '--tags') | Out-Host
    $counts = (Invoke-Git -GitArgs @('rev-list', '--left-right', '--count', "$upstream...HEAD")) -split '\s+'
    if ($counts.Count -ne 2) {
        throw "Could not compare HEAD with '$upstream'."
    }
    $behind = [int]$counts[0]
    $ahead = [int]$counts[1]
    if ($behind -gt 0) {
        throw "Branch '$branch' is behind '$upstream' by $behind commit(s). Pull or rebase before releasing."
    }

    Invoke-Git -GitArgs @('var', 'GIT_AUTHOR_IDENT') | Out-Null
    Invoke-Git -GitArgs @('var', 'GIT_COMMITTER_IDENT') | Out-Null

    $configPath = Join-Path $repoRoot 'mygo.json'
    $configText = [System.IO.File]::ReadAllText($configPath, [System.Text.Encoding]::UTF8)
    $config = $configText | ConvertFrom-Json
    $currentVersion = [string]$config.version
    if ($currentVersion -notmatch '^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$') {
        throw "mygo.json version '$currentVersion' is not a plain major.minor.patch version."
    }
    $major = [long]::Parse($Matches[1], [Globalization.CultureInfo]::InvariantCulture)
    $minor = [long]::Parse($Matches[2], [Globalization.CultureInfo]::InvariantCulture)
    $patch = [long]::Parse($Matches[3], [Globalization.CultureInfo]::InvariantCulture)

    # Resume a release if a previous run committed the version but stopped
    # before pushing its branch or tag.
    $currentTag = "v$currentVersion"
    $lastSubject = Invoke-Git -GitArgs @('log', '-1', '--format=%s')
    if ($lastSubject -eq "Bump application version to $currentVersion") {
        $currentTagRef = "refs/tags/$currentTag"
        $localTag = Invoke-GitResult -GitArgs @('rev-parse', '--verify', '--quiet', "$currentTagRef`^{commit}")
        $remoteTag = Invoke-Git -GitArgs @('ls-remote', '--refs', $remote, "refs/tags/$currentTag")
        if ([string]::IsNullOrWhiteSpace($remoteTag)) {
            $head = Invoke-Git -GitArgs @('rev-parse', 'HEAD')
            if ($localTag.ExitCode -eq 0 -and $localTag.Text -ne $head) {
                throw "Local tag '$currentTag' points to a different commit; refusing to move it."
            }
            if ($localTag.ExitCode -gt 1) {
                throw "Could not inspect local tag '$currentTag': $($localTag.Text)"
            }

            if ($WhatIf) {
                Write-Host "Would resume release $currentTag (branch is ahead by $ahead commit(s))."
                return
            }
            if ($localTag.ExitCode -ne 0) {
                Invoke-Git -GitArgs @('tag', '-a', $currentTag, '-m', "$($config.name) $currentTag") | Out-Host
            }
            if ($ahead -gt 0) {
                Invoke-Git -GitArgs @('push', $remote, "HEAD:refs/heads/$remoteBranch") | Out-Host
            }
            Invoke-Git -GitArgs @('push', $remote, "refs/tags/$currentTag") | Out-Host
            Write-Host "Resumed and pushed release $currentTag. GitHub Actions will build and publish it."
            return
        }
    }

    switch ($Level) {
        'patch' { $patch++ }
        'minor' { $minor++; $patch = 0 }
        'major' { $major++; $minor = 0; $patch = 0 }
    }
    $nextVersion = "$major.$minor.$patch"
    $nextTag = "v$nextVersion"

    $localTag = Invoke-GitResult -GitArgs @('show-ref', '--verify', '--quiet', "refs/tags/$nextTag")
    if ($localTag.ExitCode -eq 0) {
        throw "Local tag '$nextTag' already exists."
    }
    if ($localTag.ExitCode -ne 1) {
        throw "Could not check local tag '$nextTag': $($localTag.Text)"
    }
    $remoteTag = Invoke-Git -GitArgs @('ls-remote', '--refs', $remote, "refs/tags/$nextTag")
    if (-not [string]::IsNullOrWhiteSpace($remoteTag)) {
        throw "Remote tag '$nextTag' already exists."
    }

    if ($WhatIf) {
        Write-Host "Would bump $currentVersion -> $nextVersion ($Level), commit mygo.json, create tag $nextTag, then push '$branch' and the tag."
        if ($ahead -gt 0) {
            Write-Host "This will also push the $ahead already-committed local commit(s) on '$branch'."
        }
        return
    }

    $versionPattern = '(?m)^(\s*"version"\s*:\s*")[^"]+("\s*,?\s*)$'
    $versionMatches = [regex]::Matches($configText, $versionPattern)
    if ($versionMatches.Count -ne 1) {
        throw 'Expected exactly one top-level version field in mygo.json.'
    }
    $updatedConfig = [regex]::Replace($configText, $versionPattern, ('$1' + $nextVersion + '$2'), 1)
    $null = $updatedConfig | ConvertFrom-Json
    [System.IO.File]::WriteAllText($configPath, $updatedConfig, [System.Text.UTF8Encoding]::new($false))

    Invoke-Git -GitArgs @('add', '--', 'mygo.json') | Out-Host
    Invoke-Git -GitArgs @('commit', '-m', "Bump application version to $nextVersion") | Out-Host
    Invoke-Git -GitArgs @('tag', '-a', $nextTag, '-m', "$($config.name) $nextTag") | Out-Host

    try {
        Invoke-Git -GitArgs @('push', $remote, "HEAD:refs/heads/$remoteBranch") | Out-Host
        Invoke-Git -GitArgs @('push', $remote, "refs/tags/$nextTag") | Out-Host
    } catch {
        throw "Release $nextTag is committed and tagged locally, but a push failed. Run 'fnmovie $Level' again to resume, or push the branch and tag manually. $($_.Exception.Message)"
    }

    Write-Host "Pushed release $nextTag. GitHub Actions will build and publish it."
} finally {
    Pop-Location
}
