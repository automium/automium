package slack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/golang/glog"
)

// ErrWebhookNotEnabled is the error returned if the env variable SLACK_WEBHOOK_URL is not defined
var ErrWebhookNotEnabled = errors.New("webhook not enabled")

// WebhookMessage is the struct which contains the required data for the Slack webhook
type WebhookMessage struct {
	Attachments []WebhookMessageAttachment `json:"attachments"`
}

// WebhookMessageAttachment is used for the Attachment section of the webhook
type WebhookMessageAttachment struct {
	Fallback string                `json:"fallback"`
	Text     string                `json:"text"`
	Pretext  string                `json:"pretext"`
	Color    string                `json:"color"`
	Fields   []WebhookMessageField `json:"fields"`
}

// WebhookMessageField is used for the Field part of the Attachment structure
type WebhookMessageField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

func isWebhookConfigured() bool {
	checkURL := os.Getenv("SLACK_WEBHOOK_URL")
	if len(checkURL) > 0 {
		// Check if the provided hook url is a valid URL
		if _, err := url.ParseRequestURI(checkURL); err != nil {
			glog.Errorf("SLACK_WEBHOOK_URL provided but not a valid URL; check the env var.")
			return false
		}
		return true
	}
	return false
}

// SendWebhookAttachment sends a message using the provided Slack Bot Webhook URL
func SendWebhookAttachment(attachment WebhookMessageAttachment) error {
	if !isWebhookConfigured() {
		return ErrWebhookNotEnabled
	}

	// Prepare the request body
	postBody := WebhookMessage{
		Attachments: []WebhookMessageAttachment{attachment},
	}

	// Make the request
	return makePOSTRequest(postBody)
}

func makePOSTRequest(msg WebhookMessage) error {

	// Set the webhook URL
	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")

	// Marshal the struct
	postBodyJSON, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Prepare a reader for the marshalled data
	postBodyReader := bytes.NewReader(postBodyJSON)

	// Prepare the POST request
	postReq, err := http.NewRequest("POST", webhookURL, postBodyReader)
	if err != nil {
		return err
	}
	postReq.Header.Add("Content-Type", "application/json")

	// Make the POST request
	httpClient := &http.Client{}
	slackResponse, err := httpClient.Do(postReq)
	if err != nil {
		return err
	}
	defer slackResponse.Body.Close()
	respBody, err := ioutil.ReadAll(slackResponse.Body)
	if err != nil {
		return err
	}

	glog.V(5).Infof("Slack Webhook POST completed with code %d and body: %s\n", slackResponse.StatusCode, string(respBody))

	// Check if we got a 200 status code from Slack
	if slackResponse.StatusCode != 200 {
		return fmt.Errorf("got return code %d (expected 200) - response body: %s\n", slackResponse.StatusCode, string(respBody))
	}

	// All done!
	return nil

}
