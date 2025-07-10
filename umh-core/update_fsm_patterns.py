#!/usr/bin/env python3

import os
import re
import glob

def update_reconcile_external_changes_pattern(content, instance_var):
    """Update reconcileExternalChanges pattern"""
    pattern = r'if errors\.Is\(err, context\.DeadlineExceeded\) \{\s*// Context deadline exceeded should be retried with backoff, not ignored\s*' + instance_var + r'\.baseFSMInstance\.SetError\(err, snapshot\.Tick\)\s*' + instance_var + r'\.baseFSMInstance\.GetLogger\(\)\.Warnf\("Context deadline exceeded in reconcileExternalChanges, will retry with backoff"\)\s*err = nil // Clear error so reconciliation continues\s*return nil, false\s*\}'
    
    replacement = f'if err, shouldContinue := {instance_var}.baseFSMInstance.HandleDeadlineExceeded(err, snapshot.Tick, "reconcileExternalChanges"); !shouldContinue {{\n\t\t\treturn err, false\n\t\t}}'
    
    return re.sub(pattern, replacement, content, flags=re.MULTILINE | re.DOTALL)

def update_reconcile_state_transition_pattern(content, instance_var):
    """Update reconcileStateTransition pattern"""
    pattern = r'if errors\.Is\(err, context\.DeadlineExceeded\) \{\s*// Context deadline exceeded should be retried with backoff, not ignored\s*' + instance_var + r'\.baseFSMInstance\.SetError\(err, snapshot\.Tick\)\s*' + instance_var + r'\.baseFSMInstance\.GetLogger\(\)\.Warnf\("Context deadline exceeded in reconcileStateTransition, will retry with backoff"\)\s*return nil, false\s*\}'
    
    replacement = f'if err, shouldContinue := {instance_var}.baseFSMInstance.HandleDeadlineExceeded(err, snapshot.Tick, "reconcileStateTransition"); !shouldContinue {{\n\t\t\treturn err, false\n\t\t}}'
    
    return re.sub(pattern, replacement, content, flags=re.MULTILINE | re.DOTALL)

def update_manager_reconciliation_pattern(content, instance_var, manager_name, error_var):
    """Update manager reconciliation patterns (like s6Manager, nmapManager, etc)"""
    pattern = r'if errors\.Is\(' + error_var + r', context\.DeadlineExceeded\) \{\s*// Context deadline exceeded should be retried with backoff, not ignored\s*' + instance_var + r'\.baseFSMInstance\.SetError\(' + error_var + r', snapshot\.Tick\)\s*' + instance_var + r'\.baseFSMInstance\.GetLogger\(\)\.Warnf\("Context deadline exceeded in ' + manager_name + r' reconciliation, will retry with backoff"\)\s*(.*?)return nil, false\s*\}'
    
    replacement = f'if {error_var}, shouldContinue := {instance_var}.baseFSMInstance.HandleDeadlineExceeded({error_var}, snapshot.Tick, "{manager_name} reconciliation"); !shouldContinue {{\n\t\t\treturn {error_var}, false\n\t\t}}'
    
    return re.sub(pattern, replacement, content, flags=re.MULTILINE | re.DOTALL)

def update_update_observed_state_pattern(content, instance_var):
    """Update UpdateObservedStateOfInstance pattern"""
    pattern = r'if ctx\.Err\(\) != nil \{\s*if errors\.Is\(ctx\.Err\(\), context\.DeadlineExceeded\) \{\s*// Context deadline exceeded should be retried with backoff, not ignored\s*' + instance_var + r'\.baseFSMInstance\.SetError\(ctx\.Err\(\), snapshot\.Tick\)\s*' + instance_var + r'\.baseFSMInstance\.GetLogger\(\)\.Warnf\("Context deadline exceeded in UpdateObservedStateOfInstance, will retry with backoff"\)\s*return nil\s*\}\s*return ctx\.Err\(\)\s*\}'
    
    replacement = f'if ctx.Err() != nil {{\n\t\tif err, shouldContinue := {instance_var}.baseFSMInstance.HandleDeadlineExceeded(ctx.Err(), snapshot.Tick, "UpdateObservedStateOfInstance"); !shouldContinue {{\n\t\t\treturn err\n\t\t}}\n\t\treturn ctx.Err()\n\t}}'
    
    return re.sub(pattern, replacement, content, flags=re.MULTILINE | re.DOTALL)

# Define FSM files and their instance variables
fsm_files = {
    'pkg/fsm/s6/reconcile.go': 's',
    'pkg/fsm/container/reconcile.go': 'c', 
    'pkg/fsm/nmap/reconcile.go': 'n',
    'pkg/fsm/redpanda/reconcile.go': 'r',
    'pkg/fsm/agent_monitor/reconcile.go': 'a',
    'pkg/fsm/redpanda_monitor/reconcile.go': 'b',
    'pkg/fsm/dataflowcomponent/reconcile.go': 'd',
    'pkg/fsm/benthos_monitor/reconcile.go': 'b',
    'pkg/fsm/benthos/reconcile.go': 'b',
    'pkg/fsm/topicbrowser/reconcile.go': 'i',
    'pkg/fsm/protocolconverter/reconcile.go': 'p'
}

# Update each file
for file_path, instance_var in fsm_files.items():
    if os.path.exists(file_path):
        print(f"Updating {file_path}...")
        
        with open(file_path, 'r') as f:
            content = f.read()
        
        # Apply all the pattern updates
        content = update_reconcile_external_changes_pattern(content, instance_var)
        content = update_reconcile_state_transition_pattern(content, instance_var)
        
        # Manager-specific patterns
        if 's6' in file_path or 'benthos' in file_path or 'redpanda' in file_path:
            content = update_manager_reconciliation_pattern(content, instance_var, 's6Manager', 's6Err')
        if 'nmap' in file_path:
            content = update_manager_reconciliation_pattern(content, instance_var, 'monitorService', 's6Err')
        if 'connection' in file_path:
            content = update_manager_reconciliation_pattern(content, instance_var, 'nmapManager', 'nmapErr')
        if 'dataflow' in file_path:
            content = update_manager_reconciliation_pattern(content, instance_var, 'benthosManager', 'benthosErr')
        if 'protocol' in file_path or 'topic' in file_path:
            content = update_manager_reconciliation_pattern(content, instance_var, 'manager', 'managerErr')
        
        with open(file_path, 'w') as f:
            f.write(content)

print("Update complete!") 