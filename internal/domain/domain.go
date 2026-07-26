// Package domain holds the shared data structures used across store, inventory, and api.
package domain

import "time"

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Enabled      bool
	CreatedAt    time.Time
}

type Location struct {
	ID        int64
	Code      string // "@" + 3 uppercase-alpha chars
	CreatedAt time.Time
	CreatedBy int64
}

type Item struct {
	ID          int64
	AssetTag    string // 4-char uppercase alpha
	Description string // "" if not yet generated
	LocationID  *int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Image struct {
	ID          int64
	ItemID      int64
	Data        []byte
	ContentType string
	Description string
	SortOrder   int
	CreatedAt   time.Time
	CreatedBy   int64
}

// Activity actions. Kept as typed constants so callers can't typo a raw string.
type ActivityAction string

const (
	ActivityItemCreated             ActivityAction = "item_created"
	ActivityImageAdded              ActivityAction = "image_added"
	ActivityItemMoved               ActivityAction = "item_moved"
	ActivityItemRemovedFromLocation ActivityAction = "item_removed_from_location"
	ActivityLocationReconciled      ActivityAction = "location_reconciled"
	ActivityItemDeleted             ActivityAction = "item_deleted"
	ActivityDescriptionRegenerated  ActivityAction = "description_regenerated"
	ActivityDuplicateGroupDismissed ActivityAction = "duplicate_group_dismissed"
	ActivityItemsMerged             ActivityAction = "items_merged"
)

type Activity struct {
	ID         int64
	UserID     int64
	Username   string // populated on read via join, for display
	Action     ActivityAction
	ItemID     *int64
	LocationID *int64
	Detail     string
	CreatedAt  time.Time
}

type DuplicateRunStatus string

const (
	DuplicateRunCompleted DuplicateRunStatus = "completed"
	DuplicateRunFailed    DuplicateRunStatus = "failed"
)

type DuplicateRun struct {
	ID          int64
	Status      DuplicateRunStatus
	StartedBy   int64
	StartedAt   time.Time
	CompletedAt time.Time
}

type DuplicateGroupStatus string

const (
	DuplicateGroupPending   DuplicateGroupStatus = "pending"
	DuplicateGroupResolved  DuplicateGroupStatus = "resolved"
	DuplicateGroupDismissed DuplicateGroupStatus = "dismissed"
)

type DuplicateGroup struct {
	ID                 int64
	RunID              int64
	Status             DuplicateGroupStatus
	Reasoning          string
	ItemIDs            []int64 // populated on read via duplicate_group_items join
	ResolvedItemID     *int64
	ResolvedLocationID *int64
	ResolvedBy         *int64
	ResolvedAt         *time.Time
	CreatedAt          time.Time
}
