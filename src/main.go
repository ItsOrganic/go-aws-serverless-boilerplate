package main

import (
	"crypto/subtle"
	"loanmatch/src/database"
	"loanmatch/src/globals"
	"loanmatch/src/handlers"
	"loanmatch/src/utils"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"
	"github.com/gin-gonic/gin"
)

// A single binary serves both Lambda functions. APP_MODE selects which one:
//   - "api" (default): HTTP API behind API Gateway
//   - "csv-processor": SQS consumer that imports uploaded CSV files
//
// Outside Lambda the API is served on :PORT (default 8080) for local development.
func main() {
	utils.LoadEnvVariables()
	if err := database.InitDbConn(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	onLambda := os.Getenv("AWS_LAMBDA_RUNTIME_API") != ""

	switch mode := os.Getenv("APP_MODE"); mode {
	case "csv-processor":
		lambda.Start(handlers.HandleSQSCSVProcessing)
	case "", "api":
		if onLambda && globals.Config.APIKey == "" {
			log.Fatal("API_KEY must be set when running on Lambda")
		}
		router := newRouter()
		if onLambda {
			lambda.Start(ginadapter.New(router).ProxyWithContext)
			return
		}
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		log.Printf("Starting local server on :%s", port)
		if err := router.Run(":" + port); err != nil {
			log.Fatalf("Server stopped: %v", err)
		}
	default:
		log.Fatalf("Unknown APP_MODE %q", mode)
	}
}

func newRouter() *gin.Engine {
	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	csvHandler, err := handlers.NewCSVHandler()
	if err != nil {
		log.Fatalf("Failed to initialize CSV handler: %v", err)
	}
	n8nHandler := handlers.NewN8NHandler()

	api := router.Group("/api/v1", requireAPIKey(globals.Config.APIKey))
	api.GET("/hello", handlers.HelloWorldHandler)

	// CSV Upload routes
	api.POST("/csv/initiate-upload", csvHandler.InitiateCSVUpload)
	api.GET("/csv/upload/:upload_id/status", csvHandler.GetUploadStatus)

	// Webhook endpoints for n8n
	api.POST("/n8n/trigger-matching", n8nHandler.TriggerUserLoanMatching)
	api.GET("/n8n/users/new", n8nHandler.GetNewUsers)
	api.GET("/n8n/loan-products", n8nHandler.GetLoanProducts)
	api.POST("/n8n/loan-products", n8nHandler.StoreLoanProducts)
	api.POST("/n8n/matches", n8nHandler.StoreUserLoanMatches)
	api.GET("/n8n/users/notification", n8nHandler.GetUsersForNotification)
	api.GET("/n8n/users/:user_id/matches", n8nHandler.GetUserLoanMatches)
	api.POST("/n8n/notifications", n8nHandler.RecordNotification)

	return router
}

// requireAPIKey checks the X-API-Key header. An empty key disables the check,
// which main only allows outside Lambda.
func requireAPIKey(key string) gin.HandlerFunc {
	if key == "" {
		log.Println("WARNING: API_KEY not set, /api/v1 routes are unauthenticated")
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		got := c.GetHeader("X-API-Key")
		if subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}
