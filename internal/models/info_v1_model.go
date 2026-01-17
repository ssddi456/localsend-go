package models

// InfoV1 represents the v1 protocol info response
// Used for /api/localsend/v1/info endpoint
type InfoV1 struct {
	Alias       string `json:"alias"`
	DeviceModel string `json:"deviceModel,omitempty"`
	DeviceType  string `json:"deviceType,omitempty"`
	Version     string `json:"version,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Download    bool   `json:"download,omitempty"`
}
