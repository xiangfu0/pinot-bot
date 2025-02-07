package main

import (
	"bytes"
	"fmt"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"log"
	"regexp"
	"strings"
	"time"
	"crypto/tls"
	"gopkg.in/gomail.v2"
)
import "github.com/slack-go/slack"
import "github.com/sendgrid/sendgrid-go"

var re = regexp.MustCompile(`(?m)<@(\w+)>`)

const initialNumBufSize = 24

func DigestTitle() string {
	loc, _ := time.LoadLocation("America/Los_Angeles")
	t := time.Now()
	t = t.In(loc)
	return fmt.Sprintf("Apache Pinot Daily Email Digest (%s)", t.Format("2006-01-02"))
}

func RunDailyDigest(c *Config) string {
	if c.From == "" || c.To == "" || c.SendgridToken == "" {
		fmt.Println("Some config is missing. Please double check `FROM/TO/SENDGRID_TOKEN`.")
		return "Some config is missing. Please double check `FROM/TO/SENDGRID_TOKEN`."
	}

	// Initialize slack api
	api := slack.New(c.SlackAppToken)

	// Fetch user list
	userList, err := Users(api)
	if err != nil {
		fmt.Println("Failed to fetch the user list: " + err.Error())
		return "Failed to fetch the user list: " + err.Error()
	}

	// Fetch channels
	pm := &slack.GetConversationsParameters{
		ExcludeArchived: true,
		Limit: 1000,
	}
	channels, _, err := api.GetConversations(pm)
	if err != nil {
		fmt.Println("Failed to fetch the user list: " + err.Error())
		return "Failed to fetch the user list: " + err.Error()
	}

	// Construct conversation history html content
	buffer := bytes.NewBuffer(make([]byte, 0, initialNumBufSize))
	for _, channel := range channels {
		if channel.Name == "daily-digest" {
			continue
		}

		ch := &slack.GetConversationHistoryParameters{
			ChannelID: channel.ID,
			Oldest: fmt.Sprintf("%f", float64(time.Now().Add(-24 * time.Hour).Unix())),
			Latest: fmt.Sprintf("%f", float64(time.Now().Unix())),
			Limit: 10000,
		}

		history, err := api.GetConversationHistory(ch)
		if err != nil {
			log.Println("Failed to get conversation history: ", channel.Name)
		}
		if len(history.Messages) > 0 {
			buffer.WriteString(fmt.Sprintf("<h3><u>#%s</u></h3>", channel.Name))
			buffer.WriteString("<br>")
			for i := len(history.Messages) -1; i >=0; i-- {
				m := history.Messages[i]
				buffer.WriteString(fmt.Sprintf("<strong>%s: </strong>", userList[fmt.Sprintf("<@%s>", m.User)]))
				buffer.WriteString(ReplaceMentionUser(userList, m.Text))
				buffer.WriteString("<br>")

				repliesParam := &slack.GetConversationRepliesParameters{
					ChannelID: channel.ID,
					Timestamp: m.Timestamp,
					Limit: 1000,
				}

				replies, _, _, err := api.GetConversationReplies(repliesParam)
				if err != nil {
					log.Println("Failed to get conversation replies: ", channel.Name)
				}
				for j := 1; j < len(replies); j++ {
					r := replies[j].Msg
					buffer.WriteString(fmt.Sprintf("&ensp;&ensp;<strong>%s: </strong>", userList[fmt.Sprintf("<@%s>", r.User)]))
					buffer.WriteString(ReplaceMentionUser(userList, r.Text))
					buffer.WriteString("<br>")
				}
			}
		}
	}

	htmlContent := string(buffer.Bytes())
	if len(htmlContent) <= 0 {
		fmt.Println("Not sending a mail because the content size is zero")
		return "Not sending a mail because the content size is zero"
	}

	if strings.ToUpper(c.MailClientType) == "GMAIL" {
		return SendEmailViaGmail(c, DigestTitle(), htmlContent)
	} else if strings.ToUpper(c.MailClientType) == "SENDGRID" {
		// Send html content to mailing list
		return SendEmailViaSendGrid(c, DigestTitle(), htmlContent)
	} else {
		log.Println("Invalid mail client type: ", c.MailClientType)
		return "Invalid mail client type: " + c.MailClientType
	}
}

func Users(api *slack.Client) (map[string]string, error) {
	users, err := api.GetUsers()
	if err != nil {
		return nil, err
	}
	var userList = make(map[string]string)
	for _, user := range users {
		userList[fmt.Sprintf("<@%s>", user.ID)] = fmt.Sprintf("@%s", user.Name)
	}
	return userList, nil
}

func ReplaceMentionUser(userList map[string]string, text string) string {
	var msg = text
	for _, match := range re.FindAllString(msg, -1) {
		msg = strings.ReplaceAll(msg, match, userList[match])
	}
	return msg
}

func SendEmailViaGmail(c *Config, subject string, htmlContent string) string {
	d := gomail.NewDialer("smtp.gmail.com", 587, c.GmailAccount, c.GmailAppPassword)
	d.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	// Send emails using d.
	m := gomail.NewMessage()
	m.SetAddressHeader("From", c.From, "Pinot Slack Email Digest")
	m.SetHeader("To", c.To)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", htmlContent)

	err := d.DialAndSend(m)
	if err != nil {
		fmt.Println("Failed to send the mail via Sendgrid:  " + err.Error())
		return "Failed to send mail via Sendgrid: " + err.Error()
	} else {
		msg := fmt.Sprintf("Daily digest successfully sent with the title: `%s`\n", DigestTitle())
		fmt.Println(msg)
		return msg
	}
}

func SendEmailViaSendGrid(c *Config, subject string, htmlContent string) string {
	from := mail.NewEmail("Pinot Slack Email Digest", c.From)
	client := sendgrid.NewSendClient(c.SendgridToken)
	to := mail.NewEmail("Apache Pinot Dev", c.To)
	message := mail.NewSingleEmail(from, subject, to, htmlContent, htmlContent)
	response, err := client.Send(message)
	if err != nil {
		fmt.Println("Failed to send the mail via Sendgrid:  " + err.Error())
		return "Failed to send mail via Sendgrid: " + err.Error()
	}

	if response.StatusCode >= 200 && response.StatusCode <= 204 {
		msg := fmt.Sprintf("Daily digest sent with the title: `%s`\n", DigestTitle())
		msg += fmt.Sprintf("StatusCode: `%d`\n", response.StatusCode)
		fmt.Println(msg)
		return msg
	} else {
		msg := fmt.Sprintf("Failed to send the mail via Sendgrid\n")
		msg += fmt.Sprintf("StatusCode: `%d`\n", response.StatusCode)
		msg += fmt.Sprintf("Body: ```%s```", response.Body)
		fmt.Println(msg)
		return msg
	}
}
