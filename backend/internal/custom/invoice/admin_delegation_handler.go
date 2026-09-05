package invoice

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// ListAdminUserTitles returns reusable titles owned by an active target user.
func (h *Handler) ListAdminUserTitles(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	userID, ok := parseID(c, "user_id")
	if !ok {
		return
	}
	if err := h.service.EnsureActiveUser(c.Request.Context(), userID); err != nil {
		writeError(c, err)
		return
	}
	titles, err := h.service.ListTitles(c.Request.Context(), userID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, titles)
}

// CreateAdminUserTitle creates a title for an active target user.
func (h *Handler) CreateAdminUserTitle(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	userID, ok := parseID(c, "user_id")
	if !ok {
		return
	}
	if err := h.service.EnsureActiveUser(c.Request.Context(), userID); err != nil {
		writeError(c, err)
		return
	}
	var req titleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	title, err := h.service.CreateTitle(c.Request.Context(), userID, req.toInput())
	if err != nil {
		writeError(c, err)
		return
	}
	response.Created(c, title)
}

// UpdateAdminUserTitle updates a title owned by an active target user.
func (h *Handler) UpdateAdminUserTitle(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	userID, ok := parseID(c, "user_id")
	if !ok {
		return
	}
	titleID, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.EnsureActiveUser(c.Request.Context(), userID); err != nil {
		writeError(c, err)
		return
	}
	var req titleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	title, err := h.service.UpdateTitle(c.Request.Context(), userID, titleID, req.toInput())
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, title)
}

// DeleteAdminUserTitle soft-deletes a title owned by an active target user.
func (h *Handler) DeleteAdminUserTitle(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	userID, ok := parseID(c, "user_id")
	if !ok {
		return
	}
	titleID, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.EnsureActiveUser(c.Request.Context(), userID); err != nil {
		writeError(c, err)
		return
	}
	if err := h.service.DeleteTitle(c.Request.Context(), userID, titleID); err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// SetAdminUserDefaultTitle marks a target user's title as the default title.
func (h *Handler) SetAdminUserDefaultTitle(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	userID, ok := parseID(c, "user_id")
	if !ok {
		return
	}
	titleID, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.EnsureActiveUser(c.Request.Context(), userID); err != nil {
		writeError(c, err)
		return
	}
	title, err := h.service.SetDefaultTitle(c.Request.Context(), userID, titleID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, title)
}

// ListAdminEligibleOrders returns all historical eligible orders for a target user.
func (h *Handler) ListAdminEligibleOrders(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	userID, ok := parseID(c, "user_id")
	if !ok {
		return
	}
	orders, err := h.service.ListAdminEligibleOrders(c.Request.Context(), userID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, gin.H{"items": orders})
}

// CreateAdminApplication creates a pending application for the selected user.
func (h *Handler) CreateAdminApplication(c *gin.Context) {
	subject, ok := authSubject(c)
	if !ok || !requireSystemAdmin(c) {
		return
	}
	var req createAdminApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	app, err := h.service.CreateAdminApplication(c.Request.Context(), subject.UserID, CreateApplicationInput{
		UserID:   req.UserID,
		TitleID:  req.TitleID,
		OrderIDs: req.OrderIDs,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	response.Created(c, app)
}

type createAdminApplicationRequest struct {
	UserID   int64   `json:"user_id"`
	TitleID  int64   `json:"title_id"`
	OrderIDs []int64 `json:"order_ids"`
}
