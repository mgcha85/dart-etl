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

	"dart-etl/internal/database"
	"dart-etl/internal/models"
	"dart-etl/pkg/dart"

	"os/exec"

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
		log.Println("[Job] Fetching recent filings...")
		// Fetch for today
		today := time.Now().Format("20060102")
		filings, err := client.GetDailyFilings(today)
		if err != nil {
			log.Printf("Error fetching filings: %v\n", err)
			return
		}

		if len(filings) > 0 {
			result := database.DB.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "rcept_no"}},
				DoUpdates: clause.AssignmentColumns([]string{"corp_code", "corp_name", "report_nm", "rcept_dt", "flr_nm", "rm", "dcm_no"}),
			}).Create(&filings)

			if result.Error != nil {
				log.Printf("Error saving filings: %v\n", result.Error)
			} else {
				log.Printf("Saved %d filings\n", len(filings))
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

	// 3.4 Content Extraction (Every 5 minutes)
	c.AddFunc("@every 5m", func() {
		log.Println("[Job] Starting Content Extraction...")
		runContentExtraction()
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

func runContentExtraction() {
	// Find documents that haven't been extracted yet
	var pendingDocs []models.FilingDocument
	err := database.DB.Where("extracted_at IS NULL").Limit(5).Find(&pendingDocs).Error
	if err != nil {
		log.Printf("Error checking for pending extractions: %v\n", err)
		return
	}

	if len(pendingDocs) == 0 {
		return
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "dart.db"
	}

	// Must be absolute path for python script usually, or relative from execution dir
	// Assuming execution from project root
	cwd, _ := os.Getwd()
	pythonScript := filepath.Join(cwd, "python", "extractor.py")
	pythonExec := filepath.Join(cwd, ".venv", "bin", "python")

	// Fallback to system python if venv missing (dev convenience)
	if _, err := os.Stat(pythonExec); os.IsNotExist(err) {
		pythonExec = "python3"
	}

	for _, doc := range pendingDocs {
		log.Printf("Running extraction for RceptNo: %s\n", doc.RceptNo)

		// Call Python script
		cmd := exec.Command(pythonExec, pythonScript, "--rcept_no", doc.RceptNo, "--db_path", dbPath, "--file", doc.StorageURI)

		// Capture output for debug
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("Extraction failed for %s: %v\nOutput: %s\n", doc.RceptNo, err, string(output))
			continue
		}

		log.Printf("Extraction success for %s. Output: %s\n", doc.RceptNo, string(output))
		// The python script updates the DB directly, so we don't need to update ExtractedAt here if the script does it.
		// Our script does update it.
	}
}
