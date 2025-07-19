package database

import (
	"database/sql"
	"fmt"
	"loanmatch/src/globals"
	"log"
	"time"

	_ "github.com/lib/pq"
)

var Db *sql.DB

func InitDbConn() error {
	config := globals.Config.Db
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Pass, config.Database, config.SslMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	// Keep the pool small: each Lambda container handles one request at a time.
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("ping database: %w", err)
	}

	log.Printf("Database connection established (host=%s db=%s)", config.Host, config.Database)
	Db = db
	return nil
}

// ExecInSavepoint runs a prepared statement inside its own savepoint so that a
// failing row does not abort the surrounding transaction (Postgres rejects every
// statement after an error until the transaction is rolled back).
func ExecInSavepoint(tx *sql.Tx, stmt *sql.Stmt, args ...interface{}) error {
	if _, err := tx.Exec("SAVEPOINT row_sp"); err != nil {
		return err
	}
	if _, err := stmt.Exec(args...); err != nil {
		if _, rbErr := tx.Exec("ROLLBACK TO SAVEPOINT row_sp"); rbErr != nil {
			return fmt.Errorf("%w (rollback to savepoint failed: %v)", err, rbErr)
		}
		return err
	}
	_, err := tx.Exec("RELEASE SAVEPOINT row_sp")
	return err
}
