// SPDX-License-Identifier: MIT

//go:build mailpit

// Package mailpit holds the acceptance test of the Mailpit service of deploy/compose.yml. UnderDoc sends no mail yet;
// until M-22 does, the test stands for it: a mail goes in over SMTP and has to come out of the API of the catcher.
package mailpit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"testing"
	"time"
)

// TestMailpitCatchesAMail sends a mail to a real Mailpit over SMTP and reads it back through GET /api/v1/messages:
// subject, recipient and text have to be the ones sent. scripts/check-services.sh starts the Mailpit of deploy/compose.yml
// and names its ports in UNDERDOC_TEST_MAILPIT_SMTP and UNDERDOC_TEST_MAILPIT_API; without them this test fails, because
// a skipped test is not a green one.
func TestMailpitCatchesAMail(t *testing.T) {
	smtpAddr, api := os.Getenv("UNDERDOC_TEST_MAILPIT_SMTP"), os.Getenv("UNDERDOC_TEST_MAILPIT_API")
	if smtpAddr == "" || api == "" {
		t.Fatal("UNDERDOC_TEST_MAILPIT_SMTP and UNDERDOC_TEST_MAILPIT_API are not set: run scripts/check-services.sh")
	}
	const (
		subject = "Welcome to UnderDoc"
		to      = "reader@example.test"
		text    = "The page you follow has changed."
	)
	msg := "From: underdoc@example.test\r\nTo: " + to + "\r\nSubject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" + text + "\r\n"

	// The port answers a moment before the server takes mail: try for a while.
	var err error
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		if err = smtp.SendMail(smtpAddr, nil, "underdoc@example.test", []string{to}, []byte(msg)); err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("Mailpit takes no mail over SMTP: %v", err)
	}

	resp, err := http.Get("http://" + api + "/api/v1/messages")
	if err != nil {
		t.Fatalf("GET /api/v1/messages: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/messages answers %s", resp.Status)
	}
	var list struct {
		Total    int `json:"total"`
		Messages []struct {
			Subject string `json:"Subject"`
			Snippet string `json:"Snippet"`
			To      []struct {
				Address string `json:"Address"`
			} `json:"To"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("the answer of the API is no list of messages: %v", err)
	}
	if list.Total != 1 || len(list.Messages) != 1 {
		t.Fatalf("Mailpit holds %d mails, want exactly the one sent", list.Total)
	}
	m := list.Messages[0]
	if m.Subject != subject {
		t.Errorf("subject %q, want %q", m.Subject, subject)
	}
	if len(m.To) != 1 || m.To[0].Address != to {
		t.Errorf("recipients %v, want %s", fmt.Sprint(m.To), to)
	}
	if !strings.Contains(m.Snippet, text) {
		t.Errorf("text %q does not contain %q", m.Snippet, text)
	}
}
