package helper

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// TaskPricingInput contains normalized request facts used to select a configured
// pricing sub-dimension. Adaptors may help normalize these facts, but configured
// prices always remain the source of truth.
type TaskPricingInput struct {
	Duration   float64
	Resolution string
	Model      string
	Mode       string
	Sound      bool
	Voice      bool
}

// TaskPricingDecision is the authoritative pricing result for an exact
// model+group rule. Price is persisted without GroupRatio; GroupRatio is applied
// exactly once by ApplyTaskPricingDecision.
type TaskPricingDecision struct {
	HasRule          bool
	Applied          bool
	MatchedDimension bool
	BillingMode      string
	Price            float64
	OtherRatios      map[string]float64
}

// ResolveTaskGroupPricing resolves an exact model+group TaskGroupPricingRule.
// When no exact rule is available it returns Applied=false, allowing callers to
// preserve the historical adaptor/default-ratio behavior.
func ResolveTaskGroupPricing(info *relaycommon.RelayInfo, input TaskPricingInput, legacyRatios map[string]float64) TaskPricingDecision {
	decision := TaskPricingDecision{}
	if info == nil {
		return decision
	}

	rule, ok := ratio_setting.GetTaskGroupPricingRule(info.OriginModelName, info.UsingGroup)
	if !ok {
		return decision
	}
	decision.HasRule = true

	basePrice := info.PriceData.ModelPrice
	if rule.BasePrice != nil {
		basePrice = *rule.BasePrice
	}
	if basePrice < 0 || !isFiniteTaskPrice(basePrice) {
		return decision
	}

	billingMode := normalizeTaskBillingMode(rule.BillingMode, rule.Dimensions)
	if billingMode == "per_token" {
		return decision
	}

	decision.Applied = true
	decision.BillingMode = billingMode
	decision.Price = basePrice
	decision.OtherRatios = map[string]float64{}

	dimensionMode := effectiveTaskDimensionMode(rule.Dimensions)
	switch dimensionMode {
	case "flat", "per_call":
		if price, ok := taskDimensionPrice(rule.Dimensions["price"]); ok {
			decision.Price = price
			decision.MatchedDimension = true
		}
		// An explicit per-call rule must never inherit duration/size ratios.
		return decision

	case "per_second":
		if price, ok := taskDimensionPrice(rule.Dimensions["price"]); ok {
			decision.Price = price
			decision.MatchedDimension = true
		}
		decision.OtherRatios = configuredExtraRatios(legacyRatios, rule.IgnoreOtherRatios, ratioKindDuration)
		if duration := taskPricingDuration(input, legacyRatios); duration > 0 {
			decision.OtherRatios["seconds"] = duration
		}
		return decision

	case "resolution":
		price, matched := taskResolutionPrice(rule.Dimensions, input.Resolution)
		if matched {
			decision.Price = price
			decision.MatchedDimension = true
			decision.OtherRatios = configuredExtraRatios(
				legacyRatios,
				rule.IgnoreOtherRatios,
				ratioKindResolution,
				ratioKindDuration,
			)
			if billingMode == "per_second" {
				if duration := taskPricingDuration(input, legacyRatios); duration > 0 {
					decision.OtherRatios["seconds"] = duration
				}
			}
			return decision
		}

		// The requested resolution is not configured: use only the historical
		// resolution fallback, plus duration when this is a per-second model.
		decision.OtherRatios = legacyRatiosByKinds(legacyRatios, ratioKindResolution)
		if billingMode == "per_second" {
			mergeTaskRatios(decision.OtherRatios, legacyRatiosByKinds(legacyRatios, ratioKindDuration))
			if duration := taskPricingDuration(input, legacyRatios); duration > 0 {
				decision.OtherRatios["seconds"] = duration
			}
		}
		return decision

	case "duration":
		price, matched := taskDurationPrice(rule.Dimensions, input.Duration, legacyRatios)
		if matched {
			decision.Price = price
			decision.MatchedDimension = true
			omitKinds := []taskRatioKind{ratioKindDuration}
			multiplier, multiplierMatched := taskResolutionMultiplier(rule.Dimensions, input.Resolution)
			if multiplierMatched {
				omitKinds = append(omitKinds, ratioKindResolution)
			}
			decision.OtherRatios = configuredExtraRatios(legacyRatios, rule.IgnoreOtherRatios, omitKinds...)
			if multiplierMatched {
				decision.OtherRatios["resolution-"+normalizePricingResolution(input.Resolution)] = multiplier
			} else if len(taskDimensionMap(rule.Dimensions, "resolution_multiplier")) > 0 {
				// The resolution axis exists but this value is not configured, so
				// retain only the legacy resolution fallback for that missing value.
				mergeTaskRatios(decision.OtherRatios, legacyRatiosByKinds(legacyRatios, ratioKindResolution))
			}
			return decision
		}

		// A missing duration tier falls back to the historical per-second path.
		decision.OtherRatios = clonePositiveTaskRatios(legacyRatios)
		if duration := taskPricingDuration(input, legacyRatios); duration > 0 {
			decision.OtherRatios["seconds"] = duration
		}
		return decision

	case "matrix":
		if price, unit, matched := taskMatrixPrice(rule.Dimensions, input); matched {
			decision.Price = price
			decision.MatchedDimension = true
			if unit == "per_second" {
				if duration := taskPricingDuration(input, legacyRatios); duration > 0 {
					decision.OtherRatios["seconds"] = duration
				}
			}
			return decision
		}
		if price, matched := taskMatrixLegacyPrice(rule.Dimensions, legacyRatios); matched {
			decision.Price = price
			decision.MatchedDimension = true
			return decision
		}

		// Unknown matrix combination: retain the adaptor calculation as the
		// documented compatibility fallback.
		decision.OtherRatios = clonePositiveTaskRatios(legacyRatios)
		return decision
	}

	// A structured rule without a special dimension still has an authoritative
	// billing mode. Per-call ignores request multipliers; per-second consumes the
	// normalized duration and preserves unrelated legacy dimensions.
	switch billingMode {
	case "per_call":
		return decision
	case "per_second":
		decision.OtherRatios = configuredExtraRatios(legacyRatios, rule.IgnoreOtherRatios, ratioKindDuration)
		if duration := taskPricingDuration(input, legacyRatios); duration > 0 {
			decision.OtherRatios["seconds"] = duration
		}
	case "":
		// A base_price-only object has no configured billing mode/dimension.
		// Preserve legacy behavior unless ignore_other_ratios was explicit.
		if !rule.IgnoreOtherRatios {
			decision.OtherRatios = clonePositiveTaskRatios(legacyRatios)
		}
	default:
		decision.Applied = false
	}
	return decision
}

// ApplyTaskPricingDecision updates PriceData and calculates quota in one pass,
// avoiding order-dependent intermediate integer truncation.
func ApplyTaskPricingDecision(info *relaycommon.RelayInfo, decision TaskPricingDecision) {
	if info == nil || !decision.Applied {
		return
	}
	ratios := clonePositiveTaskRatios(decision.OtherRatios)
	multiplier := 1.0
	for _, ratio := range ratios {
		multiplier *= ratio
	}
	groupRatio := info.PriceData.GroupRatioInfo.GroupRatio
	quota := int(decision.Price * common.QuotaPerUnit * groupRatio * multiplier)

	info.PriceData.ModelPrice = decision.Price
	info.PriceData.UsePrice = true
	info.PriceData.OtherRatios = ratios
	info.PriceData.Quota = quota
	info.PriceData.QuotaToPreConsume = quota
	info.PriceData.FreeModel = !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume && (decision.Price == 0 || groupRatio == 0)
}

// ResolveImageGroupPrice resolves a configured per-call resolution price for
// synchronous image generation/edit endpoints.
func ResolveImageGroupPrice(info *relaycommon.RelayInfo, resolution string) (float64, bool) {
	if info == nil {
		return 0, false
	}
	rule, ok := ratio_setting.GetTaskGroupPricingRule(info.OriginModelName, info.UsingGroup)
	if !ok || normalizeTaskBillingMode(rule.BillingMode, rule.Dimensions) != "per_call" {
		return 0, false
	}
	return taskResolutionPrice(rule.Dimensions, resolution)
}

// ApplyResolvedGroupPrice applies an absolute de-group-ratio price selected
// from a configured sub-dimension.
func ApplyResolvedGroupPrice(info *relaycommon.RelayInfo, price float64) {
	if info == nil || price < 0 || !isFiniteTaskPrice(price) {
		return
	}
	groupRatio := info.PriceData.GroupRatioInfo.GroupRatio
	quota := int(price * common.QuotaPerUnit * groupRatio)
	info.PriceData.ModelPrice = price
	info.PriceData.UsePrice = true
	info.PriceData.Quota = quota
	info.PriceData.QuotaToPreConsume = quota
	info.PriceData.FreeModel = !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume && (price == 0 || groupRatio == 0)
}

type taskRatioKind int

const (
	ratioKindOther taskRatioKind = iota
	ratioKindDuration
	ratioKindResolution
)

func classifyTaskRatio(key string) taskRatioKind {
	key = strings.ToLower(strings.TrimSpace(key))
	switch {
	case key == "seconds", key == "second", key == "duration", key == "duration_seconds":
		return ratioKindDuration
	case key == "size", key == "resolution", strings.HasPrefix(key, "resolution-"):
		return ratioKindResolution
	default:
		return ratioKindOther
	}
}

func configuredExtraRatios(legacy map[string]float64, ignore bool, omit ...taskRatioKind) map[string]float64 {
	out := map[string]float64{}
	if ignore {
		return out
	}
	omitted := map[taskRatioKind]bool{}
	for _, kind := range omit {
		omitted[kind] = true
	}
	for key, ratio := range legacy {
		if ratio <= 0 || omitted[classifyTaskRatio(key)] {
			continue
		}
		out[key] = ratio
	}
	return out
}

func legacyRatiosByKinds(legacy map[string]float64, kinds ...taskRatioKind) map[string]float64 {
	allowed := map[taskRatioKind]bool{}
	for _, kind := range kinds {
		allowed[kind] = true
	}
	out := map[string]float64{}
	for key, ratio := range legacy {
		if ratio > 0 && allowed[classifyTaskRatio(key)] {
			out[key] = ratio
		}
	}
	return out
}

func clonePositiveTaskRatios(input map[string]float64) map[string]float64 {
	out := map[string]float64{}
	for key, ratio := range input {
		if ratio > 0 && isFiniteTaskPrice(ratio) {
			out[key] = ratio
		}
	}
	return out
}

func mergeTaskRatios(dst, src map[string]float64) {
	for key, ratio := range src {
		if ratio > 0 {
			dst[key] = ratio
		}
	}
}

func normalizeTaskBillingMode(mode string, dimensions map[string]any) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "" {
		return mode
	}
	switch effectiveTaskDimensionMode(dimensions) {
	case "duration", "per_second":
		return "per_second"
	case "resolution", "flat", "per_call", "matrix":
		return "per_call"
	default:
		return ""
	}
}

func effectiveTaskDimensionMode(dimensions map[string]any) string {
	if mode := taskDimensionString(dimensions, "mode"); mode != "" {
		return mode
	}
	switch {
	case len(taskDimensionMap(dimensions, "resolution_prices")) > 0:
		return "resolution"
	case len(taskDimensionMap(dimensions, "duration_prices")) > 0,
		len(taskDimensionMap(dimensions, "duration_points")) > 0:
		return "duration"
	case len(taskAnySlice(dimensions["rows"])) > 0 && len(taskAnySlice(dimensions["prices"])) > 0:
		return "matrix"
	default:
		return ""
	}
}

func taskPricingDuration(input TaskPricingInput, legacy map[string]float64) float64 {
	if input.Duration > 0 && isFiniteTaskPrice(input.Duration) {
		return input.Duration
	}
	for key, ratio := range legacy {
		if classifyTaskRatio(key) == ratioKindDuration && ratio > 0 && isFiniteTaskPrice(ratio) {
			return ratio
		}
	}
	return 0
}

func taskResolutionPrice(dimensions map[string]any, resolution string) (float64, bool) {
	prices := taskDimensionMap(dimensions, "resolution_prices")
	if len(prices) == 0 {
		return 0, false
	}
	target := normalizePricingResolution(resolution)
	if target == "" {
		return 0, false
	}
	for key, raw := range prices {
		if normalizePricingResolution(key) != target {
			continue
		}
		return taskDimensionPrice(raw)
	}
	return 0, false
}

func taskDurationPrice(dimensions map[string]any, duration float64, legacy map[string]float64) (float64, bool) {
	prices := taskDimensionMap(dimensions, "duration_prices")
	if len(prices) == 0 {
		// Compatibility with ModelPriceByGroup values imported before
		// duration_points was canonicalized to duration_prices.
		prices = taskDimensionMap(dimensions, "duration_points")
	}
	if len(prices) == 0 {
		return 0, false
	}
	if duration <= 0 {
		duration = taskPricingDuration(TaskPricingInput{}, legacy)
	}
	if duration <= 0 {
		if defaultDuration, ok := positiveTaskDimensionFloat(dimensions["default_duration"]); ok {
			duration = defaultDuration
		}
	}
	if duration <= 0 {
		return 0, false
	}
	for key, raw := range prices {
		configuredDuration, ok := parseTaskDurationLabel(key)
		if !ok || math.Abs(configuredDuration-duration) > 0.000001 {
			continue
		}
		return taskDimensionPrice(raw)
	}
	return 0, false
}

func taskResolutionMultiplier(dimensions map[string]any, resolution string) (float64, bool) {
	multipliers := taskDimensionMap(dimensions, "resolution_multiplier")
	target := normalizePricingResolution(resolution)
	if len(multipliers) == 0 || target == "" {
		return 0, false
	}
	for key, raw := range multipliers {
		if normalizePricingResolution(key) == target {
			return positiveTaskDimensionFloat(raw)
		}
	}
	return 0, false
}

func taskMatrixPrice(dimensions map[string]any, input TaskPricingInput) (float64, string, bool) {
	headers := taskStringSlice(dimensions["headers"])
	rows := taskRows(dimensions["rows"])
	prices := taskAnySlice(dimensions["prices"])
	units := taskStringSlice(dimensions["price_units"])
	if len(headers) == 0 || len(rows) == 0 || len(prices) == 0 {
		return 0, "", false
	}
	for index, row := range rows {
		if index >= len(prices) || !taskMatrixRowMatches(headers, row, input) {
			continue
		}
		price, ok := taskDimensionPrice(prices[index])
		if !ok {
			continue
		}
		unit := "per_call"
		if index < len(units) && strings.TrimSpace(units[index]) != "" {
			unit = strings.ToLower(strings.TrimSpace(units[index]))
		}
		return price, unit, true
	}
	return 0, "", false
}

func taskMatrixLegacyPrice(dimensions map[string]any, legacy map[string]float64) (float64, bool) {
	baseUnitPrice, ok := taskMatrixBaseUnitPrice(dimensions)
	if !ok {
		return 0, false
	}
	units, ok := legacy["kling_units"]
	if !ok || units <= 0 || !isFiniteTaskPrice(units) {
		return 0, false
	}
	return baseUnitPrice * units, true
}

func taskMatrixBaseUnitPrice(dimensions map[string]any) (float64, bool) {
	prices := taskAnySlice(dimensions["prices"])
	unitPrices := taskAnySlice(dimensions["unit_prices"])
	for index, rawPrice := range prices {
		if index >= len(unitPrices) {
			break
		}
		price, priceOK := taskDimensionPrice(rawPrice)
		units, unitsOK := positiveTaskDimensionFloat(unitPrices[index])
		if priceOK && unitsOK {
			return price / units, true
		}
	}
	return taskDimensionPrice(dimensions["base_unit_price"])
}

func taskMatrixRowMatches(headers, row []string, input TaskPricingInput) bool {
	if len(row) < len(headers) {
		return false
	}
	for index, header := range headers {
		if !taskMatrixCellMatches(header, row[index], input) {
			return false
		}
	}
	return true
}

func taskMatrixCellMatches(header, value string, input TaskPricingInput) bool {
	header = strings.ToLower(strings.TrimSpace(header))
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "-" || value == "*" || value == "任意" {
		return true
	}
	switch {
	case strings.Contains(header, "模型") || strings.Contains(header, "model"):
		return normalizeMatrixValue(value) == normalizeMatrixValue(input.Model)
	case strings.Contains(header, "模式") || strings.Contains(header, "mode"):
		mode := input.Mode
		if strings.TrimSpace(mode) == "" {
			mode = "std"
		}
		return normalizeMatrixValue(value) == normalizeTaskMatrixMode(input.Model, mode)
	case strings.Contains(header, "时长") || strings.Contains(header, "duration") || strings.Contains(header, "second"):
		return taskDurationLabelMatches(value, input.Duration)
	case strings.Contains(header, "音色") || strings.Contains(header, "voice"):
		return taskBoolLabelMatches(value, input.Voice)
	case strings.Contains(header, "音频") || strings.Contains(header, "sound") || strings.Contains(header, "audio"):
		return taskBoolLabelMatches(value, input.Sound)
	default:
		return false
	}
}

func normalizeTaskMatrixMode(model, mode string) string {
	model = normalizeMatrixValue(model)
	mode = normalizeMatrixValue(mode)
	if strings.HasPrefix(model, "kling-") {
		// Kling's outer mode is also used as an action selector (t2v/i2v/etc.).
		// Its pricing matrix only distinguishes pro from the standard default.
		if mode == "pro" {
			return "pro"
		}
		return "std"
	}
	return mode
}

func taskDurationLabelMatches(label string, duration float64) bool {
	label = strings.ToLower(strings.TrimSpace(label))
	if duration <= 0 {
		return false
	}
	if strings.Contains(label, "-") {
		parts := strings.SplitN(strings.TrimSuffix(label, "s"), "-", 2)
		if len(parts) == 2 {
			minValue, minErr := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			maxValue, maxErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			return minErr == nil && maxErr == nil && duration >= minValue && duration <= maxValue
		}
	}
	value, ok := parseTaskDurationLabel(label)
	return ok && math.Abs(value-duration) <= 0.000001
}

func parseTaskDurationLabel(label string) (float64, bool) {
	label = strings.ToLower(strings.TrimSpace(label))
	label = strings.TrimSuffix(label, "seconds")
	label = strings.TrimSuffix(label, "second")
	label = strings.TrimSuffix(label, "secs")
	label = strings.TrimSuffix(label, "sec")
	label = strings.TrimSuffix(label, "s")
	value, err := strconv.ParseFloat(strings.TrimSpace(label), 64)
	return value, err == nil && value > 0
}

func taskBoolLabelMatches(label string, actual bool) bool {
	label = strings.ToLower(strings.TrimSpace(label))
	want := label == "是" || label == "有" || label == "on" || label == "yes" || label == "true" || label == "1" || label == "enabled"
	if !want && label != "否" && label != "无" && label != "off" && label != "no" && label != "false" && label != "0" && label != "disabled" {
		return false
	}
	return want == actual
}

func normalizeMatrixValue(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"), " ", ""))
}

func normalizePricingResolution(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	text = strings.ReplaceAll(text, "_", "")
	text = strings.ReplaceAll(text, "-", "")
	text = strings.ReplaceAll(text, " ", "")
	switch {
	case strings.Contains(text, "1080"):
		return "1080p"
	case strings.Contains(text, "720"):
		return "720p"
	case strings.Contains(text, "540"):
		return "540p"
	case strings.Contains(text, "480"):
		return "480p"
	case strings.Contains(text, "512"):
		return "512"
	case strings.Contains(text, "4k"):
		return "4k"
	case strings.Contains(text, "2k"):
		return "2k"
	case strings.Contains(text, "1k") || strings.Contains(text, "1024"):
		return "1k"
	default:
		return text
	}
}

func taskDimensionString(dimensions map[string]any, key string) string {
	if dimensions == nil {
		return ""
	}
	value, ok := dimensions[key]
	if !ok || value == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
}

func taskDimensionMap(dimensions map[string]any, key string) map[string]any {
	if dimensions == nil {
		return nil
	}
	switch value := dimensions[key].(type) {
	case map[string]any:
		return value
	case map[string]float64:
		out := make(map[string]any, len(value))
		for k, v := range value {
			out[k] = v
		}
		return out
	default:
		return nil
	}
}

func positiveTaskDimensionFloat(value any) (float64, bool) {
	parsed, ok := taskDimensionFloat(value)
	return parsed, ok && parsed > 0
}

func taskDimensionPrice(value any) (float64, bool) {
	parsed, ok := taskDimensionFloat(value)
	return parsed, ok && parsed >= 0
}

func taskDimensionFloat(value any) (float64, bool) {
	var parsed float64
	var ok bool
	switch typed := value.(type) {
	case string:
		var err error
		parsed, err = strconv.ParseFloat(strings.TrimSpace(typed), 64)
		ok = err == nil
	case float64:
		parsed, ok = typed, true
	case float32:
		parsed, ok = float64(typed), true
	case int:
		parsed, ok = float64(typed), true
	case int64:
		parsed, ok = float64(typed), true
	case int32:
		parsed, ok = float64(typed), true
	case json.Number:
		var err error
		parsed, err = typed.Float64()
		ok = err == nil
	}
	return parsed, ok && isFiniteTaskPrice(parsed)
}

func taskAnySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []float64:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	default:
		return nil
	}
}

func taskStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, len(typed))
		for i, item := range typed {
			out[i] = fmt.Sprint(item)
		}
		return out
	default:
		return nil
	}
}

func taskRows(value any) [][]string {
	items := taskAnySlice(value)
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		if row := taskStringSlice(item); len(row) > 0 {
			rows = append(rows, row)
		}
	}
	return rows
}

func isFiniteTaskPrice(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
