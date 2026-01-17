package discovery

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/meowrain/localsend-go/internal/config"
	"github.com/meowrain/localsend-go/internal/models"
	"github.com/meowrain/localsend-go/internal/utils/logger"
)

func ListenForHttpBroadCast(updates chan<- []models.SendModel) {
	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	for range ticker.C {
		data, err := json.Marshal(Message)
		if err != nil {
			logger.Errorf("Failed to marshal message: %v", err)
			continue
		}

		ips, err := pingScan()
		if err != nil {
			logger.Errorf("Failed to discover devices via ping scan: %v", err)
			continue
		}

		var wg sync.WaitGroup
		for _, ip := range ips {
			wg.Add(1)
			go func(ip string) {
				defer wg.Done()

				// 尝试 v3 和 v2 版本的 API
				var resp *http.Response
				var err error
				var successURL string

				client := &http.Client{
					Timeout: httpTimeout,
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					},
				}

				// 尝试 v3/register
				urlV3 := fmt.Sprintf("https://%s:%d/api/localsend/v3/register", ip, config.ServerPort)
				resp, err = makeRequest(client, urlV3, data)
				if err == nil && resp.StatusCode == http.StatusOK {
					successURL = urlV3
				} else {
					// 回退到 v2/register
					urlV2 := fmt.Sprintf("https://%s:%d/api/localsend/v2/register", ip, config.ServerPort)
					resp, err = makeRequest(client, urlV2, data)
					if err == nil && resp.StatusCode == http.StatusOK {
						successURL = urlV2
					}
				}

				// 两个版本都失败
				if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
					logger.Errorf("Failed to reach %s via both v3 and v2 API versions: %v", ip, err)
					return
				}
				defer resp.Body.Close()

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					logger.Errorf("Failed to read HTTP response body from %s: %v", ip, err)
					return
				}
				var response models.BroadcastMessage
				if err := json.Unmarshal(body, &response); err != nil {
					logger.Infof("Request URL: %s", successURL)
					logger.Infof("Request Body: %s", string(body))
					logger.Errorf("Failed to parse HTTP response from %s: %v", ip, err)
					return
				}

				response.LastSeen = time.Now()

				DevicesMutex.Lock()
				DiscoveredDevices[ip] = response
				DevicesMutex.Unlock()
			}(ip)
		}

		wg.Wait()

		DevicesMutex.RLock()
		devices := make([]models.SendModel, 0, len(DiscoveredDevices))
		for ip, device := range DiscoveredDevices {
			devices = append(devices, models.SendModel{
				IP:         ip,
				DeviceName: device.Alias,
			})
		}
		DevicesMutex.RUnlock()

		select {
		case updates <- devices:
		default:
			logger.Debug("Updates channel is full, skipping update")
		}
	}
}

// makeRequest 发送 POST 请求
func makeRequest(client *http.Client, url string, data []byte) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return client.Do(req)
}
