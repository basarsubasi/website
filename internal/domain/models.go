package domain

type Image struct {
	Uuid      string `json:"uuid"`
	ImageUrl  string `json:"image_url"`
	ObjectKey string `json:"object_key"`
	Name      string `json:"name"`
	Country   string `json:"country"`
	City      string `json:"city"`
	YearTaken int64  `json:"year_taken"`
	FileType  string `json:"file_type"`
	Color     string `json:"color"`
	CreatedAt string `json:"created_at"`
}

type Post struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Excerpt   string `json:"excerpt"`
	Body      string `json:"body"`
	Published int64  `json:"published"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type PhotoInput struct {
	Name      string `json:"name"`
	Country   string `json:"country"`
	City      string `json:"city"`
	YearTaken int64  `json:"year_taken"`
	Color     string `json:"color"`
}
type PostInput struct {
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Excerpt   string `json:"excerpt"`
	Body      string `json:"body"`
	Published int64  `json:"published"`
}
type Pagination struct{ Page, Limit int64 }
type Page[T any] struct {
	Data  []T   `json:"data"`
	Total int64 `json:"total"`
	Page  int64 `json:"page"`
	Limit int64 `json:"limit"`
}

// Snapshot is the metadata and post content restored from an R2 backup.
type Snapshot struct {
	Images []Image
	Posts  []Post
}
