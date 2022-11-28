Get-ChildItem -Path .\ -Filter *.pem -Recurse -File -Name| ForEach-Object {
    $FileContent = Get-Content $_ -Raw
    $fileContentInBytes = [System.Text.Encoding]::UTF8.GetBytes($FileContent)
    $fileContentEncoded = [System.Convert]::ToBase64String($fileContentInBytes)
    $fileContentEncoded | Set-content ($_ + ".b64")
    Write-Host $_ + ".b64" + "File Encoded Successfully!"
}