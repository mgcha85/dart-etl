package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"dart-etl/internal/database"
	"dart-etl/internal/models"
	"dart-etl/pkg/dart"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	apiKey := os.Getenv("DART_API_KEY")
	database.InitDB("dart.db")
	client := dart.NewClient(apiKey)

	// 1. Manually Insert a Test Filing (e.g., Samsung Electronics or a recent one)
	// Using a recent known receipt number if possible, or just fetch today's list and pick one.
	log.Println("Fetching today's filings for example...")
	today := time.Now().Format("20060102")
	filings, err := client.GetDailyFilings(today)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	if len(filings) == 0 {
		// Fallback to yesterday if today is empty
		yesterday := time.Now().AddDate(0, 0, -1).Format("20060102")
		log.Println("Today empty, trying yesterday:", yesterday)
		filings, err = client.GetDailyFilings(yesterday)
		if err != nil {
			log.Fatalf("Error: %v", err)
		}
	}

	if len(filings) == 0 {
		log.Fatal("No filings found to verify.")
	}

	target := filings[0] // Pick the first one
	log.Printf("Selected Filing: [%s] %s (RceptNo: %s)", target.CorpName, target.ReportNm, target.RceptNo)

	// Save to DB
	database.DB.Create(&target)

	// 2. Download
	log.Println("Downloading document...")
	storageDir := "./storage"
	os.MkdirAll(storageDir, 0755)
	filename := target.RceptNo + ".zip"
	path := filepath.Join(storageDir, filename)

	err = client.DownloadDocument(target.RceptNo, path)
	if err != nil {
		log.Fatalf("Download failed: %v", err)
	}
	log.Printf("Downloaded raw file to: %s", path)

	// Create Doc Record
	doc := models.FilingDocument{
		RceptNo:    target.RceptNo,
		StorageURI: path,
		DocType:    "MAIN_XML_ZIP",
	}
	database.DB.Create(&doc)

	// 3. Run Extraction
	log.Println("Running Python Extraction...")
	cwd, _ := os.Getwd()
	pythonScript := filepath.Join(cwd, "python", "extractor.py")

	cmd := exec.Command("python3", pythonScript, "--rcept_no", target.RceptNo, "--db_path", "dart.db", "--file", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Extraction Warning/Error: %v", err)
	}
	log.Printf("Python Output:\n%s", string(out))

	// 4. Show Result
	var events []models.ExtractedEvent
	database.DB.Where("rcept_no = ?", target.RceptNo).Find(&events)
	log.Printf("Found %d extracted events in DB.", len(events))
	for _, e := range events {
		log.Printf(" - Type: %s, Payload: %s", e.EventType, e.PayloadJSON)
	}
}
