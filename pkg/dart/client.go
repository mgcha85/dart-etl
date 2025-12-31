package dart

import (
	"archive/zip"
	"bytes"
	"crypto/tls"
	"dart-etl/internal/models"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

const BaseURL = "https://opendart.fss.or.kr/api"

type Client struct {
	APIKey     string
	HTTPClient *http.Client
}

func NewClient(apiKey string) *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS10,
			CipherSuites: []uint16{
				tls.TLS_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			},
		},
	}
	return &Client{
		APIKey:     apiKey,
		HTTPClient: &http.Client{Transport: tr},
	}
}

// Responses for Corp Code
type corpCodeResult struct {
	XMLName xml.Name `xml:"result"`
	List    []struct {
		CorpCode   string `xml:"corp_code" json:"corp_code"`
		CorpName   string `xml:"corp_name" json:"corp_name"`
		StockCode  string `xml:"stock_code" json:"stock_code"`
		ModifyDate string `xml:"modify_date" json:"modify_date"`
	} `xml:"list"`
}

// Responses for Filing List
type filingListResponse struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	PageNo  int             `json:"page_no"`
	PageCo  int             `json:"page_count"`
	TotalCo int             `json:"total_count"`
	List    []models.Filing `json:"list"`
}

// GetCorpCode downloads the ZIP file, extracts XML, and parses it
func (c *Client) GetCorpCode() ([]models.Corp, error) {
	apiURL := fmt.Sprintf("%s/corpCode.xml?crtfc_key=%s", BaseURL, c.APIKey)
	resp, err := c.HTTPClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download corpCode: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Unzip
	zipReader, err := zip.NewReader(bytes.NewReader(bodyBytes), int64(len(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse zip: %w", err)
	}

	var parsedResult corpCodeResult

	for _, file := range zipReader.File {
		// usually CORPCODE.xml or similar
		if file.FileInfo().IsDir() {
			continue
		}
		f, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer f.Close()

		xmlBytes, err := io.ReadAll(f)
		if err != nil {
			return nil, err
		}

		if err := xml.Unmarshal(xmlBytes, &parsedResult); err != nil {
			return nil, fmt.Errorf("failed to unmarshal XML: %w", err)
		}
		break // verify if multiple files exist, usually just one
	}

	// Convert to Models
	var corps []models.Corp
	for _, item := range parsedResult.List {
		// modify_date is YYYYMMDD
		modTime, _ := time.Parse("20060102", item.ModifyDate)

		corps = append(corps, models.Corp{
			CorpCode:   item.CorpCode,
			CorpName:   item.CorpName,
			StockCode:  item.StockCode, // DART returns " " empty string sometimes, but let's keep it
			ModifiedAt: modTime,
		})
	}
	return corps, nil
}

// GetDailyFilings fetches filings for a specific date (YYYYMMDD) via list.json
func (c *Client) GetDailyFilings(date string) ([]models.Filing, error) {
	// API endpoint: list.json
	queryParams := url.Values{}
	queryParams.Add("crtfc_key", c.APIKey)
	queryParams.Add("bgn_de", date)
	queryParams.Add("end_de", date)
	queryParams.Add("page_count", "100") // max 100 pages logic might be needed loop

	// Initially just fetching page 1. For production, we need to loop pages.
	// But let's verify connectivity first.
	// Adding full pagination loop logic:

	var allFilings []models.Filing
	page := 1

	for {
		queryParams.Set("page_no", fmt.Sprintf("%d", page))
		apiURL := fmt.Sprintf("%s/list.json?%s", BaseURL, queryParams.Encode())

		resp, err := c.HTTPClient.Get(apiURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch list: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API status %d", resp.StatusCode)
		}

		var result filingListResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("json decode error: %w", err)
		}

		if result.Status != "000" {
			// 013: No data found
			if result.Status == "013" {
				break
			}
			return nil, fmt.Errorf("API error %s: %s", result.Status, result.Message)
		}

		allFilings = append(allFilings, result.List...)

		if page >= result.PageCo {
			break
		}
		page++
	}

	return allFilings, nil
}

// DownloadDocument downloads the 'document.xml' or original PDF/ZIP
func (c *Client) DownloadDocument(rceptNo string, destPath string) error {
	// Endpoint: /document.xml?crtfc_key=xxx&rcept_no=xxx (returns zip)
	// OR /document.xml (which gives XML inside ZIP) -> "Main Text"
	// User request 2) says: "원문(공시 문서 다운로드: XML/zip 형태)"
	// Typically /document.xml returns a ZIP containing the XML body.

	apiURL := fmt.Sprintf("%s/document.xml?crtfc_key=%s&rcept_no=%s", BaseURL, c.APIKey, rceptNo)

	resp, err := c.HTTPClient.Get(apiURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	// Just save the body to destPath
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
