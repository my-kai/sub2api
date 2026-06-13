package runtime

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// MainUserServiceReader 适配主仓 UserService，向 custom 生图模块暴露最小用户快照。
type MainUserServiceReader struct {
	service *service.UserService
}

// NewMainUserServiceReader 创建主仓用户读取适配器。
func NewMainUserServiceReader(userService *service.UserService) *MainUserServiceReader {
	return &MainUserServiceReader{service: userService}
}

// GetUserProfile 按用户 ID 读取 custom 生图需要的安全字段。
func (r *MainUserServiceReader) GetUserProfile(ctx context.Context, id int64) (UserProfile, error) {
	if r == nil || r.service == nil {
		return UserProfile{}, ErrUnauthorized
	}
	user, err := r.service.GetByID(ctx, id)
	if err != nil {
		return UserProfile{}, err
	}
	return userProfileFromServiceUser(user), nil
}

// MainAdminUserLookup 用主仓管理员用户列表能力实现 custom 用户候选搜索。
type MainAdminUserLookup struct {
	admin service.AdminService
}

// NewMainAdminUserLookup 创建管理员用户候选搜索适配器。
func NewMainAdminUserLookup(admin service.AdminService) *MainAdminUserLookup {
	return &MainAdminUserLookup{admin: admin}
}

// SearchAdminUsers 搜索可配置并发覆盖的用户候选。
func (l *MainAdminUserLookup) SearchAdminUsers(ctx context.Context, query string, limit int) ([]AdminUserSummary, error) {
	if l == nil || l.admin == nil {
		return []AdminUserSummary{}, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	includeSubscriptions := false
	users, _, err := l.admin.ListUsers(ctx, 1, limit, service.UserListFilters{
		Search:               query,
		IncludeSubscriptions: &includeSubscriptions,
	}, "id", pagination.SortOrderAsc)
	if err != nil {
		return nil, err
	}
	items := make([]AdminUserSummary, 0, len(users))
	for i := range users {
		items = append(items, adminUserSummaryFromServiceUser(&users[i]))
	}
	return items, nil
}

// GetAdminUser 按用户 ID 读取管理员选择器展示所需字段。
func (l *MainAdminUserLookup) GetAdminUser(ctx context.Context, userID int64) (AdminUserSummary, error) {
	if l == nil || l.admin == nil {
		return AdminUserSummary{}, ErrUnauthorized
	}
	user, err := l.admin.GetUser(ctx, userID)
	if err != nil {
		return AdminUserSummary{}, err
	}
	return adminUserSummaryFromServiceUser(user), nil
}

func userProfileFromServiceUser(user *service.User) UserProfile {
	if user == nil {
		return UserProfile{}
	}
	return UserProfile{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		Role:        user.Role,
		Concurrency: user.Concurrency,
	}
}

func adminUserSummaryFromServiceUser(user *service.User) AdminUserSummary {
	if user == nil {
		return AdminUserSummary{}
	}
	return AdminUserSummary{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Role:     user.Role,
	}
}
