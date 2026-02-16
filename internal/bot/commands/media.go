package commands

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/biisal/fast-stream-bot/config"
	botutils "github.com/biisal/fast-stream-bot/internal/bot/bot-utils"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"
)

type MediaForwardParams struct {
	Cfg    *config.Config
	Update *tg.UpdateNewMessage
	Client *telegram.Client
}

func (bc *Context) MediaForwarding(params MediaForwardParams) (tg.UpdatesClass, error) {
	if params.Cfg.REF {
		if bc.dbUser.Credit < params.Cfg.MIN_CREDITS_REQUIRED {
			referUrl := botutils.GetReferLink(bc.userInfo.Username, bc.userInfo.ID)
			now := time.Now()
			nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
			btn := markup.InlineKeyboard(
				markup.Row(
					markup.URL("Get Credits By Refer", referUrl),
				),
			)
			msg := fmt.Sprintf(
				"You're out of credits! 😢\nYou need %d more credits to use this bot.\n\nWait for %s to get new credits or Refer one user to earn %d credits.",
				params.Cfg.MIN_CREDITS_REQUIRED-bc.dbUser.Credit,
				nextMidnight.Sub(now).Round(time.Second).String(),
				params.Cfg.INCREMENT_CREDITS,
			)
			return bc.builder.Markup(btn).Text(bc.ctx, msg)
		}
	}

	m := params.Update.Message.(*tg.Message)
	fromPeer := &tg.InputPeerUser{UserID: bc.userInfo.ID, AccessHash: bc.userInfo.AccessHash}
	file, err := botutils.GetMediaFromMessage(m)
	if err != nil {
		slog.Error("Failed to get media from message", "error", err)
		return nil, err
	}
	msgHash := botutils.MakeHashByFileInfo(file)

	_, channelInputPeer, err := botutils.GetChannelPeer(params.Client.API(), bc.ctx, params.Cfg.DB_CHANNEL_ID)
	if err != nil {
		slog.Error("Failed to get channel peer", "error", err)
		return nil, err
	}
	fUpdate, err := bc.sender.To(channelInputPeer.InputPeer()).ForwardIDs(fromPeer, m.ID).Send(bc.ctx)
	if err != nil {
		slog.Error("Failed to forward message", "error", err)
		return nil, err
	}
	messageId := fUpdate.(*tg.Updates).Updates[0].(*tg.UpdateMessageID).ID
	streamLink := fmt.Sprintf("%s/watch/%d?hash=%s", params.Cfg.FQDN, messageId, msgHash)
	fileMsg := fmt.Sprintf(
		"File Name: %s\nFile Size: %s\n\nLink: %s",
		file.FileName, botutils.MakeSizeReadable(file.Size), streamLink,
	)
	bc.dbUser, err = bc.userService.DecrementCredits(bc.ctx, bc.userInfo.ID, params.Cfg.DECREMENT_CREDITS)
	if err != nil {
		slog.Error("Failed to decrement credit", "error", err)
		return nil, err
	}

	msg := fmt.Sprintf(
		"Your file is ready to watch or download!\n\n%s",
		fileMsg,
	)

	if params.Cfg.REF {
		msg += fmt.Sprintf("\n\nYou have %d credits to use 😊", bc.dbUser.Credit)
	}

	btn := markup.InlineKeyboard(
		markup.Row(
			markup.URL("Watch or Download", streamLink),
		),
	)

	if _, err = bc.builder.Markup(btn).Text(bc.ctx, msg); err != nil {
		slog.Error("Failed to send message", "error", err, "Link", streamLink)
		if _, err := bc.userService.IncrementCredits(bc.ctx, bc.userInfo.ID, params.Cfg.DECREMENT_CREDITS, false); err != nil {
			slog.Error("Failed to increment credit", "error", err)
		}
		return nil, err
	}
	if bc.dbUser, err = bc.userService.IncrementTotalLinkCount(bc.ctx, bc.dbUser.ID); err != nil {
		slog.Error("Failed to update user", "error", err)
	}
	fileMsg = fmt.Sprintf("User: %s\nUserId: %d\n\n%s",
		bc.userInfo.Username, bc.userInfo.ID, fileMsg)
	_, channelInputPeer, err = botutils.GetChannelPeer(params.Client.API(), bc.ctx, params.Cfg.DB_CHANNEL_ID)
	if err != nil {
		slog.Error("Failed to get channel peer", "error", err)
		return nil, err
	}
	if bc.userInfo.ID != params.Cfg.ADMIN_ID {
		_, err = params.Client.API().MessagesDeleteMessages(bc.ctx, &tg.MessagesDeleteMessagesRequest{
			Revoke: true,
			ID:     []int{m.ID},
		})
		if err != nil {
			slog.Error("Failed to delete user message", "error", err)
		}
	}
	return bc.sender.To(channelInputPeer.InputPeer()).Reply(messageId).Text(bc.ctx, fileMsg)
}
