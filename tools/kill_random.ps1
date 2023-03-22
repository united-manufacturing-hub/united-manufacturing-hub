# Copyright 2023 UMH Systems GmbH
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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
