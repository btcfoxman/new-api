package helper

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func installTaskPricingRules(t *testing.T, rules map[string]any) {
	t.Helper()
	original := ratio_setting.GetTaskGroupPricingRuleCopy()
	originalJSON, err := common.Marshal(original)
	require.NoError(t, err)
	raw, err := common.Marshal(rules)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByGroupByJSONString(string(raw)))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByGroupByJSONString(string(originalJSON)))
	})
}

func taskPricingInfo(model, group string, price, groupRatio float64) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: model,
		UsingGroup:      group,
		PriceData: types.PriceData{
			ModelPrice: price,
			UsePrice:   true,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: groupRatio,
			},
		},
	}
}

func TestResolveTaskGroupPricingPerCallOverridesEndpointSeconds(t *testing.T) {
	model := "test-gemini-omni-per-call"
	installTaskPricingRules(t, map[string]any{
		model: map[string]any{
			"default": map[string]any{
				"billing_mode":        "per_call",
				"base_price":          1.0,
				"ignore_other_ratios": true,
				"dimensions": map[string]any{
					"mode":  "per_call",
					"price": 1.0,
				},
			},
		},
	})

	info := taskPricingInfo(model, "default", 1, 1)
	decision := ResolveTaskGroupPricing(info, TaskPricingInput{Duration: 10}, map[string]float64{"seconds": 10, "size": 1.666667})

	require.True(t, decision.Applied)
	require.Equal(t, "per_call", decision.BillingMode)
	require.Equal(t, 1.0, decision.Price)
	require.Empty(t, decision.OtherRatios)
	ApplyTaskPricingDecision(info, decision)
	require.Equal(t, int(common.QuotaPerUnit), info.PriceData.Quota)
}

func TestResolveTaskGroupPricingPerSecondAppliesGroupRatioOnce(t *testing.T) {
	model := "test-seedance-per-second"
	installTaskPricingRules(t, map[string]any{
		model: map[string]any{
			"stable": map[string]any{
				"billing_mode": "per_second",
				"base_price":   0.6,
				"dimensions": map[string]any{
					"mode":  "per_second",
					"price": 0.6,
				},
			},
		},
	})

	info := taskPricingInfo(model, "stable", 0.6, 2)
	decision := ResolveTaskGroupPricing(info, TaskPricingInput{Duration: 10}, map[string]float64{"seconds": 10})
	ApplyTaskPricingDecision(info, decision)

	require.True(t, decision.Applied)
	require.Equal(t, map[string]float64{"seconds": 10}, decision.OtherRatios)
	require.Equal(t, int(12*common.QuotaPerUnit), info.PriceData.Quota)
}

func TestResolveTaskGroupPricingRequiresExactGroup(t *testing.T) {
	model := "test-exact-group"
	installTaskPricingRules(t, map[string]any{
		model: map[string]any{
			"default": map[string]any{
				"billing_mode": "per_call",
				"base_price":   1.0,
			},
		},
	})

	decision := ResolveTaskGroupPricing(
		taskPricingInfo(model, "official", 9, 5),
		TaskPricingInput{Duration: 10},
		map[string]float64{"seconds": 10},
	)
	require.False(t, decision.HasRule)
	require.False(t, decision.Applied)
}

func TestResolveTaskGroupPricingBasePriceOnlyRuleKeepsLegacyDimensionFallback(t *testing.T) {
	model := "test-base-price-only"
	installTaskPricingRules(t, map[string]any{
		model: map[string]any{
			"default": map[string]any{"base_price": 0.5},
		},
	})

	decision := ResolveTaskGroupPricing(
		taskPricingInfo(model, "default", 0.5, 1),
		TaskPricingInput{Duration: 10},
		map[string]float64{"seconds": 10, "size": 2},
	)
	require.True(t, decision.Applied)
	require.Empty(t, decision.BillingMode)
	require.Equal(t, map[string]float64{"seconds": 10, "size": 2}, decision.OtherRatios)
}

func TestResolveTaskGroupPricingResolutionUsesConfiguredPriceDespiteIgnoreFlag(t *testing.T) {
	model := "test-resolution-price"
	installTaskPricingRules(t, map[string]any{
		model: map[string]any{
			"default": map[string]any{
				"billing_mode":        "per_call",
				"base_price":          0.1,
				"ignore_other_ratios": true,
				"dimensions": map[string]any{
					"mode": "resolution",
					"resolution_prices": map[string]any{
						"1k": 0.1,
						"2k": 0.2,
					},
				},
			},
		},
	})

	decision := ResolveTaskGroupPricing(
		taskPricingInfo(model, "default", 0.1, 1),
		TaskPricingInput{Duration: 10, Resolution: "2048x2048-2K"},
		map[string]float64{"seconds": 10, "resolution": 3},
	)
	require.True(t, decision.Applied)
	require.True(t, decision.MatchedDimension)
	require.Equal(t, 0.2, decision.Price)
	require.Empty(t, decision.OtherRatios)
}

func TestResolveTaskGroupPricingDurationTableAndResolutionMultiplier(t *testing.T) {
	model := "test-duration-price"
	installTaskPricingRules(t, map[string]any{
		model: map[string]any{
			"default": map[string]any{
				"billing_mode": "per_second",
				"base_price":   0.01,
				"dimensions": map[string]any{
					"mode": "duration",
					"duration_prices": map[string]any{
						"5":  0.05,
						"10": 0.08,
					},
					"resolution_multiplier": map[string]any{
						"720p": 2.0,
					},
				},
			},
		},
	})

	decision := ResolveTaskGroupPricing(
		taskPricingInfo(model, "default", 0.01, 1),
		TaskPricingInput{Duration: 10, Resolution: "1280x720"},
		map[string]float64{"seconds": 10, "size": 3, "video_input": 0.8},
	)
	require.True(t, decision.MatchedDimension)
	require.Equal(t, 0.08, decision.Price)
	require.Equal(t, map[string]float64{"resolution-720p": 2, "video_input": 0.8}, decision.OtherRatios)

	info := taskPricingInfo(model, "default", 0.01, 1)
	ApplyTaskPricingDecision(info, decision)
	require.Equal(t, int(0.128*common.QuotaPerUnit), info.PriceData.Quota)
}

func TestResolveTaskGroupPricingMissingDurationTierUsesLegacyFallback(t *testing.T) {
	model := "test-duration-fallback"
	installTaskPricingRules(t, map[string]any{
		model: map[string]any{
			"default": map[string]any{
				"billing_mode": "per_second",
				"base_price":   0.01,
				"dimensions": map[string]any{
					"mode":            "duration",
					"duration_prices": map[string]any{"5": 0.05, "10": 0.08},
				},
			},
		},
	})

	decision := ResolveTaskGroupPricing(
		taskPricingInfo(model, "default", 0.01, 1),
		TaskPricingInput{Duration: 7},
		map[string]float64{"seconds": 7, "size": 2},
	)
	require.True(t, decision.Applied)
	require.False(t, decision.MatchedDimension)
	require.Equal(t, 0.01, decision.Price)
	require.Equal(t, map[string]float64{"seconds": 7, "size": 2}, decision.OtherRatios)
}

func TestResolveTaskGroupPricingSupportsLegacyDurationPoints(t *testing.T) {
	model := "test-legacy-duration-points"
	installTaskPricingRules(t, map[string]any{
		model: map[string]any{
			"default": map[string]any{
				"billing_mode": "per_second",
				"base_price":   0.01,
				"dimensions": map[string]any{
					"duration_points":  map[string]any{"5": 0.05, "10": 0.08},
					"default_duration": 5,
				},
			},
		},
	})

	decision := ResolveTaskGroupPricing(
		taskPricingInfo(model, "default", 0.01, 1),
		TaskPricingInput{Duration: 10},
		nil,
	)
	require.True(t, decision.Applied)
	require.True(t, decision.MatchedDimension)
	require.Equal(t, 0.08, decision.Price)
	require.Empty(t, decision.OtherRatios)
}

func TestResolveTaskGroupPricingMatrixSelectsConfiguredRow(t *testing.T) {
	model := "test-matrix-price"
	installTaskPricingRules(t, map[string]any{
		model: map[string]any{
			"default": map[string]any{
				"billing_mode": "per_call",
				"base_price":   0.6,
				"dimensions": map[string]any{
					"mode":        "matrix",
					"headers":     []any{"模型", "模式", "时长", "生成音频", "指定音色"},
					"rows":        []any{[]any{"kling-v3", "std", "3-15s", "是", "否"}},
					"prices":      []any{1.2},
					"price_units": []any{"per_second"},
				},
			},
		},
	})

	decision := ResolveTaskGroupPricing(
		taskPricingInfo(model, "default", 0.6, 1),
		TaskPricingInput{Model: "kling-v3", Mode: "std", Duration: 10, Sound: true},
		map[string]float64{"kling_units": 9},
	)
	require.True(t, decision.MatchedDimension)
	require.Equal(t, 1.2, decision.Price)
	require.Equal(t, map[string]float64{"seconds": 10}, decision.OtherRatios)
}

func TestResolveTaskGroupPricingKlingRoutingModeUsesStandardMatrixRow(t *testing.T) {
	model := "test-kling-routing-mode"
	installTaskPricingRules(t, map[string]any{
		model: map[string]any{
			"default": map[string]any{
				"billing_mode": "per_call",
				"base_price":   0.6,
				"dimensions": map[string]any{
					"mode":        "matrix",
					"headers":     []any{"模型", "模式", "时长", "生成音频", "指定音色"},
					"rows":        []any{[]any{"kling-v3", "std", "3-15s", "否", "否"}},
					"prices":      []any{0.6},
					"price_units": []any{"per_second"},
				},
			},
		},
	})

	decision := ResolveTaskGroupPricing(
		taskPricingInfo(model, "default", 0.6, 1),
		TaskPricingInput{Model: "kling-v3", Mode: "t2v", Duration: 10},
		nil,
	)

	require.True(t, decision.MatchedDimension)
	require.Equal(t, 0.6, decision.Price)
	require.Equal(t, map[string]float64{"seconds": 10}, decision.OtherRatios)
}

func TestResolveTaskGroupPricingMatrixUsesConfiguredBaseUnitForUnknownRow(t *testing.T) {
	model := "test-matrix-fallback"
	installTaskPricingRules(t, map[string]any{
		model: map[string]any{
			"default": map[string]any{
				"billing_mode": "per_call",
				"base_price":   0.1,
				"dimensions": map[string]any{
					"mode":            "matrix",
					"headers":         []any{"model"},
					"rows":            []any{[]any{"known"}},
					"prices":          []any{0.1},
					"unit_prices":     []any{1.0},
					"price_units":     []any{"per_call"},
					"base_unit_price": 9.9,
				},
			},
		},
	})

	decision := ResolveTaskGroupPricing(
		taskPricingInfo(model, "default", 0.1, 1),
		TaskPricingInput{Model: "unknown"},
		map[string]float64{"kling_units": 5},
	)
	require.True(t, decision.MatchedDimension)
	require.Equal(t, 0.5, decision.Price)
	require.Empty(t, decision.OtherRatios)
}

func TestResolveImageGroupPriceUsesExactGroupAndResolution(t *testing.T) {
	model := "test-image-resolution"
	installTaskPricingRules(t, map[string]any{
		model: map[string]any{
			"default": map[string]any{
				"billing_mode": "per_call",
				"base_price":   0.1,
				"dimensions": map[string]any{
					"mode":              "resolution",
					"resolution_prices": map[string]any{"1k": 0.1, "4k": 0.4},
				},
			},
		},
	})

	info := taskPricingInfo(model, "default", 0.1, 3)
	price, ok := ResolveImageGroupPrice(info, "4096x4096 (4K)")
	require.True(t, ok)
	require.Equal(t, 0.4, price)
	ApplyResolvedGroupPrice(info, price)
	require.Equal(t, int(1.2*common.QuotaPerUnit), info.PriceData.QuotaToPreConsume)

	_, ok = ResolveImageGroupPrice(taskPricingInfo(model, "official", 0.1, 5), "4k")
	require.False(t, ok, fmt.Sprintf("pricing must not borrow another group's dimensions"))
}
