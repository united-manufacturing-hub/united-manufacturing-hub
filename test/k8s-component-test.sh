#!/usr/bin/env bash

set -xe

helm install united-manufacturing-hub ../deployment/united-manufacturing-hub -n united-manufacturing-hub --wait

kubectl get po,svc -n united-manufacturing-hub

kubectl apply -f k8s-component-test-job.yaml -n united-manufacturing-hub

kubectl wait -n united-manufacturing-hub --for=condition=complete --timeout=1m job/component-test

kubectl logs -l type=component-test

SUCCESS=$(kubectl get job component-test -n united-manufacturing-hub -o jsonpath='{.status.succeeded}')

if [ "$SUCCESS" != '1' ]; then exit 1; fi

echo "Component test succesful"