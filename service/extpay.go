package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/shopspring/decimal"
)

const (
	ExtPayMethodType     = "extpay"
	ExtPayChannelName    = "extpay"
	extPayQueryWaiting   = "WAIT_PAY"
	extPayQuerySuccess   = "SUCCESS"
	extPayQueryFailed    = "FAILED"
	extPaySuccessStatus  = "SUCCESS"
	extPayFailedStatus   = "FAILED"
	extPayDefaultSubject = "Account Top Up"
)

type ExtPayCreateOrderRequest struct {
	AppID      string `json:"appId"`
	ExtOrderNo string `json:"extOrderNo"`
	UID        string `json:"uid"`
	Amount     string `json:"amount"`
	Subject    string `json:"subject"`
	NotifyURL  string `json:"notifyUrl"`
	ReturnURL  string `json:"returnUrl,omitempty"`
	TraceID    string `json:"traceId,omitempty"`
	Timestamp  int64  `json:"timestamp"`
	Nonce      string `json:"nonce"`
	Sign       string `json:"sign"`
}

type ExtPayCreateOrderResponse struct {
	GatewayOrderNo string `json:"gatewayOrderNo"`
	ExtOrderNo     string `json:"extOrderNo"`
	Status         string `json:"status"`
	CheckoutURL    string `json:"checkoutUrl"`
	PayURL         string `json:"payUrl"`
}

type ExtPayQueryOrderRequest struct {
	AppID          string `json:"appId"`
	ExtOrderNo     string `json:"extOrderNo,omitempty"`
	GatewayOrderNo string `json:"gatewayOrderNo,omitempty"`
	Timestamp      int64  `json:"timestamp"`
	Nonce          string `json:"nonce"`
	Sign           string `json:"sign"`
}

type ExtPayQueryOrderResponse struct {
	GatewayOrderNo string `json:"gatewayOrderNo"`
	ExtOrderNo     string `json:"extOrderNo"`
	Status         string `json:"status"`
	Amount         string `json:"amount"`
	UID            string `json:"uid"`
	PaidAt         string `json:"paidAt"`
	TradeNo        string `json:"tradeNo"`
	PayURL         string `json:"payUrl"`
}

type ExtPayNotifyPayload struct {
	AppID           string `json:"appId"`
	MerchantOrderNo string `json:"merchantOrderNo"`
	ExternalOrderNo string `json:"externalOrderNo"`
	GatewayOrderNo  string `json:"gatewayOrderNo"`
	UID             string `json:"uid"`
	Amount          string `json:"amount"`
	Status          string `json:"status"`
	PaidAt          string `json:"paidAt"`
	TradeNo         string `json:"tradeNo"`
	Timestamp       int64  `json:"timestamp"`
	Nonce           string `json:"nonce"`
	Sign            string `json:"sign"`
}

type extPayEnvelope struct {
	Code    int             `json:"code"`
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

var (
	extPayNonceMu     sync.Mutex
	extPayNonceCache  = map[string]int64{}
	extPayTaskLockTTL = 2 * time.Minute
	extPayNotifyTTL   = 5 * time.Minute
)

func ExtPayAvailable() bool {
	return extPayBoolSetting("ExtPayEnabled", setting.ExtPayEnabled) &&
		extPayStringSetting("ExtPayBaseURL", setting.ExtPayBaseURL) != "" &&
		extPayStringSetting("ExtPayAppId", setting.ExtPayAppId) != "" &&
		extPayStringSetting("ExtPaySecret", setting.ExtPaySecret) != ""
}

func GetExtPayMinTopUp() int64 {
	minTopUp := extPayIntSetting("ExtPayMinTopUp", setting.ExtPayMinTopUp)
	if minTopUp > 0 {
		return int64(minTopUp)
	}
	minTopup := operation_setting.MinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

func CreateExtPayOrder(topUp *model.TopUp) (*ExtPayCreateOrderResponse, error) {
	timestamp := time.Now().Unix()
	nonce := common.GetRandomString(16)
	req := &ExtPayCreateOrderRequest{
		AppID:      extPayStringSetting("ExtPayAppId", setting.ExtPayAppId),
		ExtOrderNo: topUp.TradeNo,
		UID:        strconv.Itoa(topUp.UserId),
		Amount:     decimal.NewFromFloat(topUp.Money).Round(2).StringFixed(2),
		Subject:    buildExtPaySubject(topUp),
		NotifyURL:  getExtPayNotifyURL(),
		ReturnURL:  getExtPayReturnURL(topUp.TradeNo),
		TraceID:    topUp.TradeNo,
		Timestamp:  timestamp,
		Nonce:      nonce,
	}
	req.Sign = signExtPayPayload(extPayStringSetting("ExtPaySecret", setting.ExtPaySecret), map[string]any{
		"amount":     req.Amount,
		"appId":      req.AppID,
		"extOrderNo": req.ExtOrderNo,
		"nonce":      req.Nonce,
		"notifyUrl":  req.NotifyURL,
		"returnUrl":  req.ReturnURL,
		"subject":    req.Subject,
		"timestamp":  req.Timestamp,
		"traceId":    req.TraceID,
		"uid":        req.UID,
	})

	body, err := extPayJSONRequest(http.MethodPost, "/api/publicly/external/pay/order/create", req)
	if err != nil {
		return nil, err
	}
	var response extPayEnvelope
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	var data ExtPayCreateOrderResponse
	if len(response.Data) > 0 && string(response.Data) != "null" {
		if err := json.Unmarshal(response.Data, &data); err != nil {
			return nil, err
		}
	}
	if response.Code != http.StatusOK &&
		!response.Success &&
		data.GatewayOrderNo == "" &&
		data.CheckoutURL == "" &&
		data.PayURL == "" {
		return nil, errors.New(response.Message)
	}
	return &data, nil
}

func QueryExtPayOrder(topUp *model.TopUp) (*ExtPayQueryOrderResponse, error) {
	timestamp := time.Now().Unix()
	nonce := common.GetRandomString(16)
	req := &ExtPayQueryOrderRequest{
		AppID:          extPayStringSetting("ExtPayAppId", setting.ExtPayAppId),
		ExtOrderNo:     topUp.TradeNo,
		GatewayOrderNo: topUp.ExternalOrderNo,
		Timestamp:      timestamp,
		Nonce:          nonce,
	}
	req.Sign = signExtPayPayload(extPayStringSetting("ExtPaySecret", setting.ExtPaySecret), map[string]any{
		"appId":          req.AppID,
		"extOrderNo":     req.ExtOrderNo,
		"gatewayOrderNo": req.GatewayOrderNo,
		"nonce":          req.Nonce,
		"timestamp":      req.Timestamp,
	})

	body, err := extPayJSONRequest(http.MethodGet, "/api/publicly/external/pay/order/query", req)
	if err != nil {
		return nil, err
	}
	var response extPayEnvelope
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	var data ExtPayQueryOrderResponse
	if len(response.Data) > 0 && string(response.Data) != "null" {
		if err := json.Unmarshal(response.Data, &data); err != nil {
			return nil, err
		}
	}
	if response.Code != http.StatusOK &&
		!response.Success &&
		data.GatewayOrderNo == "" &&
		data.Status == "" {
		return nil, errors.New(response.Message)
	}
	return &data, nil
}

func VerifyExtPayNotify(payload *ExtPayNotifyPayload) error {
	if payload.AppID != extPayStringSetting("ExtPayAppId", setting.ExtPayAppId) {
		return fmt.Errorf("非法 appId")
	}
	if payload.Timestamp <= 0 {
		return fmt.Errorf("timestamp 无效")
	}
	now := time.Now().Unix()
	if now-payload.Timestamp > 300 || payload.Timestamp-now > 300 {
		return fmt.Errorf("回调已过期")
	}
	expected := signExtPayPayload(extPayStringSetting("ExtPaySecret", setting.ExtPaySecret), map[string]any{
		"amount":          payload.Amount,
		"appId":           payload.AppID,
		"externalOrderNo": payload.ExternalOrderNo,
		"gatewayOrderNo":  payload.GatewayOrderNo,
		"merchantOrderNo": payload.MerchantOrderNo,
		"nonce":           payload.Nonce,
		"paidAt":          payload.PaidAt,
		"status":          payload.Status,
		"timestamp":       payload.Timestamp,
		"tradeNo":         payload.TradeNo,
		"uid":             payload.UID,
	})
	if !strings.EqualFold(expected, payload.Sign) {
		return fmt.Errorf("签名校验失败")
	}
	if err := registerExtPayNotifyNonce(payload.AppID, payload.Nonce, extPayNotifyTTL); err != nil {
		return err
	}
	return nil
}

func SyncTopUpWithExtPay(topUp *model.TopUp) error {
	resp, err := QueryExtPayOrder(topUp)
	raw, _ := json.Marshal(resp)
	if resp == nil {
		_ = model.UpdateExtPayQueryInfo(topUp.TradeNo, common.GetTimestamp(), topUp.QueryRetryCount+1, "")
		return err
	}
	rawText := string(raw)
	_ = model.UpdateExtPayQueryInfo(topUp.TradeNo, common.GetTimestamp(), topUp.QueryRetryCount+1, rawText)
	if resp.GatewayOrderNo != "" && topUp.ExternalOrderNo == "" {
		topUp.ExternalOrderNo = resp.GatewayOrderNo
	}
	switch strings.ToUpper(resp.Status) {
	case extPayQuerySuccess:
		amount, amountErr := parseExtPayAmount(resp.Amount)
		if amountErr != nil {
			return amountErr
		}
		return model.CompleteExtPayTopUp(topUp.TradeNo, resp.GatewayOrderNo, resp.UID, amount, rawText)
	case extPayQueryFailed:
		return model.MarkExtPayTopUpState(topUp.TradeNo, common.TopUpStatusFailed, resp.GatewayOrderNo, rawText)
	default:
		return nil
	}
}

func SyncPendingExtPayOrders(limit int) {
	if !ExtPayAvailable() || !ExtPayQueryEnabled() {
		return
	}
	threshold := common.GetTimestamp() - int64(maxInt(extPayIntSetting("ExtPayQueryIntervalSeconds", setting.ExtPayQueryIntervalSeconds), 5))
	topUps, err := model.GetPendingExtPayTopUps(limit, threshold)
	if err != nil {
		common.SysError("failed to list pending extpay topups: " + err.Error())
		return
	}
	for _, topUp := range topUps {
		if err := SyncTopUpWithExtPay(topUp); err != nil {
			common.SysError(fmt.Sprintf("failed to sync extpay order %s: %v", topUp.TradeNo, err))
		}
	}
}

func extPayJSONRequest(method string, path string, payload any) ([]byte, error) {
	client := GetHttpClient()
	if client == nil {
		client = http.DefaultClient
	}

	var (
		req *http.Request
		err error
	)
	fullURL := strings.TrimRight(extPayStringSetting("ExtPayBaseURL", setting.ExtPayBaseURL), "/") + path
	if method == http.MethodGet {
		queryValues := buildExtPayQuery(payload)
		if encoded := queryValues.Encode(); encoded != "" {
			fullURL += "?" + encoded
		}
		req, err = http.NewRequest(method, fullURL, nil)
	} else {
		body, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return nil, marshalErr
		}
		req, err = http.NewRequest(method, fullURL, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if err != nil {
		return nil, err
	}

	timeout := time.Duration(maxInt(extPayIntSetting("ExtPayQueryTimeoutSeconds", setting.ExtPayQueryTimeoutSeconds), 10)) * time.Second
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("extpay http %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func buildExtPayQuery(payload any) url.Values {
	raw, _ := json.Marshal(payload)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	data := map[string]any{}
	_ = decoder.Decode(&data)
	query := url.Values{}
	for key, value := range data {
		if value == nil {
			continue
		}
		var text string
		switch v := value.(type) {
		case json.Number:
			text = v.String()
		default:
			text = fmt.Sprint(value)
		}
		if text == "" {
			continue
		}
		query.Set(key, text)
	}
	return query
}

func signExtPayPayload(secret string, payload map[string]any) string {
	keys := make([]string, 0, len(payload))
	for key, value := range payload {
		if key == "sign" || value == nil {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+strings.TrimSpace(fmt.Sprint(payload[key])))
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strings.Join(parts, "&")))
	return hex.EncodeToString(mac.Sum(nil))
}

func parseExtPayAmount(amount string) (decimal.Decimal, error) {
	if amount == "" {
		return decimal.Zero, fmt.Errorf("amount 为空")
	}
	return decimal.NewFromString(amount)
}

func buildExtPaySubject(topUp *model.TopUp) string {
	displayAmount := topUp.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		displayAmount = int64(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())
	}
	return fmt.Sprintf("%s %d", extPayDefaultSubject, displayAmount)
}

func getExtPayNotifyURL() string {
	value := extPayStringSetting("ExtPayNotifyURL", setting.ExtPayNotifyURL)
	if value != "" {
		return value
	}
	return strings.TrimRight(GetCallbackAddress(), "/") + "/api/ext-pay/notify"
}

func getExtPayReturnURL(orderNo string) string {
	base := extPayStringSetting("ExtPayReturnURL", setting.ExtPayReturnURL)
	if base == "" {
		base = strings.TrimRight(system_setting.ServerAddress, "/") + "/api/ext-pay/return"
	}
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
	}
	return base + separator + "order_no=" + orderNo
}

func maxInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func ExtPayQueryEnabled() bool {
	return extPayBoolSetting("ExtPayQueryEnabled", setting.ExtPayQueryEnabled)
}

func ExtPayQueryIntervalSeconds() int {
	return extPayIntSetting("ExtPayQueryIntervalSeconds", setting.ExtPayQueryIntervalSeconds)
}

func extPayStringSetting(key string, current string) string {
	if strings.TrimSpace(current) != "" {
		return strings.TrimSpace(current)
	}
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return strings.TrimSpace(common.OptionMap[key])
}

func extPayBoolSetting(key string, current bool) bool {
	if current {
		return true
	}
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return strings.EqualFold(strings.TrimSpace(common.OptionMap[key]), "true")
}

func extPayIntSetting(key string, current int) int {
	if current > 0 {
		return current
	}
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	value := strings.TrimSpace(common.OptionMap[key])
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return current
	}
	return parsed
}

func registerExtPayNotifyNonce(appID string, nonce string, ttl time.Duration) error {
	if strings.TrimSpace(appID) == "" || strings.TrimSpace(nonce) == "" {
		return fmt.Errorf("nonce 无效")
	}
	key := "extpay:notify:nonce:" + appID + ":" + nonce
	if common.RedisEnabled && common.RDB != nil {
		ok, err := common.RDB.SetNX(context.Background(), key, "1", ttl).Result()
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("重复回调")
		}
		return nil
	}
	return registerLocalExtPayTTL(key, ttl, "重复回调")
}

func acquireExtPayQueryTaskLock() (bool, error) {
	lockKey := "extpay:query:task:lock"
	if common.RedisEnabled && common.RDB != nil {
		return common.RDB.SetNX(context.Background(), lockKey, strconv.FormatInt(time.Now().Unix(), 10), extPayTaskLockTTL).Result()
	}
	if err := registerLocalExtPayTTL(lockKey, extPayTaskLockTTL, "query task locked"); err != nil {
		if err.Error() == "query task locked" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func registerLocalExtPayTTL(key string, ttl time.Duration, duplicateMessage string) error {
	now := time.Now().Unix()
	expireAt := now + int64(ttl/time.Second)
	extPayNonceMu.Lock()
	defer extPayNonceMu.Unlock()
	for cacheKey, ts := range extPayNonceCache {
		if ts <= now {
			delete(extPayNonceCache, cacheKey)
		}
	}
	if cachedUntil, ok := extPayNonceCache[key]; ok && cachedUntil > now {
		return errors.New(duplicateMessage)
	}
	extPayNonceCache[key] = expireAt
	return nil
}
