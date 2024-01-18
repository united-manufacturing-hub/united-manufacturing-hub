#!/bin/bash

# Function to check if the count is correct
check_count() {
    table_name=$1
    expected_count=$2
    query="SELECT COUNT(*) FROM $table_name;"
    if ! docker exec -i timescaledb psql -U postgres -d umh_v2 -t -c "$query" |
        grep -q -E "^\s*$expected_count$"; then
        echo "Number of $table_name is incorrect"
        docker exec -i timescaledb psql -U postgres -d umh_v2 -c "SELECT * FROM $table_name;"
        exit 1
    fi
}

# Function to check if the values are correct
check_values() {
    table_name=$1
    expected_values=$2
    query="SELECT $expected_values FROM $table_name;"
    docker exec -i timescaledb psql -U postgres -d umh_v2 -t -c "$query" >/tmp/"$table_name".txt
    while IFS= read -r line; do
        if ! grep -Fxq "$line" /tmp/"$table_name".txt; then
            echo "One or more lines are missing"
            docker exec -i timescaledb psql -U postgres -d umh_v2 -c "SELECT * FROM $table_name;"
            exit 1
        fi
    done <<<"$expected_values"
}

# Send messages to RedPanda
echo "Sending messages to RedPanda..."

echo '{"timestamp_ms": 845980440000, "value": 1}
{"timestamp_ms": 845980440000, "pos": {"x": 1, "y": 2, "z": 3}}
{"timestamp_ms": 845980440000, "stringValue": "hello"}
' >/tmp/messages.txt
cat /tmp/messages.txt

docker run -t --rm --network=k2pv2_network \
    --mount type=bind,source=/tmp/messages.txt,target=/messages.txt,readonly \
    confluentinc/cp-kafkacat:7.0.13 \
    bash -c \
    'kafkacat -b redpanda:9092 -t umh.v1.enterprise.site.area.line.workcell._historian.head -P -l /messages.txt'

echo "Waiting for messages to be processed..."
sleep 5

# Check if the number of assets is correct
check_count "asset" 1

# Check if the number of tags is correct
check_count "tag" 4

# Check if the tag names and their values are correct
check_values "tag" "pos\$x, 1
pos\$y, 2
pos\$z, 3
value, 1"

# Check if the number of tag_string is correct
check_count "tag_string" 1

# Check if the tag_string names and their values are correct
check_values "tag_string" "stringValue, hello"
