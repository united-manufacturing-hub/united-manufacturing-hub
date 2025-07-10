#!/bin/bash

# Script to update all FSM files to use the new HandleDeadlineExceeded method

echo "Updating FSM files to use HandleDeadlineExceeded method..."

# Function to update start-of-reconciliation pattern
update_start_pattern() {
    local file=$1
    local instance_var=$2
    
    sed -i 's/if errors\.Is(ctx\.Err(), context\.DeadlineExceeded) {/if err, shouldContinue := '${instance_var}'.baseFSMInstance.HandleDeadlineExceeded(ctx.Err(), snapshot.Tick, "start of reconciliation"); !shouldContinue {/g' "$file"
    sed -i '/Context deadline exceeded should be retried with backoff, not ignored/d' "$file"
    sed -i '/'"${instance_var}"'\.baseFSMInstance\.SetError(ctx\.Err(), snapshot\.Tick)/d' "$file"
    sed -i '/'"${instance_var}"'\.baseFSMInstance\.GetLogger()\.Warnf("Context deadline exceeded at start of reconciliation, will retry with backoff")/d' "$file"
    sed -i 's/return nil, false/return err, false/g' "$file"
}

# Function to update reconcileExternalChanges pattern
update_external_changes_pattern() {
    local file=$1
    local instance_var=$2
    
    sed -i 's/if errors\.Is(err, context\.DeadlineExceeded) {/if err, shouldContinue := '${instance_var}'.baseFSMInstance.HandleDeadlineExceeded(err, snapshot.Tick, "reconcileExternalChanges"); !shouldContinue {/g' "$file"
}

# Function to update reconcileStateTransition pattern
update_state_transition_pattern() {
    local file=$1
    local instance_var=$2
    
    sed -i 's/if errors\.Is(err, context\.DeadlineExceeded) {/if err, shouldContinue := '${instance_var}'.baseFSMInstance.HandleDeadlineExceeded(err, snapshot.Tick, "reconcileStateTransition"); !shouldContinue {/g' "$file"
}

# Update all reconcile.go files
echo "Updating reconcile.go files..."

# Update remaining files with proper instance variable names
files=(
    "pkg/fsm/redpanda/reconcile.go:r"
    "pkg/fsm/agent_monitor/reconcile.go:a"
    "pkg/fsm/redpanda_monitor/reconcile.go:b"
    "pkg/fsm/dataflowcomponent/reconcile.go:d"
    "pkg/fsm/benthos_monitor/reconcile.go:b"
    "pkg/fsm/benthos/reconcile.go:b"
    "pkg/fsm/topicbrowser/reconcile.go:i"
    "pkg/fsm/protocolconverter/reconcile.go:p"
)

for file_info in "${files[@]}"; do
    file=$(echo $file_info | cut -d':' -f1)
    instance_var=$(echo $file_info | cut -d':' -f2)
    echo "Processing $file with instance variable $instance_var"
    update_start_pattern "$file" "$instance_var"
done

echo "Done!" 