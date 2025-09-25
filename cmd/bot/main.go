package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"coffeetrix24/internal/bot"
	"coffeetrix24/internal/config"
	"coffeetrix24/internal/db"
	"coffeetrix24/internal/scheduler"
	"coffeetrix24/internal/version"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	testMode := flag.Bool("test", false, "включить тестовый режим: мгновенное приглашение и окно набора 1 минута")
	tokenFlag := flag.String("token", "", "токен бота (перекрывает TELEGRAM_BOT_TOKEN)")
	onceInvite := flag.Bool("once-invite", false, "однократно отправить приглашения сейчас и завершить")
	showVersion := flag.Bool("version", false, "показать версию и выйти")
	flag.Parse()
	if *showVersion {
		log.Println("coffeetrix24 version", version.Version)
		return
	}
	cfg := config.FromEnv()
	if *tokenFlag != "" {
		cfg.Token = *tokenFlag
	}
	cfg.Token = strings.TrimSpace(cfg.Token)
	if cfg.Token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN не задан")
	}
	log.Printf("startup: version=%s pid=%d", version.Version, os.Getpid())
	st, err := db.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.DB.Close()
	// сохранить токен в таблицу cred
	if err := st.UpsertToken(cfg.Token); err != nil {
		log.Fatal(err)
	}
	// гарантировать настройки
	if err := st.EnsureSettings("08:00"); err != nil {
		log.Fatal(err)
	}
	var jm string
	_ = st.DB.Get(&jm, "PRAGMA journal_mode;")
	var daily string
	_ = st.DB.Get(&daily, "SELECT daily_time FROM settings WHERE id=1")
	var chatCount int
	_ = st.DB.Get(&chatCount, "SELECT COUNT(1) FROM chats")
	log.Printf("startup: db_journal=%s daily_time=%s chats=%d", jm, daily, chatCount)

	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		log.Fatal(err)
	}
	api.Debug = false

	b := bot.New(api, st)
	b.TestMode = *testMode
	if *testMode {
		b.SignupWindow = time.Minute
	}
	if *onceInvite {
		log.Println("manual once-invite trigger start")
		b.SendDailyInvites()
		log.Println("manual once-invite trigger done; exiting")
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sch := scheduler.New(st)
	sch.OnDailyInvite = func() { b.SendDailyInvites() }
	sch.OnCloseSessions = func(ids []int64) {
		for _, id := range ids {
			b.CloseAndPublish(id)
		}
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
