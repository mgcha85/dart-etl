package api

import (
	"dart-etl/internal/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Server struct {
	DB *gorm.DB
}

func NewServer(db *gorm.DB) *Server {
	return &Server{DB: db}
}

func (s *Server) Start(port string) error {
	r := gin.Default()

	api := r.Group("/api")
	{
		api.GET("/corps", s.GetCorps)
		api.GET("/filings", s.GetFilings)
		api.GET("/filings/:rcept_no", s.GetFilingDetail)
	}

	return r.Run(":" + port)
}

func (s *Server) GetCorps(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset := (page - 1) * limit

	var corps []models.Corp
	var total int64

	s.DB.Model(&models.Corp{}).Count(&total)
	result := s.DB.Limit(limit).Offset(offset).Order("corp_name ASC").Find(&corps)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  corps,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (s *Server) GetFilings(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset := (page - 1) * limit

	var filings []models.Filing
	var total int64

	s.DB.Model(&models.Filing{}).Count(&total)
	result := s.DB.Limit(limit).Offset(offset).Order("rcept_dt DESC, rcept_no DESC").Find(&filings)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  filings,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (s *Server) GetFilingDetail(c *gin.Context) {
	rceptNo := c.Param("rcept_no")

	var filing models.Filing
	if err := s.DB.Where("rcept_no = ?", rceptNo).First(&filing).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Filing not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	var documents []models.FilingDocument
	s.DB.Where("rcept_no = ?", rceptNo).Find(&documents)

	// Since we removed Python extraction, we won't have ExtractedEvent for new data,
	// but we can still show if any exists from legacy data or if we add Go extraction later.
	var events []models.ExtractedEvent
	s.DB.Where("rcept_no = ?", rceptNo).Find(&events)

	c.JSON(http.StatusOK, gin.H{
		"filing":    filing,
		"documents": documents,
		"events":    events,
	})
}
