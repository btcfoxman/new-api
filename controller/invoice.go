package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type invoiceSubjectRequest struct {
	SubjectType    string `json:"subject_type"`
	RealName       string `json:"real_name"`
	CompanyName    string `json:"company_name"`
	IdNo           string `json:"id_no"`
	TaxNo          string `json:"tax_no"`
	CertificateUrl string `json:"certificate_url"`
	ValidFrom      int64  `json:"valid_from"`
	ValidUntil     int64  `json:"valid_until"`
}

type invoiceApplyRequest struct {
	TopUpIds []int  `json:"topup_ids"`
	Email    string `json:"email"`
	Provider string `json:"provider"`
}

type invoiceSubjectReviewRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type invoiceAdminUpdateRequest struct {
	Status         string `json:"status"`
	InvoiceNo      string `json:"invoice_no"`
	InvoiceFileUrl string `json:"invoice_file_url"`
	FileType       string `json:"file_type"`
	RejectReason   string `json:"reject_reason"`
}

type invoiceEmailForwardRequest struct {
	Email string `json:"email"`
}

func parseInvoiceId(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return 0, false
	}
	return id, true
}

func GetInvoiceSubject(c *gin.Context) {
	userId := c.GetInt("id")
	subject, err := model.GetInvoiceSubjectByUserId(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if subject == nil {
		common.ApiSuccess(c, nil)
		return
	}
	common.ApiSuccess(c, subject.Response())
}

func SaveInvoiceSubject(c *gin.Context) {
	userId := c.GetInt("id")
	req := invoiceSubjectRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	subject, err := model.SaveInvoiceSubject(
		userId,
		req.SubjectType,
		req.RealName,
		req.CompanyName,
		req.IdNo,
		req.TaxNo,
		req.CertificateUrl,
		req.ValidFrom,
		req.ValidUntil,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, subject.Response())
}

func ListInvoiceTopUps(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListInvoiceTopUps(userId, pageInfo, c.Query("keyword"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func ApplyInvoice(c *gin.Context) {
	userId := c.GetInt("id")
	req := invoiceApplyRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	app, err := model.CreateInvoiceApplication(userId, req.TopUpIds, req.Email, req.Provider)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, app)
}

func ListUserInvoices(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListInvoiceApplications(userId, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func CancelInvoice(c *gin.Context) {
	userId := c.GetInt("id")
	id, ok := parseInvoiceId(c)
	if !ok {
		return
	}
	if err := model.CancelInvoiceApplication(userId, id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func ForwardInvoiceEmail(c *gin.Context) {
	userId := c.GetInt("id")
	id, ok := parseInvoiceId(c)
	if !ok {
		return
	}
	req := invoiceEmailForwardRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err := model.RequestInvoiceEmailForward(userId, id, req.Email); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminListInvoiceSubjects(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListInvoiceSubjects(pageInfo, c.Query("status"), c.Query("keyword"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func AdminReviewInvoiceSubject(c *gin.Context) {
	id, ok := parseInvoiceId(c)
	if !ok {
		return
	}
	req := invoiceSubjectReviewRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err := model.ReviewInvoiceSubject(id, req.Status, req.Reason, c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminListInvoices(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListInvoiceApplications(0, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func AdminUpdateInvoice(c *gin.Context) {
	id, ok := parseInvoiceId(c)
	if !ok {
		return
	}
	req := invoiceAdminUpdateRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err := model.UpdateInvoiceApplicationByAdmin(id, c.GetInt("id"), req.Status, req.InvoiceNo, req.InvoiceFileUrl, req.FileType, req.RejectReason); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminListInvoiceProviderConfigs(c *gin.Context) {
	configs, err := model.ListInvoiceProviderConfigs()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, configs)
}

func AdminSaveInvoiceProviderConfig(c *gin.Context) {
	config := &model.InvoiceProviderConfig{}
	if err := c.ShouldBindJSON(config); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err := model.SaveInvoiceProviderConfig(config); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, config)
}
