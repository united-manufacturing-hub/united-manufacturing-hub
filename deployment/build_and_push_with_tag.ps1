$docker_tag=$args[0]

if ([string]::IsNullOrEmpty($docker_tag))
{
    Write-Error "No tag specified"
    return
}
$git_root = git rev-parse --show-toplevel
$current_location = Get-Location

# Set to gitroot, allowing for relative paths
try
{
    Set-Location $git_root;

    $deployment_path = Join-Path -Path $git_root -ChildPath "deployment"
    $childs = Get-ChildItem -Path $deployment_path -Directory

    foreach ($child in $childs)
    {
        if ($child.Name -eq "united-manufacturing-hub")
        {
            continue
        }

        $microservice_path = Join-Path -Path $deployment_path -ChildPath $child.Name
        $docker_file_path = Join-Path -Path $microservice_path -ChildPath "Dockerfile"


        if (-not((Test-Path -Path $docker_file_path -PathType Leaf)))
        {
            continue
        }
        $tag_name = "unitedmanufacturinghub/$( $child.Name ):$docker_tag"

        Write-Output $child.Name
        Write-Output $tag_name

        Write-Output $docker_file_path
        docker build -f $docker_file_path -t $tag_name .
        # Exit if the build failed
        if ($LASTEXITCODE -ne 0)
        {
            Write-Error "Build failed"
            return
        }
        docker push $tag_name
    }
}
finally
{
    Set-Location $current_location
}