package proxy

import (
	"context"
	"fmt"
	"sync"

	identityclient "github.com/nusiss-capstone-project/identity-mservice/client"
	"github.com/nusiss-capstone-project/identity-mservice/common/identitypb"
	"github.com/nusiss-capstone-project/payment-mservice/server/config"
)

type UserProxy interface {
	GetUser(ctx context.Context, userId int64) (*identitypb.GetUserProfileResponse, error)
}

type userProxyImpl struct {
	client identitypb.IdentityServiceClient
}

func (u *userProxyImpl) GetUser(ctx context.Context, userId int64) (*identitypb.GetUserProfileResponse, error) {
	if userId <= 0 {
		return nil, fmt.Errorf("invalid user id: %d", userId)
	}

	resp, err := u.client.GetUserProfile(ctx, &identitypb.GetUserProfileRequest{
		UserId: userId,
	})
	if err != nil {
		return nil, fmt.Errorf("get user profile: %w", err)
	}
	if resp.GetBaseInfo() == nil {
		return nil, fmt.Errorf("get user profile: missing base info")
	}
	if code := resp.GetBaseInfo().GetCode(); code != identitypb.ErrorCode_ERROR_CODE_OK {
		return nil, fmt.Errorf("get user profile: code=%s message=%s",
			code.String(), resp.GetBaseInfo().GetMessage())
	}
	return resp, nil
}

var (
	userProxy         UserProxy
	userProxySyncOnce sync.Once
)

func GetUserProxy() UserProxy {
	userProxySyncOnce.Do(func() {
		cfg := config.Config.IdentityConfig
		if cfg == nil || cfg.Host == "" || cfg.Port == 0 {
			panic("identity config (host/port) is required")
		}

		client, err := identityclient.GetIdentityServiceClient(&identityclient.GRpcClientConfig{
			Host: cfg.Host,
			Port: cfg.Port,
		})
		if err != nil {
			panic(fmt.Sprintf("init identity client: %v", err))
		}

		userProxy = &userProxyImpl{client: client}
	})
	return userProxy
}

func InitUserProxy() {
	_ = GetUserProxy()
}
