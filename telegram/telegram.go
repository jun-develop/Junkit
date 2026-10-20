// Package telegram provides a Telegram Bot API client for sending messages and documents.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/jun-develop/junkit/console"
)

// Client sends Telegram messages and documents using HTML parse mode. Both methods log and return
// nil without sending when the token or chatID is empty (SendDocument also when content is nil);
// SendDocument streams content, so memory use stays constant regardless of file size.
type Client interface {
	SendMessage(ctx context.Context, chatID, message string) error
	SendDocument(ctx context.Context, chatID string, content io.Reader, filename string, caption string) error
}

// Options configures the Telegram client.
type Options struct {
	// Token is the Telegram Bot API token; an empty Token skips sending.
	Token string
}

// client is the default implementation of Client.
type client struct {
	httpClient *http.Client
	botPrefix  string
	token      string
	logger     *zap.Logger
}

// sendMessageRequest is the JSON body for Telegram's sendMessage endpoint.
type sendMessageRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

// NewClient creates a Client; a nil httpClient falls back to one with a 10s timeout.
//
// Usage:
//
//	c := telegram.NewClient(telegram.Options{Token: token}, nil, logger)
func NewClient(opts Options, httpClient *http.Client, logger *zap.Logger) Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	return &client{
		httpClient: httpClient,
		botPrefix:  fmt.Sprintf("https://api.telegram.org/bot%s", opts.Token),
		token:      opts.Token,
		logger:     logger,
	}
}

// SendMessage implements Client; it posts a JSON body to the sendMessage endpoint.
func (c *client) SendMessage(ctx context.Context, chatID, message string) error {
	if c.token == "" || chatID == "" {
		c.logger.Warn("telegram: skip sending message due to missing token or chat_id")
		return nil
	}

	reqBody := sendMessageRequest{ChatID: chatID, Text: message, ParseMode: "HTML"}
	jsonData := new(bytes.Buffer)
	if err := json.NewEncoder(jsonData).Encode(reqBody); err != nil {
		c.logger.Error("telegram: encode message failed", zap.Error(err))
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.botPrefix+"/sendMessage", jsonData)
	if err != nil {
		c.logger.Error("telegram: build request failed", zap.Error(err))
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("telegram: send message failed", zap.Error(err))
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("telegram: api returned error", zap.Int("status", resp.StatusCode))
		return c.handleError(resp)
	}

	return nil
}

// SendDocument implements Client; it streams the multipart body through an io.Pipe.
func (c *client) SendDocument(ctx context.Context, chatID string, content io.Reader, filename string, caption string) error {
	if c.token == "" || chatID == "" || content == nil {
		c.logger.Warn("telegram: skip sending document due to invalid parameters")
		return nil
	}

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		var err error
		defer func() {
			_ = writer.Close()
			_ = pw.CloseWithError(err)
		}()

		if err = writer.WriteField("chat_id", chatID); err != nil {
			return
		}

		if caption != "" {
			_ = writer.WriteField("caption", caption)
			_ = writer.WriteField("parse_mode", "HTML")
		}

		var part io.Writer
		if part, err = writer.CreateFormFile("document", filename); err != nil {
			return
		}

		_, err = io.Copy(part, content)
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.botPrefix+"/sendDocument", pr)
	if err != nil {
		c.logger.Error("telegram: build request failed", zap.Error(err))
		_ = pr.CloseWithError(err)
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("telegram: upload document failed", zap.Error(err))
		_ = pr.CloseWithError(err)
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("telegram: api returned error for document upload", zap.Int("status", resp.StatusCode))
		return c.handleError(resp)
	}

	return nil
}

// handleError wraps a non-200 response's status and body in an error.
func (c *client) handleError(resp *http.Response) error {
	bodyBytes, _ := io.ReadAll(resp.Body)
	return console.NewError("telegram api error (status %d): %s", resp.StatusCode, string(bodyBytes))
}
