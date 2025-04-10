package dataflowcomponent

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// CreateInstance is called when the FSM transitions from to_be_created -> creating.
// For container monitoring, this is a no-op as there's no actual container to create.
// This function is present for structural consistency with other FSM packages.
func (d *DataflowComponentInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	d.baseFSMInstance.GetLogger().Debugf("Creating dataflow component instance %s (no-op)", d.baseFSMInstance.GetID())
	return nil
}

// RemoveInstance is called when the FSM transitions to removing.
// For container monitoring, this is a no-op as we don't need to remove any resources.
// This function is present for structural consistency with other FSM packages.
func (d *DataflowComponentInstance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	d.baseFSMInstance.GetLogger().Debugf("Removing dataflow component instance %s (no-op)", d.baseFSMInstance.GetID())
	return nil
}

// optionally, we might have something like "enableMonitoring" / "disableMonitoring" if
// you want actual side effects. For now, do no-ops or just logs.

// StartInstance is called when the container monitoring should be enabled.
// Currently this is a no-op as the monitoring service runs independently.
func (d *DataflowComponentInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	d.baseFSMInstance.GetLogger().Infof("Enabling monitoring for %s (no-op)", d.baseFSMInstance.GetID())
	return nil
}

// StopInstance is called when the container monitoring should be disabled.
// Currently this is a no-op as the monitoring service runs independently.
func (d *DataflowComponentInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	d.baseFSMInstance.GetLogger().Infof("Disabling monitoring for %s (no-op)", d.baseFSMInstance.GetID())
	return nil
}

// UpdateObservedStateOfInstance is called when the FSM transitions to updating.
// For container monitoring, this is a no-op as we don't need to update any resources.
func (d *DataflowComponentInstance) UpdateObservedStateOfInstance(ctx context.Context, filesystemService filesystem.Service, tick uint64) error {
	d.baseFSMInstance.GetLogger().Debugf("Updating observed state for %s (no-op)", d.baseFSMInstance.GetID())
	return nil
}
