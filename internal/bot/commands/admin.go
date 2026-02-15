package commands

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	botutils "github.com/biisal/fast-stream-bot/internal/bot/bot-utils"
	repo "github.com/biisal/fast-stream-bot/internal/database/psql/sqlc"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

func (bc *Context) HandleBroadcast(adminId int64) (tg.UpdatesClass, error) {
	isAdmin := bc.userInfo.ID == adminId
	if !isAdmin {
		msg := "Only admin can use this command! :)"
		return bc.Reply(msg)
	}
	replyedMessageHeader := bc.msg.ReplyTo
	if replyedMessageHeader == nil {
		return bc.Reply("Please reply to a message to broadcast it.")
	}

	var replyedMessageId int
	switch replay := replyedMessageHeader.(type) {
	case *tg.MessageReplyHeader:
		replyedMessageId = replay.ReplyToMsgID
	case *tg.MessageReplyStoryHeader:
		return bc.Reply("can't broadcast story")
	default:
		return bc.Reply(fmt.Sprintf("can't broadcast! Unknown type: %T", replay))
	}

	users, err := bc.userService.GetAllUsers(bc.ctx)
	if err != nil {
		return bc.Reply(fmt.Sprintf("Failed to get users! Err : %s", err.Error()))
	}

	peerManager := peers.Options.Build(peers.Options{}, bc.client.API())
	fromPeer, err := peerManager.ResolvePeer(bc.ctx, &tg.PeerUser{UserID: adminId})
	if err != nil {
		slog.Error("Failed to resolve peer", "error", err)
		return bc.Reply(fmt.Sprintf("Failed to get users! Err : %s", err.Error()))
	}
	usersLen := len(users)

	if usersLen == 0 {
		return bc.Reply("No users found!")
	}

	wg := &sync.WaitGroup{}
	var (
		completedCounter atomic.Int32
		failedCounter    atomic.Int32
		userChan         = make(chan *repo.User)
		workerCount      = min(usersLen, 3)
	)
	for idx := range workerCount {
		slog.Info("Starting worker", "No. ", idx+1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			botutils.BroadcastToUsers(bc.ctx,
				userChan, replyedMessageId,
				adminId, bc.client,
				fromPeer, peerManager,
				&completedCounter, &failedCounter)
		}()
	}

	updateMsgText := fmt.Sprintf("Broadcasting to %d users", usersLen)
	randomId, _ := bc.client.RandInt64()
	updateMsg, err := bc.client.API().MessagesSendMessage(bc.ctx, &tg.MessagesSendMessageRequest{
		Peer:     fromPeer.InputPeer(),
		Message:  updateMsgText,
		RandomID: randomId,
	})
	if err != nil {
		return bc.Reply(fmt.Sprintf("Failed to send message! Err : %s", err.Error()))
	}

	updateMsgDetails, ok := updateMsg.(*tg.UpdateShortSentMessage)
	if !ok {
		return bc.Reply(fmt.Sprintf("Failed to cast to UpdateShortSentMessage! Got Type: %T", updateMsg))
	}
	updateMsgId := updateMsgDetails.ID

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(userChan)
		for idx, user := range users {
			userChan <- user
			if idx > 0 && idx%30 == 0 {
				msg := fmt.Sprintf("Completed %d, Failed %d", completedCounter.Load(), failedCounter.Load())
				bc.sender.To(fromPeer.InputPeer()).Edit(updateMsgId).Text(bc.ctx, msg)
			}
		}
	}()

	wg.Wait()
	doneMsg := fmt.Sprintf("Done! Completed %d, Failed %d", completedCounter.Load(), failedCounter.Load())
	return bc.sender.To(fromPeer.InputPeer()).Edit(updateMsgId).Text(bc.ctx, doneMsg)
}

func (bc *Context) HandleToggleBan(adminId int64, banStatus bool) (tg.UpdatesClass, error) {
	if bc.userInfo.ID != adminId {
		msg := "Only admin can use this command! :)"
		return bc.Reply(msg)
	}
	parts := strings.Split(bc.msg.Message, " ")
	command := strings.Split(parts[0], "/")[1]
	if len(parts) < 2 {
		return bc.Reply(fmt.Sprintf("Please provide user id to %s!\nUsage: %s <user_id>", command, parts[0]))
	}

	targetId, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return bc.Reply(fmt.Sprintf("Failed to parse user id! Err : %s", err.Error()))
	}

	if targetId == adminId {
		return bc.Reply(fmt.Sprintf("You can't %s admin!", command))
	}
	targetUser, err := bc.userService.GetUserByTgID(bc.ctx, targetId)
	if err != nil {
		return bc.Reply(fmt.Sprintf("Failed to get user! Err : %s", err.Error()))
	}
	if targetUser.IsBanned == banStatus {
		return bc.Reply(fmt.Sprintf("User is already %sed!", command))
	}
	targetUser.IsBanned = banStatus
	if targetUser, err = bc.userService.UpdateUser(bc.ctx, targetUser); err != nil {
		return bc.Reply(fmt.Sprintf("Failed to update user! Err : %s", err.Error()))
	}
	_, targetInputPeer, err := botutils.GetUserPeer(bc.client.API(), bc.ctx, targetUser.ID)
	if err != nil {
		bc.Reply(fmt.Sprintf("Failed to get user peer! Err : %s", err.Error()))
	}
	_, err = bc.sender.To(targetInputPeer.InputPeer()).Text(bc.ctx, fmt.Sprintf("You have been %sed by admin!", command))
	if err != nil {
		slog.Error("Failed to send message", "error", err)
	}
	return bc.Reply(fmt.Sprintf("User %sed successfully!", command))
}
