package main

import (
        "encoding/json"
        "html/template"
        "net/url"
        "strings"

        "github.com/ashwanthkumar/slack-go-webhook"
        log "github.com/sirupsen/logrus"
        "github.com/spf13/viper"
)

// Can't be const because need reference to variable for Slack webhook title
var (
        ClickedLink   string = "Clicked Link"
        SubmittedData string = "Submitted Data"
        CapturedSession string = "Captured Session"
        EmailOpened   string = "Email Opened"
        EmailOpened_evilgophish        string = "Email/SMS Opened"
        EmailSent     string = "Email Sent"
        EmailSent_evilgophish        string = "Email/SMS Sent"
        ClickedLink_title string = ":fish: Clicked Link"
        SubmittedData_title string = ":fishing_pole_and_fish: Submitted Data"
        EmailOpened_title string = ":ocean: Email Opened"
        CapturedSession_title string = ":shark: Captured Session"
)

type Sender interface {
        SendSlack() error
        SendEmail() error
}

func senderDispatch(status string, webhookResponse WebhookResponse, response []byte) (Sender, error) {
        if status == ClickedLink {
                return NewClickDetails(webhookResponse, response)
        }
        if (status == EmailOpened) || (status == EmailOpened_evilgophish) {
                return NewOpenedDetails(webhookResponse, response)
        }
        if status == SubmittedData {
                return NewSubmittedDetails(webhookResponse, response)
        }
        if status == CapturedSession {
                return NewSessionDetails(webhookResponse, response)
        }
        if (status == EmailSent) || (status == EmailSent_evilgophish) {
                return nil, nil
        }
        log.Warn("unknown status:", status)
        return nil, nil
}

// More information about events can be found here:
// https://github.com/gophish/gophish/blob/db63ee978dcd678caee0db71e5e1b91f9f293880/models/result.go#L50
type WebhookResponse struct {
        Success    bool   `json:"success"`
        CampaignID uint   `json:"campaign_id"`
        Message    string `json:"message"`
        Details    string `json:"details"`
        Email      string `json:"email"`
}

func NewWebhookResponse(body []byte) (WebhookResponse, error) {
        var response WebhookResponse
        if err := json.Unmarshal(body, &response); err != nil {
                return WebhookResponse{}, err
        }
        return response, nil
}

type EventDetails struct {
        Payload url.Values        `json:"payload"`
        Browser map[string]string `json:"browser"`
}

func NewEventDetails(detailsRaw []byte) (EventDetails, error) {
        var details EventDetails
        if err := json.Unmarshal(detailsRaw, &details); err != nil {
                return EventDetails{}, err
        }
        return details, nil
}

func (e EventDetails) ID() string {
        return e.Payload.Get("access_token_v")
}

func (e EventDetails) UserAgent() string {
        return e.Browser["user-agent"]
}
func (e EventDetails) Address() string {
        return e.Browser["address"]
}

type SessionDetails struct {
        CampaignID uint
        ID         string
        Email      string
        Address    string
        UserAgent  string
}

func NewSessionDetails(response WebhookResponse, detailsRaw []byte) (SessionDetails, error) {
        details, err := NewEventDetails(detailsRaw)
        if err != nil {
                return SessionDetails{}, err
        }
        sessionDetails := SessionDetails{
                CampaignID: response.CampaignID,
                ID:         details.ID(),
                Address:    details.Address(),
                UserAgent:  details.UserAgent(),
                Email:      response.Email,
        }
        return sessionDetails, nil
}

func (w SessionDetails) SendSlack() error {
        red := "#f05b4f"
        attachment := slack.Attachment{Title: &CapturedSession_title, Color: &red}
        attachment.AddField(slack.Field{Title: "ID", Value: w.ID})
        attachment.AddField(slack.Field{Title: "Address", Value: slackFormatIP(w.Address)})
        attachment.AddField(slack.Field{Title: "User Agent", Value: w.UserAgent})
        return sendSlackAttachment(attachment)
}

type SubmittedDetails struct {
        CampaignID uint
        ID         string
        Email      string
        Address    string
        UserAgent  string
        Username   string
        Password   string
}

func NewSubmittedDetails(response WebhookResponse, detailsRaw []byte) (SubmittedDetails, error) {
        details, err := NewEventDetails(detailsRaw)
        if err != nil {
                return SubmittedDetails{}, err
        }
        submittedDetails := SubmittedDetails{
                CampaignID: response.CampaignID,
                ID:         details.ID(),
                Address:    details.Address(),
                UserAgent:  details.UserAgent(),
                Email:      response.Email,
                Username:   details.Payload.Get("unme"),
                Password:   details.Payload.Get("password"),
        }
        return submittedDetails, nil
}

func (w SubmittedDetails) SendSlack() error {
        red := "#f05b4f"
        attachment := slack.Attachment{Title: &SubmittedData_title, Color: &red}
        attachment.AddField(slack.Field{Title: "ID", Value: w.ID})
        attachment.AddField(slack.Field{Title: "Address", Value: slackFormatIP(w.Address)})
        attachment.AddField(slack.Field{Title: "User Agent", Value: w.UserAgent})
        if !viper.GetBool("slack.disable_credentials") {
                anonymised_email := firstN(w.Email,2) + "***" + lastN(w.Email,2)
                attachment.AddField(slack.Field{Title: "Email", Value: anonymised_email})
                if len(w.Username) > 0 {
                        anonymised_username := firstN(w.Username,2) + "***" + lastN(w.Username,2)
                        attachment.AddField(slack.Field{Title: "Username", Value: anonymised_username})
                }
                if len(w.Password) > 0 {
                        anonymised_password := firstN(w.Password,2) + "***" + lastN(w.Password,2)
                        attachment.AddField(slack.Field{Title: "Password", Value: anonymised_password})
                }
        }
        return sendSlackAttachment(attachment)
}


func firstN(s string, n int) string {
     if len(s) > n {
          return s[:n]
     }
     return ""
}

func lastN(s string, n int) string {
     if len(s) > n {
          return s[len(s)-n:]
     }
     return ""
}

func (w SubmittedDetails) SendEmail() error {
        templateString := viper.GetString("email_submitted_credentials_template")
        body, err := getEmailBody(templateString, w)
        if err != nil {
                return err
        }
        return sendEmail("PhishBot - Credentials Submitted", body)
}

type ClickDetails struct {
        CampaignID uint
        ID         string
        Email      string
        Address    string
        UserAgent  string
}

func NewClickDetails(response WebhookResponse, detailsRaw []byte) (ClickDetails, error) {
        details, err := NewEventDetails(detailsRaw)
        if err != nil {
                return ClickDetails{}, err
        }
        clickDetails := ClickDetails{
                CampaignID: response.CampaignID,
                ID:         details.ID(),
                Address:    details.Address(),
                Email:      response.Email,
                UserAgent:  details.UserAgent(),
        }
        return clickDetails, nil
}

func (w ClickDetails) SendSlack() error {
        orange := "#ffa500"
        attachment := slack.Attachment{Title: &ClickedLink_title, Color: &orange}
        attachment.AddField(slack.Field{Title: "ID", Value: w.ID})
        attachment.AddField(slack.Field{Title: "Address", Value: slackFormatIP(w.Address)})
        attachment.AddField(slack.Field{Title: "User Agent", Value: w.UserAgent})
        if !viper.GetBool("slack.disable_credentials") {
                attachment.AddField(slack.Field{Title: "Email", Value: w.Email})
        }
        return sendSlackAttachment(attachment)
}

func (w ClickDetails) SendEmail() error {
        templateString := viper.GetString("email_send_click_template")
        body, err := getEmailBody(templateString, w)
        if err != nil {
                return err
        }
        return sendEmail("PhishBot - Email Clicked", body)
}

func getEmailBody(templateValue string, obj interface{}) (string, error) {
        out := new(strings.Builder)
        tpl, err := template.New("email").Parse(templateValue)
        if err != nil {
                return "", err
        }
        if err := tpl.Execute(out, obj); err != nil {
                return "", err
        }
        return out.String(), nil
}

type OpenedDetails struct {
        CampaignID uint
        ID         string
        Email      string
        Address    string
        UserAgent  string
}

func NewOpenedDetails(response WebhookResponse, detailsRaw []byte) (OpenedDetails, error) {
        details, err := NewEventDetails(detailsRaw)
        if err != nil {
                return OpenedDetails{}, err
        }
        clickDetails := OpenedDetails{
                CampaignID: response.CampaignID,
                ID:         details.ID(),
                Email:      response.Email,
                Address:    details.Address(),
                UserAgent:  details.UserAgent(),
        }
        return clickDetails, nil
}

func (w OpenedDetails) SendSlack() error {
        yellow := "#ffff00"
        attachment := slack.Attachment{Title: &EmailOpened_title, Color: &yellow}
        attachment.AddField(slack.Field{Title: "ID", Value: w.ID})
        attachment.AddField(slack.Field{Title: "Address", Value: slackFormatIP(w.Address)})
        attachment.AddField(slack.Field{Title: "User Agent", Value: w.UserAgent})
        if !viper.GetBool("slack.disable_credentials") {
                attachment.AddField(slack.Field{Title: "Email", Value: w.Email})
        }
        return sendSlackAttachment(attachment)
}

func (w OpenedDetails) SendEmail() error {
        templateString := viper.GetString("email_send_click_template")
        body, err := getEmailBody(templateString, w)
        if err != nil {
                return err
        }
        return sendEmail("PhishBot - Email Opened", body)
}
