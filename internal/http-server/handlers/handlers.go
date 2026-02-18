// Package handlers contains the handlers for the http server
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/biisal/fast-stream-bot/config"
	"github.com/biisal/fast-stream-bot/internal/bot"
	botutils "github.com/biisal/fast-stream-bot/internal/bot/bot-utils"
	"github.com/biisal/fast-stream-bot/internal/http-server/shortner"
	"github.com/biisal/fast-stream-bot/internal/stream"
	"github.com/biisal/fast-stream-bot/internal/types"
)

type StreamHandler struct {
	Worker   *bot.Worker
	Cfg      config.Config
	Shortner shortner.Shortner
}

func (h *StreamHandler) ServerFile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		messageID, channelID, err := botutils.ParseMessageAndChannelId(r.PathValue("messageId"), r.PathValue("channelId"), h.Cfg.DB_CHANNEL_ID)
		if err != nil {
			slog.Error("failed to parse messageId and channelId", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		downloadQuery := strings.TrimSpace(r.URL.Query().Get("d"))
		var isDownload bool
		if downloadQuery == "1" || strings.ToLower(downloadQuery) == "true" {
			isDownload = true
		}
		hash := r.PathValue("hash")
		bot, err := h.Worker.HireFreeWorker()
		if bot == nil {
			slog.Error("failed to get bots", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer h.Worker.ReleaseWorker(bot)

		fileMsg, err := botutils.GetChannelMessage(r.Context(), channelID, messageID, bot.Client.API())

		if err != nil {
			slog.Error("Failed to get file message", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		file, err := botutils.GetMediaFromMessage(fileMsg)
		if err != nil {
			slog.Error("Failed to get media from message", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if !botutils.CheckFileHash(file, hash) {
			slog.Error("Invalid hash", "hash", hash)
			http.Error(w, "Invalid hash", http.StatusForbidden)
			return
		}

		reader := stream.NewTgFileReader(bot.Client.API(), r.Context(), file.Location, file, r)
		if err = reader.SetupStream(r, w, isDownload); err != nil {
			slog.Error("Failed to setup stream", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		buffer := make([]byte, stream.TelegramChunkSize)
		if _, err = io.CopyBuffer(w, reader, buffer); err != nil {
			if errors.Is(err, context.Canceled) {
				slog.Info("context has been Canceled")
				return
			}
			if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
				slog.Info("client has closed connection")
				return
			}

			if errors.Is(err, io.EOF) {
				slog.Info("END OF FILE")
				return
			}
			slog.Error("Failed to copy file", "error", err)
			return
		}

	}

}

func renderHTML(w http.ResponseWriter, htmlTemplate string, data any) {
	t, err := template.ParseFiles("frontend/" + htmlTemplate)
	if err != nil {
		slog.Error("Failed to parse template", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = t.Execute(w, data)
	if err != nil {
		slog.Error("Failed to execute template", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
func (h *StreamHandler) HomeStream() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		var errorResp = &types.ErrorResponse{Error: ""}
		messageID, channelID, err := botutils.ParseMessageAndChannelId(r.PathValue("messageId"), r.PathValue("channelId"), h.Cfg.DB_CHANNEL_ID)
		if err != nil {
			errorResp.Error = err.Error()
			renderHTML(w, "error.html", errorResp)
			return
		}
		hash := r.URL.Query().Get("hash")
		streamLink := fmt.Sprintf("/stream/%d/%d/%s", channelID, messageID, hash)

		if strings.Contains(strings.ToLower(r.Header.Get("User-Agent")), "vlc") {
			http.Redirect(w, r, streamLink, http.StatusSeeOther)
			return
		}

		isJustVerified := false
		expireTime := (time.Duration(h.Cfg.JWT_EXPIRATION) * time.Second).String()
		if !h.Shortner.CheckJWTFromCookie(r) {
			slog.Info("No jwt found")
			if h.Shortner.VerifyUUID(r) {
				slog.Info("Found valid uuid")
				if err = h.Shortner.SetJWTCookie(w); err == nil {
					isJustVerified = true
					h.Shortner.RemoveUUID(r)
				}
			} else {
				var uuid = h.Shortner.SetUUID(r)
				reqURI := r.URL.RequestURI()
				separator := "?"
				if strings.Contains(reqURI, "?") {
					separator = "&"
				}
				finalURL := fmt.Sprintf("%s://%s%s%suuid=%s", h.Cfg.HTTP_SCHEME, r.Host, reqURI, separator, uuid)
				if redirectURL := h.Shortner.CreateShortnerLink(finalURL); redirectURL != "" {
					slog.Info("Redirecting to shortner link", "url", redirectURL)
					http.Redirect(w, r, redirectURL, http.StatusSeeOther)
					return
				}

			}

		}

		if r.URL.Query().Get("redirect") == "vlc" {
			url := fmt.Sprintf("vlc://%s%s", r.Host, streamLink)
			http.Redirect(w, r, url, http.StatusSeeOther)
			return

		}

		client, err := h.Worker.HireFreeWorker()
		if err != nil {
			slog.Error("failed to get bots", "error", err)
			errorResp.Error = "Failed to get bots. Try again later or contact to developer"
			renderHTML(w, "error.html", errorResp)
			return
		}
		defer h.Worker.ReleaseWorker(client)

		fileMsg, err := botutils.GetChannelMessage(r.Context(), channelID, messageID, client.Client.API())
		if err != nil {
			slog.Error("Failed to get file message", "error", err)
			errorResp.Error = "Failed to get file message. Check your URL"
			renderHTML(w, "error.html", errorResp)
			return
		}

		file, err := botutils.GetMediaFromMessage(fileMsg)
		if err != nil {
			slog.Error("Failed to get media from message", "error", err)
			errorResp.Error = "Failed to get media from message. Check your URL"
			renderHTML(w, "error.html", errorResp)
			return
		}

		if !botutils.CheckFileHash(file, hash) {
			slog.Error("Invalid hash", "hash", hash)
			errorResp.Error = "Invalid hash. Check your URL"
			renderHTML(w, "error.html", errorResp)
			return
		}

		downloadLink := fmt.Sprintf("%s?d=1", streamLink)
		var FileInfo = &types.FileResponse{
			Title:          file.FileName,
			Size:           botutils.MakeSizeReadable(file.Size),
			DownloadLink:   downloadLink,
			StreamLink:     streamLink,
			IsJustVerified: isJustVerified,
			ExpireTime:     expireTime,
			AppName:        h.Cfg.APP_NAME,
		}

		w.Header().Set("Cache-Control", "max-age=1200")
		renderHTML(w, "index.html", FileInfo)
	}
}

func (h *StreamHandler) LandingPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		botUsername := "biisal"
		for _, bot := range h.Worker.Bots {
			if bot.Default {
				botUsername = bot.BotUserName
				break
			}
		}
		commits := botutils.GetCommits()
		var data = struct {
			BotLink     string
			ChannelLink string
			Commits     []botutils.Commit
			AppName     string
			HeaderImage string
		}{
			BotLink:     "https://t.me/" + botUsername,
			ChannelLink: "https://t.me/" + h.Cfg.MAIN_CHANNEL_USERNAME,
			Commits:     commits,
			AppName:     h.Cfg.APP_NAME,
			HeaderImage: h.Cfg.HEADER_IMAGE,
		}
		w.Header().Set("Cache-Control", "max-age=1200")
		renderHTML(w, "home.html", data)
	}
}

func (h *StreamHandler) MakeHashByChanMsgID() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		messageIdStr := r.PathValue("messageId")
		channelIdStr := r.PathValue("channelId")
		messageId, channelId64, err := botutils.ParseMessageAndChannelId(messageIdStr, channelIdStr, 0)
		if err != nil {
			slog.Error("failed to parse messageId and channelId", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		bot, err := h.Worker.HireFreeWorker()
		if bot == nil {
			slog.Error("failed to get bots", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer h.Worker.ReleaseWorker(bot)

		fileMsg, err := botutils.GetChannelMessage(r.Context(), channelId64, messageId, bot.Client.API())
		if err != nil {
			slog.Error("Failed to get file message", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		file, err := botutils.GetMediaFromMessage(fileMsg)
		if err != nil {
			slog.Error("Failed to get media from message", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		hash := botutils.MakeHashByFileInfo(file)
		res := &types.HashResponse{
			Data: types.HashInfo{
				Hash:      hash,
				MessageId: messageId,
				ChannelId: channelId64,
			},
			StatusCode: http.StatusOK,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(res)
		if err != nil {
			slog.Error("Failed to encode response", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	}

}

func (h *StreamHandler) Ping() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "pong")
		w.WriteHeader(http.StatusOK)
	}
}
