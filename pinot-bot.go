package main

import (
	"context"
	"fmt"
	"github.com/go-joe/cron"
	"github.com/go-joe/http-server"
	"github.com/go-joe/joe"
	"github.com/go-joe/slack-adapter"
	"log"
	"os"
	"time"
)

type PinotBot struct {
	*joe.Bot
	Config *Config
}

type DailyDigestEvent struct {
}

type Config struct {
	SlackAppToken     string
	SlackBotUserToken string
	From              string
	To                string
	SendgridToken     string
	Port              string
	GmailAccount      string
	GmailAppPassword  string
	MailClientType  string
	CronSchedule string
}

func NewConfig() (*Config, error) {
	config := &Config{
		SlackAppToken:     os.Getenv("SLACK_APP_TOKEN"),
		SlackBotUserToken: os.Getenv("SLACK_BOT_USER_TOKEN"),
		From:              os.Getenv("FROM"),
		To:                os.Getenv("TO"),
		SendgridToken:     os.Getenv("SENDGRID_TOKEN"),
		Port:              os.Getenv("PORT"),
		GmailAccount:      os.Getenv("GMAIL_ACCOUNT"),
		GmailAppPassword:  os.Getenv("GMAIL_APP_PASSWORD"),
		MailClientType:   os.Getenv("MAIL_CLIENT_TYPE"),
		CronSchedule:   os.Getenv("CRON_SCHEDULE"),
	}
	fmt.Println(config)
	if config.Port == "" {
		config.Port = "80"
	}
	if config.CronSchedule == "" {
		// Schedule the daily digest cron job at 2:00:00 AM (UTC)
		config.CronSchedule = "0 0 2 * * *"
	}
	return config, nil
}

func main() {
	config, err := NewConfig()
	if err != nil {
		log.Fatal("Failed to load config: ", err)
	}
	if config.CronSchedule == "one-time" {
		modules := []joe.Module {
			joehttp.Server(":" + config.Port),
			cron.ScheduleEvent(config.CronSchedule, DailyDigestEvent{}),
		}
		if config.SlackAppToken != "" && config.SlackBotUserToken != ""  {
			modules = append(modules, slack.Adapter(config.SlackBotUserToken))
		}
	
		b := &PinotBot{
			Bot: joe.New("pinot-bot", modules...),
			Config: config,
		}
		b.Say("daily-digest", "Pinot bot is starting..")
		responseMsg := RunDailyDigest(config)
		b.Say("daily-digest", responseMsg)
		return
	}
	modules := []joe.Module {
		joehttp.Server(":" + config.Port),
		cron.ScheduleEvent(config.CronSchedule, DailyDigestEvent{}),
	}
	if config.SlackAppToken != "" && config.SlackBotUserToken != ""  {
		modules = append(modules, slack.Adapter(config.SlackBotUserToken))
	}

	b := &PinotBot{
		Bot: joe.New("pinot-bot", modules...),
		Config: config,
	}

	// Register event handlers
	b.Brain.RegisterHandler(b.HandleDailyDigestEvent)
	b.Brain.RegisterHandler(b.HandleHTTP)
	b.Respond("daily-digest", b.DailyDigest)
	b.Respond("config", b.PrintConfig)
	b.Respond("ping", Pong)
	b.Respond("time", Time)

	b.Say("daily-digest", "Pinot bot is starting..")
	err = b.Run()
	if err != nil {
		b.Logger.Fatal(err.Error())
	}
}

func (b *PinotBot) HandleDailyDigestEvent(evt DailyDigestEvent) {
	responseMsg := RunDailyDigest(b.Config)
	b.Say("daily-digest", responseMsg)
}

func (b *PinotBot) DailyDigest(msg joe.Message) error {
	responseMsg := RunDailyDigest(b.Config)
	msg.Respond(responseMsg)
	return nil
}

func (b *PinotBot) PrintConfig(msg joe.Message) error {
	fmt.Println("printconfig")
	configMsg := fmt.Sprintf("From: `%s`\n", b.Config.From)
	configMsg += fmt.Sprintf("To: `%s`\n", b.Config.To)
	configMsg += fmt.Sprintf("SlackAppToken: `%s`\n", b.Config.SlackAppToken)
	configMsg += fmt.Sprintf("SlackBotUserToken: `%s`\n", b.Config.SlackBotUserToken)
	configMsg += fmt.Sprintf("MailClientType: `%s`\n", b.Config.MailClientType)
	configMsg += fmt.Sprintf("SendgridToken: `%s`\n", b.Config.SendgridToken)
	configMsg += fmt.Sprintf("GmailAccount: `%s`", b.Config.GmailAccount)
	msg.Respond(configMsg)
	return nil
}

func (b *PinotBot) HandleHTTP(c context.Context, r joehttp.RequestEvent) {
	if r.URL.Path == "/" {
		fmt.Println("Pinot bot is running..")
	}
	if r.URL.Path == "/run-digest" {
		fmt.Println("Running Pinot daily digest..")
		responseMsg := RunDailyDigest(b.Config)
		fmt.Printf("Finished daily-digest: `%s`\n", responseMsg)
	}
}

func Time(msg joe.Message) error {
	loc, _ := time.LoadLocation("America/Los_Angeles")
	t := time.Now()
	timeMsg := fmt.Sprintf("Machine local time: `%s`\n", fmt.Sprint(t))
	timeMsg += fmt.Sprintf("Machine local time (in PDT): `%s`", fmt.Sprint(t.In(loc)))
	msg.Respond(timeMsg)
	return nil
}

func Pong(msg joe.Message) error {
	msg.Respond("PONG")
	return nil
}
