#!/usr/bin/env sh

TIMESTAMP_TO=$(date +%FT%TZ)
TIMESTAMP_FROM=$(date +%FT%TZ --date='5 min ago')

AUTH_HEADER="Authorization: ${FACTORYINSIGHT_APIKEY}"
URL="${FACTORYINSIGHT_BASEURL}/api/v2/factoryinsight/testLocation/DefaultArea/DefaultProductionLine/testMachine/tags/custom/Pressure?from=${TIMESTAMP_FROM}&to=${TIMESTAMP_TO}&gapFilling=null&tagAggregates=avg&timeBucket=1m&includePrevious=true&includeNext=true&includeRunning=true&keepStatesInteger=true'"

echo "URL: ${URL}"

# cURL factoryinsight to see if there's data in the database
curl --request GET -sL \
     --header "${AUTH_HEADER}" \
     --url "${URL}" \
     --output './output'

# Output the response
cat ./output

# Check if the datapoints array is empty
if [ "$(jq -r '.datapoints | length' ./output)" -eq 0 ]; then
  printf "\nNo data found in the database"
  exit 1
fi
printf "\nData found in the database"