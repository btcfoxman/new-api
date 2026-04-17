package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func TestMergeGroupPriceFromRulesBackfillsBasePrice(t *testing.T) {
	defaultPrice := 0.006
	officialPrice := 0.0018

	groupPrices := map[string]float64{
		"default":  0,
		"official": 0,
	}
	groupRules := map[string]ratio_setting.TaskGroupPricingRule{
		"default": {
			BillingMode: "per_second",
			BasePrice:   &defaultPrice,
		},
		"official": {
			BillingMode: "per_second",
			BasePrice:   &officialPrice,
		},
	}

	merged := mergeGroupPriceFromRules(groupPrices, groupRules)

	if merged["default"] != defaultPrice {
		t.Fatalf("expected default price %v, got %v", defaultPrice, merged["default"])
	}
	if merged["official"] != officialPrice {
		t.Fatalf("expected official price %v, got %v", officialPrice, merged["official"])
	}
}

func TestMergeGroupPriceFromRulesKeepsExistingNonZeroPrice(t *testing.T) {
	basePrice := 0.62
	existingPrice := 1.5

	groupPrices := map[string]float64{
		"default": existingPrice,
	}
	groupRules := map[string]ratio_setting.TaskGroupPricingRule{
		"default": {
			BillingMode: "per_call",
			BasePrice:   &basePrice,
		},
	}

	merged := mergeGroupPriceFromRules(groupPrices, groupRules)

	if merged["default"] != existingPrice {
		t.Fatalf("expected existing price %v to be preserved, got %v", existingPrice, merged["default"])
	}
}
