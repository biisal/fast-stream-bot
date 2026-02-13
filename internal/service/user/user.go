// Package user contains user service
package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	repo "github.com/biisal/fast-stream-bot/internal/database/sqlite/sqlc"
	rs "github.com/biisal/fast-stream-bot/internal/redis"
	"github.com/biisal/fast-stream-bot/internal/types"
	sqlite "modernc.org/sqlite"

	"github.com/gotd/td/tg"
)

type Service interface {
	GetUserByTgID(ctx context.Context, tgID int64) (*repo.User, error)
	CreateUser(ctx context.Context, params repo.CreateUserParams) (*repo.User, error)
	IncrementCredits(ctx context.Context, id int64, credit int32, updateDate bool) (*repo.User, error)
	GetUsersCount(ctx context.Context) (int64, error)
	DecrementCredits(ctx context.Context, id int64, credit int32) (*repo.User, error)
	IncrementTotalLinkCount(ctx context.Context, id int64) (*repo.User, error)
	GetAllUsers(ctx context.Context) ([]*repo.User, error)
	UpdateUser(ctx context.Context, user *repo.User) (*repo.User, error)
	GetUserInfo(ctx context.Context, m *tg.Message, e tg.Entities) *TgUser
}

type svc struct {
	redisService rs.RedisService
	repo         repo.Querier
	ttl          time.Duration
}

func NewService(repo repo.Querier, redis rs.RedisService, ttl time.Duration) Service {
	return &svc{
		repo:         repo,
		redisService: redis,
		ttl:          ttl,
	}
}

func (s *svc) GetUserByTgID(ctx context.Context, tgID int64) (*repo.User, error) {
	key := fmt.Sprintf("user:%d", tgID)
	redisValue := s.redisService.Get(ctx, key)
	var err error
	if len(redisValue) > 0 {
		var u repo.User
		if err = json.Unmarshal(redisValue, &u); err == nil {
			return &u, nil
		}
		slog.Warn("Failed to unmarshal user from redis continue to get from db", "error", err)
	}

	u, err := s.repo.GetUserByID(ctx, tgID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, types.ErrorNotFound
		}
		return nil, err
	}
	s.redisService.Set(ctx, key, u, s.ttl)
	return u, nil
}

func (s *svc) CreateUser(ctx context.Context, params repo.CreateUserParams) (*repo.User, error) {
	user, err := s.repo.CreateUser(ctx, params)
	if err != nil {
		var sqliteErr *sqlite.Error
		if errors.As(err, &sqliteErr) {
			if sqliteErr.Code() == 2067 { // SQLITE_CONSTRAINT_UNIQUE
				return nil, types.ErrorDuplicate
			}
		}
		return nil, err
	}
	key := fmt.Sprintf("user:%d", user.ID)
	s.redisService.Set(ctx, key, user, s.ttl)
	return user, nil
}

func (s *svc) IncrementCredits(ctx context.Context, id int64, credit int32, updateDate bool) (*repo.User, error) {
	var u *repo.User
	var err error
	key := fmt.Sprintf("user:%d", id)
	if updateDate {
		u, err = s.repo.IncrementCreditWithDate(ctx, repo.IncrementCreditWithDateParams{
			ID:     id,
			Credit: credit,
		})
		if err != nil {
			return nil, err
		}
	} else {
		u, err = s.repo.IncrementCredit(ctx, repo.IncrementCreditParams{
			ID:     id,
			Credit: credit,
		})
		if err != nil {
			return nil, err
		}
	}
	s.redisService.Set(ctx, key, u, s.ttl)
	return u, nil
}

func (s *svc) GetUsersCount(ctx context.Context) (int64, error) {
	return s.repo.GetTotalActiveUsersCount(ctx)
}

func (s *svc) DecrementCredits(ctx context.Context, id int64, credit int32) (*repo.User, error) {
	u, err := s.repo.DecrementCredit(ctx, repo.DecrementCreditParams{
		ID:     id,
		Credit: credit,
	})
	if err != nil {
		slog.Error("Failed to decrement credits", "error", err)
		return nil, err
	}
	s.redisService.Set(ctx, fmt.Sprintf("user:%d", id), u, s.ttl)
	return u, nil
}

func (s *svc) IncrementTotalLinkCount(ctx context.Context, id int64) (*repo.User, error) {
	u, err := s.repo.IncrementTotalLinks(ctx, id)
	if err != nil {
		return nil, err
	}
	s.redisService.Set(ctx, fmt.Sprintf("user:%d", id), u, s.ttl)
	return u, nil
}

func (s *svc) GetAllUsers(ctx context.Context) ([]*repo.User, error) {
	return s.repo.GetAllUsers(ctx)
}

func (s *svc) UpdateUser(ctx context.Context, user *repo.User) (*repo.User, error) {
	params := repo.UpdateUserByIDParams{
		ID:         user.ID,
		IsBanned:   user.IsBanned,
		IsPremium:  user.IsPremium,
		IsVerified: user.IsVerified,
		TotalLinks: user.TotalLinks,
	}
	u, err := s.repo.UpdateUserByID(ctx, params)
	if err != nil {
		return nil, err
	}
	s.redisService.Set(ctx, fmt.Sprintf("user:%d", user.ID), u, s.ttl)
	return u, nil
}

func (s *svc) GetUserInfo(ctx context.Context, m *tg.Message, e tg.Entities) *TgUser {
	var userID int64
	if u, ok := m.FromID.(*tg.PeerUser); ok {
		userID = u.UserID
	} else if u, ok := m.PeerID.(*tg.PeerUser); ok {
		userID = u.UserID
	} else {
		return nil
	}

	key := fmt.Sprintf("tguser:%d", userID)
	userData := s.redisService.Get(ctx, key)

	tgUser := &TgUser{}
	if len(userData) > 0 {
		if err := json.Unmarshal(userData, tgUser); err == nil {
			return tgUser
		}
		slog.Error("Failed to unmarshal user from redis continue to get from db")
	}

	switch peer := m.PeerID.(type) {
	case *tg.PeerUser:
		tgUser = NewTgUser(peer.UserID, e.Users[peer.UserID].Username,
			e.Users[peer.UserID].FirstName,
			e.Users[peer.UserID].LastName,
			e.Users[peer.UserID].AccessHash)
	default:
		if m.FromID != nil {
			if fromUser, ok := m.FromID.(*tg.PeerUser); ok {
				tgUser = NewTgUser(fromUser.UserID, e.Users[fromUser.UserID].Username,
					e.Users[fromUser.UserID].FirstName,
					e.Users[fromUser.UserID].LastName,
					e.Users[fromUser.UserID].AccessHash)
			}
		}
	}
	s.redisService.Set(ctx, key, tgUser, s.ttl)
	return tgUser
}
