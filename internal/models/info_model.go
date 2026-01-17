package models

// InfoV2 represents the v2 protocol info response
// Used for /api/localsend/v2/info endpoint
type InfoV2 struct {
	Alias       string `json:"alias"`
	Version     string `json:"version"`
	DeviceModel string `json:"deviceModel,omitempty"`
	DeviceType  string `json:"deviceType,omitempty"`
	Fingerprint string `json:"fingerprint"`
	Download    bool   `json:"download"`
}

// Info is an alias for backward compatibility
type Info = InfoV2
