package fsm

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/snapshot"
)

func (b *BaseFSMInstance) Reconcile(ctx context.Context, currentSnapshot snapshot.SystemSnapshot, filesystemService filesystem.Service) (error, bool) {
	panic("not implemented")
}
