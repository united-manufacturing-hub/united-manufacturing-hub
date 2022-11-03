function PrintServices
{
    param (
        [string[]]$parameter
    )

    $i = 0
    foreach ($service in $parameter)
    {
        Write-Host "$i - $service"
        $i++
    }

    Write-Host "a - All"
    Write-Host "q - Quit"
}

function PrintSelectedServices
{
    param (
        [string[]]$parameter
    )
    foreach ($serviceName in $parameter)
    {
        Write-Host -ForegroundColor Cyan "- $serviceName"
    }
}

function ValidateServiceInput {
    param (
        [Parameter(Mandatory=$true)]
        [ValidateNotNullOrEmpty()]
        [ValidatePattern("(^\d+?$)|(^q?$)|(^a?$)")]
        [string]$parameter
    )
}

function BuildAndPush
{
    param (
        [System.Collections.ArrayList]$parameter
    )

    # Set to gitroot, allowing for relative paths
    Set-Location $git_root;

    foreach ($service in $parameter)
    {
        $microservice_path = Join-Path -Path $deployment_path -ChildPath $service
        $docker_file_path = Join-Path -Path $microservice_path -ChildPath "Dockerfile"

        if (-not((Test-Path -Path $docker_file_path -PathType Leaf)))
        {
            continue
        }
        $tag_name = "$repo_owner/$($service):$docker_tag"

        Write-Host -ForegroundColor DarkCyan "Building $service with tag $tag_name"

        docker build -f $docker_file_path -t $tag_name .
        # Exit if the build failed
        if ($LASTEXITCODE -ne 0)
        {
            Write-Error "Build failed"
            return
        }

        docker push $tag_name

        if ($LASTEXITCODE -ne 0)
        {
            Write-Host -ForegroundColor Red "Push failed. Check your credentials"
            exit
        }
    }

}

$repo_owner = Read-Host "Enter the name of the repo owner: "
if ([string]::IsNullOrEmpty($repo_owner)) {
    Write-Host -ForegroundColor Red "Repo owner cannot be empty"
    exit
}

$docker_tag = Read-Host "Enter the tag to use: "
if ([string]::IsNullOrEmpty($docker_tag))
{
    $docker_tag = "latest"
}

$git_root = git rev-parse --show-toplevel
$current_location = Get-Location

$deployment_path = Join-Path -Path $git_root -ChildPath "deployment"
$childs = Get-ChildItem -Path $deployment_path -Directory -Name

$selectedServices = [System.Collections.ArrayList]@()
$servicesLeft = [System.Collections.ArrayList]@()
$servicesLeft.AddRange($childs)

do {
    Write-Host "Services available:"
    PrintServices -parameter $servicesLeft

    $selected = Read-Host "Enter the number of the service to build: "
    try
    {
        ValidateServiceInput -parameter $selected
    }
    catch [System.Management.Automation.ValidationMetadataException]
    {
        Write-Host -ForegroundColor Red "Invalid input"
        continue
    }

    if ($selected -eq "q") {
        Write-Host -ForegroundColor Green "Exiting"
        Set-Location $current_location
        exit
    }
    if ($selected -eq "a") {
        $selectedServices.AddRange($childs)
        Write-Host -ForegroundColor Green "Building all services"
        break
    }

    $selectedServices.Add($servicesLeft[$selected]) | Out-Null
    $servicesLeft.RemoveAt($selected)
    Write-Host -ForegroundColor Cyan "Selected services:"
    PrintSelectedServices -parameter $selectedServices

    $continue = Read-Host "Add more? ([y]/n)"
} until ($continue -imatch "n")

BuildAndPush -parameter $selectedServices

Write-Host -ForegroundColor Green "All done. Exiting..."
Set-Location $current_location
exit
