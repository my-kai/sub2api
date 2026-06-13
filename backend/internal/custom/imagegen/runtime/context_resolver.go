package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// UserSnapshotReader 由 tasks-3 在主仓薄接入阶段适配 UserService 或仓储实现。
//
// 生图核心包只需要展示快照，不关心主仓完整用户模型；因此这里保持窄接口，
// 避免 custom 模块反向依赖主仓 profile/balance 旧链路。
type UserSnapshotReader interface {
	GetUserProfile(ctx context.Context, id int64) (UserProfile, error)
}

// UserResolver 从 gin.Context 里的主仓 JWT 认证结果恢复 custom 生图用户快照。
type UserResolver interface {
	RequireUser(c *gin.Context) (UserProfile, error)
	RequireAdmin(c *gin.Context) (UserProfile, error)
	OptionalUser(c *gin.Context) (UserProfile, bool)
}

// ContextUserResolver 读取主仓 middleware 写入的 AuthSubject 和 user_role。
type ContextUserResolver struct {
	reader UserSnapshotReader
}

// NewContextUserResolver 创建面向主仓 JWT 上下文的用户解析器。
func NewContextUserResolver(reader UserSnapshotReader) *ContextUserResolver {
	return &ContextUserResolver{reader: reader}
}

// RequireUser 要求请求已经经过主仓 JWT 中间件。
func (r *ContextUserResolver) RequireUser(c *gin.Context) (UserProfile, error) {
	user, ok := r.OptionalUser(c)
	if !ok {
		return UserProfile{}, ErrUnauthorized
	}
	return user, nil
}

// RequireAdmin 要求当前用户具备管理员角色。
func (r *ContextUserResolver) RequireAdmin(c *gin.Context) (UserProfile, error) {
	user, err := r.RequireUser(c)
	if err != nil {
		return UserProfile{}, err
	}
	if user.Role != "admin" {
		return UserProfile{}, ErrForbidden
	}
	return user, nil
}

// OptionalUser 在公共图库等匿名可读接口里尽力读取用户上下文。
func (r *ContextUserResolver) OptionalUser(c *gin.Context) (UserProfile, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		return UserProfile{}, false
	}
	role, _ := middleware.GetUserRoleFromContext(c)
	user := UserProfile{ID: subject.UserID, Role: role, Concurrency: subject.Concurrency}
	if r != nil && r.reader != nil {
		snapshot, err := r.reader.GetUserProfile(c.Request.Context(), subject.UserID)
		if err == nil && snapshot.ID > 0 {
			user.Username = snapshot.Username
			user.Email = snapshot.Email
			if snapshot.Role != "" {
				user.Role = snapshot.Role
			}
			if snapshot.Concurrency > 0 {
				user.Concurrency = snapshot.Concurrency
			}
		}
	}
	return user, true
}

// AdminUserLookup 是管理员并发限制用户选择器所需的窄接口。
type AdminUserLookup interface {
	SearchAdminUsers(ctx context.Context, query string, limit int) ([]AdminUserSummary, error)
	GetAdminUser(ctx context.Context, userID int64) (AdminUserSummary, error)
}

// UserRepositoryAdminLookup 基于主仓用户服务/仓储实现单用户快照查询。
//
// SearchAdminUsers 需要主仓在 tasks-3 注入更完整的列表能力；这里保留默认错误，
// 防止 custom 包为了搜索能力直接依赖主仓 admin handler。
type UserRepositoryAdminLookup struct {
	reader UserSnapshotReader
}

// NewUserRepositoryAdminLookup 创建管理员用户快照查询器。
func NewUserRepositoryAdminLookup(reader UserSnapshotReader) *UserRepositoryAdminLookup {
	return &UserRepositoryAdminLookup{reader: reader}
}

// GetAdminUser 按用户 ID 读取展示快照。
func (l *UserRepositoryAdminLookup) GetAdminUser(ctx context.Context, userID int64) (AdminUserSummary, error) {
	if l == nil || l.reader == nil {
		return AdminUserSummary{}, errors.New("admin user lookup is not configured")
	}
	if userID <= 0 {
		return AdminUserSummary{}, fmt.Errorf("user id is required")
	}
	user, err := l.reader.GetUserProfile(ctx, userID)
	if err != nil {
		return AdminUserSummary{}, err
	}
	if user.ID <= 0 {
		return AdminUserSummary{}, fmt.Errorf("user not found")
	}
	return AdminUserSummary{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Role:     user.Role,
	}, nil
}

// SearchAdminUsers 由后续薄接入层注入具备列表查询能力的实现。
func (l *UserRepositoryAdminLookup) SearchAdminUsers(context.Context, string, int) ([]AdminUserSummary, error) {
	return nil, errors.New("admin user search is not configured")
}
