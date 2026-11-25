package photo

const (
	previewPrefix    = "photos/previews/v1/"
	previewExtension = ".webp"
)

func PreviewKey(id string) string { return previewPrefix + id + previewExtension }

type Image struct {
	OrderKey   string `json:"order_key,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	UUID       string `json:"uuid"`
	ImageURL   string `json:"image_url"`
	PreviewURL string `json:"preview_url"`
	ObjectKey  string `json:"object_key"`
	Name       string `json:"name"`
	Country    string `json:"country"`
	City       string `json:"city"`
	YearTaken  int64  `json:"year_taken"`
	FileType   string `json:"file_type"`
	Color      string `json:"color"`
	CreatedAt  string `json:"created_at"`
}

type PhotoInput struct {
	Name      string `json:"name"`
	Country   string `json:"country"`
	City      string `json:"city"`
	YearTaken int64  `json:"year_taken"`
	Color     string `json:"color"`
}
