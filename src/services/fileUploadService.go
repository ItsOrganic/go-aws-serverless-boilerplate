package services

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"loanmatch/src/database"
	"loanmatch/src/models"

	"github.com/google/uuid"
)

type CSVProcessor struct {
	DB        *sql.DB
	S3Service *S3Service
}

type ProcessingResult struct {
	UploadID      uuid.UUID
	TotalRows     int
	ProcessedRows int
	FailedRows    int
	Status        string
}

const BatchSize = 500

func NewCSVProcessor() (*CSVProcessor, error) {
	s3Service, err := NewS3Service()
	if err != nil {
		return nil, err
	}

	return &CSVProcessor{
		DB:        database.Db,
		S3Service: s3Service,
	}, nil
}

func (cp *CSVProcessor) InitiateCSVUpload(filename, uploadedBy string) (*models.S3UploadResponse, error) {
	uploadID := uuid.New()

	uploadURL, s3Key, err := cp.S3Service.GeneratePreSignedUploadURL(uploadID, filename)
	if err != nil {
		return nil, fmt.Errorf("failed to generate upload URL: %w", err)
	}

	upload := &models.CSVUpload{
		UploadID:        uploadID,
		S3Key:           s3Key,
		Filename:        filename,
		Status:          "pending",
		UploadedBy:      uploadedBy,
		UploadStartedAt: time.Now(),
	}

	if err := cp.createUploadRecord(upload); err != nil {
		return nil, fmt.Errorf("failed to create upload record: %w", err)
	}

	return &models.S3UploadResponse{
		Success:   true,
		UploadID:  uploadID,
		UploadURL: uploadURL,
		S3Key:     s3Key,
		Message:   "Upload initiated successfully. Use the upload_url to upload your CSV file.",
	}, nil
}

func (cp *CSVProcessor) ProcessCSVFromS3(s3Key string) error {
	log.Printf("Starting processing of CSV file: %s", s3Key)

	upload, err := cp.getUploadByS3Key(s3Key)
	if err != nil {
		return fmt.Errorf("failed to get upload record: %w", err)
	}
	if upload.Status == "completed" || upload.Status == "partial" {
		log.Printf("Upload %s already %s, skipping duplicate event", upload.UploadID, upload.Status)
		return nil
	}

	now := time.Now()
	upload.Status = "processing"
	upload.ProcessingStartedAt = &now
	cp.updateUploadRecord(upload)

	obj, err := cp.S3Service.GetObject(s3Key)
	if err != nil {
		cp.updateUploadStatus(upload.UploadID, "failed", "Failed to download file from S3")
		return fmt.Errorf("failed to download from S3: %w", err)
	}
	defer obj.Body.Close()

	result, err := cp.processCSVContent(obj.Body, upload.UploadID)
	if err != nil {
		cp.updateUploadStatus(upload.UploadID, "failed", err.Error())
		return err
	}

	status := "completed"
	if result.FailedRows > 0 && result.ProcessedRows == 0 {
		status = "failed"
	} else if result.FailedRows > 0 {
		status = "partial"
	}

	cp.finalizeUpload(upload.UploadID, status, result.TotalRows, result.ProcessedRows, result.FailedRows)

	log.Printf("Completed processing CSV: %s, Status: %s, Processed: %d/%d",
		s3Key, status, result.ProcessedRows, result.TotalRows)

	return nil
}

// csvRow pairs a validated user with its 1-based line number in the file
// (the header is line 1) so database errors can be reported against the row.
type csvRow struct {
	rowNumber int
	user      *models.User
}

func (cp *CSVProcessor) processCSVContent(reader io.Reader, uploadID uuid.UUID) (*ProcessingResult, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1

	result := &ProcessingResult{
		UploadID: uploadID,
		Status:   "processing",
	}

	// The first line is the header; columns are matched by name, not position.
	csvHeaders, err := csvReader.Read()
	if err == io.EOF {
		return nil, fmt.Errorf("CSV file is empty")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}
	if len(csvHeaders) > 0 {
		csvHeaders[0] = strings.TrimPrefix(csvHeaders[0], "\ufeff") // UTF-8 BOM from Excel exports
	}
	if missing := missingColumns(csvHeaders, "name", "email"); len(missing) > 0 {
		return nil, fmt.Errorf("CSV header is missing required columns: %s", strings.Join(missing, ", "))
	}

	batch := make([]csvRow, 0, BatchSize)
	var batchErrors []models.CSVUploadError
	rowNumber := 1

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		rowNumber++
		result.TotalRows++

		if err != nil {
			log.Printf("Error reading row %d: %v", rowNumber, err)
			batchErrors = append(batchErrors, models.CSVUploadError{
				ErrorID:      uuid.New(),
				UploadID:     uploadID,
				RowNumber:    rowNumber,
				RawData:      strings.Join(record, ","),
				ErrorMessage: fmt.Sprintf("CSV parse error: %v", err),
			})
			result.FailedRows++
			continue
		}

		user, validation := ValidateUserCSVRecord(record, csvHeaders)
		if !validation.Valid {
			batchErrors = append(batchErrors, models.CSVUploadError{
				ErrorID:      uuid.New(),
				UploadID:     uploadID,
				RowNumber:    rowNumber,
				RawData:      strings.Join(record, ","),
				ErrorMessage: fmt.Sprintf("Validation errors: %v", validation.Errors),
			})
			result.FailedRows++
		} else {
			batch = append(batch, csvRow{rowNumber: rowNumber, user: user})
		}

		if len(batch) >= BatchSize {
			processed, failed := cp.processBatch(batch, uploadID, &batchErrors)
			result.ProcessedRows += processed
			result.FailedRows += failed
			batch = batch[:0]
		}

		if len(batchErrors) >= BatchSize {
			cp.storeErrors(batchErrors)
			batchErrors = batchErrors[:0]
		}
	}

	if len(batch) > 0 {
		processed, failed := cp.processBatch(batch, uploadID, &batchErrors)
		result.ProcessedRows += processed
		result.FailedRows += failed
	}

	if len(batchErrors) > 0 {
		cp.storeErrors(batchErrors)
	}

	return result, nil
}

func missingColumns(headers []string, required ...string) []string {
	present := make(map[string]bool, len(headers))
	for _, h := range headers {
		present[strings.ToLower(strings.TrimSpace(h))] = true
	}
	var missing []string
	for _, col := range required {
		if !present[col] {
			missing = append(missing, col)
		}
	}
	return missing
}

func (cp *CSVProcessor) processBatch(rows []csvRow, uploadID uuid.UUID, errors *[]models.CSVUploadError) (processed, failed int) {
	tx, err := cp.DB.Begin()
	if err != nil {
		log.Printf("Failed to start transaction: %v", err)
		return 0, len(rows)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
        INSERT INTO users (user_id, name, email, monthly_income, credit_score, employment_status, age)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        ON CONFLICT (email) DO UPDATE SET
            name = EXCLUDED.name,
            monthly_income = EXCLUDED.monthly_income,
            credit_score = EXCLUDED.credit_score,
            employment_status = EXCLUDED.employment_status,
            age = EXCLUDED.age,
            updated_at = NOW()
    `)
	if err != nil {
		log.Printf("Failed to prepare statement: %v", err)
		return 0, len(rows)
	}
	defer stmt.Close()

	for _, row := range rows {
		user := row.user
		err := database.ExecInSavepoint(tx, stmt,
			user.UserID,
			user.Name,
			user.Email,
			user.MonthlyIncome,
			user.CreditScore,
			user.EmploymentStatus,
			user.Age,
		)

		if err != nil {
			log.Printf("Failed to insert user at row %d: %v", row.rowNumber, err)
			*errors = append(*errors, models.CSVUploadError{
				ErrorID:      uuid.New(),
				UploadID:     uploadID,
				RowNumber:    row.rowNumber,
				RawData:      user.Email,
				ErrorMessage: fmt.Sprintf("Database error: %v", err),
			})
			failed++
		} else {
			processed++
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Failed to commit transaction: %v", err)
		return 0, len(rows)
	}

	return processed, failed
}

func (cp *CSVProcessor) createUploadRecord(upload *models.CSVUpload) error {
	query := `
        INSERT INTO csv_uploads (upload_id, s3_key, filename, status, uploaded_by, upload_started_at)
        VALUES ($1, $2, $3, $4, $5, $6)
    `
	_, err := cp.DB.Exec(query, upload.UploadID, upload.S3Key, upload.Filename,
		upload.Status, upload.UploadedBy, upload.UploadStartedAt)
	return err
}

func (cp *CSVProcessor) getUploadByS3Key(s3Key string) (*models.CSVUpload, error) {
	query := `
        SELECT upload_id, s3_key, filename, status, uploaded_by, upload_started_at
        FROM csv_uploads WHERE s3_key = $1
    `
	upload := &models.CSVUpload{}
	err := cp.DB.QueryRow(query, s3Key).Scan(
		&upload.UploadID, &upload.S3Key, &upload.Filename,
		&upload.Status, &upload.UploadedBy, &upload.UploadStartedAt,
	)
	return upload, err
}

func (cp *CSVProcessor) updateUploadRecord(upload *models.CSVUpload) {
	query := `
        UPDATE csv_uploads 
        SET status = $1, processing_started_at = $2
        WHERE upload_id = $3
    `
	cp.DB.Exec(query, upload.Status, upload.ProcessingStartedAt, upload.UploadID)
}

func (cp *CSVProcessor) updateUploadStatus(uploadID uuid.UUID, status, errorSummary string) {
	query := `
        UPDATE csv_uploads 
        SET status = $1, error_summary = $2, upload_completed_at = NOW()
        WHERE upload_id = $3
    `
	cp.DB.Exec(query, status, errorSummary, uploadID)
}

func (cp *CSVProcessor) finalizeUpload(uploadID uuid.UUID, status string, total, processed, failed int) {
	query := `
        UPDATE csv_uploads 
        SET status = $1, total_rows = $2, processed_rows = $3, failed_rows = $4, upload_completed_at = NOW()
        WHERE upload_id = $5
    `
	cp.DB.Exec(query, status, total, processed, failed, uploadID)
}

func (cp *CSVProcessor) storeErrors(errors []models.CSVUploadError) {
	if len(errors) == 0 {
		return
	}

	tx, err := cp.DB.Begin()
	if err != nil {
		log.Printf("Failed to start transaction for errors: %v", err)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
        INSERT INTO csv_upload_errors (error_id, upload_id, row_number, raw_data, error_message)
        VALUES ($1, $2, $3, $4, $5)
    `)
	if err != nil {
		log.Printf("Failed to prepare error statement: %v", err)
		return
	}
	defer stmt.Close()

	for _, errorRecord := range errors {
		_, err := stmt.Exec(
			errorRecord.ErrorID,
			errorRecord.UploadID,
			errorRecord.RowNumber,
			errorRecord.RawData,
			errorRecord.ErrorMessage,
		)
		if err != nil {
			log.Printf("Failed to insert error record: %v", err)
		}
	}

	tx.Commit()
}

func (cp *CSVProcessor) GetUploadStatus(uploadID uuid.UUID) (*models.CSVUpload, error) {
	query := `
        SELECT upload_id, filename, status, total_rows, processed_rows, failed_rows,
               uploaded_by, upload_started_at, upload_completed_at, error_summary
        FROM csv_uploads
        WHERE upload_id = $1
    `

	upload := &models.CSVUpload{}
	err := cp.DB.QueryRow(query, uploadID).Scan(
		&upload.UploadID,
		&upload.Filename,
		&upload.Status,
		&upload.TotalRows,
		&upload.ProcessedRows,
		&upload.FailedRows,
		&upload.UploadedBy,
		&upload.UploadStartedAt,
		&upload.UploadCompletedAt,
		&upload.ErrorSummary,
	)

	return upload, err
}
