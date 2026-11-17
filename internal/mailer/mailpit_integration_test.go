package mailer

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"srez/internal/config"
	"srez/internal/security"
)

func TestMailpitRealSMTPVerificationMessage(t *testing.T) {
	testMailpitMessage(t, false)
}
func TestMailpitRealSMTPPasswordRecoveryMessage(t *testing.T) {
	testMailpitMessage(t, true)
}
func testMailpitMessage(t *testing.T, recovery bool) {
	t.Helper()
	endpoint, api := os.Getenv("TEST_MAILPIT_SMTP"), os.Getenv("TEST_MAILPIT_API")
	if endpoint == "" || api == "" {
		t.Skip("explicit isolated Mailpit required")
	}
	host, port, e := net.SplitHostPort(endpoint)
	if e != nil {
		t.Fatal(e)
	}
	email := "verify-" + security.Random() + "@example.test"
	path, subject, button := "verify-email", "Подтвердите email — SREZ", "Подтвердить почту"
	if recovery {
		path, subject, button = "reset-password", "Сброс пароля — SREZ", "Восстановить пароль"
	}
	link := "http://localhost:5175/" + path + "#token=" + security.Random()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	sender := NewSMTP(config.Mail{Host: host, Port: port, From: "SREZ <no-reply@srez.local>", TLSMode: "none"})
	send := sender.SendVerification
	if recovery {
		send = sender.SendPasswordReset
	}
	if e = send(ctx, email, link); e != nil {
		t.Fatal("real SMTP failed")
	}
	client := &http.Client{Timeout: 3 * time.Second}
	id := ""
	for range 20 {
		r, e := client.Get(api + "/api/v1/search?query=" + url.QueryEscape("to:"+email))
		if e != nil {
			t.Fatal("mailpit API unavailable")
		}
		var list struct {
			Messages []struct {
				ID string `json:"ID"`
			} `json:"messages"`
		}
		e = json.NewDecoder(r.Body).Decode(&list)
		r.Body.Close()
		if e != nil {
			t.Fatal("invalid mailpit response")
		}
		if len(list.Messages) > 0 {
			id = list.Messages[0].ID
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if id == "" {
		t.Fatal("SMTP message absent")
	}
	r, e := client.Get(api + "/api/v1/message/" + url.PathEscape(id))
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	var message struct{ Subject, Text, HTML string }
	if e = json.NewDecoder(r.Body).Decode(&message); e != nil {
		t.Fatal("invalid mailpit message")
	}
	if message.Subject != subject || !strings.Contains(message.Text, link) || !strings.Contains(message.HTML, button) {
		t.Fatal("mail content mismatch")
	}
}
