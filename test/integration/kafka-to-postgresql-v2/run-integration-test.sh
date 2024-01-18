#!/bin/bash

# Send messages to RedPanda
echo "Sending messages to RedPanda..."
docker run -it --rm --network=k2pv2_network \
    edenhill/kcat \
    -b redpanda:9092 \
    -t umh.v1.enterprise.site.area.line.workcell._historian.head \
    -K: \
    -P <<EOF
1:{"timestamp_ms": 845980440000, "value": 1}
2:{"timestamp_ms": 845980440000, "pos": {"x": 1, "y": 2, "z": 3}}
3:{"timestamp_ms": 845980440000, "stringValue": "hello"}
EOF

echo "Waiting for messages to be processed..."
sleep 5

# Query the database and check results
echo "Querying the database..."

## Check if the number of assets is correct
docker exec -i timescaledb psql -U postgres -d umh_v2 -t -c "SELECT COUNT(*) FROM asset;" |
    grep -q -E "^\s*1$" || echo "Number of assets is incorrect" && exit 1

## Check if the number of tags is correct
docker exec -i timescaledb psql -U postgres -d umh_v2 -t -c "SELECT COUNT(*) FROM tag;" |
    grep -q -E "^\s*4$" || echo "Number of tags is incorrect" && exit 1

## Check if the tag names and their values are correct
docker exec -i timescaledb psql -U postgres -d umh_v2 -t -c "SELECT name, value FROM tag ORDER BY name;" >/tmp/tag_values.txt
grep -Fxq " pos\$x |     1 " /tmp/tag_values.txt &&
    grep -Fxq " pos\$y |     2 " /tmp/tag_values.txt &&
    grep -Fxq " pos\$z |     3 " /tmp/tag_values.txt &&
    grep -Fxq " value |     1" /tmp/tag_values.txt &&
    echo "All lines are present" || echo "One or more lines are missing" && exit 1

## Check if the number of tag_string is correct
docker exec -i timescaledb psql -U postgres -d umh_v2 -t -c "SELECT COUNT(*) FROM tag_string;" |
    grep -q -E "^\s*1$" || echo "Number of tag_string is incorrect" && exit 1

## Check if the tag_string names and their values are correct
docker exec -i timescaledb psql -U postgres -d umh_v2 -t -c "SELECT name, value FROM tag_string;" >/tmp/tag_string_values.txt
grep -Fxq " stringValue | hello " /tmp/tag_string_values.txt &&
    echo "All lines are present" || echo "One or more lines are missing" && exit 1
