// Package bot contains telegram bot
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/biisal/fast-stream-bot/config"
	botutils "github.com/biisal/fast-stream-bot/internal/bot/bot-utils"
	"github.com/biisal/fast-stream-bot/internal/bot/commands"
	repo "github.com/biisal/fast-stream-bot/internal/database/psql/sqlc"
	"github.com/biisal/fast-stream-bot/internal/service/user"
	"github.com/biisal/fast-stream-bot/internal/types"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"
)

type Bot struct {
	WorkingPressure int
	Default         bool
	BotUserName     string
	Dispatcher      *tg.UpdateDispatcher
	Ctx             context.Context
	Client          *telegram.Client
	Sender          *message.Sender
	Cfg             *config.Config
	userService     user.Service
}

func NewBot(ctx context.Context, cfg *config.Config,
	client *telegram.Client, dispatcher *tg.UpdateDispatcher,
	userService user.Service, isDefault bool,
) *Bot {
	api := tg.NewClient(client)
	sender := message.NewSender(api)
	return &Bot{
		Default:     isDefault,
		Ctx:         ctx,
		Client:      client,
		Dispatcher:  dispatcher,
		Cfg:         cfg,
		Sender:      sender,
		userService: userService,
	}
}

func (b *Bot) HandleRefer(userInfo *user.TgUser, m *tg.Message, e tg.Entities, builder *message.Builder) (tg.UpdatesClass, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	prefix := "/start ref"
	refIdStr := strings.TrimSpace(strings.TrimPrefix(m.Message, prefix))
	if refIdStr == "" {
		return nil, nil
	}

	re := regexp.MustCompile(`^\d+$`)
	if !re.MatchString(refIdStr) {
		return nil, nil
	}

	refId, err := strconv.ParseInt(refIdStr, 10, 64)
	if err != nil || refId == userInfo.ID {
		return nil, nil
	}

	refUser, err := b.userService.IncrementCredits(ctx, refId, b.Cfg.INCREMENT_CREDITS, false)
	if err != nil {
		return nil, nil
	}
	_, refUserPeer, err := botutils.GetUserPeer(b.Client.API(), ctx, refId)
	if err != nil {
		return nil, nil
	}
	refUrl := botutils.GetReferLink(b.BotUserName, refId)
	msg := fmt.Sprintf("Hurray! Someone just used your referral code! 🎉\nYour credits have increased to %d.", refUser.Credit)
	btn := markup.InlineKeyboard(
		markup.Row(
			markup.URL("Refer More", refUrl),
		),
	)

	return b.Sender.To(refUserPeer.InputPeer()).Markup(btn).Text(ctx, msg)
}

func (b *Bot) validateAndGetUser(ctx context.Context, m *tg.Message,
	e tg.Entities, builder *message.Builder,
) (*user.TgUser, *repo.User, error) {
	userInfo := b.userService.GetUserInfo(ctx, m, e)
	dbUser, err := b.userService.GetUserByTgID(ctx, userInfo.ID)
	if err != nil {
		if !errors.Is(err, types.ErrorNotFound) {
			slog.Error("Failed to get user", "error", err)
			return nil, nil, fmt.Errorf("Internal server error")
		}
		dbUser, err = b.userService.CreateUser(ctx, repo.CreateUserParams{
			ID:     userInfo.ID,
			Credit: b.Cfg.INITIAL_CREDITS,
		})
		if err != nil && !errors.Is(err, types.ErrorDuplicate) {
			slog.Error("Failed to create user", "error", err)
			return nil, nil, fmt.Errorf("Internal server error")
		}
		go func() {
			// logMsg := fmt.Sprintf("New user joined\n\nId: %d,\nUsername: @%s", userInfo.Id, userInfo.Username)
			b.HandleRefer(userInfo, m, e, builder)
			// b.SendLogMessage(logMsg)
		}()
	}
	if dbUser.IsBanned {
		errMsg := fmt.Errorf("You are banned to use this bot\nContact admin for more info")
		builder.Text(ctx, errMsg.Error())
		return nil, nil, errMsg
	}
	if dbUser.Credit < b.Cfg.MAX_CREDITS &&
		(dbUser.LastCreditUpdate.Time.IsZero() || dbUser.LastCreditUpdate.Time.Day() != time.Now().Day()) {
		dbUser, err = b.userService.IncrementCredits(ctx, dbUser.ID, int32(b.Cfg.INCREMENT_CREDITS), true)
		if err != nil {
			slog.Error("Failed to increment credit", "error", err)
			return nil, nil, fmt.Errorf("Internal server error")
		}
	}
	return userInfo, dbUser, nil
}

func (b *Bot) SetUpOnMessage() {
	b.Dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewMessage) error {
		m, ok := update.Message.(*tg.Message)
		if !ok || m.Out {
			return nil
		}

		builder := b.Sender.Reply(e, update)
		userInfo, dbUser, err := b.validateAndGetUser(ctx, m, e, builder)
		if err != nil {
			return err
		}
		bc := commands.NewContext(ctx, m, e, builder, b.Client, b.Sender, userInfo, dbUser, b.userService, b.Cfg)
		switch m.Media.(type) {
		case *tg.MessageMediaDocument, *tg.MessageMediaPhoto:
			_, err = bc.MediaForwarding(commands.MediaForwardParams{Cfg: b.Cfg, Update: update, Client: b.Client})
			if err != nil {
				slog.Error("Failed to forward message", "error", err)
			}
			return err
		default:
			switch val := strings.TrimSpace(m.Message); {
			case val == "/broadcast":
				_, err = bc.HandleBroadcast(b.Cfg.ADMIN_ID)
			case val == "/help":
				_, err = bc.HandleHelp(b.Cfg.ADMIN_ID)
			case val == "/stat":
				_, err = bc.HandleStat(b.Cfg.ADMIN_ID)
			case strings.HasPrefix(val, "/unban"):
				_, err = bc.HandleToggleBan(b.Cfg.ADMIN_ID, false)
			case strings.HasPrefix(val, "/ban"):
				_, err = bc.HandleToggleBan(b.Cfg.ADMIN_ID, true)
			case strings.HasPrefix(val, "/report"):
				_, err = bc.HandleReport(b.Cfg.ADMIN_ID)
			default:
				if strings.HasPrefix(val, "/") && !strings.HasPrefix(val, "/start") {
					_, err = bc.HandleSendCommandList(b.Cfg.ADMIN_ID)
					return err
				}
				_, err = bc.HandleStart()
			}
			return err
		}
	})
}
