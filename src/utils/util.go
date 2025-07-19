package utils

import (
	"loanmatch/src/globals"
	"loanmatch/src/models"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

func LoadEnvVariables() {
	// .env is only used for local development; on Lambda the variables
	// come from the function configuration.
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using process environment")
	}

	port, err := strconv.Atoi(getEnv("DATABASE_PORT", "5432"))
	if err != nil {
		log.Printf("Invalid DATABASE_PORT, falling back to 5432: %v", err)
		port = 5432
	}

	globals.Config = &models.Config{
		Db: models.DBConfig{
			Host:     os.Getenv("DATABASE_ENDPOINT"),
			Port:     port,
			Database: os.Getenv("DATABASE_NAME"),
			User:     os.Getenv("DATABASE_USER"),
			Pass:     os.Getenv("DATABASE_PASSWORD"),
			SslMode:  getEnv("DATABASE_SSLMODE", "require"),
		},
		APIKey: os.Getenv("API_KEY"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
