package domain

type Image struct {
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
