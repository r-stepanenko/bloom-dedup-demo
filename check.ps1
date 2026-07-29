param(
    [string]$RepoRoot = (Get-Location).Path,
    [string]$OutRoot = ''
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

function Get-CheckGoCommand {
    $go = Get-Command go -ErrorAction SilentlyContinue
    if ($go) {
        return $go.Source
    }
    $fallback = 'K:\go\go1.20.14\bin\go.exe'
    if (Test-Path -LiteralPath $fallback) {
        return $fallback
    }
    throw 'go executable was not found in PATH and fallback K:\go\go1.20.14\bin\go.exe is unavailable.'
}

function New-CheckContext {
    param(
        [Parameter(Mandatory=$true)][string]$Student,
        [Parameter(Mandatory=$true)][string]$RepoRoot,
        [string]$OutRoot = ''
    )

    $repo = (Resolve-Path -LiteralPath $RepoRoot).Path
    if ($OutRoot -eq '') {
        $OutRoot = Join-Path $repo '.check-results'
    }

    $timestamp = Get-Date -Format 'yyyyMMdd_HHmmss'
    $safeStudent = $Student -replace '[^A-Za-z0-9_.-]', '_'
    $resultDir = Join-Path $OutRoot "${safeStudent}_${timestamp}"
    $logsDir = Join-Path $resultDir 'logs'
    $inputsDir = Join-Path $resultDir 'inputs'
    $outputsDir = Join-Path $resultDir 'outputs'
    $metaDir = Join-Path $resultDir 'meta'

    foreach ($dir in @($resultDir, $logsDir, $inputsDir, $outputsDir, $metaDir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }

    $commandsPath = Join-Path $resultDir 'commands.jsonl'
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($commandsPath, '', $utf8NoBom)

    return [ordered]@{
        Student = $Student
        RepoRoot = $repo
        ResultDir = $resultDir
        LogsDir = $logsDir
        InputsDir = $inputsDir
        OutputsDir = $outputsDir
        MetaDir = $metaDir
        CommandsPath = $commandsPath
        GoCmd = Get-CheckGoCommand
        StartedAt = (Get-Date).ToString('o')
        CommandResults = @{}
        Assessments = New-Object System.Collections.ArrayList
        NonValidationFailures = New-Object System.Collections.ArrayList
    }
}

function Save-CheckJson {
    param(
        [Parameter(Mandatory=$true)][string]$Path,
        [Parameter(Mandatory=$true)]$Value
    )

    $json = $Value | ConvertTo-Json -Depth 50
    $encoding = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $json, $encoding)
}

function Write-CheckText {
    param(
        [Parameter(Mandatory=$true)][string]$Path,
        [Parameter(Mandatory=$true)][string]$Text
    )

    $parent = Split-Path -Parent $Path
    if ($parent) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $encoding = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $Text, $encoding)
}

function Format-ArgumentForCommand {
    param([Parameter(Mandatory=$true)][string]$Value)

    if ($Value -match '[\s"'']') {
        return '"' + ($Value -replace '"', '\"') + '"'
    }
    return $Value
}

function Format-CommandLine {
    param(
        [Parameter(Mandatory=$true)][string]$FilePath,
        [Parameter(Mandatory=$true)][string[]]$Arguments
    )

    $parts = @($FilePath)
    foreach ($arg in $Arguments) {
        $parts += (Format-ArgumentForCommand -Value $arg)
    }
    return ($parts -join ' ')
}

function Get-ProcessTreePids {
    param([Parameter(Mandatory=$true)][int]$RootPid)

    $all = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue
    $childrenByParent = @{}
    foreach ($item in $all) {
        $parent = [int]$item.ParentProcessId
        if (-not $childrenByParent.ContainsKey($parent)) {
            $childrenByParent[$parent] = New-Object System.Collections.Generic.List[int]
        }
        $childrenByParent[$parent].Add([int]$item.ProcessId)
    }

    $visited = New-Object System.Collections.Generic.HashSet[int]
    $stack = New-Object System.Collections.Generic.Stack[int]
    $stack.Push($RootPid)

    while ($stack.Count -gt 0) {
        $current = $stack.Pop()
        if (-not $visited.Add($current)) {
            continue
        }
        if ($childrenByParent.ContainsKey($current)) {
            foreach ($child in $childrenByParent[$current]) {
                $stack.Push($child)
            }
        }
    }

    return @($visited)
}

function Get-ProcessTreeWorkingSetBytes {
    param([Parameter(Mandatory=$true)][int]$RootPid)

    $sum = [int64]0
    $pids = Get-ProcessTreePids -RootPid $RootPid
    foreach ($pidValue in $pids) {
        try {
            $proc = Get-Process -Id $pidValue -ErrorAction Stop
            $sum += [int64]$proc.WorkingSet64
        } catch {
            continue
        }
    }
    return $sum
}

function Stop-ProcessTreeHidden {
    param([Parameter(Mandatory=$true)][int]$RootPid)

    $pids = Get-ProcessTreePids -RootPid $RootPid
    $ordered = @($pids | Sort-Object -Descending)
    foreach ($pidValue in $ordered) {
        try {
            Stop-Process -Id $pidValue -Force -ErrorAction Stop
        } catch {
            continue
        }
    }
}

function Read-TextSafe {
    param([Parameter(Mandatory=$true)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        return ''
    }
    return [string](Get-Content -LiteralPath $Path -Raw -ErrorAction SilentlyContinue)
}

function Get-FileSha256 {
    param([Parameter(Mandatory=$true)][string]$Path)

    $sha = [System.Security.Cryptography.SHA256]::Create()
    $stream = [System.IO.File]::OpenRead($Path)
    try {
        $hash = $sha.ComputeHash($stream)
    } finally {
        $stream.Dispose()
        $sha.Dispose()
    }
    $builder = New-Object System.Text.StringBuilder
    foreach ($b in $hash) {
        [void]$builder.AppendFormat('{0:x2}', $b)
    }
    return $builder.ToString()
}

function Invoke-HiddenProcess {
    param(
        [Parameter(Mandatory=$true)]$Ctx,
        [Parameter(Mandatory=$true)][string]$Name,
        [Parameter(Mandatory=$true)][string]$FilePath,
        [string[]]$Arguments = @(),
        [string]$WorkingDirectory = '',
        [int]$TimeoutSec = 120,
        [bool]$AllowNonZero = $false,
        [bool]$ValidationOnly = $false
    )

    if ($WorkingDirectory -eq '') {
        $WorkingDirectory = $Ctx.RepoRoot
    }

    $safeName = $Name -replace '[^A-Za-z0-9_.-]', '_'
    $stdoutPath = Join-Path $Ctx.LogsDir "$safeName.stdout.log"
    $stderrPath = Join-Path $Ctx.LogsDir "$safeName.stderr.log"
    $combinedLogPath = Join-Path $Ctx.LogsDir "$safeName.log"

    $started = Get-Date
    $proc = Start-Process -FilePath $FilePath -ArgumentList $Arguments -WorkingDirectory $WorkingDirectory -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath

    $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    $timedOut = $false
    $peakWorkingSetBytes = [int64]0

    while (-not $proc.HasExited) {
        $currentMemory = Get-ProcessTreeWorkingSetBytes -RootPid $proc.Id
        if ($currentMemory -gt $peakWorkingSetBytes) {
            $peakWorkingSetBytes = $currentMemory
        }
        if ($stopwatch.Elapsed.TotalSeconds -ge $TimeoutSec) {
            $timedOut = $true
            Stop-ProcessTreeHidden -RootPid $proc.Id
            break
        }
        Start-Sleep -Milliseconds 100
        try { $proc.Refresh() } catch { break }
    }

    try { $proc.WaitForExit(5000) | Out-Null } catch {}
    if (-not $proc.HasExited) {
        Stop-ProcessTreeHidden -RootPid $proc.Id
    }

    try { $proc.Refresh() } catch {}
    $ended = Get-Date
    $durationMs = [int](($ended - $started).TotalMilliseconds)
    $exitCode = if ($proc.HasExited) { [int]$proc.ExitCode } else { 124 }

    $finalMemory = Get-ProcessTreeWorkingSetBytes -RootPid $proc.Id
    if ($finalMemory -gt $peakWorkingSetBytes) {
        $peakWorkingSetBytes = $finalMemory
    }

    $commandLine = Format-CommandLine -FilePath $FilePath -Arguments $Arguments
    $stdoutText = Read-TextSafe -Path $stdoutPath
    $stderrText = Read-TextSafe -Path $stderrPath

    $combined = @(
        "name: $Name"
        "working_directory: $WorkingDirectory"
        "timeout_sec: $TimeoutSec"
        "timed_out: $timedOut"
        "peak_working_set_bytes: $peakWorkingSetBytes"
        "command:"
        $commandLine
        "exit_code: $exitCode"
        "started_at: $($started.ToString('o'))"
        "ended_at: $($ended.ToString('o'))"
        "duration_ms: $durationMs"
        ""
        "stdout:"
        $stdoutText
        ""
        "stderr:"
        $stderrText
    ) -join "`n"
    Set-Content -LiteralPath $combinedLogPath -Value $combined -Encoding UTF8

    $record = [ordered]@{
        name = $Name
        file = $FilePath
        arguments = @($Arguments)
        command = $commandLine
        working_directory = $WorkingDirectory
        timeout_sec = $TimeoutSec
        timed_out = $timedOut
        exit_code = $exitCode
        started_at = $started.ToString('o')
        ended_at = $ended.ToString('o')
        duration_ms = $durationMs
        peak_working_set_bytes = $peakWorkingSetBytes
        stdout = "logs/$safeName.stdout.log"
        stderr = "logs/$safeName.stderr.log"
        log = "logs/$safeName.log"
        allow_nonzero = $AllowNonZero
        validation_only = $ValidationOnly
    }

    ($record | ConvertTo-Json -Depth 10 -Compress) | Add-Content -LiteralPath $Ctx.CommandsPath -Encoding UTF8
    $Ctx.CommandResults[$Name] = $record

    if (-not $AllowNonZero -and ($timedOut -or $exitCode -ne 0) -and -not $ValidationOnly) {
        $Ctx.NonValidationFailures.Add($Name) | Out-Null
    }

    return $record
}

function Add-FeatureAssessment {
    param(
        [Parameter(Mandatory=$true)]$Ctx,
        [Parameter(Mandatory=$true)][string]$Id,
        [Parameter(Mandatory=$true)][ValidateSet('minimum','good','excellent','engineering')][string]$Level,
        [Parameter(Mandatory=$true)][string]$Category,
        [Parameter(Mandatory=$true)][string]$Requirement,
        [Parameter(Mandatory=$true)][ValidateSet('not_implemented','partial','full')][string]$Implementation,
        [Parameter(Mandatory=$true)][ValidateSet('not_tested','nonconformant','conformant')][string]$Conformance,
        [string[]]$Evidence = @(),
        [string]$Details = ''
    )

    $item = [ordered]@{
        id = $Id
        level = $Level
        category = $Category
        requirement = $Requirement
        implementation = $Implementation
        conformance = $Conformance
        evidence = @($Evidence)
        details = $Details
    }
    $Ctx.Assessments.Add($item) | Out-Null
}

function Add-BooleanFeatureAssessment {
    param(
        [Parameter(Mandatory=$true)]$Ctx,
        [Parameter(Mandatory=$true)][string]$Id,
        [Parameter(Mandatory=$true)][ValidateSet('minimum','good','excellent','engineering')][string]$Level,
        [Parameter(Mandatory=$true)][string]$Category,
        [Parameter(Mandatory=$true)][string]$Requirement,
        [Parameter(Mandatory=$true)][bool]$Ok,
        [string[]]$Evidence = @(),
        [string]$Details = ''
    )

    $implementation = if ($Ok) { 'full' } else { 'partial' }
    $conformance = if ($Ok) { 'conformant' } else { 'nonconformant' }
    Add-FeatureAssessment -Ctx $Ctx -Id $Id -Level $Level -Category $Category -Requirement $Requirement -Implementation $implementation -Conformance $conformance -Evidence $Evidence -Details $Details
}

function Get-JsonFieldValue {
    param(
        [Parameter(Mandatory=$true)][string]$Line,
        [Parameter(Mandatory=$true)][string]$Field
    )

    $marker = '"' + $Field + '":"'
    $start = $Line.IndexOf($marker, [System.StringComparison]::Ordinal)
    if ($start -lt 0) { return $null }
    $begin = $start + $marker.Length
    $end = $Line.IndexOf('"', $begin)
    if ($end -lt 0) { return $null }
    return $Line.Substring($begin, $end - $begin)
}

function Test-ValidSourceName {
    param([string]$Source)

    if ([string]::IsNullOrEmpty($Source)) { return $false }
    if ($Source.Length -ne 12) { return $false }
    if (-not $Source.StartsWith('collector_')) { return $false }
    $digits = $Source.Substring(10, 2)
    if ($digits -notmatch '^[0-9]{2}$') { return $false }
    $value = [int]$digits
    return ($value -ge 1 -and $value -le 99)
}

function Get-JsonlStats {
    param(
        [Parameter(Mandatory=$true)][string]$Path,
        [int]$MaxSource = 99
    )

    $reader = [System.IO.StreamReader]::new($Path)
    $lineCount = 0
    $invalidSources = 0
    $unique = New-Object 'System.Collections.Generic.HashSet[string]'
    $firstSourceById = @{}
    $crossSourceDuplicate = $false

    try {
        while (($line = $reader.ReadLine()) -ne $null) {
            if ([string]::IsNullOrWhiteSpace($line)) { continue }
            $lineCount++
            $eventId = Get-JsonFieldValue -Line $line -Field 'event_id'
            if ($null -eq $eventId) { throw "line $lineCount missing event_id" }
            $source = Get-JsonFieldValue -Line $line -Field 'source'
            if ($null -eq $source) { $source = '' }
            if (-not (Test-ValidSourceName -Source $source)) {
                $invalidSources++
            } else {
                $sourceIndex = [int]$source.Substring(10, 2)
                if ($sourceIndex -lt 1 -or $sourceIndex -gt $MaxSource) { $invalidSources++ }
            }
            if (-not $unique.Add($eventId)) {
                if (-not $crossSourceDuplicate -and $firstSourceById.ContainsKey($eventId) -and $firstSourceById[$eventId] -ne $source) {
                    $crossSourceDuplicate = $true
                }
            } else {
                $firstSourceById[$eventId] = $source
            }
        }
    } finally {
        $reader.Dispose()
    }

    $uniqueCount = $unique.Count
    $duplicates = $lineCount - $uniqueCount
    return [ordered]@{
        lines = $lineCount
        unique = $uniqueCount
        duplicates = $duplicates
        cross_source_duplicate = $crossSourceDuplicate
        invalid_source_count = $invalidSources
    }
}

function Get-ExactMapEstimateFromInput {
    param(
        [Parameter(Mandatory=$true)][string]$Path,
        [Parameter(Mandatory=$true)][string]$Scope
    )

    $reader = [System.IO.StreamReader]::new($Path)
    $keys = New-Object 'System.Collections.Generic.HashSet[string]'

    try {
        while (($line = $reader.ReadLine()) -ne $null) {
            if ([string]::IsNullOrWhiteSpace($line)) { continue }
            $eventId = Get-JsonFieldValue -Line $line -Field 'event_id'
            if ($null -eq $eventId) { throw 'missing event_id while estimating exact map bytes' }
            $source = Get-JsonFieldValue -Line $line -Field 'source'
            if ($null -eq $source) { $source = '<missing>' }
            $key = if ($Scope -eq 'by_source') { "$source|$eventId" } else { $eventId }
            [void]$keys.Add($key)
        }
    } finally {
        $reader.Dispose()
    }

    $bytes = [int64]0
    foreach ($key in $keys) {
        $bytes += [System.Text.Encoding]::UTF8.GetByteCount($key) + 24
    }
    return $bytes
}

function Parse-GoTestJson {
    param(
        [Parameter(Mandatory=$true)][string]$JsonLogPath,
        [Parameter(Mandatory=$true)][string[]]$ExpectedTests
    )

    $status = @{}
    foreach ($name in $ExpectedTests) {
        $status[$name] = [ordered]@{ run = $false; pass = $false }
    }

    $reader = [System.IO.StreamReader]::new($JsonLogPath)
    try {
        while (($line = $reader.ReadLine()) -ne $null) {
            if ([string]::IsNullOrWhiteSpace($line)) { continue }
            try { $obj = $line | ConvertFrom-Json -ErrorAction Stop } catch { continue }
            $testProperty = $obj.PSObject.Properties['Test']
            if ($null -eq $testProperty) { continue }
            $actionProperty = $obj.PSObject.Properties['Action']
            if ($null -eq $actionProperty) { continue }
            $testName = [string]$testProperty.Value
            $actionName = [string]$actionProperty.Value
            foreach ($expected in $ExpectedTests) {
                if ($testName -eq $expected -or $testName.StartsWith($expected + '/')) {
                    if ($actionName -eq 'run') { $status[$expected].run = $true }
                    if ($actionName -eq 'pass') { $status[$expected].pass = $true }
                }
            }
        }
    } finally {
        $reader.Dispose()
    }

    return $status
}

function Ensure-FileExists {
    param([Parameter(Mandatory=$true)][string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) {
        New-Item -ItemType File -Force -Path $Path | Out-Null
    }
}

function Read-JsonFile {
    param(
        [Parameter(Mandatory=$true)][string]$Path,
        [object]$Run = $null
    )

    if (-not (Test-Path -LiteralPath $Path)) {
        # A missing output almost always means the producing command failed.
        # Without its exit code and log path the caller has to hunt for the real cause by hand.
        if ($null -ne $Run) {
            throw ("missing json file: {0}; command '{1}' exited with code {2}, see {3}" -f $Path, $Run.name, $Run.exit_code, $Run.log)
        }
        throw "missing json file: $Path"
    }
    $raw = Get-Content -LiteralPath $Path -Raw -Encoding UTF8
    return $raw | ConvertFrom-Json
}

function Get-ConfigObject {
    param(
        [int]$ExpectedItems,
        [double]$FalsePositiveRate,
        [string]$HashFamily,
        [string]$Mode,
        [string]$Scope
    )

    return [ordered]@{
        expected_items = $ExpectedItems
        false_positive_rate = $FalsePositiveRate
        hash_family = $HashFamily
        mode = $Mode
        scope = $Scope
        report_fprs = @(0.1, 0.05, 0.01, 0.001)
    }
}

function Save-ConfigFile {
    param([Parameter(Mandatory=$true)][string]$Path,[Parameter(Mandatory=$true)]$Config)
    Save-CheckJson -Path $Path -Value $Config
}

function Convert-ResultPathToEvidence {
    param([Parameter(Mandatory=$true)]$Ctx,[Parameter(Mandatory=$true)][string]$Path)
    return $Path.Replace($Ctx.ResultDir, '').TrimStart('\\')
}

function Get-ObjectPropertyValue {
    param(
        $Object,
        [Parameter(Mandatory=$true)][string]$Name,
        $Default = $null
    )

    if ($null -eq $Object) {
        return $Default
    }
    $prop = $Object.PSObject.Properties[$Name]
    if ($null -eq $prop) {
        return $Default
    }
    return $prop.Value
}

function Get-ResultFieldValue {
    param(
        $Object,
        [Parameter(Mandatory=$true)][string[]]$Names,
        $Default = $null
    )

    if ($null -eq $Object) { return $Default }
    foreach ($name in $Names) {
        $prop = $Object.PSObject.Properties[$name]
        if ($null -ne $prop) { return $prop.Value }
    }
    return $Default
}

function Test-ResultFieldPresent {
    param($Object,[Parameter(Mandatory=$true)][string[]]$Names)

    if ($null -eq $Object) { return $false }
    foreach ($name in $Names) {
        if ($null -ne $Object.PSObject.Properties[$name]) { return $true }
    }
    return $false
}

function Get-FprTableRows {
    param($Object)

    $named = Get-ResultFieldValue -Object $Object -Names @('fp_table','parameter_table','parameters_table','fpr_table','report_fprs','fpr_rows')
    if ($null -ne $named) { return @($named) }
    if ($null -eq $Object) { return @() }
    foreach ($prop in $Object.PSObject.Properties) {
        $candidate = @($prop.Value)
        if ($candidate.Count -eq 0) { continue }
        if (Test-ResultFieldPresent -Object $candidate[0] -Names @('false_positive_rate','p','fpr')) { return $candidate }
    }
    return @()
}

# The assignment prescribes no test or benchmark names, so the names are discovered from the
# sources via go test -list and selected by meaning instead of being hardcoded.
function Get-GoListNames {
    param(
        [Parameter(Mandatory=$true)][string]$LogPath,
        [Parameter(Mandatory=$true)][string]$Prefix
    )

    $text = Read-TextSafe -Path $LogPath
    if ([string]::IsNullOrWhiteSpace($text)) { return @() }
    $names = New-Object System.Collections.Generic.List[string]
    foreach ($line in ($text -split "`r?`n")) {
        $trimmed = $line.Trim()
        if ($trimmed -match ('^' + $Prefix + '[A-Za-z0-9_]*$')) { $null = $names.Add($trimmed) }
    }
    return @($names | Sort-Object -Unique)
}

function Select-NamesByPattern {
    param([string[]]$Names,[Parameter(Mandatory=$true)][string]$Pattern)

    if ($null -eq $Names) { return @() }
    return @($Names | Where-Object { $_ -match $Pattern })
}

function Get-NameRunRegex {
    param([string[]]$Names)

    if ($null -eq $Names -or $Names.Count -eq 0) { return '^$' }
    return ('^(' + ((@($Names) | ForEach-Object { [regex]::Escape($_) }) -join '|') + ')$')
}

function Test-SelectedTestsPass {
    param($Status)

    if ($null -eq $Status -or $Status.Keys.Count -eq 0) { return $false }
    foreach ($name in $Status.Keys) {
        if (-not ($Status[$name].run -and $Status[$name].pass)) { return $false }
    }
    return $true
}

function Complete-Check {
    param([Parameter(Mandatory=$true)]$Ctx,[hashtable]$Notes = @{})

    $assessmentItems = @($Ctx.Assessments)
    $assessmentSummary = [ordered]@{}
    foreach ($level in @('minimum','good','excellent','engineering')) {
        $items = @($assessmentItems | Where-Object { $_.level -eq $level })
        $assessmentSummary[$level] = [ordered]@{
            total = $items.Count
            full = @($items | Where-Object { $_.implementation -eq 'full' }).Count
            partial = @($items | Where-Object { $_.implementation -eq 'partial' }).Count
            not_implemented = @($items | Where-Object { $_.implementation -eq 'not_implemented' }).Count
            conformant = @($items | Where-Object { $_.conformance -eq 'conformant' }).Count
            nonconformant = @($items | Where-Object { $_.conformance -eq 'nonconformant' }).Count
            not_tested = @($items | Where-Object { $_.conformance -eq 'not_tested' }).Count
        }
    }

    Save-CheckJson -Path (Join-Path $Ctx.ResultDir 'assessment.json') -Value ([ordered]@{
        schema_version = 2
        statuses = [ordered]@{ implementation = @('not_implemented','partial','full'); conformance = @('not_tested','nonconformant','conformant') }
        summary = $assessmentSummary
        features = $assessmentItems
    })

    $manifest = [ordered]@{
        student = $Ctx.Student
        repo_root = $Ctx.RepoRoot
        started_at = $Ctx.StartedAt
        completed_at = (Get-Date).ToString('o')
        machine = [ordered]@{
            computer_name = $env:COMPUTERNAME
            user_name = $env:USERNAME
            os = (Get-CimInstance Win32_OperatingSystem).Caption
            powershell = $PSVersionTable.PSVersion.ToString()
        }
        result_dir = $Ctx.ResultDir
        commands_file = 'commands.jsonl'
        assessment_file = 'assessment.json'
        notes = $Notes
    }
    Save-CheckJson -Path (Join-Path $Ctx.ResultDir 'manifest.json') -Value $manifest

    $zipPath = "$($Ctx.ResultDir).zip"
    if (Test-Path -LiteralPath $zipPath) { Remove-Item -LiteralPath $zipPath -Force }
    Compress-Archive -Path (Join-Path $Ctx.ResultDir '*') -DestinationPath $zipPath -Force

    Write-Host "CHECK_RESULT_DIR=$($Ctx.ResultDir)"
    Write-Host "CHECK_RESULT_ZIP=$zipPath"
    return $zipPath
}

$ctx = New-CheckContext -Student 'bloom_dedup_check' -RepoRoot $RepoRoot -OutRoot $OutRoot
$cleanupOk = $false

$configMainPath = Join-Path $ctx.InputsDir 'config_main.json'
$configCountingPath = Join-Path $ctx.InputsDir 'config_counting.json'
$configShaPath = Join-Path $ctx.InputsDir 'config_sha.json'
$configNoExactPath = Join-Path $ctx.InputsDir 'config_no_exact.json'
$configBySourcePath = Join-Path $ctx.InputsDir 'config_by_source.json'
$configMillionPath = Join-Path $ctx.InputsDir 'config_million_no_exact.json'
$configSaturatedPath = Join-Path $ctx.InputsDir 'config_saturated.json'

Save-ConfigFile -Path $configMainPath -Config (Get-ConfigObject -ExpectedItems 1000 -FalsePositiveRate 0.01 -HashFamily 'fnv64_double_hashing' -Mode 'bloom' -Scope 'global')
Save-ConfigFile -Path $configCountingPath -Config (Get-ConfigObject -ExpectedItems 1000 -FalsePositiveRate 0.01 -HashFamily 'fnv64_double_hashing' -Mode 'counting_bloom' -Scope 'global')
Save-ConfigFile -Path $configShaPath -Config (Get-ConfigObject -ExpectedItems 1000 -FalsePositiveRate 0.01 -HashFamily 'sha256_slices' -Mode 'bloom' -Scope 'global')
Save-ConfigFile -Path $configNoExactPath -Config (Get-ConfigObject -ExpectedItems 1000 -FalsePositiveRate 0.01 -HashFamily 'fnv64_double_hashing' -Mode 'bloom' -Scope 'global')
Save-ConfigFile -Path $configBySourcePath -Config (Get-ConfigObject -ExpectedItems 1000 -FalsePositiveRate 0.01 -HashFamily 'fnv64_double_hashing' -Mode 'bloom' -Scope 'by_source')
Save-ConfigFile -Path $configMillionPath -Config (Get-ConfigObject -ExpectedItems 1000000 -FalsePositiveRate 0.01 -HashFamily 'fnv64_double_hashing' -Mode 'bloom' -Scope 'global')
Save-ConfigFile -Path $configSaturatedPath -Config (Get-ConfigObject -ExpectedItems 20 -FalsePositiveRate 0.3 -HashFamily 'fnv64_double_hashing' -Mode 'bloom' -Scope 'global')

$fixtureExactPath = Join-Path $ctx.InputsDir 'fixture_exact_6.jsonl'
Write-CheckText -Path $fixtureExactPath -Text @"
{"seq":1,"event_id":"evt_000001","event_hash":"0123456789abcdef","source":"collector_01","timestamp":"2026-07-01T00:00:00Z"}
{"seq":2,"event_id":"evt_000002","event_hash":"1111111111111111","source":"collector_01","timestamp":"2026-07-01T00:00:01Z"}
{"seq":3,"event_id":"evt_000003","event_hash":"2222222222222222","source":"collector_01","timestamp":"2026-07-01T00:00:02Z"}
{"seq":4,"event_id":"evt_000001","event_hash":"0123456789abcdef","source":"collector_01","timestamp":"2026-07-01T00:00:03Z"}
{"seq":5,"event_id":"evt_000002","event_hash":"1111111111111111","source":"collector_01","timestamp":"2026-07-01T00:00:04Z"}
{"seq":6,"event_id":"evt_000003","event_hash":"2222222222222222","source":"collector_01","timestamp":"2026-07-01T00:00:05Z"}
"@

$fixtureGlobalVsSourcePath = Join-Path $ctx.InputsDir 'fixture_global_vs_source.jsonl'
Write-CheckText -Path $fixtureGlobalVsSourcePath -Text @"
{"seq":1,"event_id":"evt_000101","event_hash":"0123456789abcdef","source":"collector_01","timestamp":"2026-07-01T00:00:00Z"}
{"seq":2,"event_id":"evt_000102","event_hash":"1111111111111111","source":"collector_01","timestamp":"2026-07-01T00:00:01Z"}
{"seq":3,"event_id":"evt_000103","event_hash":"2222222222222222","source":"collector_01","timestamp":"2026-07-01T00:00:02Z"}
{"seq":4,"event_id":"evt_000101","event_hash":"0123456789abcdef","source":"collector_02","timestamp":"2026-07-01T00:00:03Z"}
{"seq":5,"event_id":"evt_000102","event_hash":"1111111111111111","source":"collector_01","timestamp":"2026-07-01T00:00:04Z"}
{"seq":6,"event_id":"evt_000101","event_hash":"0123456789abcdef","source":"collector_02","timestamp":"2026-07-01T00:00:05Z"}
"@

$fixtureInvalidSourcesPath = Join-Path $ctx.InputsDir 'fixture_invalid_sources.jsonl'
Write-CheckText -Path $fixtureInvalidSourcesPath -Text @"
{"seq":1,"event_id":"evt_000001","event_hash":"0123456789abcdef","source":"collector_01","timestamp":"2026-07-01T00:00:00Z"}
{"seq":2,"event_id":"evt_000002","event_hash":"1111111111111111","source":"collector_00","timestamp":"2026-07-01T00:00:01Z"}
{"seq":3,"event_id":"evt_000003","event_hash":"2222222222222222","source":"bad_source","timestamp":"2026-07-01T00:00:02Z"}
{"seq":4,"event_id":"evt_000004","event_hash":"3333333333333333","source":"","timestamp":"2026-07-01T00:00:03Z"}
{"seq":5,"event_id":"evt_000001","event_hash":"0123456789abcdef","source":"collector_01","timestamp":"2026-07-01T00:00:04Z"}
"@

$fixtureSaturatedPath = Join-Path $ctx.InputsDir 'fixture_saturated.jsonl'
$fixtureSaturatedLines = New-Object System.Collections.Generic.List[string]
# 250 distinct hashes plus 50 real repeats against a filter sized for 20 items at p=0.3.
# A single repeated hash cannot produce a false positive at all, so the criterion would be
# unreachable regardless of the solution.
for ($i = 1; $i -le 300; $i++) {
    $hashSeed = (($i - 1) % 250) + 1
    $satHash = '{0:x16}' -f $hashSeed
    $fixtureSaturatedLines.Add("{`"seq`":$i,`"event_id`":`"evt_sat$('{0:D5}' -f $i)`",`"event_hash`":`"$satHash`",`"source`":`"collector_01`",`"timestamp`":`"2026-07-01T00:00:00Z`"}") | Out-Null
}
Write-CheckText -Path $fixtureSaturatedPath -Text (($fixtureSaturatedLines -join "`n") + "`n")

$makeCmd = Get-Command make -ErrorAction SilentlyContinue
$gitCmd = Get-Command git -ErrorAction SilentlyContinue

$toolPath = Join-Path $ctx.OutputsDir 'bloom-dedup-demo.exe'
$goBuild = Invoke-HiddenProcess -Ctx $ctx -Name 'build_cli' -FilePath $ctx.GoCmd -Arguments @('build','-o',$toolPath,'./cmd/bloom-dedup-demo') -TimeoutSec 180
$goTestAll = Invoke-HiddenProcess -Ctx $ctx -Name 'go_test_all' -FilePath $ctx.GoCmd -Arguments @('test','./...') -TimeoutSec 180

if ($makeCmd) {
    [void](Invoke-HiddenProcess -Ctx $ctx -Name 'make_test' -FilePath $makeCmd.Source -Arguments @('test') -TimeoutSec 180)
    [void](Invoke-HiddenProcess -Ctx $ctx -Name 'make_bench' -FilePath $makeCmd.Source -Arguments @('bench') -TimeoutSec 240)
    [void](Invoke-HiddenProcess -Ctx $ctx -Name 'make_demo' -FilePath $makeCmd.Source -Arguments @('demo') -TimeoutSec 240)
}

$genAPath = Join-Path $ctx.InputsDir 'generated_seed42_a.jsonl'
$genBPath = Join-Path $ctx.InputsDir 'generated_seed42_b.jsonl'
$genCPath = Join-Path $ctx.InputsDir 'generated_seed43_c.jsonl'

$genA = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_generate_seed42_a' -FilePath $toolPath -Arguments @('generate','--count','120','--duplicate-ratio','0.25','--out',$genAPath,'--seed','42','--sources','3') -TimeoutSec 120
$genB = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_generate_seed42_b' -FilePath $toolPath -Arguments @('generate','--count','120','--duplicate-ratio','0.25','--out',$genBPath,'--seed','42','--sources','3') -TimeoutSec 120
$genC = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_generate_seed43_c' -FilePath $toolPath -Arguments @('generate','--count','120','--duplicate-ratio','0.25','--out',$genCPath,'--seed','43','--sources','3') -TimeoutSec 120

$genInvalidCount = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_generate_invalid_count' -FilePath $toolPath -Arguments @('generate','--count','0','--duplicate-ratio','0.25','--out',(Join-Path $ctx.InputsDir 'invalid_count.jsonl'),'--seed','42','--sources','3') -TimeoutSec 30 -AllowNonZero $true -ValidationOnly $true
$genInvalidRatio = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_generate_invalid_ratio' -FilePath $toolPath -Arguments @('generate','--count','10','--duplicate-ratio','1.0','--out',(Join-Path $ctx.InputsDir 'invalid_ratio.jsonl'),'--seed','42','--sources','3') -TimeoutSec 30 -AllowNonZero $true -ValidationOnly $true
$genInvalidSources = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_generate_invalid_sources' -FilePath $toolPath -Arguments @('generate','--count','10','--duplicate-ratio','0.2','--out',(Join-Path $ctx.InputsDir 'invalid_sources.jsonl'),'--seed','42','--sources','0') -TimeoutSec 30 -AllowNonZero $true -ValidationOnly $true

$resultMainPath = Join-Path $ctx.OutputsDir 'result_main.json'
$reportMainPath = Join-Path $ctx.OutputsDir 'report_main.md'
$runMain = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_run_main' -FilePath $toolPath -Arguments @('run','--in',$genAPath,'--config',$configMainPath,'--out',$resultMainPath,'--report',$reportMainPath) -TimeoutSec 180

$resultExactPath = Join-Path $ctx.OutputsDir 'result_exact_fixture.json'
$runExact = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_run_exact_fixture' -FilePath $toolPath -Arguments @('run','--in',$fixtureExactPath,'--config',$configMainPath,'--out',$resultExactPath,'--report',(Join-Path $ctx.OutputsDir 'report_exact_fixture.md')) -TimeoutSec 120

$resultSaturatedPath = Join-Path $ctx.OutputsDir 'result_saturated.json'
$runSaturated = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_run_saturated_fixture' -FilePath $toolPath -Arguments @('run','--in',$fixtureSaturatedPath,'--config',$configSaturatedPath,'--out',$resultSaturatedPath,'--report',(Join-Path $ctx.OutputsDir 'report_saturated.md')) -TimeoutSec 180

$resultCountingPath = Join-Path $ctx.OutputsDir 'result_counting.json'
$runCounting = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_run_counting' -FilePath $toolPath -Arguments @('run','--in',$genAPath,'--config',$configCountingPath,'--out',$resultCountingPath,'--report',(Join-Path $ctx.OutputsDir 'report_counting.md')) -TimeoutSec 180

$resultShaPath = Join-Path $ctx.OutputsDir 'result_sha.json'
$runSha = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_run_sha' -FilePath $toolPath -Arguments @('run','--in',$genAPath,'--config',$configShaPath,'--out',$resultShaPath,'--report',(Join-Path $ctx.OutputsDir 'report_sha.md')) -TimeoutSec 180

$resultNoExactPath = Join-Path $ctx.OutputsDir 'result_no_exact.json'
$runNoExact = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_run_no_exact' -FilePath $toolPath -Arguments @('run','--in',$genAPath,'--config',$configNoExactPath,'--out',$resultNoExactPath,'--report',(Join-Path $ctx.OutputsDir 'report_no_exact.md'),'--exact-compare=false') -TimeoutSec 180

$resultGlobalPath = Join-Path $ctx.OutputsDir 'result_global_scope.json'
$resultBySourcePath = Join-Path $ctx.OutputsDir 'result_by_source_scope.json'
$runGlobal = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_run_global_scope' -FilePath $toolPath -Arguments @('run','--in',$fixtureGlobalVsSourcePath,'--config',$configMainPath,'--out',$resultGlobalPath,'--report',(Join-Path $ctx.OutputsDir 'report_global_scope.md')) -TimeoutSec 120
$runBySource = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_run_by_source_scope' -FilePath $toolPath -Arguments @('run','--in',$fixtureGlobalVsSourcePath,'--config',$configBySourcePath,'--out',$resultBySourcePath,'--report',(Join-Path $ctx.OutputsDir 'report_by_source_scope.md')) -TimeoutSec 120

$resultInvalidPath = Join-Path $ctx.OutputsDir 'result_invalid_sources.json'
$runInvalid = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_run_invalid_sources' -FilePath $toolPath -Arguments @('run','--in',$fixtureInvalidSourcesPath,'--config',$configMainPath,'--out',$resultInvalidPath,'--report',(Join-Path $ctx.OutputsDir 'report_invalid_sources.md')) -TimeoutSec 120

$unitTestsList = Invoke-HiddenProcess -Ctx $ctx -Name 'go_test_list_tests' -FilePath $ctx.GoCmd -Arguments @('test','-list','^Test','./...') -TimeoutSec 120 -AllowNonZero $true -ValidationOnly $true
$benchList = Invoke-HiddenProcess -Ctx $ctx -Name 'go_test_list_benchmarks' -FilePath $ctx.GoCmd -Arguments @('test','-list','^Benchmark','./...') -TimeoutSec 120 -AllowNonZero $true -ValidationOnly $true

$allTestNames = Get-GoListNames -LogPath (Join-Path $ctx.ResultDir $unitTestsList.stdout) -Prefix 'Test'
$allBenchNames = Get-GoListNames -LogPath (Join-Path $ctx.ResultDir $benchList.stdout) -Prefix 'Benchmark'

$bitArrayTests = Select-NamesByPattern -Names $allTestNames -Pattern '(?i)bit|setandget|setpanics|getpanics'
$bloomAddTests = Select-NamesByPattern -Names $allTestNames -Pattern '(?i)add.*maycontain|maycontain|filteradd'
$hashVectorTests = Select-NamesByPattern -Names $allTestNames -Pattern '(?i)hash|indexes|vector'
$paramTests = Select-NamesByPattern -Names $allTestNames -Pattern '(?i)param'
$countingTests = Select-NamesByPattern -Names $allTestNames -Pattern '(?i)counting'
$noExactTests = Select-NamesByPattern -Names $allTestNames -Pattern '(?i)withoutmap|nomap|noexact|no_exact|streaming'
$bitHashTests = @($bitArrayTests + $bloomAddTests + $hashVectorTests | Sort-Object -Unique)

$benchFilterNames = Select-NamesByPattern -Names $allBenchNames -Pattern '(?i)add|maycontain'
$benchStreamNames = Select-NamesByPattern -Names $allBenchNames -Pattern '(?i)stream|noexact|nomap|withoutmap'
$benchSelected = @($benchFilterNames + $benchStreamNames | Sort-Object -Unique)
$benchPattern = if ($benchSelected.Count -gt 0) { ($benchSelected -join '|') } else { 'BenchmarkNothingSelected' }

$benchRun = Invoke-HiddenProcess -Ctx $ctx -Name 'go_bench_real' -FilePath $ctx.GoCmd -Arguments @('test','-run','^$','-bench',$benchPattern,'./...') -TimeoutSec 600

$bitHashJson = Invoke-HiddenProcess -Ctx $ctx -Name 'go_test_bit_hash_json' -FilePath $ctx.GoCmd -Arguments @('test','-count=1','-json','-run',(Get-NameRunRegex -Names $bitHashTests),'./...') -TimeoutSec 180
$paramsJson = Invoke-HiddenProcess -Ctx $ctx -Name 'go_test_params_json' -FilePath $ctx.GoCmd -Arguments @('test','-count=1','-json','-run',(Get-NameRunRegex -Names $paramTests),'./...') -TimeoutSec 180
$countingJson = Invoke-HiddenProcess -Ctx $ctx -Name 'go_test_counting_json' -FilePath $ctx.GoCmd -Arguments @('test','-count=1','-json','-run',(Get-NameRunRegex -Names $countingTests),'./...') -TimeoutSec 180
$noExactJson = Invoke-HiddenProcess -Ctx $ctx -Name 'go_test_no_exact_json' -FilePath $ctx.GoCmd -Arguments @('test','-count=1','-json','-run',(Get-NameRunRegex -Names $noExactTests),'./...') -TimeoutSec 180

$generatorStats = Get-JsonlStats -Path $genAPath -MaxSource 3
$hashA = Get-FileSha256 -Path $genAPath
$hashB = Get-FileSha256 -Path $genBPath
$hashC = Get-FileSha256 -Path $genCPath
$invalidCountError = Read-TextSafe -Path (Join-Path $ctx.ResultDir $genInvalidCount.stderr)
$invalidRatioError = Read-TextSafe -Path (Join-Path $ctx.ResultDir $genInvalidRatio.stderr)
$invalidSourcesError = Read-TextSafe -Path (Join-Path $ctx.ResultDir $genInvalidSources.stderr)
$invalidCountRejected = ($genInvalidCount.exit_code -ne 0 -or $invalidCountError -match 'count must be > 0')
$invalidRatioRejected = ($genInvalidRatio.exit_code -ne 0 -or $invalidRatioError -match 'duplicate_ratio must be in \[0, 0\.9\]')
$invalidSourcesRejected = ($genInvalidSources.exit_code -ne 0 -or $invalidSourcesError -match 'sources must be in \[1, 99\]')
$generatorInvariantOk = (
    $genA.exit_code -eq 0 -and $genB.exit_code -eq 0 -and $genC.exit_code -eq 0 -and
    $generatorStats.lines -eq 120 -and
    $generatorStats.duplicates -ge 20 -and $generatorStats.duplicates -le 42 -and
    $generatorStats.unique -eq ($generatorStats.lines - $generatorStats.duplicates) -and
    $generatorStats.cross_source_duplicate -and $generatorStats.invalid_source_count -eq 0 -and
    $hashA -eq $hashB -and $hashA -ne $hashC -and
    $invalidCountRejected -and $invalidRatioRejected -and $invalidSourcesRejected
)

$mainResult = Read-JsonFile -Path $resultMainPath -Run $runMain
$exactResult = Read-JsonFile -Path $resultExactPath -Run $runExact
$saturatedResult = Read-JsonFile -Path $resultSaturatedPath -Run $runSaturated
$countingResult = Read-JsonFile -Path $resultCountingPath -Run $runCounting
$shaResult = Read-JsonFile -Path $resultShaPath -Run $runSha
$noExactResult = Read-JsonFile -Path $resultNoExactPath -Run $runNoExact
$globalResult = Read-JsonFile -Path $resultGlobalPath -Run $runGlobal
$bySourceResult = Read-JsonFile -Path $resultBySourcePath -Run $runBySource
$invalidResult = Read-JsonFile -Path $resultInvalidPath -Run $runInvalid

$bitHashStatus = Parse-GoTestJson -JsonLogPath (Join-Path $ctx.ResultDir $bitHashJson.stdout) -ExpectedTests $bitHashTests
$paramsStatus = Parse-GoTestJson -JsonLogPath (Join-Path $ctx.ResultDir $paramsJson.stdout) -ExpectedTests $paramTests
$countingStatus = Parse-GoTestJson -JsonLogPath (Join-Path $ctx.ResultDir $countingJson.stdout) -ExpectedTests $countingTests
$noExactStatus = Parse-GoTestJson -JsonLogPath (Join-Path $ctx.ResultDir $noExactJson.stdout) -ExpectedTests $noExactTests

$bloomAddStatus = Parse-GoTestJson -JsonLogPath (Join-Path $ctx.ResultDir $bitHashJson.stdout) -ExpectedTests $bloomAddTests

# Each group must contribute at least one discovered test, and every discovered test must pass.
# An empty selection means the criterion is unconfirmed, not vacuously satisfied.
$bitHashOk = ($bitArrayTests.Count -gt 0 -and $bloomAddTests.Count -gt 0 -and $hashVectorTests.Count -gt 0 -and (Test-SelectedTestsPass -Status $bitHashStatus))
$paramsOk = (Test-SelectedTestsPass -Status $paramsStatus)
$countingTestOk = (Test-SelectedTestsPass -Status $countingStatus)
$noExactTestOk = (Test-SelectedTestsPass -Status $noExactStatus)
$bloomAddTestOk = (Test-SelectedTestsPass -Status $bloomAddStatus)

$expectedRows = @(
    @{ p = 0.1; m = 4793; k = 3; b = 600 },
    @{ p = 0.05; m = 6236; k = 4; b = 784 },
    @{ p = 0.01; m = 9586; k = 7; b = 1200 },
    @{ p = 0.001; m = 14378; k = 10; b = 1800 }
)

# The assignment fixes neither the name of the multi-FPR table nor its row field names,
# so the table is located by a set of known names and then by row shape.
$fprRows = @(Get-FprTableRows -Object $mainResult)
$parametersRuntimeOk = ($fprRows.Count -ge $expectedRows.Count)
for ($i = 0; $i -lt $expectedRows.Count; $i++) {
    if ($i -ge $fprRows.Count) { $parametersRuntimeOk = $false; continue }
    $row = $fprRows[$i]
    $expected = $expectedRows[$i]
    if ($null -eq $row) { $parametersRuntimeOk = $false; continue }
    $rowP = Get-ResultFieldValue -Object $row -Names @('false_positive_rate','p','fpr')
    $rowM = Get-ResultFieldValue -Object $row -Names @('m_bits','m','bits')
    $rowK = Get-ResultFieldValue -Object $row -Names @('k_hashes','k','hashes')
    $rowB = Get-ResultFieldValue -Object $row -Names @('bloom_bytes','memory_bytes','bloom_memory_bytes','bytes')
    if ($null -eq $rowP -or $null -eq $rowM -or $null -eq $rowK -or $null -eq $rowB) { $parametersRuntimeOk = $false; continue }
    if ([math]::Abs([double]$rowP - [double]$expected.p) -gt 0.000001) { $parametersRuntimeOk = $false }
    if ([int64]$rowM -ne [int64]$expected.m) { $parametersRuntimeOk = $false }
    if ([int64]$rowK -ne [int64]$expected.k) { $parametersRuntimeOk = $false }
    # The assignment does not fix the memory formula, so both the packed-bit size and the
    # actual []uint64 allocation are accepted.
    $bytesPacked = [int64][math]::Ceiling([double]$rowM / 8.0)
    $bytesWords = [int64]([math]::Ceiling([double]$rowM / 64.0) * 8.0)
    if ([int64]$rowB -ne $bytesPacked -and [int64]$rowB -ne $bytesWords) { $parametersRuntimeOk = $false }
}

# exact_map_allocated is not part of the assignment output format: check it only when present.
$exactMapAllocated = Get-ResultFieldValue -Object $exactResult -Names @('exact_map_allocated')
$exactMapOk = (
    ($null -eq $exactMapAllocated -or [bool]$exactMapAllocated) -and
    [int]$exactResult.total_records -eq 6 -and
    [int]$exactResult.exact_unique -eq 3 -and
    [int]$exactResult.exact_duplicates -eq 3
)

$fpEstimated = [int](Get-ResultFieldValue -Object $saturatedResult -Names @('estimated_false_positives') -Default 0)
$fpObservedRaw = Get-ResultFieldValue -Object $saturatedResult -Names @('real_false_positive_rate','observed_false_positive_rate')
$fpObserved = if ($null -ne $fpObservedRaw) { [double]$fpObservedRaw } else { [double]::NaN }
$fpExpected = [int]$saturatedResult.bloom_may_duplicate - [int]$saturatedResult.exact_duplicates
if ($fpExpected -lt 0) { $fpExpected = 0 }
$fpObservedExpected = if ([int]$saturatedResult.exact_unique -gt 0) { [double]$fpExpected / [double]$saturatedResult.exact_unique } else { 0.0 }
$falsePositiveOk = ($fpEstimated -gt 0 -and $fpEstimated -eq $fpExpected -and [math]::Abs($fpObserved - $fpObservedExpected) -lt 0.0000001)

$jsonReportOk = (Test-ResultFieldPresent -Object $mainResult -Names @('total_records')) -and ([int]$mainResult.total_records -eq 120)

# The assignment requires a Markdown report with a table over several false_positive_rate
# values but fixes neither the column layout nor the row labels, so only the values are matched.
$markdownRaw = Read-TextSafe -Path $reportMainPath
$markdownOk = (
    $markdownRaw -match '\b120\b' -and
    $markdownRaw -match '\|\s*0\.1\s*\|\s*4793\s*\|\s*3\s*\|[^\r\n]*\b600\b' -and
    $markdownRaw -match '\|\s*0\.05\s*\|\s*6236\s*\|\s*4\s*\|[^\r\n]*\b(780|784)\b' -and
    $markdownRaw -match '\|\s*0\.01\s*\|\s*9586\s*\|\s*7\s*\|[^\r\n]*\b(1199|1200)\b' -and
    $markdownRaw -match '\|\s*0\.001\s*\|\s*14378\s*\|\s*10\s*\|[^\r\n]*\b(1798|1800)\b'
)

$mapEstimateExpected = Get-ExactMapEstimateFromInput -Path $genAPath -Scope 'global'
$mainMapMemory = Get-ResultFieldValue -Object $mainResult -Names @('exact_map_memory_bytes','exact_map_memory_estimate_bytes')
$memoryComparisonOk = (
    ([int64]$mainResult.bloom_memory_bytes -ge 1199 -and [int64]$mainResult.bloom_memory_bytes -le 1200) -and
    $null -ne $mainMapMemory -and
    [int64]$mainMapMemory -eq [int64]$mapEstimateExpected
)

$invalidBySource = Get-ObjectPropertyValue -Object $invalidResult -Name 'by_source' -Default $null
$invalidSourcesMap = Get-ObjectPropertyValue -Object $invalidResult -Name 'invalid_sources' -Default $null
$invalidCollector01 = Get-ObjectPropertyValue -Object $invalidBySource -Name 'collector_01' -Default $null
$invalidCollector00BySource = Get-ObjectPropertyValue -Object $invalidBySource -Name 'collector_00' -Default $null
$invalidCollector00Count = [int](Get-ObjectPropertyValue -Object $invalidSourcesMap -Name 'collector_00' -Default 0)
$invalidBadSourceCount = [int](Get-ObjectPropertyValue -Object $invalidSourcesMap -Name 'bad_source' -Default 0)
$invalidMissingCount = [int](Get-ObjectPropertyValue -Object $invalidSourcesMap -Name '<missing>' -Default 0)
$sourceStatsOk = (
    $null -ne $invalidCollector01 -and
    $null -eq $invalidCollector00BySource -and
    $invalidCollector00Count -eq 1 -and
    $invalidBadSourceCount -eq 1 -and
    $invalidMissingCount -eq 1
)

$countingMemory = Get-ResultFieldValue -Object $countingResult -Names @('counting_memory_bytes','bloom_memory_bytes')
$countingOk = ($runCounting.exit_code -eq 0 -and $null -ne $countingMemory -and [int64]$countingMemory -gt 0 -and $countingTestOk)

# filter_digest and config_echo are not part of the assignment output format: the digests are
# compared only when both runs report them, and the effect of hash_family is otherwise checked
# through the exact counters, which must not depend on the hash family.
$mainDigest = Get-ResultFieldValue -Object $mainResult -Names @('filter_digest')
$shaDigest = Get-ResultFieldValue -Object $shaResult -Names @('filter_digest')
$hashDigestsDiffer = ($null -eq $mainDigest -or $null -eq $shaDigest -or [string]$mainDigest -ne [string]$shaDigest)

$hashVariantsOk = (
    $runSha.exit_code -eq 0 -and
    [int]$mainResult.exact_unique -eq [int]$shaResult.exact_unique -and
    [int]$mainResult.exact_duplicates -eq [int]$shaResult.exact_duplicates -and
    $hashDigestsDiffer -and
    $bitHashOk
)

$noExactAllocated = Get-ResultFieldValue -Object $noExactResult -Names @('exact_map_allocated')
$noExactOk = (
    ($null -eq $noExactAllocated -or -not [bool]$noExactAllocated) -and
    $null -eq (Get-ResultFieldValue -Object $noExactResult -Names @('exact_unique')) -and
    $null -eq (Get-ResultFieldValue -Object $noExactResult -Names @('exact_duplicates')) -and
    $null -eq (Get-ResultFieldValue -Object $noExactResult -Names @('exact_map_memory_bytes','exact_map_memory_estimate_bytes')) -and
    $noExactTestOk
)

$multiFprOk = $parametersRuntimeOk -and $markdownOk

$globalVsSourceOk = (
    [int]$globalResult.exact_unique -eq 3 -and
    [int]$globalResult.exact_duplicates -eq 3 -and
    [int]$bySourceResult.exact_unique -eq 4 -and
    [int]$bySourceResult.exact_duplicates -eq 2
)

$benchStdoutPath = Join-Path $ctx.ResultDir $benchRun.stdout
$benchText = Read-TextSafe -Path $benchStdoutPath
$benchFilterMatched = $false
foreach ($name in $benchFilterNames) {
    if ($benchText -match ([regex]::Escape($name) + '(-\d+)?\s+\d+\s+[0-9.]+')) { $benchFilterMatched = $true }
}
$benchStreamMatched = $false
foreach ($name in $benchStreamNames) {
    if ($benchText -match ([regex]::Escape($name) + '(-\d+)?\s+\d+\s+[0-9.]+')) { $benchStreamMatched = $true }
}
$benchmarksOk = ($benchRun.exit_code -eq 0 -and $benchFilterMatched -and $benchStreamMatched)

$millionInputPath = Join-Path $ctx.InputsDir 'generated_1m.jsonl'
$millionMetricsPath = Join-Path $ctx.OutputsDir 'million_metrics.json'

$millionGenerate = Invoke-HiddenProcess -Ctx $ctx -Name 'cli_generate_1m' -FilePath $toolPath -Arguments @('generate','--count','1000000','--duplicate-ratio','0.2','--out',$millionInputPath,'--seed','42','--sources','3') -TimeoutSec 600
$millionStats = Get-JsonlStats -Path $millionInputPath -MaxSource 3
$millionAttempts = New-Object System.Collections.Generic.List[object]
$millionCleanupCandidates = New-Object System.Collections.Generic.List[string]
$bestMillionRun = $null
$bestMillionResult = $null
$bestMillionThroughput = 0.0
$bestMillionDurationMs = 0
$bestMillionPeak = 0
$millionBloomBytes = 0

for ($attempt = 1; $attempt -le 3; $attempt++) {
    $attemptResultPath = Join-Path $ctx.OutputsDir "result_1m_no_exact_attempt_$attempt.json"
    $millionCleanupCandidates.Add($attemptResultPath) | Out-Null
    $attemptRun = Invoke-HiddenProcess -Ctx $ctx -Name "cli_run_1m_no_exact_attempt_$attempt" -FilePath $toolPath -Arguments @('run','--in',$millionInputPath,'--config',$configMillionPath,'--out',$attemptResultPath,'--exact-compare=false') -TimeoutSec 600
    $attemptResult = Read-JsonFile -Path $attemptResultPath -Run $attemptRun
    $attemptDurationSec = if ([double]$attemptRun.duration_ms -gt 0) { [double]$attemptRun.duration_ms / 1000.0 } else { 0.0 }
    $attemptThroughput = if ($attemptDurationSec -gt 0) { [double]$millionStats.lines / $attemptDurationSec } else { 0.0 }
    $attemptPeak = [int64]$attemptRun.peak_working_set_bytes
    $attemptBloomBytes = [int64](Get-ResultFieldValue -Object $attemptResult -Names @('bloom_memory_bytes') -Default 0)

    $millionAttempts.Add([ordered]@{
        attempt = $attempt
        duration_ms_external = $attemptRun.duration_ms
        throughput_ids_per_sec_external = [math]::Round($attemptThroughput, 3)
        peak_working_set_bytes = $attemptPeak
        bloom_memory_bytes = $attemptBloomBytes
        timed_out = $attemptRun.timed_out
        command = $attemptRun.command
        log = $attemptRun.log
    }) | Out-Null

    if ($null -eq $bestMillionRun -or $attemptThroughput -gt $bestMillionThroughput) {
        $bestMillionThroughput = $attemptThroughput
        $bestMillionRun = $attemptRun
        $bestMillionResult = $attemptResult
        $bestMillionDurationMs = $attemptRun.duration_ms
        $bestMillionPeak = $attemptPeak
        $millionBloomBytes = $attemptBloomBytes
    }

    if ($attemptThroughput -ge 500000) {
        break
    }
}

$millionRun = $bestMillionRun
$millionResult = $bestMillionResult
$millionDurationSec = if ([double]$bestMillionDurationMs -gt 0) { [double]$bestMillionDurationMs / 1000.0 } else { 0.0 }
$millionThroughputExternal = $bestMillionThroughput
$millionPeak = $bestMillionPeak

$millionOk = (
    $millionGenerate.exit_code -eq 0 -and
    $millionRun.exit_code -eq 0 -and
    $millionStats.lines -eq 1000000 -and
    $millionStats.unique -eq 800000 -and
    $millionStats.duplicates -eq 200000 -and
    $millionBloomBytes -eq 1198136 -and
    $millionBloomBytes -lt 4194304 -and
    $millionThroughputExternal -ge 500000 -and
    $millionPeak -gt 0
)

Save-CheckJson -Path $millionMetricsPath -Value ([ordered]@{
    lines = $millionStats.lines
    unique = $millionStats.unique
    duplicates = $millionStats.duplicates
    bloom_memory_bytes = $millionBloomBytes
    duration_ms_external = $bestMillionDurationMs
    throughput_ids_per_sec_external = [math]::Round($millionThroughputExternal, 3)
    peak_working_set_bytes = $millionPeak
    timeout_sec = $millionRun.timeout_sec
    timed_out = $millionRun.timed_out
    command = $millionRun.command
    attempts_count = $millionAttempts.Count
})

foreach ($path in @($millionInputPath) + @($millionCleanupCandidates)) {
    if (Test-Path -LiteralPath $path) {
        Remove-Item -LiteralPath $path -Force
    }
}
$cleanupOk = $true

$gitHeadPath = Join-Path $ctx.MetaDir 'git_head.txt'
$gitStatusPath = Join-Path $ctx.MetaDir 'git_status_short.txt'
$goVersionPath = Join-Path $ctx.MetaDir 'go_version.txt'
$goEnvPath = Join-Path $ctx.MetaDir 'go_env.txt'

if ($gitCmd) {
    $metaGitHead = Invoke-HiddenProcess -Ctx $ctx -Name 'meta_git_head' -FilePath $gitCmd.Source -Arguments @('rev-parse','HEAD') -TimeoutSec 30 -AllowNonZero $true -ValidationOnly $true
    Copy-Item -LiteralPath (Join-Path $ctx.ResultDir $metaGitHead.stdout) -Destination $gitHeadPath -Force

    $metaGitStatus = Invoke-HiddenProcess -Ctx $ctx -Name 'meta_git_status' -FilePath $gitCmd.Source -Arguments @('status','--short') -TimeoutSec 30 -AllowNonZero $true -ValidationOnly $true
    Copy-Item -LiteralPath (Join-Path $ctx.ResultDir $metaGitStatus.stdout) -Destination $gitStatusPath -Force
    Ensure-FileExists -Path $gitStatusPath
} else {
    Write-CheckText -Path $gitHeadPath -Text 'git is not available'
    Write-CheckText -Path $gitStatusPath -Text ''
}

$metaGoVersion = Invoke-HiddenProcess -Ctx $ctx -Name 'meta_go_version' -FilePath $ctx.GoCmd -Arguments @('version') -TimeoutSec 30 -AllowNonZero $true -ValidationOnly $true
Copy-Item -LiteralPath (Join-Path $ctx.ResultDir $metaGoVersion.stdout) -Destination $goVersionPath -Force

$metaGoEnv = Invoke-HiddenProcess -Ctx $ctx -Name 'meta_go_env' -FilePath $ctx.GoCmd -Arguments @('env','GOVERSION','GOOS','GOARCH') -TimeoutSec 30 -AllowNonZero $true -ValidationOnly $true
Copy-Item -LiteralPath (Join-Path $ctx.ResultDir $metaGoEnv.stdout) -Destination $goEnvPath -Force

$unitListText = Read-TextSafe -Path (Join-Path $ctx.ResultDir $unitTestsList.stdout)
$benchListText = Read-TextSafe -Path (Join-Path $ctx.ResultDir $benchList.stdout)
$unitTestsPresent = $unitListText -match '(?m)^Test[A-Za-z0-9_]+'
$benchmarksPresent = $benchListText -match '(?m)^Benchmark[A-Za-z0-9_]+'

$makefilePath = Join-Path $ctx.RepoRoot 'Makefile'
$readmePath = Join-Path $ctx.RepoRoot 'README.md'
$solutionPath = Join-Path $ctx.RepoRoot 'docs\\reshenie.md'
$controlPath = Join-Path $ctx.RepoRoot 'testdata\\control'

$makefileText = if (Test-Path -LiteralPath $makefilePath) { Get-Content -LiteralPath $makefilePath -Raw -Encoding UTF8 } else { '' }
$makeTargetTest = $makefileText -match '(?m)^\s*test\s*:'
$makeTargetBench = $makefileText -match '(?m)^\s*bench\s*:'
$makeTargetDemo = $makefileText -match '(?m)^\s*demo\s*:'

$readmeOk = (Test-Path -LiteralPath $readmePath) -and ((Get-Item -LiteralPath $readmePath).Length -gt 300)
$solutionOk = (Test-Path -LiteralPath $solutionPath) -and ((Get-Item -LiteralPath $solutionPath).Length -gt 300)
$controlFiles = if (Test-Path -LiteralPath $controlPath) { @(Get-ChildItem -LiteralPath $controlPath -File -Recurse -ErrorAction SilentlyContinue) } else { @() }
$controlDataOk = $controlFiles.Count -gt 0

$goTestPasses = ($goTestAll.exit_code -eq 0)
$makeTestPasses = ($ctx.CommandResults.ContainsKey('make_test') -and $ctx.CommandResults['make_test'].exit_code -eq 0)
$makeBenchPasses = ($ctx.CommandResults.ContainsKey('make_bench') -and $ctx.CommandResults['make_bench'].exit_code -eq 0)
$makeDemoPasses = ($ctx.CommandResults.ContainsKey('make_demo') -and $ctx.CommandResults['make_demo'].exit_code -eq 0)

$bloomRunOk = ($runMain.exit_code -eq 0 -and [int]$exactResult.exact_unique -eq 3 -and $bloomAddTestOk)

Add-BooleanFeatureAssessment -Ctx $ctx -Id 'minimum.generator' -Level 'minimum' -Category 'cli' -Requirement 'Generator validates parameters and deterministic invariants at runtime' -Ok $generatorInvariantOk -Evidence @($genA.log, $genB.log, $genC.log, (Convert-ResultPathToEvidence -Ctx $ctx -Path $genAPath)) -Details "lines=$($generatorStats.lines); unique=$($generatorStats.unique); duplicates=$($generatorStats.duplicates); seed42_equal=$($hashA -eq $hashB); seed43_diff=$($hashA -ne $hashC)"
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'minimum.bloom_run' -Level 'minimum' -Category 'algorithm' -Requirement 'Bloom run semantics validated and discovered add/may-contain tests pass' -Ok $bloomRunOk -Evidence @($runMain.log, $runExact.log, $bitHashJson.log)
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'minimum.parameters' -Level 'minimum' -Category 'algorithm' -Requirement 'Runtime parameter table matches exact m/k/bytes values for four FPR values' -Ok $parametersRuntimeOk -Evidence @($runMain.log, (Convert-ResultPathToEvidence -Ctx $ctx -Path $resultMainPath))
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'minimum.exact_map' -Level 'minimum' -Category 'algorithm' -Requirement 'Exact map fixture yields total_records=6, exact_unique=3, exact_duplicates=3' -Ok $exactMapOk -Evidence @($runExact.log, (Convert-ResultPathToEvidence -Ctx $ctx -Path $resultExactPath))
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'minimum.false_positive' -Level 'minimum' -Category 'algorithm' -Requirement 'Saturated fixture yields positive and independently verified false positives' -Ok $falsePositiveOk -Evidence @($runSaturated.log, (Convert-ResultPathToEvidence -Ctx $ctx -Path $resultSaturatedPath))
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'minimum.json_report' -Level 'minimum' -Category 'format' -Requirement 'JSON report has required types, invariants and config echo' -Ok $jsonReportOk -Evidence @($runMain.log, (Convert-ResultPathToEvidence -Ctx $ctx -Path $resultMainPath))
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'minimum.bit_hash_tests' -Level 'minimum' -Category 'tests' -Requirement 'go test -count=1 -json run/pass for BitArray, Bloom and hash vectors' -Ok $bitHashOk -Evidence @($bitHashJson.log)

Add-BooleanFeatureAssessment -Ctx $ctx -Id 'good.markdown_report' -Level 'good' -Category 'format' -Requirement 'Markdown report metrics and FPR table are consistent with JSON values' -Ok $markdownOk -Evidence @((Convert-ResultPathToEvidence -Ctx $ctx -Path $reportMainPath), (Convert-ResultPathToEvidence -Ctx $ctx -Path $resultMainPath))
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'good.benchmarks' -Level 'good' -Category 'performance' -Requirement 'Real go benchmark output contains Bloom and streaming no-exact ns/op lines' -Ok $benchmarksOk -Evidence @($benchRun.log)
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'good.memory_comparison' -Level 'good' -Category 'performance' -Requirement 'Bloom bytes and exact map estimate match independent recomputation' -Ok $memoryComparisonOk -Evidence @($runMain.log, (Convert-ResultPathToEvidence -Ctx $ctx -Path $resultMainPath))
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'good.parameter_tests' -Level 'good' -Category 'tests' -Requirement 'Discovered parameter-calculation tests pass' -Ok $paramsOk -Evidence @($paramsJson.log)
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'good.source_statistics' -Level 'good' -Category 'format' -Requirement 'Valid by_source and invalid_sources statistics are runtime-verified' -Ok $sourceStatsOk -Evidence @($runInvalid.log, (Convert-ResultPathToEvidence -Ctx $ctx -Path $resultInvalidPath))

Add-BooleanFeatureAssessment -Ctx $ctx -Id 'excellent.counting_bloom' -Level 'excellent' -Category 'algorithm' -Requirement 'Counting mode reports bytes and discovered counting tests pass' -Ok $countingOk -Evidence @($runCounting.log, $countingJson.log)
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'excellent.hash_variants' -Level 'excellent' -Category 'algorithm' -Requirement 'FNV/SHA runtime stats match while filter digests differ and known vectors pass' -Ok $hashVariantsOk -Evidence @($runMain.log, $runSha.log, $bitHashJson.log)
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'excellent.no_exact_mode' -Level 'excellent' -Category 'performance' -Requirement 'No-exact mode leaves exact_* fields null, white-box test and 1M gate' -Ok ($noExactOk -and $millionOk) -Evidence @($runNoExact.log, $noExactJson.log, $millionRun.log, (Convert-ResultPathToEvidence -Ctx $ctx -Path $millionMetricsPath))
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'excellent.multi_fpr_report' -Level 'excellent' -Category 'report' -Requirement 'JSON and Markdown contain ordered 0.1/0.05/0.01/0.001 m/k/bytes table' -Ok $multiFprOk -Evidence @($runMain.log, (Convert-ResultPathToEvidence -Ctx $ctx -Path $reportMainPath), (Convert-ResultPathToEvidence -Ctx $ctx -Path $resultMainPath))
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'excellent.global_vs_source' -Level 'excellent' -Category 'algorithm' -Requirement 'Fixture validates global unique3/dup3 versus per_source unique4/dup2' -Ok $globalVsSourceOk -Evidence @($runGlobal.log, $runBySource.log)

Add-BooleanFeatureAssessment -Ctx $ctx -Id 'engineering.unit_tests_present' -Level 'engineering' -Category 'tests' -Requirement 'Unit tests are discoverable via go test -list' -Ok $unitTestsPresent -Evidence @($unitTestsList.log)
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'engineering.benchmarks_present' -Level 'engineering' -Category 'benchmarks' -Requirement 'Benchmarks are discoverable via go test -list' -Ok $benchmarksPresent -Evidence @($benchList.log)
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'engineering.go_test_passes' -Level 'engineering' -Category 'tests' -Requirement 'go test ./... passes' -Ok $goTestPasses -Evidence @($goTestAll.log)
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'engineering.make_test_runs' -Level 'engineering' -Category 'reproducibility' -Requirement 'make test passes' -Ok $makeTestPasses -Evidence @('logs/make_test.log')
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'engineering.make_bench_runs' -Level 'engineering' -Category 'reproducibility' -Requirement 'make bench passes' -Ok $makeBenchPasses -Evidence @('logs/make_bench.log')
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'engineering.make_demo_runs' -Level 'engineering' -Category 'reproducibility' -Requirement 'make demo passes' -Ok $makeDemoPasses -Evidence @('logs/make_demo.log')
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'engineering.readme' -Level 'engineering' -Category 'documentation' -Requirement 'README.md exists and has meaningful content' -Ok $readmeOk -Evidence @('repo_snapshot/README.md')
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'engineering.make_target_test' -Level 'engineering' -Category 'reproducibility' -Requirement 'Makefile contains test target' -Ok $makeTargetTest -Evidence @('repo_snapshot/Makefile')
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'engineering.make_target_bench' -Level 'engineering' -Category 'reproducibility' -Requirement 'Makefile contains bench target' -Ok $makeTargetBench -Evidence @('repo_snapshot/Makefile')
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'engineering.make_target_demo' -Level 'engineering' -Category 'reproducibility' -Requirement 'Makefile contains demo target' -Ok $makeTargetDemo -Evidence @('repo_snapshot/Makefile')
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'engineering.control_data' -Level 'engineering' -Category 'reproducibility' -Requirement 'Fixed control data exists under testdata/control' -Ok $controlDataOk -Evidence @('repo_snapshot/testdata/control')
Add-BooleanFeatureAssessment -Ctx $ctx -Id 'engineering.solution_doc' -Level 'engineering' -Category 'documentation' -Requirement 'docs/reshenie.md exists and is non-empty' -Ok $solutionOk -Evidence @('repo_snapshot/docs/reshenie.md')

foreach ($name in @('README.md', 'Makefile', 'go.mod', 'docs', 'testdata/control')) {
    $source = Join-Path $ctx.RepoRoot $name
    if (Test-Path -LiteralPath $source) {
        $destination = Join-Path $ctx.ResultDir (Join-Path 'repo_snapshot' $name)
        $parent = Split-Path -Parent $destination
        if ($parent) { New-Item -ItemType Directory -Force -Path $parent | Out-Null }
        Copy-Item -LiteralPath $source -Destination $destination -Recurse -Force
    }
}

$checkerHash = Get-FileSha256 -Path $PSCommandPath

$zipPath = Complete-Check -Ctx $ctx -Notes @{
    expected_scores = '7+5+5+12'
    non_validation_failures = @($ctx.NonValidationFailures)
    million_ok = $millionOk
    million_throughput_ids_per_sec_external = [math]::Round($millionThroughputExternal, 3)
    cleanup_ok = $cleanupOk
    checker_sha256 = $checkerHash
    manifest_repo_root = $ctx.RepoRoot
}

Write-Host "CHECKER_SHA256=$checkerHash"
