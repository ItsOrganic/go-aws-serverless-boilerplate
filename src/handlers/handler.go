package handlers

import (
	"encoding/json"
	"loanmatch/src/models"
	"loanmatch/src/services"
	"log"
	"net/http"
	"net/url"

	"github.com/aws/aws-lambda-go/events"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func HelloWorldHandler(c *gin.Context) {
	log.Println("HelloWorldHandler called successfully")
	c.JSON(http.StatusOK, gin.H{
		"message": "hello world",
		"path":    c.Request.URL.Path,
		"method":  c.Request.Method,
	})
}

type CSVHandler struct {
	processor *services.CSVProcessor
}

func NewCSVHandler() (*CSVHandler, error) {
	processor, err := services.NewCSVProcessor()
	if err != nil {
		return nil, err
	}

	return &CSVHandler{
		processor: processor,
	}, nil
}

// Step 1: Initiate CSV Upload (returns pre-signed URL)
func (h *CSVHandler) InitiateCSVUpload(c *gin.Context) {
	var request models.S3UploadRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	response, err := h.processor.InitiateCSVUpload(request.Filename, request.UploadedBy)
	if err != nil {
		log.Printf("Error initiating CSV upload: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to initiate upload",
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// Get upload status
func (h *CSVHandler) GetUploadStatus(c *gin.Context) {
	uploadIDStr := c.Param("upload_id")
	uploadID, err := uuid.Parse(uploadIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid upload ID",
		})
		return
	}

	upload, err := h.processor.GetUploadStatus(uploadID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Upload not found",
		})
		return
	}

	c.JSON(http.StatusOK, upload)
}

// SQS Handler for processing CSV files
func HandleSQSCSVProcessing(sqsEvent events.SQSEvent) error {
	processor, err := services.NewCSVProcessor()
	if err != nil {
		return err
	}

	for _, record := range sqsEvent.Records {
		log.Printf("Processing SQS message: %s", record.Body)

		// Parse S3 event from SQS message
		var s3Event events.S3Event
		if err := json.Unmarshal([]byte(record.Body), &s3Event); err != nil {
			log.Printf("Error parsing S3 event: %v", err)
			continue
		}

		// Process each S3 record
		for _, s3Record := range s3Event.Records {
			// S3 event keys are URL-encoded (spaces arrive as '+')
			s3Key, err := url.QueryUnescape(s3Record.S3.Object.Key)
			if err != nil {
				log.Printf("Error decoding S3 key %q: %v", s3Record.S3.Object.Key, err)
				continue
			}
			log.Printf("Processing CSV file from S3: %s", s3Key)

			if err := processor.ProcessCSVFromS3(s3Key); err != nil {
				log.Printf("Error processing CSV from S3: %v", err)
				// Don't return error here to prevent SQS retry loops
				// Error is already logged and stored in database
			}
		}
	}

	return nil
}
