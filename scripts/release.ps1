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
        throw "Git 命令执行失败：git $($GitArgs -join ' ')（退出代码 $($result.ExitCode)）。$($result.Text)"
    }
    $result.Text
}

function Write-ReleaseMessage {
    param(
        [Parameter(Mandatory)][string]$Glyph,
        [Parameter(Mandatory)][string]$Message,
        [ConsoleColor]$IconColor = [ConsoleColor]::Gray,
        [ConsoleColor]$MessageColor = [ConsoleColor]::Gray
    )

    Write-Host " $Glyph" -NoNewline -ForegroundColor $IconColor
    Write-Host " $Message" -ForegroundColor $MessageColor
}

$repoRoot = Invoke-Git -GitArgs @('-C', $PSScriptRoot, 'rev-parse', '--show-toplevel')
Push-Location -LiteralPath $repoRoot
try {
    Write-ReleaseMessage '' '正在检查工作区、远程分支和版本标签…' Cyan
    $branch = Invoke-Git -GitArgs @('branch', '--show-current')
    if ([string]::IsNullOrWhiteSpace($branch)) {
        throw '请在具名分支上运行发版命令，不能处于分离的 HEAD 状态。'
    }

    $upstream = Invoke-Git -GitArgs @('rev-parse', '--abbrev-ref', '--symbolic-full-name', '@{upstream}')
    $separator = $upstream.IndexOf('/')
    if ($separator -lt 1 -or $separator -eq ($upstream.Length - 1)) {
        throw "无法从上游分支 '$upstream' 中识别远程仓库和分支。"
    }
    $remote = $upstream.Substring(0, $separator)
    $remoteBranch = $upstream.Substring($separator + 1)

    $workingTree = Invoke-Git -GitArgs @('status', '--porcelain', '--untracked-files=all')
    if (-not [string]::IsNullOrWhiteSpace($workingTree)) {
        throw '工作区有未提交的改动。请先提交、暂存或处理这些改动，再运行发版命令。'
    }

    Invoke-Git -GitArgs @('fetch', $remote, '--tags') | Out-Null
    $counts = (Invoke-Git -GitArgs @('rev-list', '--left-right', '--count', "$upstream...HEAD")) -split '\s+'
    if ($counts.Count -ne 2) {
        throw "无法比较当前分支与上游分支 '$upstream'。"
    }
    $behind = [int]$counts[0]
    $ahead = [int]$counts[1]
    if ($behind -gt 0) {
        throw "当前分支 '$branch' 落后上游 '$upstream' $behind 个提交。请先拉取或变基，再发版。"
    }

    Invoke-Git -GitArgs @('var', 'GIT_AUTHOR_IDENT') | Out-Null
    Invoke-Git -GitArgs @('var', 'GIT_COMMITTER_IDENT') | Out-Null

    $configPath = Join-Path $repoRoot 'mygo.json'
    $configText = [System.IO.File]::ReadAllText($configPath, [System.Text.Encoding]::UTF8)
    $config = $configText | ConvertFrom-Json
    $currentVersion = [string]$config.version
    if ($currentVersion -notmatch '^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$') {
        throw "mygo.json 中的版本号 '$currentVersion' 不符合 major.minor.patch 格式。"
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
        $head = Invoke-Git -GitArgs @('rev-parse', 'HEAD')
        if (-not [string]::IsNullOrWhiteSpace($remoteTag)) {
            $remoteTagFields = $remoteTag -split '\s+'
            if ($remoteTagFields.Count -lt 2 -or $remoteTagFields[1] -ne "refs/tags/$currentTag") {
                throw "无法解析远程标签 '$currentTag'：$remoteTag"
            }
            if ($localTag.ExitCode -ne 0) {
                throw "远程标签 '$currentTag' 已存在，但本地未找到对应标签；为避免意外增加版本，已停止发版。"
            }
            $localTagObject = Invoke-GitResult -GitArgs @('rev-parse', '--verify', '--quiet', $currentTagRef)
            if ($localTagObject.ExitCode -ne 0 -or $localTagObject.Text -ne $remoteTagFields[0]) {
                throw "本地与远程标签 '$currentTag' 不一致；为避免覆盖标签或增加版本，已停止发版。"
            }
            if ($localTag.Text -ne $head) {
                throw "远程标签 '$currentTag' 指向提交 $($localTag.Text)，当前版本提交为 $head；为避免增加版本，已停止发版。"
            }
            Write-ReleaseMessage '' "$currentTag 已指向当前版本提交，远程发布已完成，无需再次增加版本。" Green Green
            return
        }
        if ($localTag.ExitCode -eq 0 -and $localTag.Text -ne $head) {
            throw "本地标签 '$currentTag' 指向了其他提交，为避免覆盖标签，已停止发版。"
        }
        if ($localTag.ExitCode -gt 1) {
            throw "无法检查本地标签 '$currentTag'：$($localTag.Text)"
        }

        if ($WhatIf) {
            Write-ReleaseMessage '' "将继续发布 $currentTag。" Cyan
            if ($ahead -gt 0) {
                Write-ReleaseMessage '' "同时推送分支 '$branch' 上已有的 $ahead 个本地提交。" DarkCyan
            }

            Write-ReleaseMessage '' '预览模式结束，未修改文件或推送内容。' DarkGray
            return
        }

        if ($localTag.ExitCode -ne 0) {
            Write-ReleaseMessage '' "创建标签 $currentTag…" Magenta
            Invoke-Git -GitArgs @('tag', '-a', $currentTag, '-m', "$($config.name) $currentTag") | Out-Null
        }
        if ($ahead -gt 0) {
            Write-ReleaseMessage '' "推送分支 '$branch'…" Blue
            Invoke-Git -GitArgs @('push', $remote, "HEAD:refs/heads/$remoteBranch") | Out-Null
        }
        Write-ReleaseMessage '' "推送标签 $currentTag…" Blue
        Invoke-Git -GitArgs @('push', $remote, "refs/tags/$currentTag") | Out-Null
        Write-ReleaseMessage '' "$currentTag 已发布，GitHub Actions 将自动构建 Windows amd64 程序并创建 Release。" Green Green
        return
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
        throw "本地标签 '$nextTag' 已存在，无法重复发布。"
    }
    if ($localTag.ExitCode -ne 1) {
        throw "无法检查本地标签 '$nextTag'：$($localTag.Text)"
    }
    $remoteTag = Invoke-Git -GitArgs @('ls-remote', '--refs', $remote, "refs/tags/$nextTag")
    if (-not [string]::IsNullOrWhiteSpace($remoteTag)) {
        throw "远程标签 '$nextTag' 已存在，无法重复发布。"
    }

    $versionPattern = '(?m)^(\s*"version"\s*:\s*")[^"]+("\s*,?\s*)$'
    $versionMatches = [regex]::Matches($configText, $versionPattern)
    if ($versionMatches.Count -ne 1) {
        throw 'mygo.json 中应当且只能有一个顶层 version 字段。'
    }
    $updatedConfig = [regex]::Replace($configText, $versionPattern, ('${1}' + $nextVersion + '${2}'), 1)
    $null = $updatedConfig | ConvertFrom-Json

    if ($WhatIf) {
        $levelName = switch ($Level) {
            'patch' { '补丁版本' }
            'minor' { '次版本' }
            'major' { '主版本' }
        }
        Write-ReleaseMessage '' "版本：$currentVersion → $nextVersion（$levelName）" DarkYellow
        Write-ReleaseMessage '' "将更新 mygo.json 并创建提交，然后创建标签 $nextTag。" Yellow
        Write-ReleaseMessage '' "将推送分支 '$branch' 和标签 $nextTag。" Blue
        if ($ahead -gt 0) {
            Write-ReleaseMessage '' "还会一并推送分支上的 $ahead 个已有本地提交。" Yellow
        }
        Write-ReleaseMessage '' '预览模式结束，未修改文件或推送内容。' DarkGray
        return
    }

    Write-ReleaseMessage '' "准备发布：$currentVersion → $nextVersion" DarkYellow
    if ($ahead -gt 0) {
        Write-ReleaseMessage '' "分支 '$branch' 还有 $ahead 个已提交的本地提交，也会一并推送。" DarkCyan
    }
    [System.IO.File]::WriteAllText($configPath, $updatedConfig, [System.Text.UTF8Encoding]::new($false))

    Write-ReleaseMessage '' '更新版本号并创建提交…' Yellow
    Invoke-Git -GitArgs @('add', '--', 'mygo.json') | Out-Null
    Invoke-Git -GitArgs @('commit', '-m', "Bump application version to $nextVersion") | Out-Null
    Write-ReleaseMessage '' "创建标签 $nextTag…" Magenta
    Invoke-Git -GitArgs @('tag', '-a', $nextTag, '-m', "$($config.name) $nextTag") | Out-Null

    try {
        Write-ReleaseMessage '' "推送分支 '$branch'…" Blue
        Invoke-Git -GitArgs @('push', $remote, "HEAD:refs/heads/$remoteBranch") | Out-Null
        Write-ReleaseMessage '' "推送标签 $nextTag…" Blue
        Invoke-Git -GitArgs @('push', $remote, "refs/tags/$nextTag") | Out-Null
    } catch {
        throw "$nextTag 已在本地提交并创建标签，但推送失败。再次运行 'fnmovie $Level' 可继续发布；也可以手动推送分支和标签。$($_.Exception.Message)"
    }

    Write-ReleaseMessage '' "$nextTag 已推送，GitHub Actions 将自动构建 Windows amd64 程序并创建 Release。" Green Green
} finally {
    Pop-Location
}
