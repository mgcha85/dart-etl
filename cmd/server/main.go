package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"dart-etl/internal/api"
	"dart-etl/internal/database"
	"dart-etl/internal/models"
	"dart-etl/pkg/dart"

	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm/clause"
)

func main() {
	// 1. Load Config
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on environment variables")
	}

	apiKey := os.Getenv("DART_API_KEY")
	if apiKey == "" {
		log.Fatal("DART_API_KEY is not set")
	}

	dbPath := os.Getenv("DB_PATH")
	database.InitDB(dbPath)

	client := dart.NewClient(apiKey)

	// Start API Server
	go func() {
		log.Println("Starting API Server on :8080")
		srv := api.NewServer(database.DB)
		if err := srv.Start("8080"); err != nil {
			log.Fatalf("API Server failed: %v", err)
		}
	}()

	// 2. Initial Setup: Corp Codes
	go func() {
		var count int64
		database.DB.Model(&models.Corp{}).Count(&count)
		if count == 0 {
			log.Println("Corp table empty. Fetching initial Corp Codes...")
			updateCorpCodes(client)
		} else {
			log.Println("Corp table has data. Skipping initial fetch.")
		}
	}()

	// 3. Scheduler
	c := cron.New()

	// 3.1 Filing List Collector (Every hour)
	c.AddFunc("@hourly", func() {
		log.Println("[Job] Fetching recent filings (3-day lookback)...")
		for i := 0; i < 3; i++ {
			targetDate := time.Now().AddDate(0, 0, -i).Format("20060102")
			log.Printf("Fetching filings for %s...", targetDate)

			filings, err := client.GetDailyFilings(targetDate)
			if err != nil {
				log.Printf("Error fetching filings for %s: %v\n", targetDate, err)
				continue
			}

			if len(filings) > 0 {
				result := database.DB.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "rcept_no"}},
					DoUpdates: clause.AssignmentColumns([]string{"corp_code", "corp_name", "report_nm", "rcept_dt", "flr_nm", "rm", "dcm_no"}),
				}).Create(&filings)

				if result.Error != nil {
					log.Printf("Error node index %d: %v\n", i, result.Error)
				} else {
					log.Printf("Processed %d filings for %s\n", len(filings), targetDate)
				}
			}
		}
	})

	// 3.2 Document Downloader (Every 5 minutes)
	c.AddFunc("@every 5m", func() {
		log.Println("[Job] Starting Document Downloader...")
		downloadPendingDocuments(client)
	})

	// 3.3 Corp Code Update (Weekly)
	c.AddFunc("@weekly", func() {
		log.Println("[Job] Updating Corp Codes...")
		updateCorpCodes(client)
	})

	c.Start()
	log.Println("Scheduler started. Press Ctrl+C to stop.")

	// Keep alive
	select {}
}

func updateCorpCodes(client *dart.Client) {
	corps, err := client.GetCorpCode()
	if err != nil {
		log.Printf("Failed to update corp codes: %v\n", err)
		return
	}

	batchSize := 100
	for i := 0; i < len(corps); i += batchSize {
		end := i + batchSize
		if end > len(corps) {
			end = len(corps)
		}

		batch := corps[i:end]
		result := database.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "corp_code"}},
			DoUpdates: clause.AssignmentColumns([]string{"corp_name", "stock_code", "modified_at"}),
		}).Create(&batch)

		if result.Error != nil {
			log.Printf("Batch fetch error at index %d: %v\n", i, result.Error)
		}
	}
	log.Printf("Successfully updated %d corp codes\n", len(corps))
}

func downloadPendingDocuments(client *dart.Client) {
	// Find filings that don't have a document in FilingDocument table
	// This is a simplified "NOT EXISTS" check logic or left join
	// For GORM, native SQL is often easiest for this specific query
	var pendingFilings []models.Filing

	// Select filings where rcept_no is NOT in filing_documents
	err := database.DB.Raw(`
		SELECT * FROM filings 
		WHERE rcept_no NOT IN (SELECT rcept_no FROM filing_documents)
		ORDER BY rcept_dt DESC
		LIMIT 10
	`).Scan(&pendingFilings).Error

	if err != nil {
		log.Printf("Error finding pending downloads: %v\n", err)
		return
	}

	if len(pendingFilings) == 0 {
		return
	}

	storageDir := os.Getenv("STORAGE_DIR")
	if storageDir == "" {
		storageDir = "./storage"
	}
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Printf("Error creating storage dir: %v\n", err)
		return
	}

	for _, f := range pendingFilings {
		log.Printf("Downloading document for %s (%s)\n", f.CorpName, f.RceptNo)

		filename := fmt.Sprintf("%s.zip", f.RceptNo)
		filePath := filepath.Join(storageDir, filename)

		if err := client.DownloadDocument(f.RceptNo, filePath); err != nil {
			log.Printf("Failed to download %s: %v\n", f.RceptNo, err)
			continue
		}

		// Calculate SHA256
		hash, err := calculateSHA256(filePath)
		if err != nil {
			log.Printf("Failed to calculate has for %s: %v\n", filePath, err)
			// Proceed anyway? Or fail? Let's proceed but warn.
		}

		// Save record
		doc := models.FilingDocument{
			RceptNo:    f.RceptNo,
			DocType:    "MAIN_XML_ZIP", // Assuming default download is valid
			StorageURI: filePath,
			SHA256:     hash,
		}

		if err := database.DB.Create(&doc).Error; err != nil {
			log.Printf("Failed to save DB record for %s: %v\n", f.RceptNo, err)
		}

		// Respect rate limits - DART has limits (approx 100/min or so depending on key type)
		time.Sleep(500 * time.Millisecond)
	}
}

func calculateSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
