# This script kills a random pod inside united-manufacturing-hub ever 0-256 seconds

while($true){
    $pods = kubectl get pods --no-headers -o custom-columns=":metadata.name" --namespace united-manufacturing-hub
    $pod_array = $pods.split()


    $pod_to_kill = Get-Random -InputObject $pod_array
    Write-Output "Killing $($pod_to_kill)"

    kubectl delete pods $pod_to_kill --grace-period=0 --force --namespace united-manufacturing-hub 2>&1 | out-null

    $sleep_time = Get-Random -Minimum -1 -Maximum 256
    Start-Sleep -Seconds $sleep_time
}