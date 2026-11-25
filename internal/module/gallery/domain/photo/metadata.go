package photo

import (
	"encoding/base64"
	"encoding/json"

	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	blackAndWhite          = "bw"
	colorPhoto             = "color"
	invalidColorMessage    = "color must be bw or color"
	invalidYearMessage     = "year_taken must be between 0 and 9999"
	metadataTooLongMessage = "Photo metadata is too long (maximum 1900 encoded bytes)"
	metadataKey            = "photo"
)

func PhotoMetadata(p PhotoInput, id string) (map[string]string, error) {
	if p.Color != blackAndWhite && p.Color != colorPhoto {
		return nil, domain.Invalid(invalidColorMessage)
	}
	if p.YearTaken < 0 || p.YearTaken > 9999 {
		return nil, domain.Invalid(invalidYearMessage)
	}
	raw, _ := json.Marshal(struct {
		UUID string `json:"uuid"`
		PhotoInput
	}{id, p})
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if len(encoded) > 1900 {
		return nil, domain.Invalid(metadataTooLongMessage)
	}
	return map[string]string{metadataKey: encoded}, nil
}
