package models

import (
	"time"
)

// Corp represents the corporate master data
type Corp struct {
	CorpCode   string    `gorm:"primaryKey;column:corp_code;type:varchar(20)" json:"corp_code"`
	CorpName   string    `gorm:"column:corp_name;type:varchar(200);index" json:"corp_name"`
	StockCode  string    `gorm:"column:stock_code;type:varchar(20);index" json:"stock_code"` // Nullable in DB logic
	ModifiedAt time.Time `gorm:"column:modified_at" json:"modified_at"`
}

// Filing represents the disclosure metadata
type Filing struct {
	RceptNo   string    `gorm:"primaryKey;column:rcept_no;type:varchar(20)" json:"rcept_no"`
	CorpCode  string    `gorm:"column:corp_code;type:varchar(20);index" json:"corp_code"`
	CorpName  string    `gorm:"column:corp_name;type:varchar(200)" json:"corp_name"`
	ReportNm  string    `gorm:"column:report_nm;type:varchar(500)" json:"report_nm"`
	RceptDt   string    `gorm:"column:rcept_dt;type:varchar(8)" json:"rcept_dt"` // DART API returns YYYYMMDD string
	FlrNm     string    `gorm:"column:flr_nm;type:varchar(100)" json:"flr_nm"`
	Rm        string    `gorm:"column:rm;type:varchar(50)" json:"rm"`
	DcmNo     string    `gorm:"column:dcm_no;type:varchar(20)" json:"dcm_no"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// FilingDocument represents the physical files downloaded
type FilingDocument struct {
	ID          uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	RceptNo     string     `gorm:"column:rcept_no;type:varchar(20);index" json:"rcept_no"`
	DocType     string     `gorm:"column:doc_type;type:varchar(20)" json:"doc_type"` // MAIN_XML, ATTACH, PDF
	StorageURI  string     `gorm:"column:storage_uri;type:varchar(500)" json:"storage_uri"`
	SHA256      string     `gorm:"column:sha256;type:varchar(64)" json:"sha256"`
	FetchedAt   time.Time  `gorm:"autoCreateTime" json:"fetched_at"`
	ExtractedAt *time.Time `gorm:"column:extracted_at" json:"extracted_at"`
}

// ExtractedEvent represents structured data parsed from filings
type ExtractedEvent struct {
	ID                uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	RceptNo           string    `gorm:"column:rcept_no;type:varchar(20);index" json:"rcept_no"`
	EventType         string    `gorm:"column:event_type;type:varchar(100);index" json:"event_type"`
	PayloadJSON       string    `gorm:"column:payload_json;type:text" json:"payload_json"`
	EvidenceSpansJSON string    `gorm:"column:evidence_spans_json;type:text" json:"evidence_spans_json"`
	CreatedAt         time.Time `gorm:"autoCreateTime" json:"created_at"`
}
