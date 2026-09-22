param(
  [ValidateSet('check', 'verify')]
  [string]$Mode = 'check'
)

$ErrorActionPreference = 'Stop'

function Await-WinRtOperation {
  param(
    [Parameter(Mandatory = $true)]
    $Operation,
    [Parameter(Mandatory = $true)]
    [Type]$ResultType
  )

  $asTask = [System.WindowsRuntimeSystemExtensions].GetMethods() |
    Where-Object {
      $_.Name -eq 'AsTask' -and
      $_.IsGenericMethod -and
      $_.GetParameters().Count -eq 1
    } |
    Select-Object -First 1

  $task = $asTask.MakeGenericMethod($ResultType).Invoke($null, @($Operation))
  $task.Wait()
  return $task.Result
}

try {
  Add-Type -AssemblyName System.Runtime.WindowsRuntime
  $verifier = [Windows.Security.Credentials.UI.UserConsentVerifier,Windows.Security.Credentials.UI,ContentType=WindowsRuntime]

  if ($Mode -eq 'check') {
    $result = Await-WinRtOperation `
      -Operation ($verifier::CheckAvailabilityAsync()) `
      -ResultType ([Windows.Security.Credentials.UI.UserConsentVerifierAvailability])
    @{ success = $true; available = ($result.ToString() -eq 'Available'); result = $result.ToString() } |
      ConvertTo-Json -Compress
    exit 0
  }

  $verification = Await-WinRtOperation `
    -Operation ($verifier::RequestVerificationAsync('解锁 Gist管理器')) `
    -ResultType ([Windows.Security.Credentials.UI.UserConsentVerificationResult])
  $isVerified = $verification.ToString() -eq 'Verified'
  @{ success = $isVerified; available = $true; result = $verification.ToString() } |
    ConvertTo-Json -Compress
  exit $(if ($isVerified) { 0 } else { 2 })
} catch {
  @{ success = $false; available = $false; result = 'Unavailable'; error = $_.Exception.Message } |
    ConvertTo-Json -Compress
  exit 1
}
