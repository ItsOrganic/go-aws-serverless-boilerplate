package services

import (
	"loanmatch/src/models"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type ValidationResult struct {
	Valid  bool
	Errors []string
}

var emailRegex = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)

func ValidateUserCSVRecord(record []string, headers []string) (*models.User, *ValidationResult) {
	result := &ValidationResult{Valid: true, Errors: []string{}}
	user := &models.User{}

	// Create a map for easier field access
	fieldMap := make(map[string]string)
	for i, header := range headers {
		if i < len(record) {
			fieldMap[strings.ToLower(strings.TrimSpace(header))] = strings.TrimSpace(record[i])
		}
	}

	// Validate User ID (optional, generated when missing)
	if idStr := fieldMap["user_id"]; idStr != "" {
		if id, err := uuid.Parse(idStr); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, "invalid user_id format (expected UUID)")
		} else {
			user.UserID = id
		}
	} else {
		user.UserID = uuid.New()
	}

	// Validate Name (required)
	if name := fieldMap["name"]; name == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "name is required")
	} else {
		user.Name = name
	}

	// Validate Email (required)
	email := fieldMap["email"]
	if email == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "email is required")
	} else if !emailRegex.MatchString(strings.ToLower(email)) {
		result.Valid = false
		result.Errors = append(result.Errors, "invalid email format")
	} else {
		user.Email = strings.ToLower(email)
	}

	// Validate Monthly Income (optional)
	if incomeStr := fieldMap["monthly_income"]; incomeStr != "" {
		if income, err := strconv.ParseFloat(incomeStr, 64); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, "invalid monthly_income format")
		} else if income < 0 {
			result.Valid = false
			result.Errors = append(result.Errors, "monthly_income cannot be negative")
		} else {
			user.MonthlyIncome = &income
		}
	}

	// Validate Credit Score (optional, CIBIL range)
	if scoreStr := fieldMap["credit_score"]; scoreStr != "" {
		if score, err := strconv.Atoi(scoreStr); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, "invalid credit_score format")
		} else if score < 300 || score > 900 {
			result.Valid = false
			result.Errors = append(result.Errors, "credit_score must be between 300 and 900")
		} else {
			user.CreditScore = &score
		}
	}

	// Validate Employment Status (optional)
	if status := fieldMap["employment_status"]; status != "" {
		validStatuses := map[string]bool{
			"employed":      true,
			"unemployed":    true,
			"self-employed": true,
			"student":       true,
			"retired":       true,
		}
		if !validStatuses[strings.ToLower(status)] {
			result.Valid = false
			result.Errors = append(result.Errors, "invalid employment_status")
		} else {
			user.EmploymentStatus = strings.ToLower(status)
		}
	}

	// Validate Age (optional)
	if ageStr := fieldMap["age"]; ageStr != "" {
		if age, err := strconv.Atoi(ageStr); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, "invalid age format")
		} else if age < 18 || age > 120 {
			result.Valid = false
			result.Errors = append(result.Errors, "age must be between 18 and 120")
		} else {
			user.Age = &age
		}
	}

	return user, result
}
