package main

import (
	"context"
	"log"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"
	"github.com/gin-gonic/gin"
)

var ginLambda *ginadapter.GinLambda

func init() {
	// Ensure we have proper logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Don't exit on panic during init
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC during init: %v", r)
			// Don't exit, let the runtime handle it
		}
	}()

	log.Println("=== Lambda Cold Start ===")

	// Set gin to release mode
	gin.SetMode(gin.ReleaseMode)

	// Create router with minimal setup
	router := gin.New()

	// Add recovery middleware
	router.Use(gin.Recovery())

	// Simple health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Your hello endpoint
	router.GET("/api/v1/hello", HelloWorldHandler)

	// Initialize the lambda adapter
	ginLambda = ginadapter.New(router)
	log.Println("=== Lambda Initialized Successfully ===")
}

func Handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Don't exit on panic during request handling
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC during request handling: %v", r)
			// Don't exit, return an error response instead
		}
	}()

	log.Printf("Request: %s %s", request.HTTPMethod, request.Path)

	// Check if ginLambda is initialized
	if ginLambda == nil {
		log.Println("ERROR: ginLambda is nil!")
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       `{"error": "Lambda not initialized"}`,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}, nil
	}

	// Process the request
	response, err := ginLambda.ProxyWithContext(ctx, request)
	if err != nil {
		log.Printf("ERROR processing request: %v", err)
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       `{"error": "Processing failed"}`,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}, nil
	}

	log.Printf("Response: %d", response.StatusCode)
	return response, nil
}

func main() {
	log.Println("=== Starting Lambda Function ===")
	lambda.Start(Handler)
}

func HelloWorldHandler(c *gin.Context) {
	log.Println("HelloWorldHandler called successfully")
	c.JSON(http.StatusOK, gin.H{
		"message": "hello world",
		"path":    c.Request.URL.Path,
		"method":  c.Request.Method,
	})
}
