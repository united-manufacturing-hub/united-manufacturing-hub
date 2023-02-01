#!/usr/bin/env sh

apk update && apk add --no-cache jq curl

TIMESTAMP_TO=$(date -I)
TIMESTAMP_FROM=$(date --date='5 min ago' -I)

# cURL factoryinsight to see if there's data in the database
curl --request GET -sL \
     --header 'Accept-Encoding: gzip' \
     --header "Authorization: $(FACTORYINSIGHT_APIKEY)" \
     --url "$(FACTORYINSIGHT_BASEURL)/api/v2/factoryinsight/testLocation/DefaultArea/DefaultProductionLine/testMachine/tags/custom/Pressure?from=$(TIMESTAMP_FROM)&to=$(TIMESTAMP_TO)&gapFilling=null&tagAggregates=avg&timeBucket=1m&includePrevious=true&includeNext=true&includeRunning=true&keepStatesInteger=true'" \
     --output './output'

# Output the response
jq -r ./output

# Check if the datapoints array is empty
if [ "$(jq -r '.datapoints | length' ./output)" -eq 0 ]; then
    echo "No data found in the database"
    exit 1
fi