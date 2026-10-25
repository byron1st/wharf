package service

// SyncResult is the outcome of a sync that stored the tasks it received.
type SyncResult struct {
	// Upserted is the number of tasks stored.
	Upserted int
	// Partial joins the errors of items that failed to load or convert. Items
	// that failed to convert are still stored. It is nil when every item synced.
	Partial error
}
