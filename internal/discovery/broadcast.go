package discovery

import (
	"time"

	"github.com/meowrain/localsend-go/internal/config"
	"github.com/meowrain/localsend-go/internal/utils/logger"

	"github.com/meowrain/localsend-go/internal/models"
)

const (
	multicastIP  = "224.0.0.167"
	ServerPort   = 53317
	httpTimeout  = 2 * time.Second
	scanInterval = 2 * time.Second
	deviceTTL    = 200 * time.Second // 设备的生存时间
)

func ListenAndStartBroadcasts(updates chan<- []models.SendModel) {
	logger.Info("Listening for broadcasts...")
	go ListenForUDPBroadcasts(updates)
	if config.ConfigData.Functions.PingScan {
		logger.Info("Ping scan enabled, starting HTTP broadcast listener...")
		go ListenForHttpBroadCast(updates)
	}
	logger.Info("Start broadcasts...")
	go StartUDPBroadcast()
}
