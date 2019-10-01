package utils

import (
	"fmt"

	corev1beta1 "github.com/automium/automium/pkg/apis/core/v1beta1"
	"github.com/automium/automium/pkg/slack"
	"github.com/golang/glog"
)

// NotifyStatusViaSlack sends a notification about a service and its phase via a Slack Webhook app
func NotifyStatusViaSlack(svcName, phase string) error {

	var color = "#ffffff"
	switch phase {
	case corev1beta1.StatusPhasePending:
		color = "#ffff66"
	case corev1beta1.StatusPhaseRunning:
		color = "#33ccff"
	case corev1beta1.StatusPhaseCompleted:
		color = "#33cc33"
	case corev1beta1.StatusPhaseFailed:
		color = "#ff3300"
	default:
		glog.Warningf("Unknown phase: %s", phase)
	}

	var message = fmt.Sprintf("Got a status update for service: %s", svcName)

	// Prepare the attachment...
	content := slack.WebhookMessageAttachment{
		Fallback: message,
		Text:     message,
		Color:    color,
		Fields: []slack.WebhookMessageField{
			{
				Title: "Current status:",
				Value: phase,
				Short: false,
			},
		},
	}

	// ...and send it
	return slack.SendWebhookAttachment(content)
}
