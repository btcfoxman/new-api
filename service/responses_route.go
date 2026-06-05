package service

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

const responsesRouteCacheTTL = 72 * time.Hour

type ResponsesRouteInfo struct {
	ChannelId int    `json:"channel_id"`
	Model     string `json:"model"`
	Group     string `json:"group,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

type responsesRouteMemoryEntry struct {
	Info      ResponsesRouteInfo
	ExpiresAt time.Time
}

var responsesRouteMemoryCache = struct {
	sync.RWMutex
	items map[string]responsesRouteMemoryEntry
}{
	items: make(map[string]responsesRouteMemoryEntry),
}

func responsesRouteCacheKey(responseID string) string {
	return "new-api:responses-route:" + responseID
}

func RecordResponsesRouteInfo(responseID string, info *relaycommon.RelayInfo) {
	if responseID == "" || info == nil || info.ChannelMeta == nil || info.ChannelId <= 0 {
		return
	}
	routeInfo := ResponsesRouteInfo{
		ChannelId: info.ChannelId,
		Model:     info.OriginModelName,
		Group:     info.UsingGroup,
		CreatedAt: time.Now().Unix(),
	}
	payload, err := json.Marshal(routeInfo)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to marshal responses route info: %s", err.Error()))
		return
	}
	if common.RedisEnabled && common.RDB != nil {
		if err := common.RedisSet(responsesRouteCacheKey(responseID), string(payload), responsesRouteCacheTTL); err != nil {
			common.SysError(fmt.Sprintf("failed to cache responses route info in redis: %s", err.Error()))
		}
	}
	responsesRouteMemoryCache.Lock()
	responsesRouteMemoryCache.items[responseID] = responsesRouteMemoryEntry{
		Info:      routeInfo,
		ExpiresAt: time.Now().Add(responsesRouteCacheTTL),
	}
	responsesRouteMemoryCache.Unlock()
}

func GetResponsesRouteInfo(responseID string) (ResponsesRouteInfo, bool, error) {
	if responseID == "" {
		return ResponsesRouteInfo{}, false, nil
	}
	if common.RedisEnabled && common.RDB != nil {
		payload, err := common.RedisGet(responsesRouteCacheKey(responseID))
		if err == nil && payload != "" {
			var routeInfo ResponsesRouteInfo
			if unmarshalErr := json.Unmarshal([]byte(payload), &routeInfo); unmarshalErr != nil {
				return ResponsesRouteInfo{}, false, unmarshalErr
			}
			if routeInfo.ChannelId > 0 {
				return routeInfo, true, nil
			}
		}
	}
	now := time.Now()
	responsesRouteMemoryCache.RLock()
	entry, ok := responsesRouteMemoryCache.items[responseID]
	responsesRouteMemoryCache.RUnlock()
	if !ok {
		return ResponsesRouteInfo{}, false, nil
	}
	if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
		responsesRouteMemoryCache.Lock()
		delete(responsesRouteMemoryCache.items, responseID)
		responsesRouteMemoryCache.Unlock()
		return ResponsesRouteInfo{}, false, nil
	}
	return entry.Info, true, nil
}
