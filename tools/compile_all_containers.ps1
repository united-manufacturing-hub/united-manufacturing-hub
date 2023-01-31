pushd (git rev-parse --show-toplevel) > $null

foreach ($directory in (Get-ChildItem -Directory -Path .\deployment\*)) {

  $DOCKER_FILE_PATH = "$($directory.FullName)\Dockerfile"

  if (Test-Path $DOCKER_FILE_PATH) {
    Write-Output "Building $directory"
    docker build -f $DOCKER_FILE_PATH .

    if ($LASTEXITCODE -ne 0) {
        Write-Output "Build failed for $directory"
        popd > $null
        exit 1
    }
  }
}

popd > $null
