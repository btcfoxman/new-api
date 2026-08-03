package controller

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestApplyConfiguredImageGroupPriceUsesModelResolutionSuffix(t *testing.T) {
	modelName := "test-image-model-4k"
	original := ratio_setting.GetTaskGroupPricingRuleCopy()
	originalJSON, err := common.Marshal(original)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByGroupByJSONString(string(originalJSON)))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByGroupByJSONString(`{
		"test-image-model-4k": {
			"default": {
				"billing_mode": "per_call",
				"base_price": 0.1,
				"dimensions": {
					"mode": "resolution",
					"resolution_prices": {"1k": 0.1, "4k": 0.4}
				}
			}
		}
	}`))

	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		UsingGroup:      "default",
		PriceData: types.PriceData{
			ModelPrice: 0.1,
			UsePrice:   true,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 2,
			},
		},
	}
	request := &dto.ImageRequest{
		Model:      modelName,
		Resolution: json.RawMessage(`""`),
	}

	applyConfiguredImageGroupPrice(info, request)

	require.Equal(t, 0.4, info.PriceData.ModelPrice)
	require.Equal(t, int(0.8*common.QuotaPerUnit), info.PriceData.QuotaToPreConsume)
}
