package article

// BlogImage describes an uploaded original without changing its bytes.
type BlogImage struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
