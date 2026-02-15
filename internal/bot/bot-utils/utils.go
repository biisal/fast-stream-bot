package botutils

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/biisal/fast-stream-bot/config"
	repo "github.com/biisal/fast-stream-bot/internal/database/psql/sqlc"
	"github.com/biisal/fast-stream-bot/internal/service/user"
	"github.com/biisal/fast-stream-bot/internal/types"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/redis/go-redis/v9"
)

var (
	cachedInviteLink string
	tgUserExpiresIn  time.Duration = 10 * time.Minute
)

func GetChannelMessage(ctx context.Context, channelID int64, messageId int, api *tg.Client) (*tg.Message, error) {
	_, inputPeer, err := GetChannelPeer(api, ctx, channelID)
	if err != nil {
		return nil, err
	}
	slog.Info("Findding message for channel", "channelId", channelID, "messageId", messageId)

	result, err := api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: inputPeer.InputChannel(),
		ID: []tg.InputMessageClass{
			&tg.InputMessageID{
				ID: messageId,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	switch res := result.(type) {

	case *tg.MessagesMessages:
		if len(res.Messages) > 0 {
			if msg, ok := res.Messages[0].(*tg.Message); ok {
				return msg, nil
			}
		}

	case *tg.MessagesMessagesSlice:
		if len(res.Messages) > 0 {
			if msg, ok := res.Messages[0].(*tg.Message); ok {
				return msg, nil
			}
		}
	case *tg.MessagesChannelMessages:
		if len(res.Messages) > 0 {
			if msg, ok := res.Messages[0].(*tg.Message); ok {
				return msg, nil
			}
		}
	default:
		return nil, fmt.Errorf("unknown result type %T", result)
	}

	return nil, fmt.Errorf("message not found for channel %d", channelID)
}

func GetMediaFromMessage(msg *tg.Message) (*types.File, error) {
	media, ok := msg.Media.(*tg.MessageMediaDocument)
	if !ok {
		return nil, fmt.Errorf("media not found")
	}

	doc := media.Document.(*tg.Document)

	var fileName string
	for _, attr := range doc.Attributes {
		if docAttr, ok := attr.(*tg.DocumentAttributeFilename); ok {
			fileName = docAttr.FileName
			break
		}
	}
	if fileName == "" {
		fileName = doc.TypeName()
	}

	return &types.File{
		Location: &tg.InputDocumentFileLocation{
			ID:         doc.GetID(),
			AccessHash: doc.AccessHash,
		},
		MimeType:   doc.MimeType,
		Size:       doc.Size,
		AccessHash: doc.AccessHash,
		FileName:   fileName,
	}, nil
}

func MakeHashByFileInfo(file *types.File) string {
	key := fmt.Sprintf("%s-%s-%d-%d", file.FileName, file.MimeType, file.Size, file.Location.ID)
	sum := sha256.Sum256([]byte(key))
	return base64.URLEncoding.EncodeToString(sum[:])[:6]
}

func CheckFileHash(file *types.File, hash string) bool {
	return MakeHashByFileInfo(file) == hash
}

func MakeSizeReadable(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "kMGTPE"[exp])
}

func CheckUserInMainChannel(ctx context.Context, client *telegram.Client, channelID int64, userID int64, userAccessHash int64, redisClient *redis.Client) bool {
	key := fmt.Sprintf("in_channel:%d:%d", channelID, userID)
	val, err := redisClient.Get(ctx, key).Result()
	if err != nil && err != redis.Nil {
		slog.Error("Failed to get value from redis", "error", err)
		return true
	}
	if val == "true" {
		return true
	}
	peerManager := peers.Options.Build(peers.Options{}, client.API())
	targetChannel, err := peerManager.ResolveChannelID(ctx, channelID)
	if err != nil {
		slog.Error("Failed to resolve channel ID", "error", err)
		return true
	}
	inputChannel := targetChannel.InputChannel()
	participant, err := client.API().ChannelsGetParticipant(ctx, &tg.ChannelsGetParticipantRequest{
		Channel: inputChannel,
		Participant: &tg.InputPeerUser{
			UserID:     userID,
			AccessHash: userAccessHash,
		},
	})
	if err != nil {
		var rpcErr *tgerr.Error
		if errors.As(err, &rpcErr) {
			if rpcErr.Code == 400 && rpcErr.Message == tg.ErrUserNotParticipant {
				return false
			}
		}
		slog.Error("Failed to get participant.", "error", err)
		return true
	}

	isInChannel := participant != nil
	if isInChannel {
		redisClient.Set(ctx, key, "true", 1*time.Minute).Err()
	}
	return isInChannel
}

func GetPublicInviteLink(ctx context.Context, targetChannel *peers.Channel, api *tg.Client) (string, error) {
	fullchannel, err := api.ChannelsGetFullChannel(ctx, targetChannel.InputChannel())
	if err != nil {
		return "", err
	}
	channel, ok := fullchannel.Chats[0].(*tg.Channel)
	if !ok {
		return "", fmt.Errorf("channel not found")
	}
	if channel.Username == "" {
		return "", fmt.Errorf("channel username not found")
	}
	inviteLink := fmt.Sprintf("https://t.me/%s", channel.Username)
	return inviteLink, nil
}

func GetPrivateInviteLink(ctx context.Context, targetChannel *peers.Channel, api *tg.Client) (string, error) {
	privateInvite, err := api.MessagesExportChatInvite(ctx, &tg.MessagesExportChatInviteRequest{
		Peer: targetChannel.InputPeer(),
	})
	if err != nil {
		return "", err
	}
	chatInvite, ok := privateInvite.(*tg.ChatInviteExported)
	if !ok {
		return "", fmt.Errorf("chat invite not found")
	}
	inviteLink := fmt.Sprintf("https://t.me/%s", chatInvite.Link)
	return inviteLink, nil
}

func GetMainChannelInviteLink(ctx context.Context, api *tg.Client, cfg *config.Config) (string, error) {
	if cachedInviteLink != "" {
		return cachedInviteLink, nil
	}
	peerManager := peers.Options.Build(peers.Options{}, api)
	targetChannel, err := peerManager.ResolveChannelID(ctx, cfg.MAIN_CHANNEL_ID)
	if err != nil {
		return "", err
	}
	link, err := GetPublicInviteLink(ctx, &targetChannel, api)
	if err == nil {
		cachedInviteLink = link
		return link, nil
	}
	link, err = GetPrivateInviteLink(ctx, &targetChannel, api)
	if err == nil {
		cachedInviteLink = link
		return link, nil
	}
	cachedInviteLink = ""
	return "", err
}

func getTgUserFromRedis(ctx context.Context, id int64, redisClient *redis.Client) (*user.TgUser, error) {
	key := fmt.Sprintf("tguser:%d", id)
	slog.Info("getting tgUser from redis", "key", key)
	tgUser := &user.TgUser{}
	b, err := redisClient.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, tgUser); err != nil {
		return nil, err
	}
	return tgUser, nil
}

func setTgUserToRedis(ctx context.Context, tgUser *user.TgUser, redisClient *redis.Client) error {
	key := fmt.Sprintf("tguser:%d", tgUser.ID)
	slog.Info("setting tgUser to redis", "key", key)
	data, err := json.Marshal(tgUser)
	if err != nil {
		return err
	}
	cmd := redisClient.Set(ctx, key, data, tgUserExpiresIn).Err()
	if cmd != nil {
		return cmd
	}
	return nil
}

type Commit struct {
	Date    string
	Message string
}

func GetCommits() []Commit {
	separator := "==="
	cmd := exec.Command("git", "log", "-3", "--pretty=%cd"+separator+"%s", "--date=short")
	output, err := cmd.Output()
	if err != nil {
		slog.Error("Failed to get commits", "error", err)
		return nil
	}
	commits := make([]Commit, 0)
	for line := range strings.SplitSeq(string(output), "\n") {
		if strings.Contains(line, separator) {
			commits = append(commits, Commit{
				Date:    strings.Split(line, separator)[0],
				Message: strings.Split(line, separator)[1],
			},
			)
		}
	}
	return commits
}

func ParseMessageAndChannelId(messageIdStr, channelIdStr string, fallbackChannelId int64) (int, int64, error) {
	messageId, err := strconv.Atoi(messageIdStr)
	if err != nil {
		slog.Error("failed to parse fileId", "error", err)
		return 0, 0, err
	}
	chnlIdLen := len(channelIdStr)
	if chnlIdLen < 10 {
		if fallbackChannelId != 0 {
			return messageId, fallbackChannelId, nil
		}
		return 0, 0, errors.New("channelId is not valid")
	}
	if chnlIdLen > 10 {
		channelIdStr = channelIdStr[chnlIdLen-10:]
	}
	channelId64, err := strconv.ParseInt(channelIdStr, 10, 64)
	if err != nil {
		if fallbackChannelId != 0 {
			return messageId, fallbackChannelId, nil
		}
		slog.Error("failed to parse channelId", "error", err)
		return 0, 0, err
	}
	return messageId, channelId64, nil
}

func GetChannelPeer(client *tg.Client, ctx context.Context, channelId int64) (*peers.Manager, *peers.Channel, error) {
	peerManager := peers.Options.Build(peers.Options{}, client)
	channelPeers, err := peerManager.ResolveChannelID(ctx, channelId)
	if err != nil {
		return nil, nil, err
	}
	return peerManager, &channelPeers, nil
}

func GetUserPeer(client *tg.Client, ctx context.Context, userId int64) (*peers.Manager, peers.Peer, error) {
	peerManager := peers.Options.Build(peers.Options{}, client)
	userPeers, err := peerManager.ResolvePeer(ctx, &tg.PeerUser{UserID: userId})
	if err != nil {
		return nil, nil, err
	}
	return peerManager, userPeers, nil
}

func BroadcastToUsers(ctx context.Context,
	usersChan chan *repo.User,
	replyedMsgId int, adminId int64,
	client *telegram.Client, fromPeer peers.Peer,
	peerManager *peers.Manager,
	completedCounter, failedCounter *atomic.Int32,
) {
	waitTime := time.Microsecond * 100

	for {
		select {
		case user, ok := <-usersChan:
			if !ok {
				return
			}
			const maxRetries = 3
			success := false

			randomId, _ := client.RandInt64()
			var targetUser peers.Peer
			var err error

			for idx := range maxRetries {
				targetUser, err = peerManager.ResolvePeer(ctx, &tg.PeerUser{UserID: user.ID})
				if err != nil {
					if idx != maxRetries-1 {
						slog.Error("Failed to resolve peer! Retrying...", "error", err)
						time.Sleep(waitTime)
					}
					continue
				}
				break
			}

			if targetUser == nil {
				failedCounter.Add(1)
				continue
			}

			for range maxRetries {
				_, err = client.API().MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
					FromPeer:   fromPeer.InputPeer(),
					ToPeer:     targetUser.InputPeer(),
					ID:         []int{replyedMsgId},
					RandomID:   []int64{randomId},
					DropAuthor: true,
				})
				if err != nil {
					floodErr, ok := tgerr.AsFloodWait(err)
					if !ok {
						slog.Error("Failed to send message", "error", err)
						break
					}

					slog.Warn("Flood wait", "seconds", floodErr.Seconds())
					<-time.After(floodErr)
					continue
				}
				success = true
				break
			}
			if success {
				completedCounter.Add(1)
			} else {
				failedCounter.Add(1)
			}
		case <-ctx.Done():
			return
		}
	}
}

func GetReferLink(botUserName string, userId int64) string {
	refUrl := fmt.Sprintf("https://t.me/%s?start=ref%d", botUserName, userId)
	shareUrl := fmt.Sprintf(
		"https://t.me/share/url?url=%s&text=Try this bot! Quickly stream or download your files with security and reliability.", refUrl)
	shareUrl = strings.ReplaceAll(shareUrl, " ", "%20")
	shareUrl = strings.ReplaceAll(shareUrl, "!", "%21")
	return shareUrl
}
