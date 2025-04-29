Please note that the actions.go needs to be adjusted to the new error handling, specifically the ignoring of connectionservice.ErrRemovalPending. See also benthos/actions.go for the same issue.

The same goes for the reconcile.go file, where the same error is ignored.