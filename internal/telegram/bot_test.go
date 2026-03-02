package telegram

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func mkUpdate(chatID, userID int64, text string) *models.Update {
	return &models.Update{
		Message: &models.Message{
			Chat: models.Chat{
				ID: chatID,
			},
			From: &models.User{
				ID: userID,
			},
			Text: text,
		},
	}
}

func TestIsAuthorizedWithConfiguredChatID(t *testing.T) {
	tb := &Bot{chatID: 12345}

	if !tb.isAuthorized(mkUpdate(12345, 999, "/status")) {
		t.Fatal("expected configured chat_id to be authorized")
	}
	if tb.isAuthorized(mkUpdate(54321, 999, "/status")) {
		t.Fatal("expected mismatched chat_id to be unauthorized")
	}
}

func TestIsAuthorizedWithoutConfiguredChatID(t *testing.T) {
	tb := &Bot{chatID: 0}

	if !tb.isAuthorized(mkUpdate(54321, 999, "/status")) {
		t.Fatal("expected any chat to be authorized when chat_id is not configured")
	}
}

func TestIsAuthorizedRejectsNilPayloads(t *testing.T) {
	tb := &Bot{chatID: 12345}

	if tb.isAuthorized(nil) {
		t.Fatal("expected nil update to be unauthorized")
	}
	if tb.isAuthorized(&models.Update{}) {
		t.Fatal("expected empty update to be unauthorized")
	}
}
