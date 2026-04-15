package ratio_setting

import (
	"github.com/gin-gonic/gin"
	"sync"
	"sync/atomic"
	"time"
)

const exposedDataTTL = 30 * time.Second

type exposedCache struct {
	data      gin.H
	expiresAt time.Time
}

var (
	exposedData atomic.Value
	rebuildMu   sync.Mutex
)

func InvalidateExposedDataCache() {
	exposedData.Store((*exposedCache)(nil))
}

func cloneGinH(src gin.H) gin.H {
	dst := make(gin.H, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func GetExposedData() gin.H {
	if c, ok := exposedData.Load().(*exposedCache); ok && c != nil && time.Now().Before(c.expiresAt) {
		return cloneGinH(c.data)
	}
	rebuildMu.Lock()
	defer rebuildMu.Unlock()
	if c, ok := exposedData.Load().(*exposedCache); ok && c != nil && time.Now().Before(c.expiresAt) {
		return cloneGinH(c.data)
	}
	newData := gin.H{
		"model_ratio":          GetModelRatioCopy(),
		"completion_ratio":     GetCompletionRatioCopy(),
		"cache_ratio":          GetCacheRatioCopy(),
		"create_cache_ratio":   GetCreateCacheRatioCopy(),
		"model_price":          GetModelPriceCopy(),
		"model_price_by_group": GetModelPriceByGroupCopy(),
		"group_pricing_rule":   GetTaskGroupPricingRuleCopy(),
	}
	exposedData.Store(&exposedCache{
		data:      newData,
		expiresAt: time.Now().Add(exposedDataTTL),
	})
	return cloneGinH(newData)
}
