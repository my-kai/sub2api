package routes

import (
	invoice "github.com/Wei-Shaw/sub2api/internal/custom/invoice"
	"github.com/gin-gonic/gin"
)

// RegisterPublicRoutes registers no-login invoice file download routes.
func RegisterPublicRoutes(group gin.IRouter, h *invoice.Handler) {
	if group == nil || h == nil {
		return
	}
	group.GET("/custom/invoice-downloads/:token", h.DownloadTemporaryFile)
}

// RegisterUserRoutes registers logged-in user invoice routes.
func RegisterUserRoutes(group gin.IRouter, h *invoice.Handler) {
	if group == nil || h == nil {
		return
	}
	group.GET("/custom/invoices/titles", h.ListTitles)
	group.POST("/custom/invoices/titles", h.CreateTitle)
	group.PUT("/custom/invoices/titles/:id", h.UpdateTitle)
	group.DELETE("/custom/invoices/titles/:id", h.DeleteTitle)
	group.POST("/custom/invoices/titles/:id/default", h.SetDefaultTitle)

	group.GET("/custom/invoices/eligible-orders", h.ListEligibleOrders)
	group.POST("/custom/invoices", h.CreateApplication)
	group.GET("/custom/invoices/my", h.ListMyApplications)
	group.GET("/custom/invoices/:id", h.GetMyApplication)
	group.GET("/custom/invoices/:id/file", h.DownloadMyFile)
}

// RegisterAdminRoutes registers admin invoice review routes.
func RegisterAdminRoutes(group gin.IRouter, h *invoice.Handler) {
	if group == nil || h == nil {
		return
	}
	group.GET("/custom/invoices", h.ListAdminApplications)
	group.POST("/custom/invoice-test-email", h.TestSendGeneratedNotification)
	group.GET("/custom/invoices/:id", h.GetAdminApplication)
	group.POST("/custom/invoices/:id/issue", h.IssueApplication)
	group.POST("/custom/invoices/:id/test-email", h.TestSendIssuedNotification)
	group.POST("/custom/invoices/:id/reject", h.RejectApplication)
	group.GET("/custom/invoices/:id/file", h.DownloadAdminFile)
}
