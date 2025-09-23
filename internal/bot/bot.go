package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"coffeetrix24/internal/db"
	"coffeetrix24/internal/logic"
	"coffeetrix24/internal/messages"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	API   *tgbotapi.BotAPI
	Store *db.Store
	// runtime options
	TestMode bool
	SignupWindow time.Duration
}

func New(api *tgbotapi.BotAPI, store *db.Store) *Bot { return &Bot{API: api, Store: store} }

func (b *Bot) Start(ctx context.Context) {
	updates := b.API.GetUpdatesChan(tgbotapi.UpdateConfig{Timeout: 30})
	for {
		select {
		case <-ctx.Done():
			return
		case upd := <-updates:
			b.handleUpdate(upd)
		}
	}
}

func (b *Bot) handleUpdate(upd tgbotapi.Update) {
	if upd.MyChatMember != nil {
		b.onMyChatMember(*upd.MyChatMember)
		return
	}
	if cb := upd.CallbackQuery; cb != nil {
		b.onCallback(cb)
	}
}

func (b *Bot) onMyChatMember(m tgbotapi.ChatMemberUpdated) {
	// Бот добавлен или стал участником/администратором
	status := m.NewChatMember.Status
	if status == "member" || status == "administrator" || status == "creator" {
		b.onAddedToGroup(m.Chat.ID, m.Chat.Title)
	}
}

func (b *Bot) onAddedToGroup(chatID int64, title string) {
	_ = b.Store.UpsertChat(chatID, title)
	txt := messages.IntroMessage
	msg := tgbotapi.NewMessage(chatID, txt)
	_, _ = b.API.Send(msg)
	if b.TestMode {
		// в тестовом режиме сразу отправляем приглашение
		b.sendInviteToChat(chatID)
	}
}

func (b *Bot) SendDailyInvites() {
	// Send to all chats
	rows, err := b.Store.DB.Queryx("SELECT chat_id FROM chats")
	if err != nil { log.Println("daily send error:", err); return }
	defer rows.Close()
	for rows.Next() {
		var chatID int64
		if err := rows.Scan(&chatID); err != nil { continue }
		b.sendInviteToChat(chatID)
	}
}

func (b *Bot) sendInviteToChat(chatID int64) {
	now := time.Now().UTC()
	date := now.Format("2006-01-02")
	// если на сегодня уже отправляли приглашение (invite_message_id не NULL), не дублировать
	if id, inviteID, err := b.Store.GetSessionByChatDate(chatID, date); err == nil && id != 0 && inviteID.Valid {
		return
	}
	window := b.SignupWindow
	if window == 0 { window = 30 * time.Minute }
	deadline := now.Add(window)
	sessionID, err := b.Store.CreateOrGetTodaySession(chatID, date, deadline)
	if err != nil { log.Println("session create error:", err); return }

	btn := tgbotapi.NewInlineKeyboardButtonData(messages.ImInButton, fmt.Sprintf("join:%d", sessionID))
	kb := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(btn))
	msg := tgbotapi.NewMessage(chatID, messages.DailyInvite)
	msg.ReplyMarkup = kb
	resp, err := b.API.Send(msg)
	if err == nil {
		_ = b.Store.SetInviteMessageID(sessionID, resp.MessageID)
	}
}

func (b *Bot) onCallback(cb *tgbotapi.CallbackQuery) {
	data := cb.Data
	if strings.HasPrefix(data, "join:") {
		var sessionID int64
		_, _ = fmt.Sscanf(data, "join:%d", &sessionID)
		user := cb.From
		name := strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
		if name == "" { name = user.UserName }
		// prevent late signups
		open, err := b.Store.SessionOpen(sessionID, time.Now())
		if err == nil && !open {
			_, _ = b.API.Request(tgbotapi.NewCallback(cb.ID, "Набор участников уже закрыт."))
			return
		}
		in, err := b.Store.IsParticipant(sessionID, user.ID)
		if err == nil && !in {
			_ = b.Store.AddParticipant(sessionID, user.ID, user.UserName, name)
			_, _ = b.API.Request(tgbotapi.NewCallback(cb.ID, messages.JoinedAck))
			return
		}
		_, _ = b.API.Request(tgbotapi.NewCallback(cb.ID, messages.AlreadyIn))
	}
}

func (b *Bot) CloseAndPublish(sessionID int64) {
	chatID, _, err := b.Store.GetSessionInfo(sessionID)
	if err != nil { return }
	parts, err := b.Store.GetParticipants(sessionID)
	if err != nil { return }
	// In test mode, if only one participant, add few fake participants
	if b.TestMode && len(parts) == 1 {
		fakes := []db.Participant{
			{UserID: 900001, Username: "", DisplayName: "Тестовый участник 1"},
			{UserID: 900002, Username: "", DisplayName: "Тестовый участник 2"},
			{UserID: 900003, Username: "", DisplayName: "Тестовый участник 3"},
			{UserID: 900004, Username: "", DisplayName: "Тестовый участник 4"},
		}
		for _, fp := range fakes {
			_ = b.Store.AddParticipant(sessionID, fp.UserID, fp.Username, fp.DisplayName)
		}
		parts, _ = b.Store.GetParticipants(sessionID)
	}
	if len(parts) == 0 {
		msg := tgbotapi.NewMessage(chatID, messages.NoParticipants)
		_, _ = b.API.Send(msg)
		_ = b.Store.CloseSession(sessionID)
		return
	}
	users := make([]logic.User, 0, len(parts))
	for _, p := range parts {
		name := p.DisplayName
		if name == "" && p.Username != "" { name = "@" + p.Username }
		if name == "" { name = fmt.Sprintf("id:%d", p.UserID) }
		users = append(users, logic.User{ID: p.UserID, Name: name})
	}
	groups := logic.MakeGroups(users)
	var sb strings.Builder
	sb.WriteString("Итоги Random Coffee на сегодня:\n")
	for i, g := range groups {
		sb.WriteString(fmt.Sprintf("Группа %d: ", i+1))
		for j, u := range g.Members {
			if j > 0 { sb.WriteString(", ") }
			sb.WriteString(u.Name)
		}
		sb.WriteString("\n")
	}
	msg := tgbotapi.NewMessage(chatID, sb.String())
	_, _ = b.API.Send(msg)
	_ = b.Store.CloseSession(sessionID)
}
