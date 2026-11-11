package domain

// Snapshot is the metadata and post content restored from an R2 backup.
type Snapshot struct {
	Images []Image
	Posts  []Post
}
