package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	"strings"

	"coffeetrix24/internal/bot"
	"coffeetrix24/internal/config"
	"coffeetrix24/internal/db"
	"coffeetrix24/internal/scheduler"

	"github.com/joho/godotenv"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	_ = godotenv.Load()
	testMode := flag.Bool("test", false, "включить тестовый режим: мгновенное приглашение и окно набора 1 минута")
	tokenFlag := flag.String("token", "", "токен бота (перекрывает TELEGRAM_BOT_TOKEN)")
	flag.Parse()
	cfg := config.FromEnv()
	if *tokenFlag != "" { cfg.Token = *tokenFlag }
	cfg.Token = strings.TrimSpace(cfg.Token)
	if cfg.Token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN не задан")
	}
	st, err := db.Open(cfg.DatabasePath)
	if err != nil { log.Fatal(err) }
	defer st.DB.Close()
	// сохранить токен в таблицу cred
	if err := st.UpsertToken(cfg.Token); err != nil { log.Fatal(err) }
	// гарантировать настройки
	if err := st.EnsureSettings("08:00"); err != nil { log.Fatal(err) }

	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil { log.Fatal(err) }
	api.Debug = false

	b := bot.New(api, st)
	b.TestMode = *testMode
	if *testMode { b.SignupWindow = time.Minute }

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sch := scheduler.New(st)
	sch.OnDailyInvite = func() { b.SendDailyInvites() }
	sch.OnCloseSessions = func(ids []int64) {
		for _, id := range ids { b.CloseAndPublish(id) }
	}
	if *testMode {
		sch.DisableDaily = true
		sch.CloseInterval = 5 * time.Second // 5s polling to close
		// немедленно отправить приглашение во все чаты для удобства теста
		b.SendDailyInvites()
	}
	sch.Start(ctx)

	b.Start(ctx)
}
