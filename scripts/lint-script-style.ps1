[CmdletBinding()]
param(
    # 扫描目录，默认 scripts/ 自身。
    [string]$Path
)

# 机械拦截一类 PowerShell 错误：把 -f（格式运算符）写在**命令参数**位置。
#
# 典型错法（本仓库在 autosync.ps1 上踩了 4 次）：
#     Write-Log 'WARN' ("{0} 失败") -f $x
# 这里的 -f 不会被当成运算符，而会被解析成名为 f 的参数，
# 运行时抛「找不到与参数名称 'f' 匹配的参数」——只在走到那一行时才炸，
# 所以很容易漏测。正确写法是把 -f 关进括号，或先赋值再传：
#     $msg = "{0} 失败" -f $x ;  Write-Log 'WARN' $msg
#
# 为什么必须用 AST 而不是正则：正则无法区分下面两种 -f
#     Write-Log 'INFO'  (("x={0}") -f $a)     # 正确，-f 在括号内
#     Write-Log 'INFO'  ("x={0}") -f $a       # 错误，-f 在参数位
# 两者含同样的 `) -f` 子串。只有语法树能判断 -f 落在 CommandAst 的元素里。
#
# 唯一例外是原生命令（git / cargo / taskkill …）：它们确实可能有 -f 形式的开关。
# PowerShell 自带的 cmdlet 与仓库里的函数都没有 -f —— 它和 -Filter / -Force / -File
# 冲突，必然是歧义参数，所以出现即是写错。

. (Join-Path $PSScriptRoot 'common.ps1')

$scanRoot = if ($Path) { $Path } else { $PSScriptRoot }
$nativeAllow = @(
    'git', 'npm', 'npx', 'node', 'go', 'gofmt', 'cargo', 'rustc', 'python', 'python3',
    'taskkill', 'robocopy', 'findstr', 'scp', 'ssh', 'where', 'cmd', 'pwsh', 'powershell',
    'curl', 'tar', 'zip', 'unzip', 'msbuild', 'dotnet', 'winget'
)

$problems = [System.Collections.Generic.List[string]]::new()

foreach ($file in Get-ChildItem -LiteralPath $scanRoot -Filter '*.ps1' -File) {
    $tokens = $null
    $parseErrors = $null
    $ast = [System.Management.Automation.Language.Parser]::ParseFile(
        $file.FullName, [ref]$tokens, [ref]$parseErrors)

    if ($parseErrors -and $parseErrors.Count -gt 0) {
        foreach ($pe in $parseErrors) {
            $problems.Add(("{0}:{1} 语法错误: {2}" -f $file.Name, $pe.Extent.StartLineNumber, $pe.Message))
        }
        continue
    }

    $commands = $ast.FindAll(
        { $args[0] -is [System.Management.Automation.Language.CommandAst] }, $true)

    foreach ($cmd in $commands) {
        $name = $cmd.GetCommandName()
        if (-not $name) { continue }
        if ($nativeAllow -contains $name.ToLowerInvariant()) { continue }

        foreach ($el in $cmd.CommandElements) {
            if ($el -isnot [System.Management.Automation.Language.CommandParameterAst]) { continue }
            # 只认 `-x` 形式；Windows 风格的 `/F`（taskkill）不算参数写法。
            if (-not $el.Extent.Text.StartsWith('-')) { continue }
            if ($el.ParameterName -ne 'f') { continue }

            $snippet = ($cmd.Extent.Text -split "`n")[0].Trim()
            $problems.Add(('{0}:{1} -f 被放在了 {2} 的参数位；应写成（"…" -f …）或在括号内先算好：' -f
                $file.Name, $el.Extent.StartLineNumber, $name) + $snippet)
        }
    }
}

if ($problems.Count -gt 0) {
    Write-Host "PowerShell 脚本风格检查：发现 $($problems.Count) 处问题"
    foreach ($p in $problems) { Write-Host "  $p" }
    throw "scripts/ 下有 $($problems.Count) 处 -f 误用（详见上方清单）"
}
Write-Host "PowerShell 脚本风格检查：通过（$scanRoot）"
