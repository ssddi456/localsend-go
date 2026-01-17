package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/meowrain/localsend-go/internal/discovery"
	"github.com/meowrain/localsend-go/internal/models"
	"github.com/meowrain/localsend-go/internal/utils/logger"
)

// GetInfoV1Handler handles GET /api/localsend/v1/info
// Returns device info in v1 protocol format (without version, fingerprint, port, protocol, download)
func GetInfoV1Handler(w http.ResponseWriter, r *http.Request) {
	// V1 protocol only returns: alias, deviceModel, deviceType
	msg := discovery.Message
	infoV1 := models.InfoV2{
		Alias:       msg.Alias,
		DeviceModel: msg.DeviceModel,
		DeviceType:  msg.DeviceType,
		Version:     msg.Version,
		Fingerprint: msg.Fingerprint,
		Download:    msg.Download,
	}

	res, err := json.Marshal(infoV1)
	if err != nil {
		logger.Errorf("JSON conversion failed: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	logger.Debugf("V1 Info response: %s", string(res))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err = w.Write(res); err != nil {
		logger.Errorf("Error writing response: %v", err)
	}
}

// GetInfoV2Handler handles GET /api/localsend/v2/info
// Returns device info in v2 protocol format (with version, fingerprint, download)
func GetInfoV2Handler(w http.ResponseWriter, r *http.Request) {
	msg := discovery.Message

	// V2 protocol returns full info except port and protocol
	infoV2 := models.InfoV2{
		Alias:       msg.Alias,
		Version:     msg.Version,
		DeviceModel: msg.DeviceModel,
		DeviceType:  msg.DeviceType,
		Fingerprint: msg.Fingerprint,
		Download:    msg.Download,
	}

	res, err := json.Marshal(infoV2)
	if err != nil {
		logger.Errorf("JSON conversion failed: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	logger.Debugf("V2 Info response: %s", string(res))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err = w.Write(res); err != nil {
		logger.Errorf("Error writing response: %v", err)
	}
}
