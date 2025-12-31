package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"dart-etl/internal/database"
	"dart-etl/internal/models"
	"dart-etl/pkg/dart"

	"github.com/joho/godotenv"
	"gorm.io/gorm/clause"
)

func main() {
	godotenv.Load()
	apiKey := os.Getenv("DART_API_KEY")
	database.InitDB("dart.db")
	client := dart.NewClient(apiKey)

	// Target: Samsung Electronics 2023 Business Report (사업보고서)
	targetRceptNo := "20240312000736"
	log.Printf("Targeting Samsung Electronics Business Report: %s", targetRceptNo)

	// Mock or fetch filing metadata if not in DB
	var target models.Filing
	err := database.DB.Where("rcept_no = ?", targetRceptNo).First(&target).Error
	if err != nil {
		log.Println("Filing not in DB, creating mock entry for verification...")
		target = models.Filing{
			RceptNo:  targetRceptNo,
			CorpCode: "00126380",
			CorpName: "삼성전자",
			ReportNm: "사업보고서 (2023.12)",
			RceptDt:  "20240312",
			FlrNm:    "삼성전자",
		}
		database.DB.Create(&target)
	}

	// 2. Download
	storageDir := "./storage"
	os.MkdirAll(storageDir, 0755)
	filename := target.RceptNo + ".zip"
	path := filepath.Join(storageDir, filename)

	log.Println("Downloading document...")
	err = client.DownloadDocument(target.RceptNo, path)
	if err != nil {
		log.Fatalf("Download failed: %v", err)
	}

	// Validation: Check if it looks like a ZIP (first bytes PK)
	f, err := os.Open(path)
	if err == nil {
		buf := make([]byte, 2)
		f.Read(buf)
		f.Close()
		if string(buf) != "PK" {
			log.Printf("Downloaded file is NOT a zip (starts with %s). Printing content preview...", string(buf))
			content, _ := os.ReadFile(path)
			log.Printf("File content preview: %s", string(content[:min(len(content), 200)]))
			log.Fatal("Stopping as file is not a valid document.")
		}
	}

	// Create/Update Doc Record
	doc := models.FilingDocument{
		RceptNo:    target.RceptNo,
		DocType:    "MAIN_XML_ZIP",
		StorageURI: path,
	}
	database.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&doc)

	// 3. Run Extraction
	log.Println("Running Python Extraction via .venv...")
	cwd, _ := os.Getwd()
	pythonScript := filepath.Join(cwd, "python", "extractor.py")
	pythonExec := filepath.Join(cwd, ".venv", "bin", "python")

	if _, err := os.Stat(pythonExec); os.IsNotExist(err) {
		pythonExec = "python3"
	}

	cmd := exec.Command(pythonExec, pythonScript, "--rcept_no", target.RceptNo, "--db_path", "dart.db", "--file", path)
	out, err := cmd.CombinedOutput()
	log.Printf("Python Output:\n%s", string(out))

	if err != nil {
		log.Fatalf("Extraction failed: %v", err)
	}

	// 4. Show Result
	var events []models.ExtractedEvent
	database.DB.Where("rcept_no = ?", target.RceptNo).Find(&events)
	log.Printf("SUCCESS! Found %d extracted events in DB.", len(events))
	for _, e := range events {
		log.Printf(" - ID: %d", e.ID)
		log.Printf(" - Type: %s", e.EventType)
		log.Printf(" - Payload: %s", e.PayloadJSON)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
