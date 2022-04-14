$docker_tag=$args[0]

if ([string]::IsNullOrEmpty($docker_tag))
{
    Write-Error "No tag specified"
    return
}
$git_root = git rev-parse --show-toplevel
$current_location = Get-Location

# Set to gitroot, allowing for relative paths
Set-Location $git_root;

$deployment_path = Join-Path -Path $git_root -ChildPath "deployment"
$childs = Get-ChildItem -Path $deployment_path -Directory | Out-Null


for ($i = 0; $i -lt $childs.Count; $i++) {
    $child = $childs[$i]
    Write-Output $child
}

Set-Location $current_location;