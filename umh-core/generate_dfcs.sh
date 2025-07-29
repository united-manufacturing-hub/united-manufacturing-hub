#!/bin/bash

# Script to generate a specified number of DFCs in config.yaml
# Usage: ./generate_dfcs.sh [number_of_dfcs]

CONFIG_FILE="data/config.yaml"

# Function to display usage
usage() {
    echo "Usage: $0 [number_of_dfcs]"
    echo "Example: $0 25"
    echo "If no number is provided, you'll be prompted to enter one."
    exit 1
}

# Function to generate DFC YAML
generate_dfc() {
    local dfc_num=$1
    local dfc_name="hello-world-dfc"
    
    # For the first DFC, don't add a number suffix
    if [ $dfc_num -gt 1 ]; then
        dfc_name="${dfc_name}${dfc_num}"
    fi
    
    cat << EOF
    - name: $dfc_name
      desiredState: active
      dataFlowComponentConfig:
        benthos:
            input:
                generate:
                    count: 0
                    interval: 1s
                    mapping: root = "hello world from DFC ${dfc_num}!"
            pipeline:
                processors:
                    - bloblang: root = content()
            output:
                kafka:
                    addresses:
                        - localhost:9092
                    topic: messages
EOF
}

# Get number of DFCs
if [ $# -eq 0 ]; then
    read -p "Enter number of DFCs to generate: " NUM_DFCS
elif [ $# -eq 1 ]; then
    NUM_DFCS=$1
else
    usage
fi

# Validate input
if ! [[ "$NUM_DFCS" =~ ^[0-9]+$ ]] || [ "$NUM_DFCS" -lt 1 ]; then
    echo "Error: Please provide a valid positive number"
    exit 1
fi

# Check if config file exists
if [ ! -f "$CONFIG_FILE" ]; then
    echo "Error: $CONFIG_FILE not found"
    exit 1
fi

echo "Generating $NUM_DFCS DFCs in $CONFIG_FILE..."

# Create backup
cp "$CONFIG_FILE" "${CONFIG_FILE}.backup"
echo "Backup created: ${CONFIG_FILE}.backup"

# Create the agent section with proper config
cat << 'EOF' > temp_config.yaml
agent:
    metricsPort: 8080
    communicator:
        apiUrl: https://management.umh.app/api
        authToken: 7d4c499b04050cf9502f7dca277df4fa813749b3c43dae059c7021627db95f46
    releaseChannel: stable
    location:
        0: test-enterprise
        1: test-site
        2: test-area
        3: test-line
dataFlow:
EOF

# Generate all DFCs
for i in $(seq 1 $NUM_DFCS); do
    generate_dfc $i >> temp_config.yaml
done

# Extract everything after the dataFlow section (internal: section onwards)
sed -n '/^internal:/,$p' "$CONFIG_FILE" >> temp_config.yaml

# Replace original config
mv temp_config.yaml "$CONFIG_FILE"

echo "Successfully generated $NUM_DFCS DFCs in $CONFIG_FILE"
echo "The DFCs are named: hello-world-dfc, hello-world-dfc2, hello-world-dfc3, ..., hello-world-dfc$NUM_DFCS" 