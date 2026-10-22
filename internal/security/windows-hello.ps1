param([ValidateSet('check', 'verify')][string]$Mode = 'check')
$ErrorActionPreference = 'Stop'

function Await-WinRtOperation {
  param($Operation, [Type]$ResultType)
  $asTask = [System.WindowsRuntimeSystemExtensions].GetMethods() |
    Where-Object { $_.Name -eq 'AsTask' -and $_.IsGenericMethod -and $_.GetParameters().Count -eq 1 } |
    Select-Object -First 1
  $task = $asTask.MakeGenericMethod($ResultType).Invoke($null, @($Operation))
  $task.Wait()
  return $task.Result
}

try {
  Add-Type -AssemblyName System.Runtime.WindowsRuntime
  $verifier = [Windows.Security.Credentials.UI.UserConsentVerifier,Windows.Security.Credentials.UI,ContentType=WindowsRuntime]
  if ($Mode -eq 'check') {
    $result = Await-WinRtOperation ($verifier::CheckAvailabilityAsync()) ([Windows.Security.Credentials.UI.UserConsentVerifierAvailability])
    @{ success = $true; available = ($result.ToString() -eq 'Available') } | ConvertTo-Json -Compress
  } else {
    $result = Await-WinRtOperation ($verifier::RequestVerificationAsync('Unlock Gist Manager')) ([Windows.Security.Credentials.UI.UserConsentVerificationResult])
    @{ success = ($result.ToString() -eq 'Verified'); available = $true } | ConvertTo-Json -Compress
  }
} catch {
  @{ success = $false; available = $false } | ConvertTo-Json -Compress
}
