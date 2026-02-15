// Package commands contains commands of bot
package commands

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/biisal/fast-stream-bot/config"
	botutils "github.com/biisal/fast-stream-bot/internal/bot/bot-utils"
	repo "github.com/biisal/fast-stream-bot/internal/database/psql/sqlc"
	"github.com/biisal/fast-stream-bot/internal/service/user"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"
)

type Context struct {
	ctx         context.Context
	msg         *tg.Message
	entities    tg.Entities
	builder     *message.Builder
	userInfo    *user.TgUser
	dbUser      *repo.User
	userService user.Service
	sender      *message.Sender
	client      *telegram.Client
	cfg         *config.Config
}

func (bc *Context) Reply(msg string) (tg.UpdatesClass, error) {
	return bc.builder.Text(bc.ctx, msg)
}

func NewContext(ctx context.Context, msg *tg.Message,
	entities tg.Entities, builder *message.Builder,
	client *telegram.Client, sender *message.Sender,
	userInfo *user.TgUser, dbUser *repo.User, userService user.Service, cfg *config.Config,
) *Context {
	return &Context{
		ctx, msg, entities, builder, userInfo,
		dbUser, userService, sender, client, cfg,
	}
}

const helpMsg = `Using this bot is easy!
	
1. Send me a file.
2. I’ll create a link for it.
3. Open the link in your browser to stream or download the file instantly.`

func (bc *Context) HandleSendCommandList(adminID int64) (tg.UpdatesClass, error) {
	msg := `Available commands:
/start - Start the bot
/help - Get help
/stat - Get your statistics
/report - Replay a message to report to admin`

	if bc.userInfo.ID == adminID {
		msg += `/broadcast - Broadcast a message to all users

"/ban - Ban a user
"/unban - Unban a user`
	}

	return bc.Reply(msg)
}

func (bc *Context) HandleStart() (tg.UpdatesClass, error) {
	username := "@" + bc.userInfo.Username
	if bc.userInfo.Username == "" {
		username = strings.TrimSpace(bc.userInfo.FirstName + " " + bc.userInfo.LastName)
	}
	msg := fmt.Sprintf(`Hello %s,
Welcome! I’m your file streaming bot.
Simply send me a file, and I’ll generate a fast, streamable link so you can watch it online or download it anytime, anywhere.
Please don't send any sensitive adult content or any illegal content. Otherwise, you will be banned permanently.
Your Credits: %d

for help send /help`, username, bc.dbUser.Credit)

	shareLink := botutils.GetReferLink(bc.userInfo.Username, bc.userInfo.ID)
	keyboard := markup.InlineKeyboard(
		markup.Row(
			markup.URL("Refer", shareLink),
		),
	)
	return bc.builder.Markup(keyboard).Text(bc.ctx, msg)
}

func (bc *Context) HandleHelp(adminID int64) (tg.UpdatesClass, error) {
	commands := `Available commands:
/start - Start the bot
/help - Get help 
/stat - Get your statistics
`
	if bc.userInfo.ID == adminID {
		commands += `/broadcast - Broadcast a message to all users
/ban - Ban a user 
/unban - Unban a user`
	}

	fullMsg := fmt.Sprintf("%s\n\n%s", helpMsg, commands)
	return bc.Reply(fullMsg)
}

func (b *Context) SendLogMessage(msg string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, inputPeer, err := botutils.GetChannelPeer(b.client.API(), ctx, b.cfg.LOG_CHANNEL_ID)
	if err != nil {
		return err
	}
	b.sender.To(inputPeer.InputPeer()).Text(ctx, msg)
	return nil
}

func (b *Context) SendMainChannrlInviteLink(ctx context.Context, builder *message.Builder) (tg.UpdatesClass, error) {
	inviteLink, err := botutils.GetMainChannelInviteLink(ctx, b.client.API(), b.cfg)
	if err != nil {
		return nil, err
	}
	msg := `Due to server overload only our channel users can use this bot!
Please join our channel and continue using this bot :)`
	keyboard := markup.InlineKeyboard(
		markup.Row(
			markup.URL("Join Channel", inviteLink),
		),
	)

	return builder.Markup(keyboard).Text(ctx, msg)
}

func (bc *Context) HandleStat(adminID int64) (tg.UpdatesClass, error) {
	statMsg := fmt.Sprintf("Your statistics:\n\nTotal links: %d\nTotal credits: %d", bc.dbUser.TotalLinks,
		bc.dbUser.Credit)
	if bc.userInfo.ID == adminID {
		totalUserCount, err := bc.userService.GetUsersCount(bc.ctx)
		if err != nil {
			slog.Error("Failed to get total users count", "error", err)
			statMsg += fmt.Sprintf("\nFailed to get total users count! Err : %s\n", err.Error())
		}
		statMsg += fmt.Sprintf("\n\nTotal users: %d\n", totalUserCount)
	}
	return bc.Reply(statMsg)
}

func (bc *Context) ForwardMsgToLogChannel(replyedMessageID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, inputPeer, err := botutils.GetChannelPeer(bc.client.API(), ctx, bc.cfg.LOG_CHANNEL_ID)
	if err != nil {
		return err
	}

	fromPeer := &tg.InputPeerUser{UserID: bc.userInfo.ID, AccessHash: bc.userInfo.AccessHash}
	_, err = bc.sender.To(inputPeer.InputPeer()).ForwardIDs(fromPeer, replyedMessageID).Send(bc.ctx)
	return err
}

func (bc *Context) HandleReport(adminId int64) (tg.UpdatesClass, error) {
	if bc.userInfo.ID == adminId {
		return bc.Reply("not for admin")
	}
	var replyedMessageId int
	switch replay := bc.msg.ReplyTo.(type) {
	case *tg.MessageReplyHeader:
		replyedMessageId = replay.ReplyToMsgID
	case *tg.MessageReplyStoryHeader:
		return bc.Reply("can't report story")
	default:
		return bc.Reply("Reply a valid message to report")
	}

	fromPeer := &tg.InputPeerUser{UserID: bc.userInfo.ID, AccessHash: bc.userInfo.AccessHash}
	_, adminPeer, err := botutils.GetUserPeer(bc.client.API(), bc.ctx, adminId)
	if err != nil {
		if err = bc.ForwardMsgToLogChannel(replyedMessageId); err != nil {
			slog.Error("Failed to forward report message to log channel", "error", err)
			return bc.Reply("Failed to report your messsage! please try again later")
		}
		return bc.Reply("Reported your message to admin")
	}

	randomID := time.Now().UnixNano()
	_, err = bc.client.API().MessagesForwardMessages(bc.ctx, &tg.MessagesForwardMessagesRequest{
		FromPeer:   fromPeer,
		ToPeer:     adminPeer.InputPeer(),
		ID:         []int{replyedMessageId},
		RandomID:   []int64{randomID},
		DropAuthor: false,
	})
	if err != nil {
		if err = bc.ForwardMsgToLogChannel(replyedMessageId); err != nil {
			slog.Error("Failed to forward report message to log channel", "error", err)
			return bc.Reply("Failed to report your messsage! please try again later")
		}
		return bc.Reply("Reported your message to admin")
	}
	return bc.Reply("Reported your message to admin")
}
