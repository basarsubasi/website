package domain

type Image struct {
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	UUID      string `json:"uuid"`
	ImageURL  string `json:"image_url"`
	ObjectKey string `json:"object_key"`
	Name      string `json:"name"`
	Country   string `json:"country"`
	City      string `json:"city"`
	YearTaken int64  `json:"year_taken"`
	FileType  string `json:"file_type"`
	Color     string `json:"color"`
	CreatedAt string `json:"created_at"`
}

type PhotoInput struct {
	Name      string `json:"name"`
	Country   string `json:"country"`
	City      string `json:"city"`
	YearTaken int64  `json:"year_taken"`
	Color     string `json:"color"`
}

// BlogImage describes an uploaded original without changing its bytes.
type BlogImage struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
