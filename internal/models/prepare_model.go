package models

// PrepareReceiveRequest represents the prepare-upload request (v2)
type PrepareReceiveRequest struct {
	Info  BroadcastMessage    `json:"info"`
	Files map[string]FileInfo `json:"files"`
}

// PrepareReceiveResponse represents the prepare-upload response
type PrepareReceiveResponse struct {
	SessionID string            `json:"sessionId"`
	Files     map[string]string `json:"files"` // File ID to Token map
}

// PrepareUploadRequest is an alias for backward compatibility
type PrepareUploadRequest = PrepareReceiveRequest
