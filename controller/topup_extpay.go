package controller

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type ExtPayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

func RequestExtPay(c *gin.Context) {
	if !service.ExtPayAvailable() {
		common.SysLog("extpay request rejected: unavailable")
		common.ApiErrorMsg(c, "ExtPay 未启用")
		return
	}

	var req ExtPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SysError("extpay request rejected: invalid payload: " + err.Error())
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if req.PaymentMethod != service.ExtPayMethodType {
		common.SysLog("extpay request rejected: unsupported payment method")
		common.ApiErrorMsg(c, "支付方式不支持")
		return
	}
	if req.Amount < service.GetExtPayMinTopUp() {
		common.SysLog("extpay request rejected: amount below minimum")
		common.ApiErrorMsg(c, "充值数量低于最小限制")
		return
	}

	userID := c.GetInt("id")
	group, err := model.GetUserGroup(userID, true)
	if err != nil {
		common.SysError("extpay request failed: get user group: " + err.Error())
		common.ApiErrorMsg(c, "获取用户分组失败")
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		common.SysLog("extpay request rejected: pay money below threshold")
		common.ApiErrorMsg(c, "充值金额过低")
		return
	}

	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(req.Amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}

	tradeNo := buildExtPayTradeNo(userID)
	topUp := &model.TopUp{
		UserId:         userID,
		Amount:         amount,
		Money:          payMoney,
		TradeNo:        tradeNo,
		PaymentMethod:  service.ExtPayMethodType,
		PaymentChannel: service.ExtPayChannelName,
		CreateTime:     time.Now().Unix(),
		Status:         common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		common.SysError("extpay request failed: insert topup: " + err.Error())
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}

	resp, err := service.CreateExtPayOrder(topUp)
	if err != nil {
		common.SysError("extpay request failed: create extpay order: " + strconv.Quote(err.Error()))
		_ = model.MarkExtPayTopUpState(tradeNo, common.TopUpStatusFailed, "", err.Error())
		common.ApiError(c, err)
		return
	}

	raw, _ := json.Marshal(resp)
	_ = model.UpdateExtPayQueryInfo(tradeNo, 0, 0, string(raw))
	_ = model.MarkExtPayTopUpState(tradeNo, common.TopUpStatusPending, resp.GatewayOrderNo, string(raw))

	common.ApiSuccess(c, gin.H{
		"order_no":          tradeNo,
		"status":            common.TopUpStatusPending,
		"url":               firstNonEmpty(resp.CheckoutURL, resp.PayURL),
		"external_order_no": resp.GatewayOrderNo,
	})
}

func GetTopUpDetail(c *gin.Context) {
	userID := c.GetInt("id")
	orderNo := c.Param("order_no")
	topUp := model.GetUserTopUpByTradeNo(userID, orderNo)
	if topUp == nil {
		common.ApiErrorMsg(c, "订单不存在")
		return
	}

	if topUp.PaymentMethod == service.ExtPayMethodType &&
		topUp.Status == common.TopUpStatusPending &&
		service.ExtPayAvailable() &&
		service.ExtPayQueryEnabled() &&
		(common.GetTimestamp()-topUp.LastQueryTime >= int64(getExtPayQueryInterval()) || topUp.LastQueryTime == 0) {
		_ = service.SyncTopUpWithExtPay(topUp)
		topUp = model.GetUserTopUpByTradeNo(userID, orderNo)
	}

	status := topUp.Status
	if status == common.TopUpStatusExpired {
		status = "closed"
	}
	common.ApiSuccess(c, gin.H{
		"order_no":          topUp.TradeNo,
		"status":            status,
		"amount":            topUp.Amount,
		"payment_method":    topUp.PaymentMethod,
		"external_order_no": topUp.ExternalOrderNo,
		"paid_at":           topUp.CompleteTime,
		"created_at":        topUp.CreateTime,
	})
}

func ExtPayNotify(c *gin.Context) {
	var payload service.ExtPayNotifyPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.String(http.StatusOK, "fail")
		return
	}
	if err := service.VerifyExtPayNotify(&payload); err != nil {
		c.String(http.StatusOK, "fail")
		return
	}
	status := strings.ToUpper(payload.Status)
	if status != "SUCCESS" && status != "FAILED" {
		c.String(http.StatusOK, "fail")
		return
	}

	amount, err := decimal.NewFromString(payload.Amount)
	if err != nil {
		c.String(http.StatusOK, "fail")
		return
	}
	raw, _ := json.Marshal(payload)
	switch status {
	case "SUCCESS":
		err = model.CompleteExtPayTopUp(payload.MerchantOrderNo, firstNonEmpty(payload.ExternalOrderNo, payload.GatewayOrderNo), payload.UID, amount, string(raw))
		if err != nil && !strings.Contains(err.Error(), "状态错误") {
			c.String(http.StatusOK, "fail")
			return
		}
	case "FAILED":
		_ = model.MarkExtPayTopUpState(payload.MerchantOrderNo, common.TopUpStatusFailed, firstNonEmpty(payload.ExternalOrderNo, payload.GatewayOrderNo), string(raw))
	}
	c.String(http.StatusOK, "success")
}

func ExtPayReturn(c *gin.Context) {
	target := strings.TrimRight(system_setting.ServerAddress, "/") + "/console/topup?show_history=true"
	orderNo := c.Query("order_no")
	if orderNo != "" {
		target += "&order_no=" + url.QueryEscape(orderNo)
	}
	c.Redirect(http.StatusFound, target)
}

func buildExtPayTradeNo(userID int) string {
	return "EXT" + strconv.Itoa(userID) + time.Now().Format("20060102150405") + common.GetRandomString(6)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func getExtPayQueryInterval() int {
	if interval := service.ExtPayQueryIntervalSeconds(); interval > 0 {
		return interval
	}
	return 5
}
