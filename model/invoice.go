package model

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	InvoiceSubjectTypePersonal = "personal"
	InvoiceSubjectTypeCompany  = "company"

	InvoiceSubjectStatusPending  = "pending"
	InvoiceSubjectStatusVerified = "verified"
	InvoiceSubjectStatusRejected = "rejected"

	InvoiceApplicationStatusPending    = "pending"
	InvoiceApplicationStatusApproved   = "approved"
	InvoiceApplicationStatusIssued     = "issued"
	InvoiceApplicationStatusRejected   = "rejected"
	InvoiceApplicationStatusCancelled  = "cancelled"
	InvoiceApplicationStatusFailed     = "failed"
	InvoiceApplicationStatusRedPending = "red_pending"
	InvoiceApplicationStatusRedIssued  = "red_issued"

	InvoiceTypeVatNormal = "vat_normal"

	InvoiceProviderManual      = "manual"
	InvoiceProviderAlipay      = "alipay_invoice"
	InvoiceProviderWechat      = "wechat_invoice"
	InvoiceProviderFapiaoCloud = "fapiao_cloud"
	InvoiceProviderNuonuo      = "nuonuo"

	InvoiceProviderTaskOpCreate       = "create"
	InvoiceProviderTaskOpRed          = "red"
	InvoiceProviderTaskOpQuery        = "query"
	InvoiceProviderTaskOpDownload     = "download"
	InvoiceProviderTaskOpEmailForward = "email_forward"

	InvoiceTopUpAvailableStartTime = int64(1783526400) // 2026-07-09 00:00:00 +08:00
)

var activeInvoiceStatuses = []string{
	InvoiceApplicationStatusPending,
	InvoiceApplicationStatusApproved,
	InvoiceApplicationStatusIssued,
	InvoiceApplicationStatusRedPending,
}

type InvoiceSubject struct {
	Id             int    `json:"id"`
	UserId         int    `json:"user_id" gorm:"uniqueIndex"`
	SubjectType    string `json:"subject_type" gorm:"type:varchar(20);index"`
	Status         string `json:"status" gorm:"type:varchar(20);index"`
	RealName       string `json:"real_name" gorm:"type:varchar(128);default:''"`
	CompanyName    string `json:"company_name" gorm:"type:varchar(255);default:''"`
	IdNoCipher     string `json:"-" gorm:"type:text"`
	TaxNoCipher    string `json:"-" gorm:"type:text"`
	IdNoHash       string `json:"-" gorm:"type:varchar(128);index"`
	TaxNoHash      string `json:"-" gorm:"type:varchar(128);index"`
	CertificateUrl string `json:"certificate_url" gorm:"type:text"`
	ValidFrom      int64  `json:"valid_from"`
	ValidUntil     int64  `json:"valid_until" gorm:"index"`
	ReviewedBy     int    `json:"reviewed_by"`
	ReviewedAt     int64  `json:"reviewed_at"`
	RejectReason   string `json:"reject_reason" gorm:"type:text"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

type InvoiceApplication struct {
	Id                    int     `json:"id"`
	UserId                int     `json:"user_id" gorm:"index"`
	SubjectId             int     `json:"subject_id" gorm:"index"`
	SubjectType           string  `json:"subject_type" gorm:"type:varchar(20);index"`
	InvoiceType           string  `json:"invoice_type" gorm:"type:varchar(50);default:'vat_normal'"`
	Title                 string  `json:"title" gorm:"type:varchar(255)"`
	Amount                float64 `json:"amount"`
	Email                 string  `json:"email" gorm:"type:varchar(255)"`
	PaymentChannel        string  `json:"payment_channel" gorm:"type:varchar(64);index"`
	ProviderCode          string  `json:"provider_code" gorm:"type:varchar(64);index"`
	ProviderApplicationId string  `json:"provider_application_id" gorm:"type:varchar(255);default:'';index"`
	ProviderInvoiceId     string  `json:"provider_invoice_id" gorm:"type:varchar(255);default:'';index"`
	InvoiceNo             string  `json:"invoice_no" gorm:"type:varchar(255);default:'';index"`
	InvoiceFileUrl        string  `json:"invoice_file_url" gorm:"type:text"`
	SubjectSnapshot       string  `json:"subject_snapshot" gorm:"type:text"`
	Status                string  `json:"status" gorm:"type:varchar(32);index"`
	RejectReason          string  `json:"reject_reason" gorm:"type:text"`
	CreatedAt             int64   `json:"created_at"`
	UpdatedAt             int64   `json:"updated_at"`
	ReviewedAt            int64   `json:"reviewed_at"`
	IssuedAt              int64   `json:"issued_at"`
	RedIssuedAt           int64   `json:"red_issued_at"`
}

type InvoiceApplicationItem struct {
	Id            int     `json:"id"`
	ApplicationId int     `json:"application_id" gorm:"index"`
	TopUpId       int     `json:"topup_id" gorm:"index"`
	TradeNo       string  `json:"trade_no" gorm:"type:varchar(255);index"`
	Amount        float64 `json:"amount"`
	CreatedAt     int64   `json:"created_at"`
}

type InvoiceProviderConfig struct {
	Id                       int    `json:"id"`
	ProviderCode             string `json:"provider_code" gorm:"type:varchar(64);uniqueIndex"`
	Name                     string `json:"name" gorm:"type:varchar(128)"`
	Enabled                  bool   `json:"enabled"`
	SupportedPaymentChannels string `json:"supported_payment_channels" gorm:"type:text"`
	AllowCrossChannel        bool   `json:"allow_cross_channel"`
	SupportsCreate           bool   `json:"supports_create"`
	SupportsRed              bool   `json:"supports_red"`
	SupportsQuery            bool   `json:"supports_query"`
	SupportsDownload         bool   `json:"supports_download"`
	SupportsEmailForward     bool   `json:"supports_email_forward"`
	Config                   string `json:"config" gorm:"type:text"`
	CreatedAt                int64  `json:"created_at"`
	UpdatedAt                int64  `json:"updated_at"`
}

type InvoiceProviderTask struct {
	Id                    int    `json:"id"`
	ApplicationId         int    `json:"application_id" gorm:"index"`
	ProviderCode          string `json:"provider_code" gorm:"type:varchar(64);index"`
	Operation             string `json:"operation" gorm:"type:varchar(32);index"`
	Status                string `json:"status" gorm:"type:varchar(32);index"`
	ExternalRequestId     string `json:"external_request_id" gorm:"type:varchar(255);default:'';index"`
	ExternalApplicationId string `json:"external_application_id" gorm:"type:varchar(255);default:'';index"`
	ExternalInvoiceId     string `json:"external_invoice_id" gorm:"type:varchar(255);default:'';index"`
	RequestSnapshot       string `json:"request_snapshot" gorm:"type:text"`
	ResponseSnapshot      string `json:"response_snapshot" gorm:"type:text"`
	ErrorMessage          string `json:"error_message" gorm:"type:text"`
	RetryCount            int    `json:"retry_count"`
	NextRetryAt           int64  `json:"next_retry_at"`
	CreatedAt             int64  `json:"created_at"`
	UpdatedAt             int64  `json:"updated_at"`
}

type InvoiceFile struct {
	Id            int    `json:"id"`
	ApplicationId int    `json:"application_id" gorm:"index"`
	FileType      string `json:"file_type" gorm:"type:varchar(32)"`
	SourceUrl     string `json:"source_url" gorm:"type:text"`
	StorageUrl    string `json:"storage_url" gorm:"type:text"`
	FileHash      string `json:"file_hash" gorm:"type:varchar(128);default:''"`
	ExpiresAt     int64  `json:"expires_at"`
	CreatedAt     int64  `json:"created_at"`
}

type InvoiceOperationLog struct {
	Id            int    `json:"id"`
	ApplicationId int    `json:"application_id" gorm:"index"`
	SubjectId     int    `json:"subject_id" gorm:"index"`
	UserId        int    `json:"user_id" gorm:"index"`
	OperatorId    int    `json:"operator_id" gorm:"index"`
	Operation     string `json:"operation" gorm:"type:varchar(64);index"`
	Content       string `json:"content" gorm:"type:text"`
	CreatedAt     int64  `json:"created_at"`
}

type InvoiceUserPreference struct {
	Id           int    `json:"id"`
	UserId       int    `json:"user_id" gorm:"uniqueIndex"`
	DefaultEmail string `json:"default_email" gorm:"type:varchar(255)"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

type InvoiceSubjectResponse struct {
	Id             int    `json:"id"`
	UserId         int    `json:"user_id"`
	Username       string `json:"username"`
	SubjectType    string `json:"subject_type"`
	Status         string `json:"status"`
	RealName       string `json:"real_name"`
	CompanyName    string `json:"company_name"`
	IdNo           string `json:"id_no"`
	TaxNo          string `json:"tax_no"`
	MaskedIdNo     string `json:"masked_id_no"`
	MaskedTaxNo    string `json:"masked_tax_no"`
	CertificateUrl string `json:"certificate_url"`
	ValidFrom      int64  `json:"valid_from"`
	ValidUntil     int64  `json:"valid_until"`
	ReviewedBy     int    `json:"reviewed_by"`
	ReviewedAt     int64  `json:"reviewed_at"`
	RejectReason   string `json:"reject_reason"`
	DefaultEmail   string `json:"default_email,omitempty"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

type InvoiceTopUpResponse struct {
	Id               int     `json:"id"`
	UserId           int     `json:"user_id"`
	Username         string  `json:"username"`
	Amount           int64   `json:"amount"`
	Money            float64 `json:"money"`
	TradeNo          string  `json:"trade_no"`
	PaymentMethod    string  `json:"payment_method"`
	PaymentChannel   string  `json:"payment_channel"`
	Status           string  `json:"status"`
	CreateTime       int64   `json:"create_time"`
	CompleteTime     int64   `json:"complete_time"`
	InvoiceStatus    string  `json:"invoice_status"`
	InvoiceAvailable bool    `json:"invoice_available"`
}

type InvoiceApplicationResponse struct {
	Id             int                       `json:"id"`
	UserId         int                       `json:"user_id"`
	Username       string                    `json:"username"`
	SubjectId      int                       `json:"subject_id"`
	SubjectType    string                    `json:"subject_type"`
	InvoiceType    string                    `json:"invoice_type"`
	Title          string                    `json:"title"`
	Amount         float64                   `json:"amount"`
	Email          string                    `json:"email"`
	PaymentChannel string                    `json:"payment_channel"`
	ProviderCode   string                    `json:"provider_code"`
	InvoiceNo      string                    `json:"invoice_no"`
	InvoiceFileUrl string                    `json:"invoice_file_url"`
	Status         string                    `json:"status"`
	RejectReason   string                    `json:"reject_reason"`
	CreatedAt      int64                     `json:"created_at"`
	UpdatedAt      int64                     `json:"updated_at"`
	IssuedAt       int64                     `json:"issued_at"`
	Subject        *InvoiceSubjectResponse   `json:"subject,omitempty"`
	SubjectSnapshot map[string]interface{}    `json:"subject_snapshot,omitempty"`
	Items          []*InvoiceApplicationItem `json:"items,omitempty"`
}

func invoiceSecretKey() []byte {
	seed := common.CryptoSecret
	if seed == "" {
		seed = "new-api-invoice-secret"
	}
	sum := sha256.Sum256([]byte(seed))
	return sum[:]
}

func encryptInvoiceSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(invoiceSecretKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

func decryptInvoiceSecret(cipherText string) string {
	if cipherText == "" {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return ""
	}
	block, err := aes.NewCipher(invoiceSecretKey())
	if err != nil {
		return ""
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ""
	}
	if len(raw) < gcm.NonceSize() {
		return ""
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return ""
	}
	return string(plain)
}

func maskInvoiceSecret(v string, left int, right int) string {
	rs := []rune(strings.TrimSpace(v))
	if len(rs) == 0 {
		return ""
	}
	if len(rs) <= left+right {
		return strings.Repeat("*", len(rs))
	}
	return string(rs[:left]) + strings.Repeat("*", len(rs)-left-right) + string(rs[len(rs)-right:])
}

func (s *InvoiceSubject) idNo() string {
	return decryptInvoiceSecret(s.IdNoCipher)
}

func (s *InvoiceSubject) taxNo() string {
	return decryptInvoiceSecret(s.TaxNoCipher)
}

func (s *InvoiceSubject) title() string {
	if s.SubjectType == InvoiceSubjectTypeCompany {
		return s.CompanyName
	}
	return s.RealName
}

func (s *InvoiceSubject) Response() *InvoiceSubjectResponse {
	if s == nil || s.Id == 0 {
		return nil
	}
	idNo := s.idNo()
	taxNo := s.taxNo()
	return &InvoiceSubjectResponse{
		Id:             s.Id,
		UserId:         s.UserId,
		SubjectType:    s.SubjectType,
		Status:         s.Status,
		RealName:       s.RealName,
		CompanyName:    s.CompanyName,
		IdNo:           idNo,
		TaxNo:          taxNo,
		MaskedIdNo:     maskInvoiceSecret(idNo, 3, 4),
		MaskedTaxNo:    maskInvoiceSecret(taxNo, 4, 4),
		CertificateUrl: s.CertificateUrl,
		ValidFrom:      s.ValidFrom,
		ValidUntil:     s.ValidUntil,
		ReviewedBy:     s.ReviewedBy,
		ReviewedAt:     s.ReviewedAt,
		RejectReason:   s.RejectReason,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
}

func invoiceJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func invoiceHMAC(v string) string {
	if v == "" {
		return ""
	}
	return common.GenerateHMAC(v)
}

func GetInvoiceSubjectByUserId(userId int) (*InvoiceSubject, error) {
	subject := &InvoiceSubject{}
	err := DB.Where("user_id = ?", userId).First(subject).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return subject, err
}

func GetInvoiceDefaultEmail(userId int) string {
	if userId <= 0 {
		return ""
	}
	preference := &InvoiceUserPreference{}
	if err := DB.Where("user_id = ?", userId).First(preference).Error; err != nil {
		return ""
	}
	return preference.DefaultEmail
}

func saveInvoiceDefaultEmail(tx *gorm.DB, userId int, email string) error {
	email = strings.TrimSpace(email)
	if userId <= 0 || email == "" {
		return nil
	}
	now := common.GetTimestamp()
	preference := &InvoiceUserPreference{}
	err := tx.Where("user_id = ?", userId).First(preference).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&InvoiceUserPreference{
			UserId:       userId,
			DefaultEmail: email,
			CreatedAt:    now,
			UpdatedAt:    now,
		}).Error
	}
	if err != nil {
		return err
	}
	return tx.Model(preference).Updates(map[string]interface{}{
		"default_email": email,
		"updated_at":    now,
	}).Error
}

func SaveInvoiceSubject(userId int, subjectType, realName, companyName, idNo, taxNo, certificateUrl string, validFrom, validUntil int64) (*InvoiceSubject, error) {
	now := common.GetTimestamp()
	if subjectType != InvoiceSubjectTypePersonal && subjectType != InvoiceSubjectTypeCompany {
		return nil, errors.New("认证类型错误")
	}
	if validUntil != 0 && validUntil <= now {
		return nil, errors.New("证件有效期必须晚于当前时间")
	}
	if subjectType == InvoiceSubjectTypePersonal {
		if strings.TrimSpace(realName) == "" || strings.TrimSpace(idNo) == "" {
			return nil, errors.New("请填写个人姓名和证件号码")
		}
		companyName = ""
		taxNo = ""
	} else {
		if strings.TrimSpace(companyName) == "" || strings.TrimSpace(taxNo) == "" {
			return nil, errors.New("请填写企业名称和纳税人识别号")
		}
		realName = ""
		idNo = ""
	}
	idCipher, err := encryptInvoiceSecret(strings.TrimSpace(idNo))
	if err != nil {
		return nil, err
	}
	taxCipher, err := encryptInvoiceSecret(strings.TrimSpace(taxNo))
	if err != nil {
		return nil, err
	}
	subject := &InvoiceSubject{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("user_id = ?", userId).First(subject).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if subject.Id > 0 {
			if subject.Status == InvoiceSubjectStatusVerified && (subject.ValidUntil == 0 || subject.ValidUntil >= now) {
				return errors.New("认证有效期内不允许修改认证信息")
			}
			if subject.SubjectType != "" && subject.SubjectType != subjectType {
				return errors.New("企业/个人认证仅支持一种类型，不能切换")
			}
			subject.SubjectType = subjectType
			subject.Status = InvoiceSubjectStatusPending
			subject.RealName = strings.TrimSpace(realName)
			subject.CompanyName = strings.TrimSpace(companyName)
			subject.IdNoCipher = idCipher
			subject.TaxNoCipher = taxCipher
			subject.IdNoHash = invoiceHMAC(strings.TrimSpace(idNo))
			subject.TaxNoHash = invoiceHMAC(strings.TrimSpace(taxNo))
			subject.CertificateUrl = strings.TrimSpace(certificateUrl)
			subject.ValidFrom = validFrom
			subject.ValidUntil = validUntil
			subject.ReviewedBy = 0
			subject.ReviewedAt = 0
			subject.RejectReason = ""
			subject.UpdatedAt = now
			return tx.Save(subject).Error
		}
		*subject = InvoiceSubject{
			UserId:         userId,
			SubjectType:    subjectType,
			Status:         InvoiceSubjectStatusPending,
			RealName:       strings.TrimSpace(realName),
			CompanyName:    strings.TrimSpace(companyName),
			IdNoCipher:     idCipher,
			TaxNoCipher:    taxCipher,
			IdNoHash:       invoiceHMAC(strings.TrimSpace(idNo)),
			TaxNoHash:      invoiceHMAC(strings.TrimSpace(taxNo)),
			CertificateUrl: strings.TrimSpace(certificateUrl),
			ValidFrom:      validFrom,
			ValidUntil:     validUntil,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		return tx.Create(subject).Error
	})
	return subject, err
}

func ReviewInvoiceSubject(id int, status, reason string, adminId int) error {
	if status != InvoiceSubjectStatusVerified && status != InvoiceSubjectStatusRejected {
		return errors.New("认证审核状态错误")
	}
	now := common.GetTimestamp()
	return DB.Model(&InvoiceSubject{}).Where("id = ?", id).Updates(map[string]any{
		"status":        status,
		"reject_reason": strings.TrimSpace(reason),
		"reviewed_by":   adminId,
		"reviewed_at":   now,
		"updated_at":    now,
	}).Error
}

func ListInvoiceSubjects(pageInfo *common.PageInfo, status string, keyword string) ([]*InvoiceSubjectResponse, int64, error) {
	var subjects []*InvoiceSubject
	var total int64
	query := DB.Model(&InvoiceSubject{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("real_name LIKE ? OR company_name LIKE ?", like, like)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&subjects).Error; err != nil {
		return nil, 0, err
	}
	responses := make([]*InvoiceSubjectResponse, 0, len(subjects))
	for _, subject := range subjects {
		resp := subject.Response()
		if resp != nil {
			resp.Username, _ = GetUsernameById(subject.UserId, false)
		}
		responses = append(responses, resp)
	}
	return responses, total, nil
}

func normalizeInvoicePaymentChannel(topUp *TopUp) string {
	if topUp == nil {
		return ""
	}
	channel := strings.TrimSpace(topUp.PaymentChannel)
	if channel == "" {
		channel = strings.TrimSpace(topUp.PaymentMethod)
	}
	return strings.ToLower(channel)
}

func invoiceQuotaForTopUp(topUp *TopUp) int64 {
	if topUp == nil {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(topUp.PaymentMethod)) {
	case "creem":
		return topUp.Amount
	case "stripe":
		return int64(topUp.Money * common.QuotaPerUnit)
	default:
		return int64(float64(topUp.Amount) * common.QuotaPerUnit)
	}
}

func invoiceStatusForTopUp(tx *gorm.DB, topUpId int) (string, error) {
	var status string
	err := tx.Table("invoice_application_items").
		Select("invoice_applications.status").
		Joins("JOIN invoice_applications ON invoice_application_items.application_id = invoice_applications.id").
		Where("invoice_application_items.top_up_id = ?", topUpId).
		Order("invoice_application_items.id desc").
		Limit(1).
		Scan(&status).Error
	return status, err
}

func invoiceAvailableByStatus(status string) bool {
	switch status {
	case "", InvoiceApplicationStatusRejected, InvoiceApplicationStatusCancelled, InvoiceApplicationStatusFailed, InvoiceApplicationStatusRedIssued:
		return true
	default:
		return false
	}
}

func ListInvoiceTopUps(userId int, pageInfo *common.PageInfo, keyword string) ([]*InvoiceTopUpResponse, int64, error) {
	var topUps []*TopUp
	var total int64
	query := DB.Model(&TopUp{}).Where("user_id = ? AND status = ? AND money > 0 AND create_time >= ?", userId, common.TopUpStatusSuccess, InvoiceTopUpAvailableStartTime)
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("trade_no LIKE ?", like)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topUps).Error; err != nil {
		return nil, 0, err
	}
	username, _ := GetUsernameById(userId, false)
	items := make([]*InvoiceTopUpResponse, 0, len(topUps))
	for _, topUp := range topUps {
		invoiceStatus, err := invoiceStatusForTopUp(DB, topUp.Id)
		if err != nil {
			return nil, 0, err
		}
		channel := normalizeInvoicePaymentChannel(topUp)
		items = append(items, &InvoiceTopUpResponse{
			Id:               topUp.Id,
			UserId:           topUp.UserId,
			Username:         username,
			Amount:          invoiceQuotaForTopUp(topUp),
			Money:            topUp.Money,
			TradeNo:          topUp.TradeNo,
			PaymentMethod:    topUp.PaymentMethod,
			PaymentChannel:   channel,
			Status:           topUp.Status,
			CreateTime:       topUp.CreateTime,
			CompleteTime:     topUp.CompleteTime,
			InvoiceStatus:    invoiceStatus,
			InvoiceAvailable: invoiceAvailableByStatus(invoiceStatus),
		})
	}
	return items, total, nil
}

func defaultInvoiceProviderConfigs() map[string]*InvoiceProviderConfig {
	now := common.GetTimestamp()
	return map[string]*InvoiceProviderConfig{
		InvoiceProviderManual: {
			ProviderCode:             InvoiceProviderManual,
			Name:                     "人工开票",
			Enabled:                  true,
			SupportedPaymentChannels: `["*"]`,
			AllowCrossChannel:        true,
			SupportsCreate:           true,
			SupportsRed:              true,
			SupportsQuery:            true,
			SupportsDownload:         true,
			SupportsEmailForward:     true,
			CreatedAt:                now,
			UpdatedAt:                now,
		},
		InvoiceProviderAlipay: {
			ProviderCode:             InvoiceProviderAlipay,
			Name:                     "支付宝开票",
			Enabled:                  false,
			SupportedPaymentChannels: `["alipay","epay","extpay"]`,
			SupportsCreate:           true,
			SupportsRed:              true,
			SupportsQuery:            true,
			SupportsDownload:         true,
			SupportsEmailForward:     true,
			CreatedAt:                now,
			UpdatedAt:                now,
		},
		InvoiceProviderWechat: {
			ProviderCode:             InvoiceProviderWechat,
			Name:                     "微信支付开票",
			Enabled:                  false,
			SupportedPaymentChannels: `["wxpay","wechat"]`,
			SupportsCreate:           true,
			SupportsRed:              true,
			SupportsQuery:            true,
			SupportsDownload:         true,
			SupportsEmailForward:     true,
			CreatedAt:                now,
			UpdatedAt:                now,
		},
		InvoiceProviderFapiaoCloud: {
			ProviderCode:             InvoiceProviderFapiaoCloud,
			Name:                     "发票云",
			Enabled:                  false,
			SupportedPaymentChannels: `["*"]`,
			AllowCrossChannel:        true,
			SupportsCreate:           true,
			SupportsRed:              true,
			SupportsQuery:            true,
			SupportsDownload:         true,
			SupportsEmailForward:     true,
			CreatedAt:                now,
			UpdatedAt:                now,
		},
		InvoiceProviderNuonuo: {
			ProviderCode:             InvoiceProviderNuonuo,
			Name:                     "诺诺开放平台",
			Enabled:                  false,
			SupportedPaymentChannels: `["*"]`,
			AllowCrossChannel:        true,
			SupportsCreate:           true,
			SupportsRed:              true,
			SupportsQuery:            true,
			SupportsDownload:         true,
			SupportsEmailForward:     true,
			CreatedAt:                now,
			UpdatedAt:                now,
		},
	}
}

func GetInvoiceProviderConfig(providerCode string) (*InvoiceProviderConfig, error) {
	config := &InvoiceProviderConfig{}
	err := DB.Where("provider_code = ?", providerCode).First(config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if def, ok := defaultInvoiceProviderConfigs()[providerCode]; ok {
			return def, nil
		}
		return nil, nil
	}
	return config, err
}

func providerSupportsChannel(config *InvoiceProviderConfig, channel string) bool {
	if config == nil {
		return false
	}
	if config.AllowCrossChannel {
		return true
	}
	var channels []string
	_ = json.Unmarshal([]byte(config.SupportedPaymentChannels), &channels)
	if len(channels) == 0 {
		return false
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	for _, c := range channels {
		c = strings.ToLower(strings.TrimSpace(c))
		if c == "*" || c == channel || (c != "" && strings.Contains(channel, c)) {
			return true
		}
	}
	return false
}

func ResolveInvoiceProviderCode(paymentChannel string) string {
	channel := strings.ToLower(paymentChannel)
	candidates := []string{InvoiceProviderManual}
	if strings.Contains(channel, "wechat") || strings.Contains(channel, "wx") {
		candidates = []string{InvoiceProviderWechat, InvoiceProviderManual}
	} else if strings.Contains(channel, "alipay") || strings.Contains(channel, "epay") || strings.Contains(channel, "extpay") {
		candidates = []string{InvoiceProviderAlipay, InvoiceProviderManual}
	}
	for _, code := range candidates {
		config, err := GetInvoiceProviderConfig(code)
		if err != nil || config == nil || !config.Enabled || !config.SupportsCreate {
			continue
		}
		if providerSupportsChannel(config, paymentChannel) {
			return code
		}
	}
	return InvoiceProviderManual
}

func CreateInvoiceApplication(userId int, topUpIds []int, email string, providerCode string) (*InvoiceApplication, error) {
	if len(topUpIds) == 0 {
		return nil, errors.New("请选择需要开票的充值订单")
	}
	now := common.GetTimestamp()
	subject, err := GetInvoiceSubjectByUserId(userId)
	if err != nil {
		return nil, err
	}
	if subject == nil || subject.Status != InvoiceSubjectStatusVerified {
		return nil, errors.New("请先完成实名认证")
	}
	if subject.ValidUntil > 0 && subject.ValidUntil < now {
		return nil, errors.New("实名认证已过有效期，请重新认证")
	}
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		return nil, errors.New("请填写接收发票的邮箱")
	}
	var app *InvoiceApplication
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := saveInvoiceDefaultEmail(tx, userId, email); err != nil {
			return err
		}
		var topUps []*TopUp
		if err := tx.Where("user_id = ? AND id IN ? AND status = ? AND money > 0 AND create_time >= ?", userId, topUpIds, common.TopUpStatusSuccess, InvoiceTopUpAvailableStartTime).Find(&topUps).Error; err != nil {
			return err
		}
		if len(topUps) != len(topUpIds) {
			return errors.New("存在不可开票的充值订单")
		}
		channel := ""
		amount := 0.0
		for _, topUp := range topUps {
			topUpChannel := normalizeInvoicePaymentChannel(topUp)
			if channel == "" {
				channel = topUpChannel
			}
			if channel != topUpChannel {
				return errors.New("不同支付渠道的充值订单不能合并开票")
			}
			var count int64
			if err := tx.Model(&InvoiceApplicationItem{}).
				Joins("JOIN invoice_applications ON invoice_application_items.application_id = invoice_applications.id").
				Where("invoice_application_items.top_up_id = ? AND invoice_applications.status IN ?", topUp.Id, activeInvoiceStatuses).
				Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return fmt.Errorf("充值订单 %s 已提交开票申请", topUp.TradeNo)
			}
			amount += topUp.Money
		}
		if providerCode == "" {
			providerCode = ResolveInvoiceProviderCode(channel)
		}
		config, err := GetInvoiceProviderConfig(providerCode)
		if err != nil {
			return err
		}
		if config == nil || !config.Enabled || !config.SupportsCreate || !providerSupportsChannel(config, channel) {
			providerCode = InvoiceProviderManual
		}
		snapshot := map[string]any{
			"subject_type":     subject.SubjectType,
			"real_name":        subject.RealName,
			"company_name":     subject.CompanyName,
			"id_no":            subject.idNo(),
			"tax_no":           subject.taxNo(),
			"masked_id_no":     maskInvoiceSecret(subject.idNo(), 3, 4),
			"masked_tax_no":    maskInvoiceSecret(subject.taxNo(), 4, 4),
			"certificate_url":  subject.CertificateUrl,
			"valid_from":       subject.ValidFrom,
			"valid_until":      subject.ValidUntil,
			"id_no_hash":       subject.IdNoHash,
			"tax_no_hash":      subject.TaxNoHash,
			"provider_warning": "发票抬头与认证主体一致，不支持手动改写",
		}
		app = &InvoiceApplication{
			UserId:          userId,
			SubjectId:       subject.Id,
			SubjectType:     subject.SubjectType,
			InvoiceType:     InvoiceTypeVatNormal,
			Title:           subject.title(),
			Amount:          amount,
			Email:           email,
			PaymentChannel:  channel,
			ProviderCode:    providerCode,
			SubjectSnapshot: invoiceJSON(snapshot),
			Status:          InvoiceApplicationStatusPending,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.Create(app).Error; err != nil {
			return err
		}
		for _, topUp := range topUps {
			item := &InvoiceApplicationItem{
				ApplicationId: app.Id,
				TopUpId:       topUp.Id,
				TradeNo:       topUp.TradeNo,
				Amount:        topUp.Money,
				CreatedAt:     now,
			}
			if err := tx.Create(item).Error; err != nil {
				return err
			}
		}
		task := &InvoiceProviderTask{
			ApplicationId:   app.Id,
			ProviderCode:    providerCode,
			Operation:       InvoiceProviderTaskOpCreate,
			Status:          InvoiceApplicationStatusPending,
			RequestSnapshot: invoiceJSON(map[string]any{"topup_ids": topUpIds, "email": email, "payment_channel": channel}),
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		return tx.Create(&InvoiceOperationLog{
			ApplicationId: app.Id,
			SubjectId:     subject.Id,
			UserId:        userId,
			Operation:     "apply",
			Content:       invoiceJSON(map[string]any{"topup_ids": topUpIds, "amount": amount, "provider": providerCode}),
			CreatedAt:     now,
		}).Error
	})
	return app, err
}

func buildInvoiceApplicationResponse(app *InvoiceApplication, includeItems bool) (*InvoiceApplicationResponse, error) {
	if app == nil {
		return nil, nil
	}
	username, _ := GetUsernameById(app.UserId, false)
	resp := &InvoiceApplicationResponse{
		Id:             app.Id,
		UserId:         app.UserId,
		Username:       username,
		SubjectId:      app.SubjectId,
		SubjectType:    app.SubjectType,
		InvoiceType:    app.InvoiceType,
		Title:          app.Title,
		Amount:         app.Amount,
		Email:          app.Email,
		PaymentChannel: app.PaymentChannel,
		ProviderCode:   app.ProviderCode,
		InvoiceNo:      app.InvoiceNo,
		InvoiceFileUrl: app.InvoiceFileUrl,
		Status:         app.Status,
		RejectReason:   app.RejectReason,
		CreatedAt:      app.CreatedAt,
		UpdatedAt:      app.UpdatedAt,
		IssuedAt:       app.IssuedAt,
	}
	if strings.TrimSpace(app.SubjectSnapshot) != "" {
		var snapshot map[string]interface{}
		if err := json.Unmarshal([]byte(app.SubjectSnapshot), &snapshot); err == nil {
			resp.SubjectSnapshot = snapshot
		}
	}
	var subject InvoiceSubject
	if err := DB.Where("id = ?", app.SubjectId).First(&subject).Error; err == nil {
		resp.Subject = subject.Response()
	}
	if includeItems {
		var items []*InvoiceApplicationItem
		if err := DB.Where("application_id = ?", app.Id).Order("id asc").Find(&items).Error; err != nil {
			return nil, err
		}
		resp.Items = items
	}
	return resp, nil
}

func ListInvoiceApplications(userId int, pageInfo *common.PageInfo, status string, keyword string, includeItems bool) ([]*InvoiceApplicationResponse, int64, error) {
	var apps []*InvoiceApplication
	var total int64
	query := DB.Model(&InvoiceApplication{})
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where(
			"title LIKE ? OR invoice_no LIKE ? OR email LIKE ? OR EXISTS (SELECT 1 FROM invoice_application_items WHERE invoice_application_items.application_id = invoice_applications.id AND invoice_application_items.trade_no LIKE ?)",
			like, like, like, like,
		)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&apps).Error; err != nil {
		return nil, 0, err
	}
	responses := make([]*InvoiceApplicationResponse, 0, len(apps))
	for _, app := range apps {
		resp, err := buildInvoiceApplicationResponse(app, includeItems)
		if err != nil {
			return nil, 0, err
		}
		responses = append(responses, resp)
	}
	return responses, total, nil
}

func CancelInvoiceApplication(userId int, id int) error {
	now := common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		app := &InvoiceApplication{}
		if err := tx.Where("id = ? AND user_id = ?", id, userId).First(app).Error; err != nil {
			return err
		}
		if app.Status != InvoiceApplicationStatusPending {
			return errors.New("仅待处理的开票申请可取消")
		}
		if err := tx.Model(app).Updates(map[string]any{"status": InvoiceApplicationStatusCancelled, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Create(&InvoiceOperationLog{
			ApplicationId: app.Id,
			SubjectId:     app.SubjectId,
			UserId:        app.UserId,
			Operation:     "cancel",
			Content:       "用户取消开票申请",
			CreatedAt:     now,
		}).Error
	})
}

func RequestInvoiceEmailForward(userId int, id int, email string) error {
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		return errors.New("请填写接收发票的邮箱")
	}
	now := common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		app := &InvoiceApplication{}
		if err := tx.Where("id = ? AND user_id = ?", id, userId).First(app).Error; err != nil {
			return err
		}
		if app.Status != InvoiceApplicationStatusIssued {
			return errors.New("仅已开票记录支持转发邮箱")
		}
		if err := tx.Create(&InvoiceProviderTask{
			ApplicationId:   app.Id,
			ProviderCode:    app.ProviderCode,
			Operation:       InvoiceProviderTaskOpEmailForward,
			Status:          InvoiceApplicationStatusPending,
			RequestSnapshot: invoiceJSON(map[string]any{"email": email}),
			CreatedAt:       now,
			UpdatedAt:       now,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&InvoiceOperationLog{
			ApplicationId: app.Id,
			SubjectId:     app.SubjectId,
			UserId:        app.UserId,
			Operation:     "email_forward",
			Content:       invoiceJSON(map[string]any{"email": email}),
			CreatedAt:     now,
		}).Error
	})
}

func sendInvoiceIssuedEmail(app *InvoiceApplication, invoiceFileUrl string) error {
	if app == nil {
		return nil
	}
	email := strings.TrimSpace(app.Email)
	invoiceFileUrl = strings.TrimSpace(invoiceFileUrl)
	if email == "" || invoiceFileUrl == "" {
		return nil
	}
	title := strings.TrimSpace(app.Title)
	if title == "" {
		title = "增值税普通发票"
	}
	invoiceNo := strings.TrimSpace(app.InvoiceNo)
	if invoiceNo == "" {
		invoiceNo = "-"
	}
	content := fmt.Sprintf(
		`<p>您好，您的发票已开具。</p>
<p>发票抬头：%s</p>
<p>发票号：%s</p>
<p>发票金额：¥%.2f</p>
<p><a href="%s" target="_blank" rel="noopener noreferrer">点击下载发票</a></p>
<p>如链接无法打开，请复制以下地址到浏览器访问：</p>
<p>%s</p>`,
		html.EscapeString(title),
		html.EscapeString(invoiceNo),
		app.Amount,
		html.EscapeString(invoiceFileUrl),
		html.EscapeString(invoiceFileUrl),
	)
	return common.SendEmail("发票已开具", email, content)
}

func UpdateInvoiceApplicationByAdmin(id int, adminId int, status string, invoiceNo string, invoiceFileUrl string, fileType string, rejectReason string) error {
	allowed := map[string]bool{
		InvoiceApplicationStatusPending:    true,
		InvoiceApplicationStatusApproved:   true,
		InvoiceApplicationStatusIssued:     true,
		InvoiceApplicationStatusRejected:   true,
		InvoiceApplicationStatusFailed:     true,
		InvoiceApplicationStatusRedPending: true,
		InvoiceApplicationStatusRedIssued:  true,
	}
	if !allowed[status] {
		return errors.New("开票状态错误")
	}
	now := common.GetTimestamp()
	var issuedApp *InvoiceApplication
	issuedFileUrl := strings.TrimSpace(invoiceFileUrl)
	err := DB.Transaction(func(tx *gorm.DB) error {
		app := &InvoiceApplication{}
		if err := tx.Where("id = ?", id).First(app).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"status":        status,
			"invoice_no":    strings.TrimSpace(invoiceNo),
			"reject_reason": strings.TrimSpace(rejectReason),
			"updated_at":    now,
		}
		if status == InvoiceApplicationStatusIssued {
			updates["issued_at"] = now
			if issuedFileUrl == "" {
				issuedFileUrl = strings.TrimSpace(app.InvoiceFileUrl)
			}
			if issuedFileUrl == "" {
				return errors.New("已开票状态需要填写发票文件链接")
			}
			app.Status = status
			app.InvoiceNo = strings.TrimSpace(invoiceNo)
			app.InvoiceFileUrl = issuedFileUrl
			app.IssuedAt = now
			issuedApp = app
		}
		if status == InvoiceApplicationStatusRedIssued {
			updates["red_issued_at"] = now
		}
		if invoiceFileUrl != "" {
			updates["invoice_file_url"] = strings.TrimSpace(invoiceFileUrl)
		}
		if err := tx.Model(app).Updates(updates).Error; err != nil {
			return err
		}
		if invoiceFileUrl != "" {
			if fileType == "" {
				fileType = "pdf"
			}
			if err := tx.Create(&InvoiceFile{
				ApplicationId: app.Id,
				FileType:      fileType,
				SourceUrl:     strings.TrimSpace(invoiceFileUrl),
				StorageUrl:    strings.TrimSpace(invoiceFileUrl),
				CreatedAt:     now,
			}).Error; err != nil {
				return err
			}
		}
		return tx.Create(&InvoiceOperationLog{
			ApplicationId: app.Id,
			SubjectId:     app.SubjectId,
			UserId:        app.UserId,
			OperatorId:    adminId,
			Operation:     "admin_update",
			Content:       invoiceJSON(updates),
			CreatedAt:     now,
		}).Error
	})
	if err != nil {
		return err
	}
	if issuedApp != nil && issuedFileUrl != "" && strings.TrimSpace(issuedApp.Email) != "" {
		if err := sendInvoiceIssuedEmail(issuedApp, issuedFileUrl); err != nil {
			_ = DB.Create(&InvoiceOperationLog{
				ApplicationId: issuedApp.Id,
				SubjectId:     issuedApp.SubjectId,
				UserId:        issuedApp.UserId,
				OperatorId:    adminId,
				Operation:     "email_auto_failed",
				Content:       invoiceJSON(map[string]any{"email": issuedApp.Email, "error": err.Error()}),
				CreatedAt:     common.GetTimestamp(),
			}).Error
			return fmt.Errorf("开票状态已更新，但发票邮件发送失败：%w", err)
		}
		_ = DB.Create(&InvoiceOperationLog{
			ApplicationId: issuedApp.Id,
			SubjectId:     issuedApp.SubjectId,
			UserId:        issuedApp.UserId,
			OperatorId:    adminId,
			Operation:     "email_auto_sent",
			Content:       invoiceJSON(map[string]any{"email": issuedApp.Email}),
			CreatedAt:     common.GetTimestamp(),
		}).Error
	}
	return nil
}

func ListInvoiceProviderConfigs() ([]*InvoiceProviderConfig, error) {
	defaults := defaultInvoiceProviderConfigs()
	var configs []*InvoiceProviderConfig
	if err := DB.Order("id asc").Find(&configs).Error; err != nil {
		return nil, err
	}
	existing := make(map[string]*InvoiceProviderConfig)
	for _, config := range configs {
		existing[config.ProviderCode] = config
	}
	for code, def := range defaults {
		if _, ok := existing[code]; !ok {
			configs = append(configs, def)
		}
	}
	return configs, nil
}

func SaveInvoiceProviderConfig(config *InvoiceProviderConfig) error {
	now := common.GetTimestamp()
	if config.ProviderCode == "" {
		return errors.New("provider_code 不能为空")
	}
	if config.Name == "" {
		config.Name = config.ProviderCode
	}
	if config.SupportedPaymentChannels == "" {
		config.SupportedPaymentChannels = `[]`
	}
	if !json.Valid([]byte(config.SupportedPaymentChannels)) {
		return errors.New("supported_payment_channels 必须是 JSON 数组")
	}
	existing := &InvoiceProviderConfig{}
	err := DB.Where("provider_code = ?", config.ProviderCode).First(existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	config.UpdatedAt = now
	if existing.Id > 0 {
		config.Id = existing.Id
		config.CreatedAt = existing.CreatedAt
		return DB.Save(config).Error
	}
	config.CreatedAt = now
	return DB.Create(config).Error
}
