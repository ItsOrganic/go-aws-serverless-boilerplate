package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"loanmatch/src/database"
	"loanmatch/src/models"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

type N8NHandler struct {
	DB *sql.DB
}

func NewN8NHandler() *N8NHandler {
	return &N8NHandler{DB: database.Db}
}

// nonNil turns a missing JSON array into an empty one; pq.Array(nil) would send NULL.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Webhook endpoint for triggering user-loan matching after CSV upload
func (h *N8NHandler) TriggerUserLoanMatching(c *gin.Context) {
	var request struct {
		UploadID string `json:"upload_id"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	// Verify upload exists and is completed
	query := `
			SELECT upload_id, status, processed_rows 
			FROM csv_uploads 
			WHERE upload_id = $1 AND status IN ('completed', 'partial')
		`

	var uploadID string
	var status string
	var processedRows int

	err := h.DB.QueryRow(query, request.UploadID).Scan(&uploadID, &status, &processedRows)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Upload not found or not completed",
		})
		return
	}

	// Return data for n8n workflow
	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"upload_id":        uploadID,
		"status":           status,
		"processed_rows":   processedRows,
		"trigger_matching": true,
		"message":          "User-loan matching workflow can proceed",
	})
}

// API endpoint to get newly added users for matching
func (h *N8NHandler) GetNewUsers(c *gin.Context) {
	// Get users added in the last 24 hours or since last matching run
	sinceParam := c.DefaultQuery("since", "24h")

	var query string
	var args []interface{}

	if sinceParam == "24h" {
		query = `
				SELECT user_id, name, email, monthly_income, credit_score, 
					employment_status, age, created_at
				FROM users 
				WHERE created_at >= NOW() - INTERVAL '24 hours'
				ORDER BY created_at DESC
			`
	} else {
		// Expect timestamp format
		query = `
				SELECT user_id, name, email, monthly_income, credit_score, 
					employment_status, age, created_at
				FROM users 
				WHERE created_at >= $1
				ORDER BY created_at DESC
			`
		since, err := time.Parse(time.RFC3339, sinceParam)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "since must be '24h' or an RFC3339 timestamp",
			})
			return
		}
		args = append(args, since)
	}

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		log.Printf("Error fetching users: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch users",
		})
		return
	}
	defer rows.Close()

	users := []models.User{}
	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.UserID, &user.Name, &user.Email,
			&user.MonthlyIncome, &user.CreditScore,
			&user.EmploymentStatus, &user.Age, &user.CreatedAt,
		)
		if err != nil {
			log.Printf("Error scanning user: %v", err)
			continue
		}
		users = append(users, user)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"users":   users,
		"count":   len(users),
	})
}

// API endpoint to get active loan products
func (h *N8NHandler) GetLoanProducts(c *gin.Context) {
	query := `
			SELECT product_id, product_name, lender_name, 
				interest_rate_min, interest_rate_max,
				loan_amount_min, loan_amount_max,
				min_monthly_income, min_credit_score, max_credit_score,
				employment_types, min_age, max_age,
				source_url, crawled_at
			FROM loan_products 
			WHERE is_active = true
			ORDER BY lender_name, product_name
		`

	rows, err := h.DB.Query(query)
	if err != nil {
		log.Printf("Error fetching loan products: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch loan products",
		})
		return
	}
	defer rows.Close()

	products := []map[string]interface{}{}
	for rows.Next() {
		var product map[string]interface{} = make(map[string]interface{})
		var productID, productName, lenderName string
		var interestRateMin, interestRateMax, loanAmountMin, loanAmountMax, minMonthlyIncome *float64
		var minCreditScore, maxCreditScore, minAge, maxAge *int
		var employmentTypes pq.StringArray
		var sourceURL *string
		var crawledAt time.Time

		err := rows.Scan(
			&productID, &productName, &lenderName,
			&interestRateMin, &interestRateMax,
			&loanAmountMin, &loanAmountMax,
			&minMonthlyIncome, &minCreditScore, &maxCreditScore,
			&employmentTypes, &minAge, &maxAge,
			&sourceURL, &crawledAt,
		)
		if err != nil {
			log.Printf("Error scanning loan product: %v", err)
			continue
		}

		product["product_id"] = productID
		product["product_name"] = productName
		product["lender_name"] = lenderName
		product["interest_rate_min"] = interestRateMin
		product["interest_rate_max"] = interestRateMax
		product["loan_amount_min"] = loanAmountMin
		product["loan_amount_max"] = loanAmountMax
		product["min_monthly_income"] = minMonthlyIncome
		product["min_credit_score"] = minCreditScore
		product["max_credit_score"] = maxCreditScore
		product["employment_types"] = []string(employmentTypes)
		product["min_age"] = minAge
		product["max_age"] = maxAge
		product["source_url"] = sourceURL
		product["crawled_at"] = crawledAt

		products = append(products, product)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"products": products,
		"count":    len(products),
	})
}

// API endpoint for bulk storing loan products (from web crawler)
func (h *N8NHandler) StoreLoanProducts(c *gin.Context) {
	var request struct {
		Products []struct {
			ProductName      string   `json:"product_name"`
			LenderName       string   `json:"lender_name"`
			InterestRateMin  *float64 `json:"interest_rate_min"`
			InterestRateMax  *float64 `json:"interest_rate_max"`
			LoanAmountMin    *float64 `json:"loan_amount_min"`
			LoanAmountMax    *float64 `json:"loan_amount_max"`
			MinMonthlyIncome *float64 `json:"min_monthly_income"`
			MinCreditScore   *int     `json:"min_credit_score"`
			MaxCreditScore   *int     `json:"max_credit_score"`
			EmploymentTypes  []string `json:"employment_types"`
			MinAge           *int     `json:"min_age"`
			MaxAge           *int     `json:"max_age"`
			SourceURL        string   `json:"source_url"`
		} `json:"products"`
		Source string `json:"source"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to start transaction",
		})
		return
	}
	defer tx.Rollback()

	// Deactivate existing products from this source
	if request.Source != "" {
		_, err = tx.Exec(`
				UPDATE loan_products 
				SET is_active = false, updated_at = NOW() 
				WHERE source_url LIKE $1
			`, "%"+request.Source+"%")
		if err != nil {
			log.Printf("Error deactivating existing products: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to deactivate existing products",
			})
			return
		}
	}

	stmt, err := tx.Prepare(`
			INSERT INTO loan_products (
				product_name, lender_name, interest_rate_min, interest_rate_max,
				loan_amount_min, loan_amount_max, min_monthly_income, 
				min_credit_score, max_credit_score, employment_types,
				min_age, max_age, source_url
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			ON CONFLICT (product_name, lender_name) DO UPDATE SET
				interest_rate_min = EXCLUDED.interest_rate_min,
				interest_rate_max = EXCLUDED.interest_rate_max,
				loan_amount_min = EXCLUDED.loan_amount_min,
				loan_amount_max = EXCLUDED.loan_amount_max,
				min_monthly_income = EXCLUDED.min_monthly_income,
				min_credit_score = EXCLUDED.min_credit_score,
				max_credit_score = EXCLUDED.max_credit_score,
				employment_types = EXCLUDED.employment_types,
				min_age = EXCLUDED.min_age,
				max_age = EXCLUDED.max_age,
				source_url = EXCLUDED.source_url,
				is_active = true,
				updated_at = NOW()
		`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to prepare statement",
		})
		return
	}
	defer stmt.Close()

	processed := 0
	failed := 0

	for _, product := range request.Products {
		err = database.ExecInSavepoint(tx, stmt,
			product.ProductName, product.LenderName,
			product.InterestRateMin, product.InterestRateMax,
			product.LoanAmountMin, product.LoanAmountMax,
			product.MinMonthlyIncome, product.MinCreditScore, product.MaxCreditScore,
			pq.Array(nonNil(product.EmploymentTypes)),
			product.MinAge, product.MaxAge, product.SourceURL,
		)

		if err != nil {
			log.Printf("Failed to insert product %s: %v", product.ProductName, err)
			failed++
		} else {
			processed++
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to commit transaction",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"processed": processed,
		"failed":    failed,
		"total":     len(request.Products),
		"message":   fmt.Sprintf("Successfully processed %d out of %d loan products", processed, len(request.Products)),
	})
}

// API endpoint for storing user-loan matches
func (h *N8NHandler) StoreUserLoanMatches(c *gin.Context) {
	var request struct {
		Matches []struct {
			UserID            string   `json:"user_id"`
			ProductID         string   `json:"product_id"`
			MatchScore        float64  `json:"match_score"`
			EligibilityStatus string   `json:"eligibility_status"`
			MatchReasons      []string `json:"match_reasons"`
			AIAnalysis        *string  `json:"ai_analysis,omitempty"`
		} `json:"matches"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to start transaction",
		})
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
			INSERT INTO user_loan_matches (
				user_id, product_id, match_score, eligibility_status, 
				match_reasons, ai_analysis
			) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (user_id, product_id) DO UPDATE SET
				match_score = EXCLUDED.match_score,
				eligibility_status = EXCLUDED.eligibility_status,
				match_reasons = EXCLUDED.match_reasons,
				ai_analysis = EXCLUDED.ai_analysis,
				created_at = NOW()
		`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to prepare statement",
		})
		return
	}
	defer stmt.Close()

	processed := 0
	failed := 0
	for _, match := range request.Matches {
		err = database.ExecInSavepoint(tx, stmt,
			match.UserID, match.ProductID, match.MatchScore,
			match.EligibilityStatus, pq.Array(nonNil(match.MatchReasons)),
			match.AIAnalysis,
		)
		if err != nil {
			log.Printf("Failed to insert match: %v", err)
			failed++
		} else {
			processed++
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to commit transaction",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"processed": processed,
		"failed":    failed,
		"total":     len(request.Matches),
	})
}

// Get users with new matches for notification
func (h *N8NHandler) GetUsersForNotification(c *gin.Context) {
	// Get users who have matches created in the last hour
	query := `
			SELECT DISTINCT u.user_id, u.name, u.email,
				COUNT(m.match_id) as match_count
			FROM users u
			JOIN user_loan_matches m ON u.user_id = m.user_id
			WHERE m.created_at >= NOW() - INTERVAL '1 hour'
			AND m.eligibility_status IN ('eligible', 'partially_eligible')
			GROUP BY u.user_id, u.name, u.email
			ORDER BY match_count DESC
		`

	rows, err := h.DB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch users for notification",
		})
		return
	}
	defer rows.Close()

	users := []map[string]interface{}{}
	for rows.Next() {
		var userID, name, email string
		var matchCount int

		err := rows.Scan(&userID, &name, &email, &matchCount)
		if err != nil {
			continue
		}

		users = append(users, map[string]interface{}{
			"user_id":     userID,
			"name":        name,
			"email":       email,
			"match_count": matchCount,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"users":   users,
		"count":   len(users),
	})
}

// Get loan matches for a specific user
func (h *N8NHandler) GetUserLoanMatches(c *gin.Context) {
	userID := c.Param("user_id")

	query := `
			SELECT m.match_id, m.match_score, m.eligibility_status, m.match_reasons,
				p.product_name, p.lender_name, p.interest_rate_min, p.interest_rate_max,
				p.loan_amount_min, p.loan_amount_max
			FROM user_loan_matches m
			JOIN loan_products p ON m.product_id = p.product_id
			WHERE m.user_id = $1 
			AND m.eligibility_status IN ('eligible', 'partially_eligible')
			ORDER BY m.match_score DESC
		`

	rows, err := h.DB.Query(query, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch user matches",
		})
		return
	}
	defer rows.Close()

	matches := []map[string]interface{}{}
	for rows.Next() {
		var matchID, eligibilityStatus, productName, lenderName string
		var matchScore float64
		var matchReasons pq.StringArray
		var interestRateMin, interestRateMax, loanAmountMin, loanAmountMax *float64

		err := rows.Scan(
			&matchID, &matchScore, &eligibilityStatus, &matchReasons,
			&productName, &lenderName, &interestRateMin, &interestRateMax,
			&loanAmountMin, &loanAmountMax,
		)
		if err != nil {
			continue
		}

		matches = append(matches, map[string]interface{}{
			"match_id":           matchID,
			"match_score":        matchScore,
			"eligibility_status": eligibilityStatus,
			"match_reasons":      []string(matchReasons),
			"product_name":       productName,
			"lender_name":        lenderName,
			"interest_rate_min":  interestRateMin,
			"interest_rate_max":  interestRateMax,
			"loan_amount_min":    loanAmountMin,
			"loan_amount_max":    loanAmountMax,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"matches": matches,
		"count":   len(matches),
	})
}

// Record notification sent
func (h *N8NHandler) RecordNotification(c *gin.Context) {
	var request struct {
		UserID               string `json:"user_id"`
		NotificationType     string `json:"notification_type"`
		EmailSubject         string `json:"email_subject"`
		EmailBody            string `json:"email_body"`
		MatchedProductsCount int    `json:"matched_products_count"`
		EmailStatus          string `json:"email_status"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
		})
		return
	}

	query := `
			INSERT INTO user_notifications (
				user_id, notification_type, email_subject, email_body,
				matched_products_count, email_status
			) VALUES ($1, $2, $3, $4, $5, $6)
		`

	_, err := h.DB.Exec(query,
		request.UserID, request.NotificationType, request.EmailSubject,
		request.EmailBody, request.MatchedProductsCount, request.EmailStatus,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to record notification",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Notification recorded successfully",
	})
}
