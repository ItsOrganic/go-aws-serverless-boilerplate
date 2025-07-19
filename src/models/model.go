package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	UserID           uuid.UUID `json:"user_id" db:"user_id"`
	Name             string    `json:"name" db:"name" validate:"required"`
	Email            string    `json:"email" db:"email" validate:"required,email"`
	MonthlyIncome    *float64  `json:"monthly_income" db:"monthly_income" validate:"omitempty,min=0"`
	CreditScore      *int      `json:"credit_score" db:"credit_score" validate:"omitempty,min=300,max=900"`
	EmploymentStatus string    `json:"employment_status" db:"employment_status"`
	Age              *int      `json:"age" db:"age" validate:"omitempty,min=18,max=120"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

type UserResponse struct {
	Success bool `json:"success"`
	User    User `json:"user"`
}

type Config struct {
	Db     DBConfig
	APIKey string
}

type DBConfig struct {
	User     string
	Pass     string
	Host     string
	Port     int
	Database string
	Driver   string
	SslMode  string
}

type CSVUploadResponse struct {
	Success       bool     `json:"success"`
	TotalRows     int      `json:"total_rows"`
	ProcessedRows int      `json:"processed_rows"`
	FailedRows    int      `json:"failed_rows"`
	Errors        []string `json:"errors,omitempty"`
	Message       string   `json:"message"`
}

// Enhanced CSV Upload tracking
type CSVUpload struct {
	UploadID            uuid.UUID  `json:"upload_id" db:"upload_id"`
	S3Key               string     `json:"s3_key" db:"s3_key"`
	Filename            string     `json:"filename" db:"filename"`
	FileSize            int64      `json:"file_size" db:"file_size"`
	Status              string     `json:"status" db:"status"` // pending, processing, completed, failed, partial
	TotalRows           int        `json:"total_rows" db:"total_rows"`
	ProcessedRows       int        `json:"processed_rows" db:"processed_rows"`
	FailedRows          int        `json:"failed_rows" db:"failed_rows"`
	UploadedBy          string     `json:"uploaded_by" db:"uploaded_by"`
	UploadStartedAt     time.Time  `json:"upload_started_at" db:"upload_started_at"`
	ProcessingStartedAt *time.Time `json:"processing_started_at" db:"processing_started_at"`
	UploadCompletedAt   *time.Time `json:"upload_completed_at" db:"upload_completed_at"`
	ErrorSummary        *string    `json:"error_summary" db:"error_summary"`
}

type CSVUploadError struct {
	ErrorID      uuid.UUID `json:"error_id" db:"error_id"`
	UploadID     uuid.UUID `json:"upload_id" db:"upload_id"`
	RowNumber    int       `json:"row_number" db:"row_number"`
	RawData      string    `json:"raw_data" db:"raw_data"`
	ErrorMessage string    `json:"error_message" db:"error_message"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type S3UploadRequest struct {
	Filename   string `json:"filename" binding:"required"`
	UploadedBy string `json:"uploaded_by" binding:"required"`
}

type S3UploadResponse struct {
	Success   bool      `json:"success"`
	UploadID  uuid.UUID `json:"upload_id"`
	UploadURL string    `json:"upload_url"`
	S3Key     string    `json:"s3_key"`
	Message   string    `json:"message"`
}

type LoanProduct struct {
	ProductID        uuid.UUID `json:"product_id" db:"product_id"`
	ProductName      string    `json:"product_name" db:"product_name"`
	LenderName       string    `json:"lender_name" db:"lender_name"`
	InterestRateMin  *float64  `json:"interest_rate_min" db:"interest_rate_min"`
	InterestRateMax  *float64  `json:"interest_rate_max" db:"interest_rate_max"`
	LoanAmountMin    *float64  `json:"loan_amount_min" db:"loan_amount_min"`
	LoanAmountMax    *float64  `json:"loan_amount_max" db:"loan_amount_max"`
	MinMonthlyIncome *float64  `json:"min_monthly_income" db:"min_monthly_income"`
	MinCreditScore   *int      `json:"min_credit_score" db:"min_credit_score"`
	MaxCreditScore   *int      `json:"max_credit_score" db:"max_credit_score"`
	EmploymentTypes  []string  `json:"employment_types" db:"employment_types"`
	MinAge           *int      `json:"min_age" db:"min_age"`
	MaxAge           *int      `json:"max_age" db:"max_age"`
	SourceURL        string    `json:"source_url" db:"source_url"`
	IsActive         bool      `json:"is_active" db:"is_active"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

type UserLoanMatch struct {
	MatchID           uuid.UUID `json:"match_id" db:"match_id"`
	UserID            uuid.UUID `json:"user_id" db:"user_id"`
	ProductID         uuid.UUID `json:"product_id" db:"product_id"`
	MatchScore        float64   `json:"match_score" db:"match_score"`
	EligibilityStatus string    `json:"eligibility_status" db:"eligibility_status"`
	MatchReasons      []string  `json:"match_reasons" db:"match_reasons"`
	AIAnalysis        *string   `json:"ai_analysis" db:"ai_analysis"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
}
